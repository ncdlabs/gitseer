// Package auth implements bootstrap and Gitea OAuth (Authorization Code + PKCE) sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	lenscrypto "github.com/ncdlabs/gitea-lens/internal/crypto"
	"github.com/ncdlabs/gitea-lens/internal/forge/gitea"
	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/store"
)

const CookieName = "lens_session"
const CSRFCookieName = "lens_csrf"
const CSRFHeaderName = "X-CSRF-Token"

var ErrUnauthorized = errors.New("unauthorized")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrOAuthNotConfigured = errors.New("oauth is not configured")

// Config holds auth runtime settings.
type Config struct {
	BootstrapPassword string
	SessionTTL        time.Duration
	CookieSecure      bool
	CookiePath        string
	GiteaBaseURL      string
	OAuthClientID     string
	OAuthClientSecret string
	ExternalURL       string // public Lens URL (used for redirect_uri)
	AllowPrivateNet   bool
	EncryptionKey     string
}

// Service manages login and sessions.
type Service struct {
	mu     sync.RWMutex
	store  *store.Store
	cfg    Config
	client *http.Client
	encKey []byte
}

// New creates an auth service.
func New(st *store.Store, cfg Config) *Service {
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24 * time.Hour
	}
	if cfg.CookiePath == "" {
		cfg.CookiePath = "/"
	}
	var encKey []byte
	if cfg.EncryptionKey != "" {
		if k, err := lenscrypto.KeyFromString(cfg.EncryptionKey); err == nil {
			encKey = k
		}
	}
	return &Service{
		store:  st,
		cfg:    cfg,
		client: gitea.NewHTTPClient(cfg.AllowPrivateNet, 30*time.Second),
		encKey: encKey,
	}
}

func (s *Service) BootstrapEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.BootstrapPassword != ""
}

func (s *Service) OAuthEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.GiteaBaseURL != "" && s.cfg.OAuthClientID != "" && s.cfg.ExternalURL != ""
}

func (s *Service) RedirectURI() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	base := strings.TrimRight(s.cfg.ExternalURL, "/")
	return base + "/api/v1/auth/callback"
}

// UpdateGiteaAuth refreshes Gitea base URL, OAuth client credentials, and private-net HTTP client.
func (s *Service) UpdateGiteaAuth(baseURL, clientID, clientSecret string, allowPrivateNet bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.GiteaBaseURL = strings.TrimSpace(baseURL)
	s.cfg.OAuthClientID = strings.TrimSpace(clientID)
	s.cfg.OAuthClientSecret = clientSecret
	s.cfg.AllowPrivateNet = allowPrivateNet
	s.client = gitea.NewHTTPClient(allowPrivateNet, 30*time.Second)
}

// UpdateExternalURL refreshes the public Lens URL used for OAuth redirect_uri.
func (s *Service) UpdateExternalURL(externalURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.ExternalURL = strings.TrimRight(strings.TrimSpace(externalURL), "/")
}

// SafeRedirectPath allows only same-app relative paths (no scheme, no //).
func SafeRedirectPath(redirectTo string) string {
	redirectTo = strings.TrimSpace(redirectTo)
	if redirectTo == "" {
		return "/"
	}
	if strings.Contains(redirectTo, "://") || strings.HasPrefix(redirectTo, "//") {
		return "/"
	}
	if strings.ContainsAny(redirectTo, "\\\r\n\t") {
		return "/"
	}
	if !strings.HasPrefix(redirectTo, "/") {
		return "/"
	}
	return redirectTo
}

// LoginBootstrap validates the shared setup password and creates a session.
func (s *Service) LoginBootstrap(ctx context.Context, password, ip, ua string) (*models.User, string, error) {
	s.mu.RLock()
	bootstrapPW := s.cfg.BootstrapPassword
	s.mu.RUnlock()
	if bootstrapPW == "" {
		return nil, "", fmt.Errorf("bootstrap auth is not configured")
	}
	// Hash both sides so ConstantTimeCompare always runs on equal-length digests.
	got := sha256.Sum256([]byte(password))
	want := sha256.Sum256([]byte(bootstrapPW))
	if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
		return nil, "", ErrInvalidCredentials
	}
	user, err := s.store.EnsureBootstrapUser(ctx)
	if err != nil {
		return nil, "", err
	}
	token, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// BeginOAuth creates PKCE state and returns the Gitea authorize URL.
func (s *Service) BeginOAuth(ctx context.Context, redirectTo string) (authorizeURL string, err error) {
	if !s.OAuthEnabled() {
		return "", ErrOAuthNotConfigured
	}
	redirectTo = SafeRedirectPath(redirectTo)
	state, err := randomToken()
	if err != nil {
		return "", err
	}
	verifier, err := randomPKCEVerifier()
	if err != nil {
		return "", err
	}
	challenge := pkceChallengeS256(verifier)
	expires := time.Now().UTC().Add(10 * time.Minute)
	if err := s.store.SaveOAuthState(ctx, state, verifier, redirectTo, expires); err != nil {
		return "", err
	}
	s.mu.RLock()
	clientID := s.cfg.OAuthClientID
	baseURL := s.cfg.GiteaBaseURL
	redirectURI := strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/callback"
	s.mu.RUnlock()
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	authURL := strings.TrimRight(baseURL, "/") + "/login/oauth/authorize?" + q.Encode()
	return authURL, nil
}

// CompleteOAuth validates state, exchanges the code, upserts the user, and creates a session.
// Returns user, session cookie token, post-login redirect path, and access token for ACL refresh.
func (s *Service) CompleteOAuth(ctx context.Context, code, state, ip, ua string) (*models.User, string, string, string, error) {
	if !s.OAuthEnabled() {
		return nil, "", "", "", ErrOAuthNotConfigured
	}
	if code == "" || state == "" {
		return nil, "", "", "", fmt.Errorf("missing code or state")
	}
	verifier, redirectTo, err := s.store.TakeOAuthState(ctx, state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", "", "", fmt.Errorf("invalid or expired oauth state")
		}
		return nil, "", "", "", err
	}
	redirectTo = SafeRedirectPath(redirectTo)
	tok, err := s.exchangeCode(ctx, code, verifier)
	if err != nil {
		return nil, "", "", "", err
	}
	gu, err := s.fetchGiteaUser(ctx, tok.AccessToken)
	if err != nil {
		return nil, "", "", "", err
	}
	inst, err := s.store.GetPrimaryInstance(ctx)
	var instanceID *int64
	if err == nil && inst != nil {
		id := inst.ID
		instanceID = &id
	}
	user, err := s.store.UpsertGiteaUser(ctx, instanceID, *gu)
	if err != nil {
		return nil, "", "", "", err
	}
	if err := s.persistUserToken(ctx, user.ID, tok); err != nil {
		// Non-fatal: ACL still refreshed in-memory by caller.
		_ = err
	}
	sessionToken, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, "", "", "", err
	}
	return user, sessionToken, redirectTo, tok.AccessToken, nil
}

func (s *Service) persistUserToken(ctx context.Context, userID int64, tok *tokenResponse) error {
	if len(s.encKey) != 32 || tok == nil || tok.AccessToken == "" {
		return nil
	}
	accessCipher, err := lenscrypto.Encrypt(s.encKey, tok.AccessToken)
	if err != nil {
		return err
	}
	refreshCipher := ""
	if tok.RefreshToken != "" {
		refreshCipher, err = lenscrypto.Encrypt(s.encKey, tok.RefreshToken)
		if err != nil {
			return err
		}
	}
	var expires *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expires = &t
	}
	return s.store.SaveUserToken(ctx, userID, accessCipher, refreshCipher, expires)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func (s *Service) exchangeCode(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	s.mu.RLock()
	baseURL := s.cfg.GiteaBaseURL
	clientID := s.cfg.OAuthClientID
	clientSecret := s.cfg.OAuthClientSecret
	redirectURI := strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/callback"
	client := s.client
	s.mu.RUnlock()
	endpoint := strings.TrimRight(baseURL, "/") + "/login/oauth/access_token"
	body := map[string]string{
		"client_id":     clientID,
		"code":          code,
		"grant_type":    "authorization_code",
		"redirect_uri":  redirectURI,
		"code_verifier": verifier,
	}
	if clientSecret != "" {
		body["client_secret"] = clientSecret
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token exchange failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}
	return &tok, nil
}

func (s *Service) fetchGiteaUser(ctx context.Context, accessToken string) (*models.User, error) {
	s.mu.RLock()
	baseURL := s.cfg.GiteaBaseURL
	client := s.client
	s.mu.RUnlock()
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch user failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var gu struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		FullName  string `json:"full_name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(raw, &gu); err != nil {
		return nil, err
	}
	id := gu.ID
	return &models.User{
		GiteaUserID: &id,
		Login:       gu.Login,
		Email:       gu.Email,
		DisplayName: gu.FullName,
		AvatarURL:   gu.AvatarURL,
	}, nil
}

func (s *Service) createSession(ctx context.Context, userID int64, ip, ua string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	sessID, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.RLock()
	ttl := s.cfg.SessionTTL
	s.mu.RUnlock()
	expires := time.Now().UTC().Add(ttl)
	if err := s.store.CreateSession(ctx, sessID, hashToken(token), userID, expires, ip, ua); err != nil {
		return "", err
	}
	return token, nil
}

// UserAccessToken decrypts the stored OAuth access token for userID.
// Returns empty string when encryption is unset or no token is stored.
func (s *Service) UserAccessToken(ctx context.Context, userID int64) (string, error) {
	if len(s.encKey) != 32 {
		return "", nil
	}
	cipher, err := s.store.GetUserAccessTokenCipher(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if cipher == "" {
		return "", nil
	}
	return lenscrypto.Decrypt(s.encKey, cipher)
}

func (s *Service) UserFromRequest(ctx context.Context, r *http.Request) (*models.User, *models.Session, error) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil, nil, ErrUnauthorized
	}
	sess, err := s.store.GetSessionByTokenHash(ctx, hashToken(c.Value))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrUnauthorized
		}
		return nil, nil, err
	}
	user, err := s.store.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return nil, nil, err
	}
	return user, sess, nil
}

func (s *Service) Logout(ctx context.Context, r *http.Request) error {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	sess, err := s.store.GetSessionByTokenHash(ctx, hashToken(c.Value))
	if err != nil {
		return nil
	}
	return s.store.DeleteSession(ctx, sess.ID)
}

func (s *Service) SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     s.cfg.CookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
}

// IssueCSRFToken sets a non-HttpOnly double-submit CSRF cookie and returns the token value.
func (s *Service) IssueCSRFToken(w http.ResponseWriter) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     s.cfg.CookiePath,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
	return token, nil
}

func (s *Service) ClearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     s.cfg.CookiePath,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   -1,
	})
}

// ValidCSRF reports whether the request CSRF header matches the cookie.
func (s *Service) ValidCSRF(r *http.Request) bool {
	c, err := r.Cookie(CSRFCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	header := r.Header.Get(CSRFHeaderName)
	if header == "" {
		if err := r.ParseForm(); err == nil {
			header = r.FormValue("csrf_token")
		}
	}
	if header == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(header)) == 1
}

func (s *Service) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     s.cfg.CookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   -1,
	})
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomPKCEVerifier() (string, error) {
	// 43–128 chars; 32 bytes → 43 rawurl chars
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// PathPrefixFromExternalURL returns the URL path (e.g. "/lens") or "/".
func PathPrefixFromExternalURL(external string) string {
	if external == "" {
		return "/"
	}
	u, err := url.Parse(external)
	if err != nil || u.Path == "" || u.Path == "/" {
		return "/"
	}
	return strings.TrimRight(u.Path, "/")
}
