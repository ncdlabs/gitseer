// Package retention runs periodic purge jobs for aged sync/webhook/attention data.
package retention

import (
	"context"
	"log/slog"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/config"
	"github.com/ncdlabs/gitea-lens/internal/store"
)

// RetentionSource supplies the current retention windows (may change at runtime).
type RetentionSource interface {
	Retention() config.RetentionConfig
}

type staticRetention struct{ cfg config.RetentionConfig }

func (s staticRetention) Retention() config.RetentionConfig { return s.cfg }

type Runner struct {
	store *store.Store
	src   RetentionSource
	log   *slog.Logger
}

func New(st *store.Store, cfg config.RetentionConfig, log *slog.Logger) *Runner {
	return NewWithSource(st, staticRetention{cfg: cfg}, log)
}

func NewWithSource(st *store.Store, src RetentionSource, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	if src == nil {
		src = staticRetention{}
	}
	return &Runner{store: st, src: src, log: log}
}

func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	r.once(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.once(ctx)
		}
	}
}

func (r *Runner) once(ctx context.Context) {
	cfg := r.src.Retention()
	stats, err := r.store.PurgeRetention(ctx, cfg.RunsDays, cfg.WebhooksDays, cfg.AttentionDays)
	if err != nil {
		r.log.Error("retention purge failed", "err", err)
		return
	}
	r.log.Info("retention purge complete", "stats", stats)
}
