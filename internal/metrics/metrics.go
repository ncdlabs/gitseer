// Package metrics registers the PRD §41 Prometheus series for Lens.
package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/store"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once

	RepositoriesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lens_repositories_total",
		Help: "Number of non-deleted repositories indexed by Lens",
	})
	OpenPullRequestsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lens_open_pull_requests_total",
		Help: "Number of open pull requests indexed by Lens",
	})
	WorkflowRunsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lens_workflow_runs_total",
		Help: "Number of workflow runs retained by Lens",
	})
	WorkflowFailuresTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lens_workflow_failures_total",
		Help: "Number of workflow runs with failure conclusion",
	})
	WorkflowRunsActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lens_workflow_runs_active",
		Help: "Number of workflow runs currently running",
	})
	WebhooksReceivedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "lens_webhooks_received_total",
		Help: "Webhook deliveries accepted by Lens",
	}, []string{"event"})
	WebhookProcessingErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "lens_webhook_processing_errors_total",
		Help: "Webhook apply failures",
	}, []string{"event"})
	SyncDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "lens_sync_duration_seconds",
		Help:    "Duration of full forge syncs",
		Buckets: prometheus.DefBuckets,
	})
	SyncErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "lens_sync_errors_total",
		Help: "Full sync failures",
	})
	GiteaAPIRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "lens_gitea_api_requests_total",
		Help: "Outbound Gitea API requests",
	}, []string{"status"})
	GiteaAPIErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "lens_gitea_api_errors_total",
		Help: "Outbound Gitea API errors",
	})
)

// Register registers all Lens metrics with the default Prometheus registry once.
func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(
			RepositoriesTotal,
			OpenPullRequestsTotal,
			WorkflowRunsTotal,
			WorkflowFailuresTotal,
			WorkflowRunsActive,
			WebhooksReceivedTotal,
			WebhookProcessingErrorsTotal,
			SyncDurationSeconds,
			SyncErrorsTotal,
			GiteaAPIRequestsTotal,
			GiteaAPIErrorsTotal,
		)
	})
}

// RefreshGauges updates DB-backed gauges from the store.
func RefreshGauges(ctx context.Context, st *store.Store) {
	if st == nil {
		return
	}
	sum, err := st.Summary(ctx, 0, true, nil, false)
	if err != nil || sum == nil {
		return
	}
	RepositoriesTotal.Set(float64(sum.Repositories))
	OpenPullRequestsTotal.Set(float64(sum.OpenPRs))
	WorkflowFailuresTotal.Set(float64(sum.FailedRuns))
	WorkflowRunsActive.Set(float64(sum.RunningRuns))
	if n, err := st.CountWorkflowRuns(ctx); err == nil {
		WorkflowRunsTotal.Set(float64(n))
	}
}

// ObserveSync records a sync duration and optional error.
func ObserveSync(started time.Time, err error) {
	SyncDurationSeconds.Observe(time.Since(started).Seconds())
	if err != nil {
		SyncErrorsTotal.Inc()
	}
}
