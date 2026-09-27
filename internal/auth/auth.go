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
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/forge/gitea"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

const CookieName = "gitseer_session"
const CSRFCookieName = "gitseer_csrf"
const CSRFHeaderName = "X-CSRF-Token"
const OAuthStateCookieName = "gitseer_oauth_state"

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
	GitHubBaseURL     string
	GitHubOAuthClientID     string
	GitHubOAuthClientSecret string
	GitHubAllowPrivateNet   bool
	ExternalURL       string // public GitSeer URL (used for redirect_uri)
	AllowPrivateNet   bool
	EncryptionKey     string
}

// Service manages login and sessions.
type Service struct {
	mu           sync.RWMutex
	store        *store.Store
	cfg          Config
	client       *http.Client
	githubClient *http.Client
	encKey       []byte
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
		k, err := gitseercrypto.KeyFromString(cfg.EncryptionKey)
		if err != nil {
			// Invalid keys are rejected by config.Validate; leave encryption disabled only if empty.
			encKey = nil
		} else {
			encKey = k
		}
	}
	return &Service{
		store:        st,
		cfg:          cfg,
		client:       gitea.NewHTTPClient(cfg.AllowPrivateNet, 30*time.Second),
		githubClient: newGitHubHTTPClient(cfg.GitHubAllowPrivateNet),
		encKey:       encKey,
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

// UpdateExternalURL refreshes the public GitSeer URL used for OAuth redirect_uri.
func (s *Service) UpdateExternalURL(externalURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.ExternalURL = strings.TrimRight(strings.TrimSpace(externalURL), "/")
}

// SetEncryptionKey applies a 32-byte AES key at runtime (setup wizard).
func (s *Service) SetEncryptionKey(key []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(key) != 32 {
		return
	}
	s.encKey = append([]byte(nil), key...)
}

// SafeRedirectPath allows only same-app relative paths (no scheme, no // or /\).
func SafeRedirectPath(redirectTo string) string {
	redirectTo = strings.TrimSpace(redirectTo)
	if redirectTo == "" {
		return "/"
	}
	// Require a single leading "/", and reject protocol-relative "//" / "/\" forms.
	if len(redirectTo) < 1 || redirectTo[0] != '/' ||
		(len(redirectTo) > 1 && (redirectTo[1] == '/' || redirectTo[1] == '\\')) {
		return "/"
	}
	if strings.Contains(redirectTo, "://") || strings.ContainsAny(redirectTo, "\\\r\n\t") {
		return "/"
	}
	return redirectTo
}

// ApplyPathPrefix prepends the app path prefix to a SafeRedirectPath result when needed.
func ApplyPathPrefix(redirectTo, prefix string) string {
	redirectTo = SafeRedirectPath(redirectTo)
	if prefix == "" || strings.HasPrefix(redirectTo, prefix) {
		return redirectTo
	}
	if redirectTo == "/" {
		return strings.TrimRight(prefix, "/") + "/"
	}
	return strings.TrimRight(prefix, "/") + redirectTo
}

// constantTimeStringEqual compares a and b in roughly constant time without using a
// password-hashing algorithm. Length is compared first via ConstantTimeEq; both
// values are padded into fixed buffers so ConstantTimeCompare always sees equal length.
func constantTimeStringEqual(a, b string) bool {
	const maxLen = 4096
	if len(a) > maxLen || len(b) > maxLen {
		return false
	}
	var ab, bb [maxLen]byte
	copy(ab[:], a)
	copy(bb[:], b)
	lenEq := subtle.ConstantTimeEq(int32(len(a)), int32(len(b)))
	return subtle.ConstantTimeCompare(ab[:], bb[:]) == 1 && lenEq == 1
}

// LoginBootstrap validates the shared setup password and creates a session.
func (s *Service) LoginBootstrap(ctx context.Context, password, ip, ua string) (*models.User, string, error) {
	s.mu.RLock()
	bootstrapPW := s.cfg.BootstrapPassword
	s.mu.RUnlock()
	if bootstrapPW == "" {
		return nil, "", fmt.Errorf("bootstrap auth is not configured")
	}
	if !constantTimeStringEqual(password, bootstrapPW) {
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

// BeginOAuth creates PKCE state, sets an HttpOnly state cookie, and returns the Gitea authorize URL.
func (s *Service) BeginOAuth(ctx context.Context, w http.ResponseWriter, redirectTo string) (authorizeURL string, err error) {
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
	var giteaInstanceID *int64
	if inst, ierr := s.store.GetPrimaryGiteaInstance(ctx); ierr == nil && inst != nil {
		id := inst.ID
		giteaInstanceID = &id
	}
	if err := s.store.SaveOAuthState(ctx, state, verifier, redirectTo, expires, store.OAuthStateMeta{
		Provider:   "gitea",
		InstanceID: giteaInstanceID,
	}); err != nil {
		return "", err
	}
	s.setOAuthStateCookie(w, state)
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

// CompleteOAuth validates state (DB + cookie), exchanges the code, upserts the user, and creates a session.
// Returns user, session cookie token, post-login redirect path, and access token for ACL refresh.
func (s *Service) CompleteOAuth(ctx context.Context, r *http.Request, w http.ResponseWriter, code, state, ip, ua string) (*models.User, string, string, string, error) {
	if !s.OAuthEnabled() {
		return nil, "", "", "", ErrOAuthNotConfigured
	}
	if code == "" || state == "" {
		return nil, "", "", "", fmt.Errorf("missing code or state")
	}
	cookieState := ""
	if c, err := r.Cookie(OAuthStateCookieName); err == nil {
		cookieState = c.Value
	}
	s.clearOAuthStateCookie(w)
	if cookieState == "" || subtle.ConstantTimeCompare([]byte(cookieState), []byte(state)) != 1 {
		return nil, "", "", "", fmt.Errorf("oauth state cookie mismatch")
	}
	verifier, redirectTo, meta, err := s.store.TakeOAuthState(ctx, state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", "", "", fmt.Errorf("invalid or expired oauth state")
		}
		return nil, "", "", "", err
	}
	if meta.Provider != "" && meta.Provider != "gitea" {
		return nil, "", "", "", fmt.Errorf("oauth state provider mismatch")
	}
	_ = meta
	redirectTo = SafeRedirectPath(redirectTo)
	tok, err := s.exchangeCode(ctx, code, verifier)
	if err != nil {
		return nil, "", "", "", err
	}
	gu, err := s.fetchGiteaUser(ctx, tok.AccessToken)
	if err != nil {
		return nil, "", "", "", err
	}
	inst, err := s.store.GetPrimaryGiteaInstance(ctx)
	var instanceID *int64
	if meta.InstanceID != nil {
		instanceID = meta.InstanceID
	} else if err == nil && inst != nil {
		id := inst.ID
		instanceID = &id
	}
	user, err := s.store.UpsertGiteaUser(ctx, instanceID, *gu)
	if err != nil {
		if errors.Is(err, store.ErrReservedLogin) || errors.Is(err, store.ErrBootstrapClash) {
			return nil, "", "", "", fmt.Errorf("oauth login %q is reserved for the GitSeer bootstrap admin", gu.Login)
		}
		if errors.Is(err, store.ErrLoginConflict) {
			return nil, "", "", "", fmt.Errorf("oauth login %q is already linked to another account", gu.Login)
		}
		return nil, "", "", "", err
	}
	tokenInstanceID := int64(0)
	if instanceID != nil {
		tokenInstanceID = *instanceID
	}
	if err := s.persistUserToken(ctx, user.ID, tokenInstanceID, tok); err != nil {
		// Non-fatal for session creation (login still succeeds), but ACL refresh needs the token.
		slog.Warn("oauth token persist failed", "err", err, "user_id", user.ID, "login", user.Login)
	}
	sessionToken, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, "", "", "", err
	}
	return user, sessionToken, redirectTo, tok.AccessToken, nil
}

func (s *Service) persistUserToken(ctx context.Context, userID, instanceID int64, tok *tokenResponse) error {
	if len(s.encKey) != 32 || tok == nil || tok.AccessToken == "" || instanceID <= 0 {
		return nil
	}
	accessCipher, err := gitseercrypto.Encrypt(s.encKey, tok.AccessToken)
	if err != nil {
		return err
	}
	refreshCipher := ""
	if tok.RefreshToken != "" {
		refreshCipher, err = gitseercrypto.Encrypt(s.encKey, tok.RefreshToken)
		if err != nil {
			return err
		}
	}
	var expires *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expires = &t
	}
	return s.store.SaveUserToken(ctx, userID, instanceID, accessCipher, refreshCipher, expires)
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

// UserAccessToken decrypts a stored OAuth access token for userID.
// PreferInstanceID, when >0, selects that forge instance's token; otherwise any token is returned
// (Gitea primary first via the user's instance_id).
// Returns empty string when encryption is unset or no token is stored.
func (s *Service) UserAccessToken(ctx context.Context, userID int64) (string, error) {
	prefer := int64(0)
	if u, err := s.store.GetUserByID(ctx, userID); err == nil && u != nil && u.InstanceID != nil {
		prefer = *u.InstanceID
	}
	return s.UserAccessTokenForInstance(ctx, userID, prefer)
}

// UserAccessTokenForInstance decrypts the stored OAuth access token for userID on instanceID.
// When instanceID is 0, returns any decryptable token for the user.
func (s *Service) UserAccessTokenForInstance(ctx context.Context, userID, instanceID int64) (string, error) {
	if len(s.encKey) != 32 {
		return "", nil
	}
	var row *store.UserTokenRow
	var err error
	if instanceID > 0 {
		row, err = s.store.GetUserToken(ctx, userID, instanceID)
	} else {
		row, err = s.store.GetAnyUserToken(ctx, userID, 0)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if row.AccessCipher == "" {
		return "", nil
	}
	access, err := gitseercrypto.Decrypt(s.encKey, row.AccessCipher)
	if err != nil {
		return "", err
	}
	if !tokenNeedsRefresh(row.ExpiresAt) || row.RefreshCipher == "" {
		return access, nil
	}
	refresh, err := gitseercrypto.Decrypt(s.encKey, row.RefreshCipher)
	if err != nil || refresh == "" {
		return access, nil
	}
	tok, err := s.refreshAccessTokenForInstance(ctx, row.InstanceID, refresh)
	if err != nil {
		// Keep serving the existing access token; caller may still succeed until hard expiry.
		return access, nil
	}
	if err := s.persistUserToken(ctx, userID, row.InstanceID, tok); err != nil {
		slog.Warn("oauth refreshed token persist failed", "err", err, "user_id", userID)
		return tok.AccessToken, nil
	}
	return tok.AccessToken, nil
}

func tokenNeedsRefresh(expires *time.Time) bool {
	if expires == nil {
		return false
	}
	return time.Now().UTC().Add(2 * time.Minute).After(expires.UTC())
}

func (s *Service) refreshAccessTokenForInstance(ctx context.Context, instanceID int64, refreshToken string) (*tokenResponse, error) {
	if instanceID > 0 {
		if inst, err := s.store.GetInstanceByID(ctx, instanceID); err == nil && inst != nil {
			ft := inst.ForgeType
			if ft == "" {
				ft = models.ForgeTypeGitea
			}
			if ft == models.ForgeTypeGitHub {
				return s.refreshGitHubAccessToken(ctx, refreshToken)
			}
		}
	}
	return s.refreshAccessToken(ctx, refreshToken)
}

func (s *Service) refreshAccessToken(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	s.mu.RLock()
	baseURL := s.cfg.GiteaBaseURL
	clientID := s.cfg.OAuthClientID
	clientSecret := s.cfg.OAuthClientSecret
	client := s.client
	s.mu.RUnlock()
	endpoint := strings.TrimRight(baseURL, "/") + "/login/oauth/access_token"
	body := map[string]string{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
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
		return nil, fmt.Errorf("token refresh failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token refresh missing access_token")
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	return &tok, nil
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

func (s *Service) setOAuthStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     OAuthStateCookieName,
		Value:    state,
		Path:     s.cfg.CookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   int((10 * time.Minute).Seconds()),
	})
}

func (s *Service) clearOAuthStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     OAuthStateCookieName,
		Value:    "",
		Path:     s.cfg.CookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   -1,
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

// PathPrefixFromExternalURL returns the URL path (e.g. "/gitseer") or "/".
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
