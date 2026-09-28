// Package api implements /api/v1 HTTP handlers.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/attention"
	"github.com/ncdlabs/gitseer/internal/auth"
	"github.com/ncdlabs/gitseer/internal/authz"
	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/forge"
	_ "github.com/ncdlabs/gitseer/internal/forge/all"
	"github.com/ncdlabs/gitseer/internal/forge/bitbucket"
	"github.com/ncdlabs/gitseer/internal/forge/forgejo"
	"github.com/ncdlabs/gitseer/internal/forge/gitea"
	"github.com/ncdlabs/gitseer/internal/forge/github"
	"github.com/ncdlabs/gitseer/internal/forge/gitlab"
	gitseermetrics "github.com/ncdlabs/gitseer/internal/metrics"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/notify"
	"github.com/ncdlabs/gitseer/internal/ratelimit"
	"github.com/ncdlabs/gitseer/internal/realtime"
	"github.com/ncdlabs/gitseer/internal/settings"
	"github.com/ncdlabs/gitseer/internal/store"
	"github.com/ncdlabs/gitseer/internal/sync"
	"github.com/ncdlabs/gitseer/internal/theme"
	"github.com/ncdlabs/gitseer/internal/webhooks"
	"github.com/ncdlabs/gitseer/internal/workflows"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// errUserTokenRequired is returned when an OAuth user has no decryptable forge token for user-scoped calls.
var errUserTokenRequired = errors.New("user forge token unavailable; sign in with OAuth again (requires GITSEER_ENCRYPTION_KEY)")

type ctxKey int

const userCtxKey ctxKey = 1

type Handler struct {
	cfg      config.Config
	store    *store.Store
	auth     *auth.Service
	authz    *authz.Service
	syncer   *sync.Service
	wh       *webhooks.Processor
	att      *attention.Engine
	hub      *realtime.Hub
	settings *settings.Manager
	notify   *notify.Service
	log      *slog.Logger
	version  string
}

// SetNotify wires the outbound notification service (optional).
func (h *Handler) SetNotify(n *notify.Service) {
	h.notify = n
}

func New(
	cfg config.Config,
	st *store.Store,
	authsvc *auth.Service,
	authzsvc *authz.Service,
	syncer *sync.Service,
	wh *webhooks.Processor,
	att *attention.Engine,
	hub *realtime.Hub,
	settingsMgr *settings.Manager,
	log *slog.Logger,
	version string,
) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{
		cfg: cfg, store: st, auth: authsvc, authz: authzsvc, syncer: syncer,
		wh: wh, att: att, hub: hub, settings: settingsMgr, log: log, version: version,
	}
}

func (h *Handler) MetricsHandler() http.Handler {
	gitseermetrics.Register()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitseermetrics.RefreshGauges(r.Context(), h.store)
		promhttp.Handler().ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := strings.TrimSpace(h.cfg.Server.MetricsToken)
		if want != "" {
			// When a scrape token is configured, require Bearer only (no session bypass).
			if h.metricsBearerOK(r) {
				inner.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h.requireAuth(inner).ServeHTTP(w, r)
	})
}

// metricsBearerOK accepts Authorization: Bearer <token> when server.metrics_token is configured.
func (h *Handler) metricsBearerOK(r *http.Request) bool {
	want := strings.TrimSpace(h.cfg.Server.MetricsToken)
	if want == "" {
		return false
	}
	got := ""
	if hdr := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
		got = strings.TrimSpace(hdr[7:])
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// bootstrapAuthAvailable is true when the bootstrap password is set and either
// setup is incomplete or local skip-setup recovery is enabled.
func (h *Handler) bootstrapAuthAvailable() bool {
	if !h.auth.BootstrapEnabled() {
		return false
	}
	if h.settings != nil && h.settings.SetupCompleted() && !h.cfg.Dev.AllowSkipSetup {
		return false
	}
	return true
}

func (h *Handler) Routes(r chi.Router) {
	trusted, _ := h.cfg.TrustedProxyNets()
	authLimit := ratelimit.New(time.Minute, 20, trusted...)
	webhookLimit := ratelimit.New(time.Minute, 120, trusted...)

	r.Route("/api/v1", func(r chi.Router) {
		r.With(h.requireAuth).Get("/system/status", h.systemStatus)
		r.With(h.requireAuth).Get("/settings", h.getSettings)
		r.With(h.requireAuth, h.requireCSRF).Put("/settings", h.putSettings)

		r.With(h.requireAuth).Get("/notifications/settings", h.getNotificationSettings)
		r.With(h.requireAuth, h.requireCSRF).Put("/notifications/settings", h.putNotificationSettings)
		r.With(h.requireAuth, h.requireCSRF).Post("/notifications/test", h.sendTestNotification)

		r.With(h.requireAuth).Get("/instances", h.listInstances)
		r.With(h.requireAuth, h.requireCSRF).Post("/instances", h.createInstance)
		r.With(h.requireAuth, h.requireCSRF).Put("/instances/{id}", h.updateInstance)
		r.With(h.requireAuth, h.requireCSRF).Delete("/instances/{id}", h.deleteInstance)
		r.With(h.requireAuth, h.requireCSRF).Post("/instances/{id}/ensure-webhook", h.ensureInstanceWebhook)
		r.With(h.requireAuth, h.requireCSRF).Post("/instances/{id}/verify-webhook", h.verifyInstanceWebhook)
		r.With(h.requireAuth).Get("/instances/{id}/gitea-ui-snippets", h.downloadGiteaUISnippets)
		r.With(h.requireAuth, h.requireCSRF).Post("/admin/purge-retention", h.purgeRetention)

		r.Route("/auth", func(r chi.Router) {
			r.With(authLimit.Middleware).Get("/login", h.oauthLogin)
			r.Get("/callback", h.oauthCallback)
			r.With(authLimit.Middleware).Get("/github/login", h.githubOAuthLogin)
			r.Get("/github/callback", h.githubOAuthCallback)
			r.With(authLimit.Middleware).Get("/gitlab/login", h.forgeOAuthLogin("gitlab"))
			r.Get("/gitlab/callback", h.forgeOAuthCallback("gitlab"))
			r.With(authLimit.Middleware).Get("/bitbucket/login", h.forgeOAuthLogin("bitbucket"))
			r.Get("/bitbucket/callback", h.forgeOAuthCallback("bitbucket"))
			r.With(authLimit.Middleware).Get("/forgejo/login", h.forgeOAuthLogin("forgejo"))
			r.Get("/forgejo/callback", h.forgeOAuthCallback("forgejo"))
			r.With(authLimit.Middleware, h.requireCSRF).Post("/bootstrap/login", h.bootstrapLogin)
			r.With(h.requireCSRF).Post("/logout", h.logout)
			r.Get("/me", h.me)
		})

		r.With(h.requireAuth).Get("/users", h.listUsers)
		r.With(h.requireAuth).Get("/users/{id}/access", h.getUserAccess)
		r.With(h.requireAuth, h.requireCSRF).Put("/users/{id}/access", h.putUserAccess)

		r.With(h.requireAuth).Get("/setup/encryption", h.setupGetEncryption)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/encryption", h.setupSetEncryption)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/test-connection", h.setupTestConnection)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/check-gitea-url", h.setupCheckGiteaURL)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/create-webhook", h.setupCreateWebhook)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/create-oauth", h.setupCreateOAuth)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/complete", h.setupComplete)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/sync-repos", h.syncRepos)
		r.With(h.requireAuth).Get("/summary", h.summary)
		r.With(h.requireAuth).Get("/organizations", h.listOrganizations)
		r.With(h.requireAuth).Get("/stats", h.stats)
		r.With(h.requireAuth).Get("/runners/utilization", h.runnerUtilization)
		r.With(h.requireAuth).Get("/flaky-jobs", h.listFlakyJobs)
		r.With(h.requireAuth).Get("/releases", h.listReleases)
		r.With(h.requireAuth).Get("/wallboard/tokens", h.listWallboardTokens)
		r.With(h.requireAuth, h.requireCSRF).Post("/wallboard/tokens", h.createWallboardToken)
		r.With(h.requireAuth, h.requireCSRF).Delete("/wallboard/tokens/{id}", h.revokeWallboardToken)
		r.With(h.requireWallboardToken).Get("/wallboard/snapshot", h.wallboardSnapshot)
		r.With(h.requireAuth).Get("/attention", h.listAttention)
		r.With(h.requireAuth).Get("/attention/rule-overrides", h.getAttentionRuleOverrides)
		r.With(h.requireAuth, h.requireCSRF).Put("/attention/rule-overrides", h.putAttentionRuleOverrides)
		r.With(h.requireAuth, h.requireCSRF).Post("/attention/{id}/mute", h.muteAttention)
		r.With(h.requireAuth, h.requireCSRF).Delete("/attention/{id}/mute", h.unmuteAttention)
		r.With(h.requireAuth).Get("/attention/{id}/log-snippet", h.getAttentionLogSnippet)
		r.With(h.requireAuth).Get("/inbox", h.getInbox)
		r.With(h.requireAuth).Get("/saved-filters", h.listSavedFilters)
		r.With(h.requireAuth, h.requireCSRF).Post("/saved-filters", h.createSavedFilter)
		r.With(h.requireAuth, h.requireCSRF).Put("/saved-filters/{id}", h.updateSavedFilter)
		r.With(h.requireAuth, h.requireCSRF).Delete("/saved-filters/{id}", h.deleteSavedFilter)
		r.With(h.requireAuth).Get("/repositories", h.listRepositories)
		r.With(h.requireAuth).Get("/repositories/{owner}/{repo}/health", h.getRepositoryHealth)
		r.With(h.requireAuth).Get("/repositories/{owner}/{repo}/failure-clusters", h.getRepositoryFailureClusters)
		r.With(h.requireAuth).Get("/repositories/{owner}/{repo}/flaky-jobs", h.getRepositoryFlakyJobs)
		r.With(h.requireAuth).Get("/repositories/{owner}/{repo}", h.getRepository)
		r.With(h.requireAuth).Get("/pull-requests", h.listPRs)
		r.With(h.requireAuth).Get("/workflow-runs", h.listRuns)
		r.With(h.requireAuth).Get("/workflow-runs/active", h.listActiveRuns)
		r.With(h.requireAuth).Get("/workflow-runs/{id}", h.getRun)
		r.With(h.requireAuth, h.requireCSRF).Post("/workflow-runs/{id}/rerun", h.rerunWorkflowRun)
		r.With(h.requireAuth, h.requireCSRF).Post("/workflow-runs/{id}/cancel", h.cancelWorkflowRun)
		r.With(h.requireAuth).Get("/jobs/{id}", h.getJob)
		r.With(h.requireAuth).Get("/jobs/{id}/logs", h.getJobLogs)
		r.With(h.requireAuth).Get("/search", h.search)
		r.With(h.requireAuth).Get("/events", h.serveEvents)
	})

	r.With(webhookLimit.Middleware).Post("/api/webhooks/gitea", h.wh.HandleHTTP)
	r.With(webhookLimit.Middleware).Post("/api/webhooks/gitea/{instanceID}", h.webhookByInstance)
	r.With(webhookLimit.Middleware).Post("/api/webhooks/github/{instanceID}", h.webhookGitHubByInstance)
	r.With(webhookLimit.Middleware).Post("/api/webhooks/gitlab/{instanceID}", h.webhookGitLabByInstance)
	r.With(webhookLimit.Middleware).Post("/api/webhooks/bitbucket/{instanceID}", h.webhookBitbucketByInstance)
	r.With(webhookLimit.Middleware).Post("/api/webhooks/forgejo/{instanceID}", h.webhookForgejoByInstance)
}

func (h *Handler) webhookByInstance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "instanceID"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	h.wh.HandleHTTPForInstance(w, r, id)
}

func (h *Handler) webhookGitHubByInstance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "instanceID"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	h.wh.HandleGitHubHTTPForInstance(w, r, id)
}

func (h *Handler) webhookGitLabByInstance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "instanceID"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	h.wh.HandleGitLabHTTPForInstance(w, r, id)
}

func (h *Handler) webhookBitbucketByInstance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "instanceID"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	h.wh.HandleBitbucketHTTPForInstance(w, r, id)
}

func (h *Handler) webhookForgejoByInstance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "instanceID"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid instance", http.StatusBadRequest)
		return
	}
	h.wh.HandleForgejoHTTPForInstance(w, r, id)
}

func (h *Handler) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.auth.ValidCSRF(r) {
			writeError(w, http.StatusForbidden, "csrf validation failed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, err := h.auth.UserFromRequest(r.Context(), r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userCtxKey, user)))
	})
}

func userFromCtx(ctx context.Context) *models.User {
	u, _ := ctx.Value(userCtxKey).(*models.User)
	return u
}

func (h *Handler) scope(user *models.User) authz.Scope {
	return h.authz.ScopeFor(user)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) bootstrapLogin(w http.ResponseWriter, r *http.Request) {
	if !h.bootstrapAuthAvailable() {
		if h.auth.BootstrapEnabled() {
			writeError(w, http.StatusServiceUnavailable, "bootstrap auth is disabled after setup; use OAuth or set GITSEER_ALLOW_SKIP_SETUP for local recovery")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "bootstrap auth is not configured; set GITSEER_AUTH_BOOTSTRAP_PASSWORD")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, token, err := h.auth.LoginBootstrap(r.Context(), body.Password, r.RemoteAddr, r.UserAgent())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inst, _ := h.store.GetPrimaryInstance(r.Context()); inst != nil {
		_ = h.store.GrantBootstrapAllAccessAllInstances(r.Context(), user.ID)
	}
	h.auth.SetSessionCookie(w, token)
	csrf, _ := h.auth.IssueCSRFToken(w)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id": user.ID, "login": user.Login, "display_name": user.DisplayName,
			"is_bootstrap_admin": user.IsBootstrapAdmin,
		},
		"csrf_token": csrf,
	})
}

func (h *Handler) oauthLogin(w http.ResponseWriter, r *http.Request) {
	redirectTo := auth.SafeRedirectPath(r.URL.Query().Get("redirect"))
	url, err := h.auth.BeginOAuth(r.Context(), w, redirectTo)
	if err != nil {
		if errors.Is(err, auth.ErrOAuthNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "oauth is not configured; set GITSEER_AUTH_OAUTH_CLIENT_ID, GITSEER_SERVER_EXTERNAL_URL, and GITSEER_GITEA_URL")
			return
		}
		h.log.Error("oauth begin", "err", err)
		writeError(w, http.StatusInternalServerError, "oauth start failed")
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		writeError(w, http.StatusBadRequest, "oauth denied")
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	user, token, redirectTo, accessToken, err := h.auth.CompleteOAuth(r.Context(), r, w, code, state, r.RemoteAddr, r.UserAgent())
	if err != nil {
		h.log.Error("oauth callback", "err", err)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.refreshUserACL(r.Context(), user.ID, accessToken); err != nil {
		h.log.Warn("acl refresh after oauth", "err", err, "user", user.Login)
	}
	h.auth.SetSessionCookie(w, token)
	_, _ = h.auth.IssueCSRFToken(w)
	http.Redirect(w, r, auth.ApplyPathPrefix(redirectTo, h.cfg.PathPrefix()), http.StatusFound)
}

func (h *Handler) effectiveIntegration() settings.Integration {
	if h.settings != nil {
		return h.settings.Integration()
	}
	return settings.Integration{
		URL:                   h.cfg.Gitea.URL,
		Token:                 h.cfg.Gitea.Token,
		WebhookSecret:         h.cfg.Gitea.WebhookSecret,
		AllowPrivateNetwork:   h.cfg.Gitea.AllowPrivateNetwork,
		AllowUnsignedWebhooks: h.cfg.Gitea.AllowUnsignedWebhooks,
		OAuthClientID:         h.cfg.Auth.OAuthClientID,
		OAuthClientSecret:     h.cfg.Auth.OAuthClientSecret,
	}
}

func (h *Handler) effectiveGitHub() settings.GitHubIntegration {
	if h.settings != nil {
		return h.settings.GitHub()
	}
	return settings.GitHubIntegration{
		URL:                   h.cfg.GitHub.URL,
		Token:                 h.cfg.GitHub.Token,
		WebhookSecret:         h.cfg.GitHub.WebhookSecret,
		AllowPrivateNetwork:   h.cfg.GitHub.AllowPrivateNetwork,
		AllowUnsignedWebhooks: h.cfg.GitHub.AllowUnsignedWebhooks,
	}
}

// forgeClientForUser returns a forge client authenticated as the caller for user-scoped reads (e.g. job logs).
// Bootstrap admins use the service token; OAuth users must have a decryptable stored access token.
// Prefer forgeClientForRepo when the target repository is known.
func (h *Handler) forgeClientForUser(ctx context.Context, user *models.User) (forge.Forge, error) {
	integ := h.effectiveIntegration()
	if integ.URL == "" {
		return nil, fmt.Errorf("gitea not configured")
	}
	token := ""
	switch {
	case user != nil && user.IsBootstrapAdmin:
		token = integ.Token
	case user != nil:
		ut, err := h.auth.UserAccessToken(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		if ut == "" {
			return nil, errUserTokenRequired
		}
		token = ut
	default:
		token = integ.Token
	}
	if token == "" {
		return nil, fmt.Errorf("gitea not configured")
	}
	return gitea.New(integ.URL, token, integ.AllowPrivateNetwork)
}

// forgeClientForRepo selects the forge client for the repository's instance.
// Bootstrap admins use the instance service token. OAuth users use their per-instance
// token when available (Gitea or GitHub). GitHub never silently falls back to the
// service PAT for non-admin users.
func (h *Handler) forgeClientForRepo(ctx context.Context, user *models.User, repo *models.Repository) (forge.Forge, error) {
	if repo == nil || repo.InstanceID <= 0 {
		return h.forgeClientForUser(ctx, user)
	}
	inst, err := h.store.GetInstanceByID(ctx, repo.InstanceID)
	if err != nil || inst == nil {
		return nil, fmt.Errorf("instance not found")
	}
	ft := inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	token := ""
	switch {
	case user != nil && user.IsBootstrapAdmin:
		tok, err := h.instanceSyncToken(ctx, *inst)
		if err != nil || tok == "" {
			if ft == models.ForgeTypeGitHub {
				gh := h.effectiveGitHub()
				if gh.URL == "" || gh.Token == "" {
					return nil, fmt.Errorf("github not configured")
				}
				return github.New(gh.URL, gh.Token, gh.AllowPrivateNetwork)
			}
			return h.forgeClientForUser(ctx, user)
		}
		token = tok
	case user != nil:
		ut, err := h.auth.UserAccessTokenForInstance(ctx, user.ID, inst.ID)
		if err != nil {
			return nil, err
		}
		if ut == "" {
			if ft == models.ForgeTypeGitHub {
				return nil, errUserTokenRequired
			}
			return nil, errUserTokenRequired
		}
		token = ut
	default:
		tok, err := h.instanceSyncToken(ctx, *inst)
		if err != nil || tok == "" {
			return h.forgeClientForUser(ctx, user)
		}
		token = tok
	}
	return forge.NewFromInstance(*inst, token)
}

func (h *Handler) refreshUserACL(ctx context.Context, userID int64, userAccessToken string) error {
	if userAccessToken == "" {
		return fmt.Errorf("missing user token")
	}
	user, err := h.store.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}
	prefer := int64(0)
	if user.InstanceID != nil {
		prefer = *user.InstanceID
	}
	return h.refreshUserACLWithToken(ctx, user, prefer, userAccessToken)
}

// refreshUserACLForInstance refreshes ACL rows for one forge instance using the given user token.
func (h *Handler) refreshUserACLForInstance(ctx context.Context, userID, instanceID int64, userAccessToken string) error {
	if userAccessToken == "" {
		return fmt.Errorf("missing user token")
	}
	user, err := h.store.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}
	return h.refreshUserACLWithToken(ctx, user, instanceID, userAccessToken)
}

func (h *Handler) refreshUserACLWithToken(ctx context.Context, user *models.User, preferInstanceID int64, preferToken string) error {
	instances, err := h.store.ListInstances(ctx)
	if err != nil {
		return err
	}
	tokenInstIDs, _ := h.store.ListUserTokenInstanceIDs(ctx, user.ID)
	tokenSet := map[int64]struct{}{}
	for _, id := range tokenInstIDs {
		tokenSet[id] = struct{}{}
	}

	for _, inst := range instances {
		ft := inst.ForgeType
		if ft == "" {
			ft = models.ForgeTypeGitea
		}
		userToken := ""
		switch {
		case preferInstanceID == inst.ID && preferToken != "":
			userToken = preferToken
		case preferInstanceID > 0 && preferInstanceID != inst.ID:
			// Caller asked to refresh a specific instance only when preferToken matches;
			// still refresh other instances that have stored tokens.
			if _, ok := tokenSet[inst.ID]; !ok {
				continue
			}
			tok, err := h.auth.UserAccessTokenForInstance(ctx, user.ID, inst.ID)
			if err != nil || tok == "" {
				continue
			}
			userToken = tok
		default:
			if _, ok := tokenSet[inst.ID]; !ok {
				// No per-user token for this instance: leave existing ACL rows alone
				// (bootstrap-admin grants for service-PAT-only GitHub setups).
				continue
			}
			tok, err := h.auth.UserAccessTokenForInstance(ctx, user.ID, inst.ID)
			if err != nil || tok == "" {
				continue
			}
			userToken = tok
		}
		if userToken == "" {
			continue
		}
		if ft == models.ForgeTypeGitea {
			if user.InstanceID != nil && *user.InstanceID != inst.ID {
				continue
			}
			if user.InstanceID == nil && preferInstanceID != inst.ID {
				prim, perr := h.store.GetPrimaryGiteaInstance(ctx)
				if perr != nil || prim == nil || prim.ID != inst.ID {
					continue
				}
			}
		} else if ft == models.ForgeTypeGitHub {
			if user.GitHubInstanceID != nil && *user.GitHubInstanceID != inst.ID {
				continue
			}
		} else if ft == models.ForgeTypeGitLab {
			if user.GitLabInstanceID != nil && *user.GitLabInstanceID != inst.ID {
				continue
			}
		} else if ft == models.ForgeTypeBitbucket {
			if user.BitbucketInstanceID != nil && *user.BitbucketInstanceID != inst.ID {
				continue
			}
		} else if ft == models.ForgeTypeForgejo {
			if user.InstanceID != nil && *user.InstanceID != inst.ID {
				continue
			}
		}
		if err := h.replaceACLForInstance(ctx, user.ID, inst, userToken); err != nil {
			return err
		}
	}

	// Legacy: no instance rows yet — use primary Gitea + integration.
	if len(instances) == 0 && preferToken != "" {
		integ := h.effectiveIntegration()
		if integ.URL == "" {
			return fmt.Errorf("missing gitea url or user token")
		}
		client, err := gitea.New(integ.URL, integ.Token, integ.AllowPrivateNetwork)
		if err != nil {
			return err
		}
		inst, err := h.store.GetPrimaryGiteaInstance(ctx)
		if err != nil || inst == nil {
			return fmt.Errorf("no synced instance yet; run sync first")
		}
		var externalIDs []int64
		for page := 1; ; page++ {
			p, err := client.ListAccessibleReposForUser(ctx, preferToken, forge.ListReposOpts{Page: page, PageSize: 50})
			if err != nil {
				return err
			}
			for _, repo := range p.Items {
				externalIDs = append(externalIDs, repo.ExternalID)
			}
			if !p.HasMore {
				break
			}
		}
		ids, err := h.store.MapExternalIDsToRepoIDs(ctx, inst.ID, externalIDs)
		if err != nil {
			return err
		}
		return h.store.ReplaceUserRepoAccessForInstances(ctx, user.ID, []int64{inst.ID}, ids)
	}
	return nil
}

func (h *Handler) replaceACLForInstance(ctx context.Context, userID int64, inst models.Instance, userAccessToken string) error {
	// ListAccessibleReposForUser authenticates as the user; the client token is only a
	// fallback when the forge client requires a non-empty constructor token.
	client, err := forge.NewFromInstance(inst, userAccessToken)
	if err != nil {
		return err
	}
	var repoIDs []int64
	for page := 1; ; page++ {
		p, err := client.ListAccessibleReposForUser(ctx, userAccessToken, forge.ListReposOpts{Page: page, PageSize: 50})
		if err != nil {
			return err
		}
		var externalIDs []int64
		for _, repo := range p.Items {
			externalIDs = append(externalIDs, repo.ExternalID)
		}
		ids, err := h.store.MapExternalIDsToRepoIDs(ctx, inst.ID, externalIDs)
		if err != nil {
			return err
		}
		repoIDs = append(repoIDs, ids...)
		if !p.HasMore {
			break
		}
	}
	return h.store.ReplaceUserRepoAccessForInstances(ctx, userID, []int64{inst.ID}, repoIDs)
}

func (h *Handler) instanceSyncToken(_ context.Context, inst models.Instance) (string, error) {
	if strings.TrimSpace(inst.SyncTokenCiphertext) == "" {
		return "", fmt.Errorf("no sync token")
	}
	if h.settings == nil {
		return "", fmt.Errorf("settings unavailable")
	}
	return h.settings.OpenSecret(inst.SyncTokenCiphertext)
}

// RefreshAllUserACLs reloads ACL rows for users with decryptable OAuth tokens.
func (h *Handler) RefreshAllUserACLs(ctx context.Context) {
	users, err := h.store.ListUsersWithTokenCiphers(ctx)
	if err != nil {
		h.log.Warn("acl refresh list users", "err", err)
		return
	}
	for _, u := range users {
		instIDs, err := h.store.ListUserTokenInstanceIDs(ctx, u.ID)
		if err != nil || len(instIDs) == 0 {
			continue
		}
		if err := h.refreshUserACLWithToken(ctx, &u, 0, ""); err != nil {
			h.log.Warn("acl refresh failed", "user", u.Login, "err", err)
		}
	}
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	_ = h.auth.Logout(r.Context(), r)
	h.auth.ClearSessionCookie(w)
	h.auth.ClearCSRFCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, _, err := h.auth.UserFromRequest(r.Context(), r)
	csrf := ""
	if c, err := r.Cookie(auth.CSRFCookieName); err == nil {
		csrf = c.Value
	}
	if csrf == "" {
		csrf, _ = h.auth.IssueCSRFToken(w)
	}
	if err != nil || user == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
			"csrf_token":    csrf,
		})
		return
	}
	authzMode := "acl"
	if user.IsBootstrapAdmin {
		authzMode = "bootstrap_allow_all"
	}
	out := map[string]any{
		"authenticated":         true,
		"id":                    user.ID,
		"login":                 user.Login,
		"display_name":          user.DisplayName,
		"is_bootstrap_admin":    user.IsBootstrapAdmin,
		"authz":                 authzMode,
		"csrf_token":            csrf,
		"has_gitea":             user.GiteaUserID != nil,
		"has_github":            user.GitHubUserID != nil,
		"has_gitlab":            user.GitLabUserID != nil,
		"has_bitbucket":         user.BitbucketUserID != nil,
		"github_oauth_enabled":  h.auth.GitHubOAuthEnabled(),
		"gitlab_oauth_enabled":  h.auth.GitLabOAuthEnabled(),
		"bitbucket_oauth_enabled": h.auth.BitbucketOAuthEnabled(),
		"forgejo_oauth_enabled": h.auth.ForgejoOAuthEnabled(),
	}
	if themeID, giteaName, ok := h.giteaThemeForUser(r.Context(), user); ok {
		out["theme"] = string(themeID)
		out["gitea_theme"] = giteaName
	}
	writeJSON(w, http.StatusOK, out)
}

// giteaThemeForUser reads the user's Gitea theme preference and maps built-in
// defaults onto GitSeer themes. Custom/unknown themes fail closed (ok=false).
func (h *Handler) giteaThemeForUser(ctx context.Context, user *models.User) (theme.ID, string, bool) {
	integ := h.effectiveIntegration()
	if user == nil || user.IsBootstrapAdmin || integ.URL == "" {
		return "", "", false
	}
	accessToken, err := h.auth.UserAccessToken(ctx, user.ID)
	if err != nil || accessToken == "" {
		return "", "", false
	}
	client, err := gitea.New(integ.URL, integ.Token, integ.AllowPrivateNetwork)
	if err != nil {
		return "", "", false
	}
	settings, err := client.GetUserSettings(ctx, accessToken)
	if err != nil || settings == nil {
		h.log.Debug("gitea user settings", "err", err, "user", user.Login)
		return "", "", false
	}
	id, ok := theme.MapGitea(settings.Theme)
	if !ok {
		return "", settings.Theme, false
	}
	return id, settings.Theme, true
}

func (h *Handler) systemStatus(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	redact := user == nil || !user.IsBootstrapAdmin
	writeJSON(w, http.StatusOK, h.buildSystemStatus(r.Context(), redact))
}

func (h *Handler) buildSystemStatus(ctx context.Context, redactSensitive bool) map[string]any {
	uiName := h.cfg.UI.InstanceName
	integ := h.effectiveIntegration()
	gh := h.effectiveGitHub()
	setupCompleted := false
	if h.settings != nil {
		uiName = h.settings.Get().InstanceName
		setupCompleted = h.settings.SetupCompleted()
	}

	instances, _ := h.store.ListInstances(ctx)
	forges := make([]map[string]any, 0, len(instances))
	var firstGitea, firstGitHub *models.Instance
	for i := range instances {
		inst := &instances[i]
		entry := forgeStatusEntry(inst, redactSensitive)
		h.enrichForgeStatusOps(ctx, inst, entry)
		forges = append(forges, entry)
		switch inst.ForgeType {
		case models.ForgeTypeGitHub:
			if firstGitHub == nil {
				firstGitHub = inst
			}
		default:
			if firstGitea == nil {
				firstGitea = inst
			}
		}
	}

	giteaConfigured := integ.URL != "" && integ.Token != ""
	if !giteaConfigured && firstGitea != nil {
		giteaConfigured = strings.TrimSpace(firstGitea.SyncTokenCiphertext) != ""
	}
	githubConfigured := gh.URL != "" && gh.Token != ""
	if !githubConfigured && firstGitHub != nil {
		githubConfigured = strings.TrimSpace(firstGitHub.SyncTokenCiphertext) != ""
	}

	webhookHMAC := integ.WebhookSecret != ""
	if !webhookHMAC && firstGitea != nil {
		webhookHMAC = strings.TrimSpace(firstGitea.WebhookSecretCiphertext) != ""
	}
	githubWebhookHMAC := gh.WebhookSecret != ""
	if !githubWebhookHMAC && firstGitHub != nil {
		githubWebhookHMAC = strings.TrimSpace(firstGitHub.WebhookSecretCiphertext) != ""
	}

	status := map[string]any{
		"version":             h.version,
		"ui_name":             uiName,
		"gitea_configured":    giteaConfigured,
		"github_configured":   githubConfigured,
		"bootstrap_auth":      h.bootstrapAuthAvailable(),
		"oauth_enabled":             h.auth.OAuthEnabled(),
		"github_oauth_enabled":      h.auth.GitHubOAuthEnabled(),
		"gitlab_oauth_enabled":      h.auth.GitLabOAuthEnabled(),
		"bitbucket_oauth_enabled":   h.auth.BitbucketOAuthEnabled(),
		"forgejo_oauth_enabled":     h.auth.ForgejoOAuthEnabled(),
		"path_prefix":               h.cfg.PathPrefix(),
		"instance_connected":        len(instances) > 0,
		"webhook_hmac":              webhookHMAC,
		"github_webhook_hmac":       githubWebhookHMAC,
		"setup_completed":           setupCompleted,
		"encryption_configured":     h.settings != nil && h.settings.EncryptionConfigured(),
		"oauth_redirect_uri":        h.auth.RedirectURI(),
		"github_oauth_redirect_uri": h.auth.GitHubRedirectURI(),
		"gitlab_oauth_redirect_uri": h.auth.GitLabRedirectURI(),
		"bitbucket_oauth_redirect_uri": h.auth.BitbucketRedirectURI(),
		"forgejo_oauth_redirect_uri":   h.auth.ForgejoRedirectURI(),
		"server_external_url":       h.webhookDeliveryURLBase(),
		"forges":                    forges,
	}
	encHealthy, encSource, encErr := h.encryptionHealth(ctx)
	status["encryption_source"] = encSource
	status["encryption_healthy"] = encHealthy
	if encErr != "" {
		status["encryption_error"] = encErr
	}
	if !redactSensitive {
		globalStats, _ := h.store.WebhookStatsSince(ctx, 0, time.Now().UTC().Add(-24*time.Hour))
		if globalStats != nil {
			status["webhook_stats_24h"] = map[string]any{
				"ok":         globalStats.OK,
				"failed":     globalStats.Failed,
				"pending":    globalStats.Pending,
				"processing": globalStats.Processing,
				"total":      globalStats.Total,
			}
		}
		inFlight, _ := h.store.CountInFlightWorkflowRuns(ctx, 0)
		wfHooks, _ := h.store.CountWebhookEventsByTypeSince(ctx, 0, time.Now().UTC().Add(-24*time.Hour),
			[]string{"workflow_run", "workflow_job", "check_run", "check_suite"})
		status["active_actions_in_flight"] = inFlight
		status["active_actions_hint"] = activeActionsHint(inFlight, wfHooks, map[string]any{})
	}
	if firstGitea != nil {
		status["gitea_version"] = firstGitea.Version
		if strings.TrimSpace(firstGitea.CapabilitiesJSON) != "" {
			status["capabilities"] = json.RawMessage(firstGitea.CapabilitiesJSON)
		}
	}
	if firstGitHub != nil {
		status["github_version"] = firstGitHub.Version
	}
	status["storage"] = h.storageStatus(ctx, redactSensitive)
	return status
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	editable := user != nil && user.IsBootstrapAdmin
	integ := h.settings.IntegrationPublic()
	if !editable {
		// Non-admins get forge URLs / configured flags only — no OAuth client id or SSRF/unsigned flags.
		integ.OAuthClientID = ""
		integ.GiteaAllowPrivateNetwork = false
		integ.GiteaAllowUnsignedWebhooks = false
		integ.GitHubAllowPrivateNetwork = false
		integ.GitHubAllowUnsignedWebhooks = false
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"editable":               editable,
		"settings":               h.settings.Get(),
		"integration":            integ,
		"setup_completed":        h.settings.SetupCompleted(),
		"encryption_configured":  h.settings.EncryptionConfigured(),
		"encryption_source":      h.settings.EncryptionSource(),
		"status":                 h.buildSystemStatus(r.Context(), !editable),
	})
}

type putSettingsBody struct {
	settings.Values
	Integration    *settings.IntegrationPatch `json:"integration,omitempty"`
	SetupCompleted *bool                      `json:"setup_completed,omitempty"`
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	var body putSettingsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	updated := h.settings.Get()
	// Values update when instance_name is present (full settings form); integration-only PUTs skip it.
	if strings.TrimSpace(body.InstanceName) != "" {
		var err error
		updated, err = h.settings.Update(r.Context(), body.Values)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var integ settings.IntegrationPublic
	if body.Integration != nil {
		var err error
		integ, err = h.settings.UpdateIntegration(r.Context(), *body.Integration)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		integ = h.settings.IntegrationPublic()
	}
	if body.SetupCompleted != nil {
		if err := h.settings.SetSetupCompleted(r.Context(), *body.SetupCompleted); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update setup_completed")
			return
		}
	}
	if h.att != nil {
		h.att.SetLongRunningAfter(h.settings.LongRunningAfter())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"editable":        true,
		"settings":        updated,
		"integration":     integ,
		"setup_completed": h.settings.SetupCompleted(),
		"status":          h.buildSystemStatus(r.Context(), false),
	})
}

type setupTestConnectionBody struct {
	ForgeType string `json:"forge_type"` // gitea | github | gitlab | bitbucket | forgejo

	GiteaURL                 string `json:"gitea_url"`
	GiteaToken               string `json:"gitea_token"`
	GiteaAllowPrivateNetwork *bool  `json:"gitea_allow_private_network"`

	GitHubURL                 string `json:"github_url"`
	GitHubToken               string `json:"github_token"`
	GitHubAllowPrivateNetwork *bool  `json:"github_allow_private_network"`

	// Generic fields used by gitlab / bitbucket / forgejo (and accepted as aliases).
	BaseURL             string `json:"base_url"`
	Token               string `json:"token"`
	AllowPrivateNetwork *bool  `json:"allow_private_network"`
}

type setupWebhookBody struct {
	ForgeType string `json:"forge_type"` // gitea | github; default gitea

	GiteaURL                 string `json:"gitea_url"`
	GiteaToken               string `json:"gitea_token"`
	GiteaAllowPrivateNetwork *bool  `json:"gitea_allow_private_network"`
	Create                   bool   `json:"create"` // true = create/update on Gitea; false = save secret only

	GitHubURL                 string `json:"github_url"`
	GitHubToken               string `json:"github_token"`
	GitHubAllowPrivateNetwork *bool  `json:"github_allow_private_network"`
}

func normalizeSetupForgeType(raw string) string {
	ft := strings.ToLower(strings.TrimSpace(raw))
	if models.IsSupportedForgeType(ft) {
		return ft
	}
	if ft == "" {
		return models.ForgeTypeGitea
	}
	return ""
}

func (h *Handler) webhookDeliveryURLBase() string {
	if h.settings != nil {
		if u := h.settings.ExternalURL(); u != "" {
			return u
		}
	}
	return strings.TrimRight(strings.TrimSpace(h.cfg.Server.ExternalURL), "/")
}

func (h *Handler) webhookDeliveryURL() string {
	return h.giteaWebhookDeliveryURL(0)
}

func (h *Handler) giteaWebhookDeliveryURL(instanceID int64) string {
	base := h.webhookDeliveryURLBase()
	if base == "" {
		return ""
	}
	if instanceID > 0 {
		return fmt.Sprintf("%s/api/webhooks/gitea/%d", base, instanceID)
	}
	// Legacy unscoped route still accepted for the primary Gitea instance.
	return base + "/api/webhooks/gitea"
}

func (h *Handler) githubWebhookDeliveryURL(instanceID int64) string {
	base := h.webhookDeliveryURLBase()
	if base == "" {
		return ""
	}
	if instanceID > 0 {
		return fmt.Sprintf("%s/api/webhooks/github/%d", base, instanceID)
	}
	// Never advertise an unscoped GitHub route (none is registered). Preview uses a placeholder.
	return base + "/api/webhooks/github/{instance_id}"
}

func (h *Handler) forgeWebhookDeliveryURL(forgeType string, instanceID int64) string {
	base := h.webhookDeliveryURLBase()
	if base == "" {
		return ""
	}
	ft := strings.ToLower(strings.TrimSpace(forgeType))
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	if instanceID > 0 {
		return fmt.Sprintf("%s/api/webhooks/%s/%d", base, ft, instanceID)
	}
	return fmt.Sprintf("%s/api/webhooks/%s/{instance_id}", base, ft)
}

func (h *Handler) setupClientFromBody(body setupTestConnectionBody) (*gitea.Client, error) {
	integ := h.effectiveIntegration()
	url := strings.TrimSpace(body.GiteaURL)
	if url == "" {
		url = integ.URL
	}
	token := strings.TrimSpace(body.GiteaToken)
	if token == "" {
		token = integ.Token
	}
	allowPrivate := integ.AllowPrivateNetwork
	if body.GiteaAllowPrivateNetwork != nil {
		allowPrivate = *body.GiteaAllowPrivateNetwork
	}
	if url == "" || token == "" {
		return nil, fmt.Errorf("gitea_url and gitea_token are required")
	}
	return gitea.New(url, token, allowPrivate)
}

func (h *Handler) setupGitHubClientFromBody(body setupTestConnectionBody) (*github.Client, error) {
	cur := h.effectiveGitHub()
	url := strings.TrimSpace(body.GitHubURL)
	if url == "" {
		url = cur.URL
	}
	token := strings.TrimSpace(body.GitHubToken)
	if token == "" {
		token = cur.Token
	}
	allowPrivate := cur.AllowPrivateNetwork
	if body.GitHubAllowPrivateNetwork != nil {
		allowPrivate = *body.GitHubAllowPrivateNetwork
	}
	if url == "" || token == "" {
		return nil, fmt.Errorf("github_url and github_token are required")
	}
	return github.New(url, token, allowPrivate)
}

type setupCheckGiteaURLBody struct {
	ForgeType                 string `json:"forge_type"` // gitea | github; default gitea
	GiteaURL                  string `json:"gitea_url"`
	GiteaAllowPrivateNetwork  *bool  `json:"gitea_allow_private_network"`
	GitHubURL                 string `json:"github_url"`
	GitHubAllowPrivateNetwork *bool  `json:"github_allow_private_network"`
}

func (h *Handler) setupCheckGiteaURL(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body setupCheckGiteaURLBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ft := normalizeSetupForgeType(body.ForgeType)
	if ft == "" {
		writeError(w, http.StatusBadRequest, "forge_type must be a supported forge (gitea, github, gitlab, bitbucket, forgejo)")
		return
	}
	if ft == models.ForgeTypeGitHub {
		allowPrivate := false
		if body.GitHubAllowPrivateNetwork != nil {
			allowPrivate = *body.GitHubAllowPrivateNetwork
		} else {
			allowPrivate = h.effectiveGitHub().AllowPrivateNetwork
		}
		ok, normalized, err := github.CheckGitHubURL(body.GitHubURL, allowPrivate)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"ok":         false,
				"detail":     err.Error(),
				"forge_type": models.ForgeTypeGitHub,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"forge_type": models.ForgeTypeGitHub,
			"github_url": normalized,
		})
		return
	}
	allowPrivate := false
	if body.GiteaAllowPrivateNetwork != nil {
		allowPrivate = *body.GiteaAllowPrivateNetwork
	} else if h.settings != nil {
		allowPrivate = h.effectiveIntegration().AllowPrivateNetwork
	}
	result := gitea.CheckGiteaURL(r.Context(), body.GiteaURL, allowPrivate)
	status := http.StatusOK
	if !result.OK {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

func (h *Handler) setupTestConnection(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body setupTestConnectionBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ft := normalizeSetupForgeType(body.ForgeType)
	if ft == "" {
		writeError(w, http.StatusBadRequest, "forge_type must be a supported forge (gitea, github, gitlab, bitbucket, forgejo)")
		return
	}
	if ft == models.ForgeTypeGitHub {
		client, err := h.setupGitHubClientFromBody(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Preview URL without instance id; create-webhook returns the id-scoped URL.
		delivery := h.githubWebhookDeliveryURL(0)
		probe, err := client.ProbeConnection(r.Context(), delivery)
		if err != nil {
			h.log.Error("setup test-connection github", "err", err)
			writeError(w, http.StatusBadGateway, "connection probe failed")
			return
		}
		writeJSON(w, http.StatusOK, probe)
		return
	}
	if ft == models.ForgeTypeGitLab || ft == models.ForgeTypeBitbucket || ft == models.ForgeTypeForgejo {
		baseURL := strings.TrimSpace(body.BaseURL)
		token := strings.TrimSpace(body.Token)
		if baseURL == "" {
			baseURL = strings.TrimSpace(body.GiteaURL)
		}
		if token == "" {
			token = strings.TrimSpace(body.GiteaToken)
		}
		allowPrivate := false
		if body.AllowPrivateNetwork != nil {
			allowPrivate = *body.AllowPrivateNetwork
		} else if body.GiteaAllowPrivateNetwork != nil {
			allowPrivate = *body.GiteaAllowPrivateNetwork
		}
		if baseURL == "" || token == "" {
			writeError(w, http.StatusBadRequest, "base_url and token are required")
			return
		}
		client, err := forge.New(forge.Options{ForgeType: ft, BaseURL: baseURL, Token: token, AllowPrivateNetwork: allowPrivate})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		delivery := h.forgeWebhookDeliveryURL(ft, 0)
		switch c := client.(type) {
		case *forgejo.Client:
			probe, err := c.ProbeConnection(r.Context(), delivery, h.auth.RedirectURI())
			if err != nil {
				writeError(w, http.StatusBadGateway, "connection probe failed")
				return
			}
			writeJSON(w, http.StatusOK, probe)
			return
		case *gitlab.Client:
			probe, err := c.ProbeConnection(r.Context(), delivery)
			if err != nil {
				writeError(w, http.StatusBadGateway, "connection probe failed")
				return
			}
			writeJSON(w, http.StatusOK, probe)
			return
		case *bitbucket.Client:
			probe, err := c.ProbeConnection(r.Context(), delivery)
			if err != nil {
				writeError(w, http.StatusBadGateway, "connection probe failed")
				return
			}
			writeJSON(w, http.StatusOK, probe)
			return
		default:
			writeError(w, http.StatusInternalServerError, "unexpected forge client type")
			return
		}
	}
	client, err := h.setupClientFromBody(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	delivery := h.webhookDeliveryURL()
	redirectURI := h.auth.RedirectURI()
	probe, err := client.ProbeConnection(r.Context(), delivery, redirectURI)
	if err != nil {
		h.log.Error("setup test-connection", "err", err)
		writeError(w, http.StatusBadGateway, "connection probe failed")
		return
	}
	writeJSON(w, http.StatusOK, probe)
}

type setupOAuthBody struct {
	ForgeType                string `json:"forge_type"` // gitea only; github rejected
	GiteaURL                 string `json:"gitea_url"`
	GiteaToken               string `json:"gitea_token"`
	GiteaAllowPrivateNetwork *bool  `json:"gitea_allow_private_network"`
	Create                   bool   `json:"create"` // true = create/update on Gitea
	OAuthClientID            string `json:"oauth_client_id"`
	OAuthClientSecret        string `json:"oauth_client_secret"`
}

func (h *Handler) setupCreateOAuth(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	var body setupOAuthBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ft := normalizeSetupForgeType(body.ForgeType)
	if ft == "" {
		writeError(w, http.StatusBadRequest, "forge_type must be a supported forge (gitea, github, gitlab, bitbucket, forgejo)")
		return
	}
	if ft == models.ForgeTypeGitHub {
		writeError(w, http.StatusBadRequest, "GitHub OAuth app auto-create is not supported; use a service PAT and complete setup without OAuth")
		return
	}

	redirectURI := h.auth.RedirectURI()
	if redirectURI == "" || strings.HasPrefix(redirectURI, "/api/") {
		writeError(w, http.StatusBadRequest, "GitSeer public URL (server.external_url) is required to build the OAuth redirect URI")
		return
	}

	integ := h.effectiveIntegration()
	allowPrivate := integ.AllowPrivateNetwork
	if body.GiteaAllowPrivateNetwork != nil {
		allowPrivate = *body.GiteaAllowPrivateNetwork
	}
	giteaURL := strings.TrimSpace(body.GiteaURL)
	if giteaURL == "" {
		giteaURL = integ.URL
	}
	token := strings.TrimSpace(body.GiteaToken)

	clientID := strings.TrimSpace(body.OAuthClientID)
	clientSecret := strings.TrimSpace(body.OAuthClientSecret)
	created := false
	updated := false

	if body.Create {
		client, err := h.setupClientFromBody(setupTestConnectionBody{
			GiteaURL:                 body.GiteaURL,
			GiteaToken:               body.GiteaToken,
			GiteaAllowPrivateNetwork: body.GiteaAllowPrivateNetwork,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		app, wasCreated, err := client.EnsureOAuthApplication(r.Context(), gitea.DefaultOAuthAppName, redirectURI)
		if err != nil {
			h.log.Error("setup create-oauth", "err", err)
			writeError(w, http.StatusBadGateway, "failed to create OAuth application: "+err.Error())
			return
		}
		created = wasCreated
		updated = !wasCreated
		clientID = app.ClientID
		clientSecret = app.ClientSecret
	} else {
		if clientID == "" || clientSecret == "" {
			writeError(w, http.StatusBadRequest, "oauth_client_id and oauth_client_secret are required when create is false")
			return
		}
	}

	patch := settings.IntegrationPatch{
		GiteaURL:                   giteaURL,
		GiteaToken:                 token,
		GiteaAllowPrivateNetwork:   allowPrivate,
		GiteaAllowUnsignedWebhooks: integ.AllowUnsignedWebhooks,
		OAuthClientID:              clientID,
		OAuthClientSecret:          clientSecret,
	}
	pub, err := h.settings.UpdateIntegration(r.Context(), patch)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"created":      created,
		"updated":      updated,
		"manual":       !body.Create,
		"redirect_uri": redirectURI,
		"oauth_app":    gitea.NewOAuthAppPreview(redirectURI),
		"client_id":    clientID,
		"integration":  pub,
	})
}

func (h *Handler) setupCreateWebhook(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	var body setupWebhookBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ft := normalizeSetupForgeType(body.ForgeType)
	if ft == "" {
		writeError(w, http.StatusBadRequest, "forge_type must be a supported forge (gitea, github, gitlab, bitbucket, forgejo)")
		return
	}
	if ft == models.ForgeTypeGitHub {
		h.setupCreateGitHubWebhook(w, r, body)
		return
	}

	client, err := h.setupClientFromBody(setupTestConnectionBody{
		GiteaURL:                 body.GiteaURL,
		GiteaToken:               body.GiteaToken,
		GiteaAllowPrivateNetwork: body.GiteaAllowPrivateNetwork,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.webhookDeliveryURLBase() == "" {
		writeError(w, http.StatusBadRequest, "GitSeer public URL (server.external_url) is required to build the webhook URL")
		return
	}
	secret, err := gitea.GenerateWebhookSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate webhook secret")
		return
	}

	integ := h.effectiveIntegration()
	allowPrivate := integ.AllowPrivateNetwork
	if body.GiteaAllowPrivateNetwork != nil {
		allowPrivate = *body.GiteaAllowPrivateNetwork
	}
	giteaURL := strings.TrimSpace(body.GiteaURL)
	if giteaURL == "" {
		giteaURL = integ.URL
	}
	token := strings.TrimSpace(body.GiteaToken)
	patch := settings.IntegrationPatch{
		GiteaURL:                   giteaURL,
		GiteaToken:                 token,
		GiteaWebhookSecret:         secret,
		GiteaAllowPrivateNetwork:   allowPrivate,
		GiteaAllowUnsignedWebhooks: false,
		OAuthClientID:              integ.OAuthClientID,
	}
	pub, err := h.settings.UpdateIntegration(r.Context(), patch)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var instanceID int64
	if inst, ierr := h.store.GetInstanceByForgeAndURL(r.Context(), models.ForgeTypeGitea, strings.TrimRight(giteaURL, "/")); ierr == nil && inst != nil {
		instanceID = inst.ID
	}
	delivery := h.giteaWebhookDeliveryURL(instanceID)

	var hook *gitea.Hook
	created := false
	if body.Create {
		hook, created, err = client.EnsureSystemWebhook(r.Context(), delivery, secret)
		if err != nil {
			h.log.Error("setup create-webhook", "err", err)
			writeError(w, http.StatusBadGateway, "failed to create system webhook: "+err.Error())
			return
		}
	}

	preview := gitea.NewWebhookPreview(delivery, secret)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"forge_type":   models.ForgeTypeGitea,
		"created":      created,
		"updated":      body.Create && !created,
		"manual":       !body.Create,
		"hook_id":      hookID(hook),
		"webhook":      preview,
		"integration":  pub,
		"delivery_url": delivery,
		"instance_id":  instanceID,
	})
}

func (h *Handler) setupCreateGitHubWebhook(w http.ResponseWriter, r *http.Request, body setupWebhookBody) {
	cur := h.effectiveGitHub()
	allowPrivate := cur.AllowPrivateNetwork
	if body.GitHubAllowPrivateNetwork != nil {
		allowPrivate = *body.GitHubAllowPrivateNetwork
	}
	ghURL := strings.TrimSpace(body.GitHubURL)
	if ghURL == "" {
		ghURL = cur.URL
	}
	token := strings.TrimSpace(body.GitHubToken)
	if token == "" {
		token = cur.Token
	}
	if ghURL == "" || token == "" {
		writeError(w, http.StatusBadRequest, "github_url and github_token are required")
		return
	}
	if h.webhookDeliveryURLBase() == "" {
		writeError(w, http.StatusBadRequest, "GitSeer public URL (server.external_url) is required to build the webhook URL")
		return
	}
	secret, err := gitea.GenerateWebhookSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate webhook secret")
		return
	}

	// Preserve Gitea side; apply GitHub credentials + generated webhook secret.
	giteaCur := h.effectiveIntegration()
	patch := settings.IntegrationPatch{
		GiteaURL:                    giteaCur.URL,
		GiteaAllowPrivateNetwork:    giteaCur.AllowPrivateNetwork,
		GiteaAllowUnsignedWebhooks:  giteaCur.AllowUnsignedWebhooks,
		OAuthClientID:               giteaCur.OAuthClientID,
		ApplyGitHub:                 true,
		GitHubURL:                   ghURL,
		GitHubToken:                 token,
		GitHubWebhookSecret:         secret,
		GitHubAllowPrivateNetwork:   allowPrivate,
		GitHubAllowUnsignedWebhooks: false,
	}
	pub, err := h.settings.UpdateIntegration(r.Context(), patch)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var instanceID int64
	if norm, nerr := github.NormalizeBaseURL(ghURL); nerr == nil {
		if inst, ierr := h.store.GetInstanceByForgeAndURL(r.Context(), models.ForgeTypeGitHub, norm); ierr == nil && inst != nil {
			instanceID = inst.ID
		}
	}
	delivery := h.githubWebhookDeliveryURL(instanceID)
	preview := github.NewWebhookPreview(delivery, secret)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"forge_type":   models.ForgeTypeGitHub,
		"created":      false,
		"updated":      false,
		"manual":       true,
		"webhook":      preview,
		"integration":  pub,
		"delivery_url": delivery,
		"instance_id":  instanceID,
		"hint":         "Add this webhook manually in GitHub (org or repo settings). Auto-create is not supported.",
	})
}

func hookID(h *gitea.Hook) int64 {
	if h == nil {
		return 0
	}
	return h.ID
}

func (h *Handler) setupGetEncryption(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": h.settings.EncryptionConfigured(),
		"source":     h.settings.EncryptionSource(),
	})
}

type setupEncryptionBody struct {
	Generate      bool   `json:"generate"`
	EncryptionKey string `json:"encryption_key"`
}

func (h *Handler) setupSetEncryption(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	if h.settings.EncryptionConfigured() {
		writeError(w, http.StatusConflict, "encryption key is already configured")
		return
	}
	var body setupEncryptionBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Generate {
		key, err := h.settings.GenerateEncryptionKey()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"configured":      true,
			"source":          h.settings.EncryptionSource(),
			"encryption_key":  key,
			"generated":       true,
		})
		return
	}
	pass := strings.TrimSpace(body.EncryptionKey)
	if pass == "" {
		writeError(w, http.StatusBadRequest, "encryption_key is required (or set generate=true)")
		return
	}
	if err := h.settings.SetEncryptionKey(pass); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"source":     h.settings.EncryptionSource(),
		"generated":  false,
	})
}

func (h *Handler) setupComplete(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	if !h.settings.EncryptionConfigured() && !h.cfg.Dev.AllowSkipSetup {
		writeError(w, http.StatusBadRequest, "set an encryption key before completing setup")
		return
	}
	if !h.settings.AnyForgeConfigured() && !h.cfg.Dev.AllowSkipSetup {
		writeError(w, http.StatusBadRequest, "configure at least one forge (Gitea or GitHub) before completing setup")
		return
	}
	if err := h.settings.SetSetupCompleted(r.Context(), true); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete setup")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setup_completed": true,
		"status":          h.buildSystemStatus(r.Context(), false),
	})
}

func (h *Handler) syncRepos(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	result, err := h.syncer.SyncFromConfig(r.Context())
	if err != nil {
		h.log.Error("sync", "err", err)
		writeError(w, http.StatusBadGateway, "sync failed")
		return
	}
	_ = h.store.GrantBootstrapAllAccessAllInstances(r.Context(), user.ID)
	writeJSON(w, http.StatusOK, result)
}

// summaryDaysAllowlist is the set of dashboard range presets (default 0 = Now).
// 0 = "Now": current snapshot with no historical lookback.
var summaryDaysAllowlist = map[int]struct{}{0: {}, 1: {}, 7: {}, 30: {}, 90: {}}

var errSummaryDaysAllowlist = errors.New("days must be one of 0, 1, 7, 30, 90")

func parseSummaryDays(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("days must be an integer")
	}
	if _, ok := summaryDaysAllowlist[parsed]; !ok {
		return 0, errSummaryDaysAllowlist
	}
	return parsed, nil
}

var errStatsSectionAllowlist = errors.New("section must be one of core, trends, duration, all")

func parseStatsSection(raw string) (string, error) {
	section := strings.TrimSpace(raw)
	if section == "" {
		return store.StatsSectionAll, nil
	}
	switch section {
	case store.StatsSectionAll, store.StatsSectionCore, store.StatsSectionTrends, store.StatsSectionDuration:
		return section, nil
	default:
		return "", errStatsSectionAllowlist
	}
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	days, err := parseSummaryDays(r.URL.Query().Get("days"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	snapshot := days == 0
	var sincePtr *time.Time
	if !snapshot {
		since := time.Now().UTC().AddDate(0, 0, -days)
		sincePtr = &since
	}
	scope := h.dashboardScopeFromQuery(r)
	sum, err := h.store.SummaryScoped(r.Context(), sc.UserID, sc.BootstrapAll, sincePtr, snapshot, scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	sum.Days = days
	writeJSON(w, http.StatusOK, sum)
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	days, err := parseSummaryDays(r.URL.Query().Get("days"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	section, err := parseStatsSection(r.URL.Query().Get("section"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	snapshot := days == 0
	since := time.Now().UTC()
	if !snapshot {
		since = since.AddDate(0, 0, -days)
	}
	scope := h.dashboardScopeFromQuery(r)
	rep, err := h.store.StatsBySectionScoped(r.Context(), sc.UserID, sc.BootstrapAll, since, snapshot, section, scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rep.Days = days
	writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) listAttention(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit = limitOr(limit, 50)
	items, total, err := h.store.ListAttention(r.Context(), store.ListAttentionOpts{
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll, Severity: q.Get("severity"),
		Type: q.Get("type"), Query: q.Get("q"), OpenOnly: q.Get("resolved") != "1",
		ForgeType: q.Get("forge_type"), InstanceID: parseQueryInt64(q.Get("instance_id")),
		ExcludeMutedForUser: sc.UserID,
		Limit:               limit, Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []models.AttentionItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) listRepositories(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit = limitOr(limit, 50)
	repos, total, err := h.store.ListRepositories(r.Context(), store.ListRepositoriesOpts{
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll, Query: q.Get("q"),
		ForgeType: q.Get("forge_type"), InstanceID: parseQueryInt64(q.Get("instance_id")),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if repos == nil {
		repos = []models.Repository{}
	}
	items, ok := h.attachRepoHealth(w, r, repos)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) getRepository(w http.ResponseWriter, r *http.Request) {
	repo, ok := h.resolveAccessibleRepo(w, r)
	if !ok {
		return
	}
	health, err := h.store.RepoHealthSummary(r.Context(), repo.ID, h.healthOptsFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, repoWithHealth{Repository: *repo, Health: health})
}

func (h *Handler) listPRs(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit = limitOr(limit, 50)
	state := q.Get("state")
	if state == "" {
		state = "open"
	}
	items, total, err := h.store.ListPullRequests(r.Context(), store.ListPRsOpts{
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll, State: state, Query: q.Get("q"),
		ForgeType: q.Get("forge_type"), InstanceID: parseQueryInt64(q.Get("instance_id")),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []models.PullRequest{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit = limitOr(limit, 50)
	items, total, err := h.store.ListWorkflowRuns(r.Context(), store.ListRunsOpts{
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll, Status: q.Get("status"),
		Conclusion: q.Get("conclusion"), Query: q.Get("q"),
		ForgeType: q.Get("forge_type"), InstanceID: parseQueryInt64(q.Get("instance_id")),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []models.WorkflowRun{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) listActiveRuns(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	runs, total, err := h.store.ListWorkflowRuns(r.Context(), store.ListRunsOpts{
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll,
		Statuses: []string{models.StatusQueued, models.StatusWaiting, models.StatusRunning},
		Limit:    50,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	runIDs := make([]int64, len(runs))
	for i, run := range runs {
		runIDs[i] = run.ID
	}
	jobsByRun, err := h.store.ListJobsByRunIDs(r.Context(), runIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	type activeItem struct {
		Run  models.WorkflowRun `json:"run"`
		Jobs []models.Job       `json:"jobs"`
	}
	items := make([]activeItem, 0, len(runs))
	for _, run := range runs {
		jobs := jobsByRun[run.ID]
		if jobs == nil {
			jobs = []models.Job{}
		}
		items = append(items, activeItem{Run: run, Jobs: jobs})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	run, err := h.store.GetWorkflowRunByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	ok, err := h.authz.CanAccessRepo(r.Context(), user, run.RepoID)
	if err != nil || !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	jobs, _ := h.store.ListJobsByRunID(r.Context(), run.ID)
	if jobs == nil {
		jobs = []models.Job{}
	}
	var graph any
	workflowPath := workflows.NormalizeWorkflowPath(run.WorkflowPath)
	if workflowPath == "" {
		workflowPath = run.WorkflowPath
	}
	if workflowPath != "" && run.CommitSHA != "" {
		if nodesJSON, err := h.store.GetWorkflowGraph(r.Context(), run.RepoID, workflowPath, run.CommitSHA); err == nil {
			_ = json.Unmarshal([]byte(nodesJSON), &graph)
		} else if repo, rerr := h.store.GetRepositoryByID(r.Context(), run.RepoID); rerr == nil {
			if client, err := h.forgeClientForRepo(r.Context(), user, repo); err == nil {
				yamlBytes, err := client.GetWorkflowYAML(r.Context(), models.RepoRef{Owner: run.RepoOwner, Name: run.RepoName}, workflowPath, run.CommitSHA)
				if err == nil {
					nodes, err := workflows.ParseNeedsDAG(yamlBytes)
					if err == nil {
						b, _ := json.Marshal(nodes)
						_ = h.store.UpsertWorkflowGraph(r.Context(), run.RepoID, workflowPath, run.CommitSHA, string(b))
						graph = nodes
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "jobs": jobs, "graph": graph})
}

func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	job, err := h.store.GetJobByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	ok, err := h.authz.CanAccessRepo(r.Context(), user, job.RepoID)
	if err != nil || !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) getJobLogs(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	job, err := h.store.GetJobByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	ok, err := h.authz.CanAccessRepo(r.Context(), user, job.RepoID)
	if err != nil || !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	repo, err := h.store.GetRepositoryByID(r.Context(), job.RepoID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	client, err := h.forgeClientForRepo(r.Context(), user, repo)
	if err != nil {
		if errors.Is(err, errUserTokenRequired) {
			writeError(w, http.StatusForbidden, errUserTokenRequired.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "forge not configured")
		return
	}
	rc, err := client.GetJobLogs(r.Context(), models.RepoRef{Owner: repo.Owner, Name: repo.Name}, job.ExternalID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch logs")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, io.LimitReader(rc, 8<<20))
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"repositories":  []any{},
			"organizations": []any{},
			"pull_requests": []any{},
			"workflow_runs": []any{},
			"attention":     []any{},
		})
		return
	}
	res, err := h.store.Search(r.Context(), sc.UserID, sc.BootstrapAll, q, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) serveEvents(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	allow := func(ev realtime.Event) bool {
		if user == nil {
			return false
		}
		if user.IsBootstrapAdmin {
			return true
		}
		if ev.RepoID <= 0 {
			return false
		}
		ok, err := h.authz.CanAccessRepo(r.Context(), user, ev.RepoID)
		return err == nil && ok
	}
	h.hub.ServeFiltered(w, r, allow)
}

func limitOr(n, def int) int {
	if n <= 0 {
		return def
	}
	if n > 200 {
		return 200
	}
	return n
}

func parseQueryInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
