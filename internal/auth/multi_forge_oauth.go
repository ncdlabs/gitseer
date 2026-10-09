package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/forge/bitbucket"
	"github.com/ncdlabs/gitseer/internal/forge/gitea"
	"github.com/ncdlabs/gitseer/internal/forge/gitlab"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

const (
	gitLabOAuthProvider    = "gitlab"
	bitbucketOAuthProvider = "bitbucket"
	forgejoOAuthProvider   = "forgejo"
	bitbucketOAuthScopes   = "account repository"
)

// CompleteForgeOAuthResult is returned after a successful non-Gitea forge OAuth callback.
type CompleteForgeOAuthResult struct {
	User           *models.User
	SessionToken   string
	RedirectTo     string
	AccessToken    string
	InstanceID     int64
	Provider       string
	LinkedExisting bool
}

// --- GitLab ---

func (s *Service) GitLabOAuthEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.GitLabBaseURL != "" && s.cfg.GitLabOAuthClientID != "" && s.cfg.ExternalURL != ""
}

func (s *Service) GitLabRedirectURI() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/gitlab/callback"
}

func (s *Service) UpdateGitLabAuth(baseURL, clientID, clientSecret string, allowPrivateNet bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.GitLabBaseURL = strings.TrimSpace(baseURL)
	s.cfg.GitLabOAuthClientID = strings.TrimSpace(clientID)
	s.cfg.GitLabOAuthClientSecret = clientSecret
	s.cfg.GitLabAllowPrivateNet = allowPrivateNet
	s.gitlabClient = gitlab.NewHTTPClient(allowPrivateNet, 30*time.Second)
}

func (s *Service) BeginGitLabOAuth(ctx context.Context, w http.ResponseWriter, redirectTo string, linkUserID *int64) (string, error) {
	if !s.GitLabOAuthEnabled() {
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

	inst, err := s.store.GetPrimaryGitLabInstance(ctx)
	if err != nil || inst == nil {
		return "", fmt.Errorf("no gitlab instance configured")
	}
	instID := inst.ID
	if err := s.store.SaveOAuthState(ctx, state, verifier, redirectTo, expires, store.OAuthStateMeta{
		Provider: gitLabOAuthProvider, InstanceID: &instID, LinkUserID: linkUserID,
	}); err != nil {
		return "", err
	}
	s.setOAuthStateCookie(w, state)

	s.mu.RLock()
	clientID := s.cfg.GitLabOAuthClientID
	baseURL := s.cfg.GitLabBaseURL
	redirectURI := strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/gitlab/callback"
	s.mu.RUnlock()
	return gitlab.BuildAuthorizeURL(baseURL, clientID, redirectURI, state, challenge)
}

func (s *Service) CompleteGitLabOAuth(ctx context.Context, r *http.Request, w http.ResponseWriter, code, state, ip, ua string) (*CompleteForgeOAuthResult, error) {
	if !s.GitLabOAuthEnabled() {
		return nil, ErrOAuthNotConfigured
	}
	verifier, redirectTo, meta, err := s.takeOAuthState(r, w, code, state, gitLabOAuthProvider)
	if err != nil {
		return nil, err
	}
	tok, err := s.exchangeGitLabCode(ctx, code, verifier)
	if err != nil {
		return nil, err
	}
	gu, err := s.fetchGitLabUser(ctx, tok.AccessToken)
	if err != nil {
		return nil, err
	}
	instanceID := oauthInstanceID(meta, func() (*models.Instance, error) { return s.store.GetPrimaryGitLabInstance(ctx) })
	if instanceID <= 0 {
		return nil, fmt.Errorf("gitlab instance not found")
	}
	user, err := s.store.UpsertGitLabUser(ctx, instanceID, *gu, meta.LinkUserID)
	if err != nil {
		return nil, mapOAuthUpsertErr(err, gu.Login, "GitLab")
	}
	if err := s.persistUserToken(ctx, user.ID, instanceID, tok); err != nil {
		return nil, fmt.Errorf("persist gitlab oauth token: %w", err)
	}
	sessionToken, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, err
	}
	return &CompleteForgeOAuthResult{
		User: user, SessionToken: sessionToken, RedirectTo: redirectTo,
		AccessToken: tok.AccessToken, InstanceID: instanceID, Provider: gitLabOAuthProvider,
		LinkedExisting: meta.LinkUserID != nil,
	}, nil
}

func (s *Service) exchangeGitLabCode(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	s.mu.RLock()
	exchanger := &oauthTokenExchanger{
		endpointFn:  func() (string, error) { return gitlab.TokenEndpoint(s.cfg.GitLabBaseURL) },
		client:      s.gitlabClient,
		clientID:    s.cfg.GitLabOAuthClientID,
		secret:      s.cfg.GitLabOAuthClientSecret,
		style:       tokenExchangeForm,
		redirectURI: strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/gitlab/callback",
	}
	s.mu.RUnlock()
	return exchanger.exchange(ctx, code, verifier)
}

func (s *Service) refreshGitLabAccessToken(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	s.mu.RLock()
	exchanger := &oauthTokenExchanger{
		endpointFn: func() (string, error) { return gitlab.TokenEndpoint(s.cfg.GitLabBaseURL) },
		client:     s.gitlabClient,
		clientID:   s.cfg.GitLabOAuthClientID,
		secret:     s.cfg.GitLabOAuthClientSecret,
		style:      tokenExchangeForm,
	}
	s.mu.RUnlock()
	return exchanger.refresh(ctx, refreshToken)
}

func (s *Service) fetchGitLabUser(ctx context.Context, accessToken string) (*models.User, error) {
	s.mu.RLock()
	baseURL := s.cfg.GitLabBaseURL
	client := s.gitlabClient
	s.mu.RUnlock()
	apiBase, err := gitlab.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBase, "/")+"/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
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
		return nil, fmt.Errorf("fetch gitlab user failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var gu struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(raw, &gu); err != nil {
		return nil, err
	}
	id := gu.ID
	return &models.User{
		GitLabUserID: &id,
		Login:        gu.Username,
		Email:        gu.Email,
		DisplayName:  gu.Name,
		AvatarURL:    gu.AvatarURL,
	}, nil
}

// --- Bitbucket ---

func (s *Service) BitbucketOAuthEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.BitbucketBaseURL != "" && s.cfg.BitbucketOAuthClientID != "" && s.cfg.ExternalURL != ""
}

func (s *Service) BitbucketRedirectURI() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/bitbucket/callback"
}

func (s *Service) UpdateBitbucketAuth(baseURL, clientID, clientSecret string, allowPrivateNet bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.BitbucketBaseURL = strings.TrimSpace(baseURL)
	s.cfg.BitbucketOAuthClientID = strings.TrimSpace(clientID)
	s.cfg.BitbucketOAuthClientSecret = clientSecret
	s.cfg.BitbucketAllowPrivateNet = allowPrivateNet
	s.bitbucketClient = bitbucket.NewHTTPClient(allowPrivateNet, 30*time.Second)
}

func (s *Service) BeginBitbucketOAuth(ctx context.Context, w http.ResponseWriter, redirectTo string, linkUserID *int64) (string, error) {
	if !s.BitbucketOAuthEnabled() {
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

	inst, err := s.store.GetPrimaryBitbucketInstance(ctx)
	if err != nil || inst == nil {
		return "", fmt.Errorf("no bitbucket instance configured")
	}
	instID := inst.ID
	if err := s.store.SaveOAuthState(ctx, state, verifier, redirectTo, expires, store.OAuthStateMeta{
		Provider: bitbucketOAuthProvider, InstanceID: &instID, LinkUserID: linkUserID,
	}); err != nil {
		return "", err
	}
	s.setOAuthStateCookie(w, state)

	s.mu.RLock()
	clientID := s.cfg.BitbucketOAuthClientID
	baseURL := s.cfg.BitbucketBaseURL
	redirectURI := strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/bitbucket/callback"
	s.mu.RUnlock()
	authURL, err := bitbucket.BuildAuthorizeURL(baseURL, clientID, redirectURI, state, challenge)
	if err != nil {
		return "", err
	}
	// Bitbucket Cloud also accepts scope as a query param on authorize.
	if u, perr := url.Parse(authURL); perr == nil {
		q := u.Query()
		if q.Get("scope") == "" {
			q.Set("scope", bitbucketOAuthScopes)
			u.RawQuery = q.Encode()
			authURL = u.String()
		}
	}
	return authURL, nil
}

func (s *Service) CompleteBitbucketOAuth(ctx context.Context, r *http.Request, w http.ResponseWriter, code, state, ip, ua string) (*CompleteForgeOAuthResult, error) {
	if !s.BitbucketOAuthEnabled() {
		return nil, ErrOAuthNotConfigured
	}
	verifier, redirectTo, meta, err := s.takeOAuthState(r, w, code, state, bitbucketOAuthProvider)
	if err != nil {
		return nil, err
	}
	tok, err := s.exchangeBitbucketToken(ctx, code, verifier)
	if err != nil {
		return nil, err
	}
	gu, err := s.fetchBitbucketUser(ctx, tok.AccessToken)
	if err != nil {
		return nil, err
	}
	instanceID := oauthInstanceID(meta, func() (*models.Instance, error) { return s.store.GetPrimaryBitbucketInstance(ctx) })
	if instanceID <= 0 {
		return nil, fmt.Errorf("bitbucket instance not found")
	}
	user, err := s.store.UpsertBitbucketUser(ctx, instanceID, *gu, meta.LinkUserID)
	if err != nil {
		return nil, mapOAuthUpsertErr(err, gu.Login, "Bitbucket")
	}
	if err := s.persistUserToken(ctx, user.ID, instanceID, tok); err != nil {
		return nil, fmt.Errorf("persist bitbucket oauth token: %w", err)
	}
	sessionToken, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, err
	}
	return &CompleteForgeOAuthResult{
		User: user, SessionToken: sessionToken, RedirectTo: redirectTo,
		AccessToken: tok.AccessToken, InstanceID: instanceID, Provider: bitbucketOAuthProvider,
		LinkedExisting: meta.LinkUserID != nil,
	}, nil
}

func (s *Service) exchangeBitbucketToken(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	s.mu.RLock()
	exchanger := &oauthTokenExchanger{
		endpointFn:  func() (string, error) { return bitbucket.TokenEndpoint(s.cfg.BitbucketBaseURL) },
		client:      s.bitbucketClient,
		clientID:    s.cfg.BitbucketOAuthClientID,
		secret:      s.cfg.BitbucketOAuthClientSecret,
		style:       tokenExchangeForm,
		basicAuth:   true,
		redirectURI: strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/bitbucket/callback",
	}
	s.mu.RUnlock()
	return exchanger.exchange(ctx, code, verifier)
}

func (s *Service) refreshBitbucketAccessToken(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	s.mu.RLock()
	exchanger := &oauthTokenExchanger{
		endpointFn: func() (string, error) { return bitbucket.TokenEndpoint(s.cfg.BitbucketBaseURL) },
		client:     s.bitbucketClient,
		clientID:   s.cfg.BitbucketOAuthClientID,
		secret:     s.cfg.BitbucketOAuthClientSecret,
		style:      tokenExchangeForm,
		basicAuth:  true,
	}
	s.mu.RUnlock()
	return exchanger.refresh(ctx, refreshToken)
}

func (s *Service) fetchBitbucketUser(ctx context.Context, accessToken string) (*models.User, error) {
	s.mu.RLock()
	baseURL := s.cfg.BitbucketBaseURL
	client := s.bitbucketClient
	s.mu.RUnlock()
	apiBase, err := bitbucket.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBase, "/")+"/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
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
		return nil, fmt.Errorf("fetch bitbucket user failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var gu struct {
		UUID        string `json:"uuid"`
		AccountID   string `json:"account_id"`
		Username    string `json:"username"`
		Nickname    string `json:"nickname"`
		DisplayName string `json:"display_name"`
		Links       *struct {
			Avatar *struct {
				Href string `json:"href"`
			} `json:"avatar"`
		} `json:"links"`
	}
	if err := json.Unmarshal(raw, &gu); err != nil {
		return nil, err
	}
	login := gu.Username
	if login == "" {
		login = gu.Nickname
	}
	if login == "" {
		login = gu.DisplayName
	}
	avatar := ""
	if gu.Links != nil && gu.Links.Avatar != nil {
		avatar = gu.Links.Avatar.Href
	}
	id := bitbucket.StableID(firstNonEmpty(gu.UUID, gu.AccountID))
	return &models.User{
		BitbucketUserID: &id,
		Login:           login,
		DisplayName:     gu.DisplayName,
		AvatarURL:       avatar,
	}, nil
}

// --- Forgejo (Gitea-compatible OAuth paths) ---

func (s *Service) ForgejoOAuthEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.ForgejoBaseURL != "" && s.cfg.ForgejoOAuthClientID != "" && s.cfg.ExternalURL != ""
}

func (s *Service) ForgejoRedirectURI() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/forgejo/callback"
}

func (s *Service) UpdateForgejoAuth(baseURL, clientID, clientSecret string, allowPrivateNet bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.ForgejoBaseURL = strings.TrimSpace(baseURL)
	s.cfg.ForgejoOAuthClientID = strings.TrimSpace(clientID)
	s.cfg.ForgejoOAuthClientSecret = clientSecret
	s.cfg.ForgejoAllowPrivateNet = allowPrivateNet
	s.forgejoClient = gitea.NewHTTPClient(allowPrivateNet, 30*time.Second)
}

func (s *Service) BeginForgejoOAuth(ctx context.Context, w http.ResponseWriter, redirectTo string, linkUserID *int64) (string, error) {
	if !s.ForgejoOAuthEnabled() {
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

	inst, err := s.store.GetPrimaryForgejoInstance(ctx)
	if err != nil || inst == nil {
		return "", fmt.Errorf("no forgejo instance configured")
	}
	instID := inst.ID
	if err := s.store.SaveOAuthState(ctx, state, verifier, redirectTo, expires, store.OAuthStateMeta{
		Provider: forgejoOAuthProvider, InstanceID: &instID, LinkUserID: linkUserID,
	}); err != nil {
		return "", err
	}
	s.setOAuthStateCookie(w, state)

	s.mu.RLock()
	clientID := s.cfg.ForgejoOAuthClientID
	baseURL := s.cfg.ForgejoBaseURL
	redirectURI := strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/forgejo/callback"
	s.mu.RUnlock()
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return strings.TrimRight(baseURL, "/") + "/login/oauth/authorize?" + q.Encode(), nil
}

func (s *Service) CompleteForgejoOAuth(ctx context.Context, r *http.Request, w http.ResponseWriter, code, state, ip, ua string) (*CompleteForgeOAuthResult, error) {
	if !s.ForgejoOAuthEnabled() {
		return nil, ErrOAuthNotConfigured
	}
	verifier, redirectTo, meta, err := s.takeOAuthState(r, w, code, state, forgejoOAuthProvider)
	if err != nil {
		return nil, err
	}
	tok, err := s.exchangeForgejoCode(ctx, code, verifier)
	if err != nil {
		return nil, err
	}
	gu, err := s.fetchForgejoUser(ctx, tok.AccessToken)
	if err != nil {
		return nil, err
	}
	instanceID := oauthInstanceID(meta, func() (*models.Instance, error) { return s.store.GetPrimaryForgejoInstance(ctx) })
	if instanceID <= 0 {
		return nil, fmt.Errorf("forgejo instance not found")
	}
	id := instanceID
	// Forgejo reuses Gitea identity columns; UpsertGiteaUser with linkUserID UPDATEs the
	// target row (same as GitHub/GitLab/Bitbucket) — never orphan-inserts on link.
	user, err := s.store.UpsertGiteaUser(ctx, &id, *gu, meta.LinkUserID)
	if err != nil {
		return nil, mapOAuthUpsertErr(err, gu.Login, "Forgejo")
	}
	if err := s.persistUserToken(ctx, user.ID, instanceID, tok); err != nil {
		return nil, fmt.Errorf("persist forgejo oauth token: %w", err)
	}
	sessionToken, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, err
	}
	return &CompleteForgeOAuthResult{
		User: user, SessionToken: sessionToken, RedirectTo: redirectTo,
		AccessToken: tok.AccessToken, InstanceID: instanceID, Provider: forgejoOAuthProvider,
		LinkedExisting: meta.LinkUserID != nil,
	}, nil
}

func (s *Service) exchangeForgejoCode(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	s.mu.RLock()
	exchanger := &oauthTokenExchanger{
		endpointFn: func() (string, error) {
			return strings.TrimRight(s.cfg.ForgejoBaseURL, "/") + "/login/oauth/access_token", nil
		},
		client:      s.forgejoClient,
		clientID:    s.cfg.ForgejoOAuthClientID,
		secret:      s.cfg.ForgejoOAuthClientSecret,
		style:       tokenExchangeJSON,
		redirectURI: strings.TrimRight(s.cfg.ExternalURL, "/") + "/api/v1/auth/forgejo/callback",
	}
	s.mu.RUnlock()
	return exchanger.exchange(ctx, code, verifier)
}

func (s *Service) refreshForgejoAccessToken(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	s.mu.RLock()
	exchanger := &oauthTokenExchanger{
		endpointFn: func() (string, error) {
			return strings.TrimRight(s.cfg.ForgejoBaseURL, "/") + "/login/oauth/access_token", nil
		},
		client:   s.forgejoClient,
		clientID: s.cfg.ForgejoOAuthClientID,
		secret:   s.cfg.ForgejoOAuthClientSecret,
		style:    tokenExchangeJSON,
	}
	s.mu.RUnlock()
	return exchanger.refresh(ctx, refreshToken)
}

func (s *Service) fetchForgejoUser(ctx context.Context, accessToken string) (*models.User, error) {
	s.mu.RLock()
	baseURL := s.cfg.ForgejoBaseURL
	client := s.forgejoClient
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
		return nil, fmt.Errorf("fetch forgejo user failed: %s: %s", resp.Status, truncate(string(raw), 200))
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

// --- shared helpers ---

func (s *Service) takeOAuthState(r *http.Request, w http.ResponseWriter, code, state, wantProvider string) (verifier, redirectTo string, meta store.OAuthStateMeta, err error) {
	if code == "" || state == "" {
		return "", "", meta, fmt.Errorf("missing code or state")
	}
	cookieState := ""
	if c, cerr := r.Cookie(OAuthStateCookieName); cerr == nil {
		cookieState = c.Value
	}
	s.clearOAuthStateCookie(w)
	if cookieState == "" || subtle.ConstantTimeCompare([]byte(cookieState), []byte(state)) != 1 {
		return "", "", meta, fmt.Errorf("oauth state cookie mismatch")
	}
	verifier, redirectTo, meta, err = s.store.TakeOAuthState(r.Context(), state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", meta, fmt.Errorf("invalid or expired oauth state")
		}
		return "", "", meta, err
	}
	if meta.Provider != wantProvider {
		return "", "", meta, fmt.Errorf("oauth state provider mismatch")
	}
	return verifier, SafeRedirectPath(redirectTo), meta, nil
}

func oauthInstanceID(meta store.OAuthStateMeta, fallback func() (*models.Instance, error)) int64 {
	if meta.InstanceID != nil {
		return *meta.InstanceID
	}
	if inst, err := fallback(); err == nil && inst != nil {
		return inst.ID
	}
	return 0
}

func mapOAuthUpsertErr(err error, login, forge string) error {
	if errors.Is(err, store.ErrReservedLogin) || errors.Is(err, store.ErrBootstrapClash) {
		return fmt.Errorf("oauth login %q is reserved for the GitSeer bootstrap admin", login)
	}
	if errors.Is(err, store.ErrLoginConflict) {
		return fmt.Errorf("oauth login %q is already linked to another account; sign in with that account and use Link %s", login, forge)
	}
	if errors.Is(err, store.ErrIdentityLinked) {
		return fmt.Errorf("this %s identity is already linked to another GitSeer account", forge)
	}
	return err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
