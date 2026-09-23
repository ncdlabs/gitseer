// Package api implements /api/v1 HTTP handlers.
package api

import (
	"context"
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
	"github.com/ncdlabs/gitea-lens/internal/attention"
	"github.com/ncdlabs/gitea-lens/internal/auth"
	"github.com/ncdlabs/gitea-lens/internal/authz"
	"github.com/ncdlabs/gitea-lens/internal/config"
	"github.com/ncdlabs/gitea-lens/internal/forge"
	"github.com/ncdlabs/gitea-lens/internal/forge/gitea"
	lensmetrics "github.com/ncdlabs/gitea-lens/internal/metrics"
	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/ratelimit"
	"github.com/ncdlabs/gitea-lens/internal/realtime"
	"github.com/ncdlabs/gitea-lens/internal/settings"
	"github.com/ncdlabs/gitea-lens/internal/store"
	"github.com/ncdlabs/gitea-lens/internal/sync"
	"github.com/ncdlabs/gitea-lens/internal/theme"
	"github.com/ncdlabs/gitea-lens/internal/webhooks"
	"github.com/ncdlabs/gitea-lens/internal/workflows"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

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
	log      *slog.Logger
	version  string
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
	lensmetrics.Register()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lensmetrics.RefreshGauges(r.Context(), h.store)
		promhttp.Handler().ServeHTTP(w, r)
	})
	return h.requireAuth(inner)
}

func (h *Handler) Routes(r chi.Router) {
	trusted, _ := h.cfg.TrustedProxyNets()
	authLimit := ratelimit.New(time.Minute, 20, trusted...)
	webhookLimit := ratelimit.New(time.Minute, 120, trusted...)

	r.Route("/api/v1", func(r chi.Router) {
		r.With(h.requireAuth).Get("/system/status", h.systemStatus)
		r.With(h.requireAuth).Get("/settings", h.getSettings)
		r.With(h.requireAuth, h.requireCSRF).Put("/settings", h.putSettings)

		r.Route("/auth", func(r chi.Router) {
			r.With(authLimit.Middleware).Get("/login", h.oauthLogin)
			r.Get("/callback", h.oauthCallback)
			r.With(authLimit.Middleware, h.requireCSRF).Post("/bootstrap/login", h.bootstrapLogin)
			r.With(h.requireCSRF).Post("/logout", h.logout)
			r.Get("/me", h.me)
		})

		r.With(h.requireAuth, h.requireCSRF).Post("/setup/test-connection", h.setupTestConnection)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/check-gitea-url", h.setupCheckGiteaURL)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/create-webhook", h.setupCreateWebhook)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/create-oauth", h.setupCreateOAuth)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/complete", h.setupComplete)
		r.With(h.requireAuth, h.requireCSRF).Post("/setup/sync-repos", h.syncRepos)
		r.With(h.requireAuth).Get("/summary", h.summary)
		r.With(h.requireAuth).Get("/stats", h.stats)
		r.With(h.requireAuth).Get("/attention", h.listAttention)
		r.With(h.requireAuth).Get("/repositories", h.listRepositories)
		r.With(h.requireAuth).Get("/repositories/{owner}/{repo}", h.getRepository)
		r.With(h.requireAuth).Get("/pull-requests", h.listPRs)
		r.With(h.requireAuth).Get("/workflow-runs", h.listRuns)
		r.With(h.requireAuth).Get("/workflow-runs/active", h.listActiveRuns)
		r.With(h.requireAuth).Get("/workflow-runs/{id}", h.getRun)
		r.With(h.requireAuth).Get("/jobs/{id}", h.getJob)
		r.With(h.requireAuth).Get("/jobs/{id}/logs", h.getJobLogs)
		r.With(h.requireAuth).Get("/search", h.search)
		r.With(h.requireAuth).Get("/events", h.serveEvents)
	})

	r.With(webhookLimit.Middleware).Post("/api/webhooks/gitea", h.wh.HandleHTTP)
	r.With(webhookLimit.Middleware).Post("/api/webhooks/gitea/{instanceID}", h.wh.HandleHTTP)
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
	if !h.auth.BootstrapEnabled() {
		writeError(w, http.StatusServiceUnavailable, "bootstrap auth is not configured; set LENS_AUTH_BOOTSTRAP_PASSWORD")
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
		_ = h.store.GrantBootstrapAllAccess(r.Context(), user.ID, inst.ID)
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
	url, err := h.auth.BeginOAuth(r.Context(), redirectTo)
	if err != nil {
		if errors.Is(err, auth.ErrOAuthNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "oauth is not configured; set LENS_AUTH_OAUTH_CLIENT_ID, LENS_SERVER_EXTERNAL_URL, and LENS_GITEA_URL")
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
	user, token, redirectTo, accessToken, err := h.auth.CompleteOAuth(r.Context(), code, state, r.RemoteAddr, r.UserAgent())
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
	redirectTo = auth.SafeRedirectPath(redirectTo)
	if prefix := h.cfg.PathPrefix(); prefix != "" && !strings.HasPrefix(redirectTo, prefix) {
		if redirectTo == "/" {
			redirectTo = prefix + "/"
		} else if strings.HasPrefix(redirectTo, "/") {
			redirectTo = prefix + redirectTo
		}
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
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

func (h *Handler) forgeClient() (forge.Forge, error) {
	integ := h.effectiveIntegration()
	if integ.URL == "" || integ.Token == "" {
		return nil, fmt.Errorf("gitea not configured")
	}
	return gitea.New(integ.URL, integ.Token, integ.AllowPrivateNetwork)
}

func (h *Handler) refreshUserACL(ctx context.Context, userID int64, userAccessToken string) error {
	integ := h.effectiveIntegration()
	if integ.URL == "" || userAccessToken == "" {
		return fmt.Errorf("missing gitea url or user token")
	}
	client, err := gitea.New(integ.URL, integ.Token, integ.AllowPrivateNetwork)
	if err != nil {
		return err
	}
	inst, err := h.store.GetPrimaryInstance(ctx)
	if err != nil || inst == nil {
		return fmt.Errorf("no synced instance yet; run sync first")
	}
	var externalIDs []int64
	for page := 1; ; page++ {
		p, err := client.ListAccessibleReposForUser(ctx, userAccessToken, forge.ListReposOpts{Page: page, PageSize: 50})
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
	return h.store.ReplaceUserRepoAccess(ctx, userID, ids)
}

// RefreshAllUserACLs reloads ACL rows for users with decryptable OAuth tokens.
func (h *Handler) RefreshAllUserACLs(ctx context.Context) {
	users, err := h.store.ListUsersWithTokenCiphers(ctx)
	if err != nil {
		h.log.Warn("acl refresh list users", "err", err)
		return
	}
	for _, u := range users {
		tok, err := h.auth.UserAccessToken(ctx, u.ID)
		if err != nil || tok == "" {
			continue
		}
		if err := h.refreshUserACL(ctx, u.ID, tok); err != nil {
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
		"authenticated":      true,
		"id":                 user.ID,
		"login":              user.Login,
		"display_name":       user.DisplayName,
		"is_bootstrap_admin": user.IsBootstrapAdmin,
		"authz":              authzMode,
		"csrf_token":         csrf,
	}
	if themeID, giteaName, ok := h.giteaThemeForUser(r.Context(), user); ok {
		out["theme"] = string(themeID)
		out["gitea_theme"] = giteaName
	}
	writeJSON(w, http.StatusOK, out)
}

// giteaThemeForUser reads the user's Gitea theme preference and maps built-in
// defaults onto Lens themes. Custom/unknown themes fail closed (ok=false).
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
	writeJSON(w, http.StatusOK, h.buildSystemStatus(r.Context()))
}

func (h *Handler) buildSystemStatus(ctx context.Context) map[string]any {
	inst, _ := h.store.GetPrimaryInstance(ctx)
	uiName := h.cfg.UI.InstanceName
	integ := h.effectiveIntegration()
	setupCompleted := false
	if h.settings != nil {
		uiName = h.settings.Get().InstanceName
		setupCompleted = h.settings.SetupCompleted()
	}
	status := map[string]any{
		"version":             h.version,
		"ui_name":             uiName,
		"gitea_configured":    integ.URL != "" && integ.Token != "",
		"bootstrap_auth":      h.auth.BootstrapEnabled(),
		"oauth_enabled":       h.auth.OAuthEnabled(),
		"path_prefix":         h.cfg.PathPrefix(),
		"instance_connected":  inst != nil,
		"webhook_hmac":        integ.WebhookSecret != "",
		"setup_completed":     setupCompleted,
		"oauth_redirect_uri":  h.auth.RedirectURI(),
		"server_external_url": h.webhookDeliveryURLBase(),
	}
	if inst != nil {
		status["gitea_version"] = inst.Version
		status["capabilities"] = json.RawMessage(inst.CapabilitiesJSON)
	}
	return status
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"editable":        user != nil && user.IsBootstrapAdmin,
		"settings":        h.settings.Get(),
		"integration":     h.settings.IntegrationPublic(),
		"setup_completed": h.settings.SetupCompleted(),
		"status":          h.buildSystemStatus(r.Context()),
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
		"status":          h.buildSystemStatus(r.Context()),
	})
}

type setupTestConnectionBody struct {
	GiteaURL                 string `json:"gitea_url"`
	GiteaToken               string `json:"gitea_token"`
	GiteaAllowPrivateNetwork *bool  `json:"gitea_allow_private_network"`
}

type setupWebhookBody struct {
	GiteaURL                 string `json:"gitea_url"`
	GiteaToken               string `json:"gitea_token"`
	GiteaAllowPrivateNetwork *bool  `json:"gitea_allow_private_network"`
	Create                   bool   `json:"create"` // true = create/update on Gitea; false = save secret only
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
	base := h.webhookDeliveryURLBase()
	if base == "" {
		return ""
	}
	return base + "/api/webhooks/gitea"
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

type setupCheckGiteaURLBody struct {
	GiteaURL                 string `json:"gitea_url"`
	GiteaAllowPrivateNetwork *bool  `json:"gitea_allow_private_network"`
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

	redirectURI := h.auth.RedirectURI()
	if redirectURI == "" || strings.HasPrefix(redirectURI, "/api/") {
		writeError(w, http.StatusBadRequest, "Lens public URL (server.external_url) is required to build the OAuth redirect URI")
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
	client, err := h.setupClientFromBody(setupTestConnectionBody{
		GiteaURL:                 body.GiteaURL,
		GiteaToken:               body.GiteaToken,
		GiteaAllowPrivateNetwork: body.GiteaAllowPrivateNetwork,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	delivery := h.webhookDeliveryURL()
	if delivery == "" {
		writeError(w, http.StatusBadRequest, "Lens public URL (server.external_url) is required to build the webhook URL")
		return
	}
	secret, err := gitea.GenerateWebhookSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate webhook secret")
		return
	}

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

	preview := gitea.NewWebhookPreview(delivery, secret)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"created":      created,
		"updated":      body.Create && !created,
		"manual":       !body.Create,
		"hook_id":      hookID(hook),
		"webhook":      preview,
		"integration":  pub,
		"delivery_url": delivery,
	})
}

func hookID(h *gitea.Hook) int64 {
	if h == nil {
		return 0
	}
	return h.ID
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
	if err := h.settings.SetSetupCompleted(r.Context(), true); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete setup")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setup_completed": true,
		"status":          h.buildSystemStatus(r.Context()),
	})
}

func (h *Handler) syncRepos(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	result, err := h.syncer.SyncFromConfig(r.Context())
	if err != nil {
		h.log.Error("sync", "err", err)
		writeError(w, http.StatusBadGateway, "sync failed")
		return
	}
	_ = h.store.GrantBootstrapAllAccess(r.Context(), user.ID, result.InstanceID)
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
	sum, err := h.store.Summary(r.Context(), sc.UserID, sc.BootstrapAll, sincePtr, snapshot)
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
	snapshot := days == 0
	since := time.Now().UTC()
	if !snapshot {
		since = since.AddDate(0, 0, -days)
	}
	rep, err := h.store.StatsReport(r.Context(), sc.UserID, sc.BootstrapAll, since, snapshot)
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
		Type: q.Get("type"), Query: q.Get("q"), OpenOnly: q.Get("resolved") != "1", Limit: limit, Offset: offset,
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
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll, Query: q.Get("q"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if repos == nil {
		repos = []models.Repository{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": repos, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) getRepository(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	owner := chi.URLParam(r, "owner")
	name := chi.URLParam(r, "repo")
	repo, err := h.store.GetRepositoryByOwnerName(r.Context(), owner, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	ok, err := h.authz.CanAccessRepo(r.Context(), user, repo.ID)
	if err != nil || !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, repo)
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
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll, State: state, Query: q.Get("q"), Limit: limit, Offset: offset,
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
		Conclusion: q.Get("conclusion"), Query: q.Get("q"), Limit: limit, Offset: offset,
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
		} else if client, err := h.forgeClient(); err == nil {
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
	client, err := h.forgeClient()
	if err != nil {
		writeError(w, http.StatusBadRequest, "gitea not configured")
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
		writeJSON(w, http.StatusOK, map[string]any{"repositories": []any{}, "pull_requests": []any{}, "workflow_runs": []any{}})
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
