// Package server wires HTTP routes, middleware, workers, and the embedded UI.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/ncdlabs/gitea-lens/internal/api"
	"github.com/ncdlabs/gitea-lens/internal/attention"
	"github.com/ncdlabs/gitea-lens/internal/auth"
	"github.com/ncdlabs/gitea-lens/internal/authz"
	"github.com/ncdlabs/gitea-lens/internal/config"
	"github.com/ncdlabs/gitea-lens/internal/database"
	"github.com/ncdlabs/gitea-lens/internal/realtime"
	"github.com/ncdlabs/gitea-lens/internal/retention"
	"github.com/ncdlabs/gitea-lens/internal/server/proxyprefix"
	"github.com/ncdlabs/gitea-lens/internal/server/ui"
	"github.com/ncdlabs/gitea-lens/internal/settings"
	"github.com/ncdlabs/gitea-lens/internal/store"
	"github.com/ncdlabs/gitea-lens/internal/sync"
	"github.com/ncdlabs/gitea-lens/internal/webhooks"
)

// Server is the Lens HTTP process.
type Server struct {
	cfg     config.Config
	log     *slog.Logger
	version string
	http    *http.Server
	store   *store.Store
	hub     *realtime.Hub
	syncer  *sync.Service
	wh      *webhooks.Processor
	retain  *retention.Runner
	att     *attention.Engine
	api     *api.Handler
	auth    *auth.Service
}

// New constructs a ready-to-run server.
func New(cfg config.Config, log *slog.Logger, version string) (*Server, error) {
	db, err := database.Open(context.Background(), cfg.Database.Driver, cfg.Database.Path, cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	st := store.NewWithDriver(db, cfg.Database.Driver)
	settingsMgr := settings.New(cfg, st)
	if err := settingsMgr.Load(context.Background()); err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	authsvc := auth.New(st, auth.Config{
		BootstrapPassword: cfg.Auth.BootstrapPassword,
		SessionTTL:        cfg.Auth.SessionTTL,
		CookieSecure:      cfg.CookieSecureResolved(),
		CookiePath:        cfg.CookiePath(),
		GiteaBaseURL:      cfg.Gitea.URL,
		OAuthClientID:     cfg.Auth.OAuthClientID,
		OAuthClientSecret: cfg.Auth.OAuthClientSecret,
		ExternalURL:       cfg.Server.ExternalURL,
		AllowPrivateNet:   cfg.Gitea.AllowPrivateNetwork,
		EncryptionKey:     cfg.Auth.EncryptionKey,
	})
	authzsvc := authz.New(st)
	hub := realtime.NewHub()
	att := attention.NewWithConfig(st, log, settingsMgr.LongRunningAfter())
	syncer := sync.NewService(st, att, hub, cfg, log)
	syncer.SetPrefsSource(syncPrefsAdapter{mgr: settingsMgr})
	syncer.SetGiteaSource(settingsMgr)
	wh := webhooks.NewProcessor(st, att, hub, log)
	applyIntegration := func(integ settings.Integration) {
		authsvc.UpdateGiteaAuth(integ.URL, integ.OAuthClientID, integ.OAuthClientSecret, integ.AllowPrivateNetwork)
		wh.SetSecret(integ.WebhookSecret)
		wh.SetAllowUnsigned(integ.AllowUnsignedWebhooks)
	}
	settingsMgr.SetOnIntegrationChange(applyIntegration)
	settingsMgr.SetOnExternalURLChange(authsvc.UpdateExternalURL)
	applyIntegration(settingsMgr.Integration())
	authsvc.UpdateExternalURL(settingsMgr.ExternalURL())
	if integ := settingsMgr.Integration(); integ.WebhookSecret == "" && integ.AllowUnsignedWebhooks {
		log.Warn("gitea webhook HMAC secret not set; unsigned payloads allowed")
	}
	retain := retention.NewWithSource(st, settingsMgr, log)
	apiHandler := api.New(cfg, st, authsvc, authzsvc, syncer, wh, att, hub, settingsMgr, log, version)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	// Do not use chi RealIP: client-controlled X-Forwarded-* must not rewrite RemoteAddr
	// unless the peer is in server.trusted_proxies (see ratelimit.ClientIP).
	r.Use(chimw.Recoverer)
	r.Use(proxyprefix.Middleware(cfg.PathPrefix()))
	r.Use(requestLogger(log))
	r.Use(securityHeaders)

	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/health/ready", func(w http.ResponseWriter, req *http.Request) {
		if err := db.PingContext(req.Context()); err != nil {
			http.Error(w, `{"status":"not_ready"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	r.Handle("/metrics", apiHandler.MetricsHandler())
	r.Get("/api/v1/ui-config", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		prefix := cfg.PathPrefix()
		csrf := ""
		if c, err := req.Cookie(auth.CSRFCookieName); err == nil && c.Value != "" {
			csrf = c.Value
		} else {
			csrf, _ = authsvc.IssueCSRFToken(w)
		}
		payload := map[string]any{
			"base_path":         prefix,
			"oauth_enabled":     authsvc.OAuthEnabled(),
			"bootstrap_enabled": cfg.Auth.BootstrapPassword != "",
			"allow_skip_setup":  cfg.Dev.AllowSkipSetup,
			"csrf_token":        csrf,
		}
		// Local npm start only (LENS_ALLOW_SKIP_SETUP): prefill login with the bootstrap password.
		if cfg.Dev.AllowSkipSetup && cfg.Auth.BootstrapPassword != "" {
			payload["dev_bootstrap_password"] = cfg.Auth.BootstrapPassword
		}
		_ = json.NewEncoder(w).Encode(payload)
	})

	apiHandler.Routes(r)
	r.Mount("/", ui.Handler(cfg.PathPrefix()))

	srv := &Server{
		cfg:     cfg,
		log:     log,
		version: version,
		store:   st,
		hub:     hub,
		syncer:  syncer,
		wh:      wh,
		retain:  retain,
		att:     att,
		api:     apiHandler,
		auth:    authsvc,
		http: &http.Server{
			Addr:              cfg.Server.Listen,
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
			// WriteTimeout intentionally unset: SSE (/api/v1/events) and on-demand job log
			// streaming would be cut off by a global write deadline.
		},
	}
	return srv, nil
}

// Run starts background workers and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	go s.hub.Run(ctx)
	go s.wh.Run(ctx)
	go s.syncer.RunReconcile(ctx)
	go s.retain.Run(ctx)
	go s.runACLRefresh(ctx)
	if s.att != nil {
		go s.att.RunPeriodicSweep(ctx, 10*time.Minute, func(c context.Context) (int64, error) {
			inst, err := s.store.GetPrimaryInstance(c)
			if err != nil || inst == nil {
				return 0, err
			}
			return inst.ID, nil
		})
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("listening",
			"addr", s.cfg.Server.Listen,
			"version", s.version,
			"path_prefix", s.cfg.PathPrefix(),
			"oauth", s.auth.OAuthEnabled(),
		)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) runACLRefresh(ctx context.Context) {
	interval := s.cfg.Auth.ACLRefreshInterval
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.api == nil {
				continue
			}
			s.api.RefreshAllUserACLs(ctx)
		}
	}
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", chimw.GetReqID(r.Context()),
			)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		csp := "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'self'"
		w.Header().Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

type syncPrefsAdapter struct {
	mgr *settings.Manager
}

func (a syncPrefsAdapter) SyncPrefs() sync.Prefs {
	v := a.mgr.Get()
	return sync.Prefs{InstanceName: v.InstanceName, SyncHistoryDays: v.SyncHistoryDays}
}
