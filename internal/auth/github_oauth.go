package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/forge/github"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

const (
	gitHubOAuthProvider = "github"
	// Classic OAuth App scopes: identity + private repo listing (ACL + future write ops).
	gitHubOAuthScopes = "read:user user:email repo"
)

func newGitHubHTTPClient(allowPrivate bool) *http.Client {
	return github.NewHTTPClient(allowPrivate, 30*time.Second)
}

// GitHubOAuthEnabled reports whether GitHub Authorization Code + PKCE login is configured.
func (s *Service) GitHubOAuthEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.GitHubBaseURL != "" && s.cfg.GitHubOAuthClientID != "" && s.cfg.ExternalURL != ""
}

// GitHubRedirectURI is the callback registered on the GitHub OAuth App.
func (s *Service) GitHubRedirectURI() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	base := strings.TrimRight(s.cfg.ExternalURL, "/")
	return base + "/api/v1/auth/github/callback"
}

// UpdateGitHubAuth refreshes GitHub base URL, OAuth client credentials, and HTTP client.
func (s *Service) UpdateGitHubAuth(baseURL, clientID, clientSecret string, allowPrivateNet bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.GitHubBaseURL = strings.TrimSpace(baseURL)
	s.cfg.GitHubOAuthClientID = strings.TrimSpace(clientID)
	s.cfg.GitHubOAuthClientSecret = clientSecret
	s.cfg.GitHubAllowPrivateNet = allowPrivateNet
	s.githubClient = newGitHubHTTPClient(allowPrivateNet)
}

func (s *Service) gitHubOAuthRedirectURILocked() string {
	base := strings.TrimRight(s.cfg.ExternalURL, "/")
	return base + "/api/v1/auth/github/callback"
}

// BeginGitHubOAuth starts GitHub Authorization Code + PKCE.
// When linkUserID is non-nil, the resulting GitHub identity is attached to that user.
func (s *Service) BeginGitHubOAuth(ctx context.Context, w http.ResponseWriter, redirectTo string, linkUserID *int64) (authorizeURL string, err error) {
	if !s.GitHubOAuthEnabled() {
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

	inst, err := s.store.GetPrimaryGitHubInstance(ctx)
	if err != nil || inst == nil {
		return "", fmt.Errorf("no github instance configured")
	}
	instID := inst.ID

	if err := s.store.SaveOAuthState(ctx, state, verifier, redirectTo, expires, store.OAuthStateMeta{
		Provider:   gitHubOAuthProvider,
		InstanceID: &instID,
		LinkUserID: linkUserID,
	}); err != nil {
		return "", err
	}
	s.setOAuthStateCookie(w, state)

	s.mu.RLock()
	clientID := s.cfg.GitHubOAuthClientID
	baseURL := s.cfg.GitHubBaseURL
	redirectURI := s.gitHubOAuthRedirectURILocked()
	s.mu.RUnlock()

	webBase, err := github.OAuthWebBase(baseURL)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", gitHubOAuthScopes)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return strings.TrimRight(webBase, "/") + "/login/oauth/authorize?" + q.Encode(), nil
}

// CompleteGitHubOAuthResult is returned after a successful GitHub OAuth callback.
type CompleteGitHubOAuthResult struct {
	User           *models.User
	SessionToken   string
	RedirectTo     string
	AccessToken    string
	InstanceID     int64
	LinkedExisting bool
}

// CompleteGitHubOAuth validates state, exchanges the code, upserts/links the user, and creates a session.
func (s *Service) CompleteGitHubOAuth(ctx context.Context, r *http.Request, w http.ResponseWriter, code, state, ip, ua string) (*CompleteGitHubOAuthResult, error) {
	if !s.GitHubOAuthEnabled() {
		return nil, ErrOAuthNotConfigured
	}
	if code == "" || state == "" {
		return nil, fmt.Errorf("missing code or state")
	}
	cookieState := ""
	if c, err := r.Cookie(OAuthStateCookieName); err == nil {
		cookieState = c.Value
	}
	s.clearOAuthStateCookie(w)
	if cookieState == "" || subtle.ConstantTimeCompare([]byte(cookieState), []byte(state)) != 1 {
		return nil, fmt.Errorf("oauth state cookie mismatch")
	}
	verifier, redirectTo, meta, err := s.store.TakeOAuthState(ctx, state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("invalid or expired oauth state")
		}
		return nil, err
	}
	if meta.Provider != gitHubOAuthProvider {
		return nil, fmt.Errorf("oauth state provider mismatch")
	}
	redirectTo = SafeRedirectPath(redirectTo)

	tok, err := s.exchangeGitHubCode(ctx, code, verifier)
	if err != nil {
		return nil, err
	}
	gu, err := s.fetchGitHubUser(ctx, tok.AccessToken)
	if err != nil {
		return nil, err
	}

	instanceID := int64(0)
	if meta.InstanceID != nil {
		instanceID = *meta.InstanceID
	} else if inst, ierr := s.store.GetPrimaryGitHubInstance(ctx); ierr == nil && inst != nil {
		instanceID = inst.ID
	}
	if instanceID <= 0 {
		return nil, fmt.Errorf("github instance not found")
	}

	user, err := s.store.UpsertGitHubUser(ctx, instanceID, *gu, meta.LinkUserID)
	if err != nil {
		if errors.Is(err, store.ErrReservedLogin) || errors.Is(err, store.ErrBootstrapClash) {
			return nil, fmt.Errorf("oauth login %q is reserved for the GitSeer bootstrap admin", gu.Login)
		}
		if errors.Is(err, store.ErrLoginConflict) {
			return nil, fmt.Errorf("oauth login %q is already linked to another account; sign in with that account and use Link GitHub", gu.Login)
		}
		if errors.Is(err, store.ErrIdentityLinked) {
			return nil, fmt.Errorf("this GitHub identity is already linked to another GitSeer account")
		}
		return nil, err
	}
	if err := s.persistUserToken(ctx, user.ID, instanceID, tok); err != nil {
		slog.Warn("github oauth token persist failed", "err", err, "user_id", user.ID, "login", user.Login)
	}
	sessionToken, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, err
	}
	return &CompleteGitHubOAuthResult{
		User:           user,
		SessionToken:   sessionToken,
		RedirectTo:     redirectTo,
		AccessToken:    tok.AccessToken,
		InstanceID:     instanceID,
		LinkedExisting: meta.LinkUserID != nil,
	}, nil
}

func (s *Service) exchangeGitHubCode(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	s.mu.RLock()
	baseURL := s.cfg.GitHubBaseURL
	clientID := s.cfg.GitHubOAuthClientID
	clientSecret := s.cfg.GitHubOAuthClientSecret
	redirectURI := s.gitHubOAuthRedirectURILocked()
	client := s.githubClient
	s.mu.RUnlock()

	webBase, err := github.OAuthWebBase(baseURL)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(webBase, "/") + "/login/oauth/access_token"
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
		return nil, fmt.Errorf("github token exchange failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("decode github token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("github token response missing access_token")
	}
	return &tok, nil
}

func (s *Service) refreshGitHubAccessToken(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	s.mu.RLock()
	baseURL := s.cfg.GitHubBaseURL
	clientID := s.cfg.GitHubOAuthClientID
	clientSecret := s.cfg.GitHubOAuthClientSecret
	client := s.githubClient
	s.mu.RUnlock()

	webBase, err := github.OAuthWebBase(baseURL)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(webBase, "/") + "/login/oauth/access_token"
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
		return nil, fmt.Errorf("github token refresh failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("decode github token refresh: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("github token refresh missing access_token")
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	return &tok, nil
}

func (s *Service) fetchGitHubUser(ctx context.Context, accessToken string) (*models.User, error) {
	s.mu.RLock()
	baseURL := s.cfg.GitHubBaseURL
	client := s.githubClient
	s.mu.RUnlock()

	apiBase, err := github.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(apiBase, "/") + "/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
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
		return nil, fmt.Errorf("fetch github user failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var gu struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(raw, &gu); err != nil {
		return nil, err
	}
	id := gu.ID
	return &models.User{
		GitHubUserID: &id,
		Login:        gu.Login,
		Email:        gu.Email,
		DisplayName:  gu.Name,
		AvatarURL:    gu.AvatarURL,
	}, nil
}
