// Package attention evaluates operational attention rules (PRD §10).
package attention

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityWaiting  = "waiting"

	TypeFailedDefaultBranch = "failed_default_branch_workflow"
	TypePRCIFailure         = "pr_ci_failure"
	TypeDeployFailure       = "deployment_workflow_failure"
	TypeRequiredCheckFailed = "required_check_failed"
	TypeAwaitingManual      = "awaiting_manual"
	TypeApprovedBlockedCI   = "approved_blocked_by_ci"
	TypeAwaitingReview      = "awaiting_review"
	TypeApprovedBehind      = "approved_behind_target"
	TypeRunnerUnavailable   = "runner_unavailable_queued"
	TypeLongRunning         = "long_running_workflow"
	TypeMergeConflict       = "merge_conflict"
)

// OnOpenedFunc is invoked when an attention item is newly opened or reopened after resolve.
type OnOpenedFunc func(ctx context.Context, item *models.AttentionItem)

// Engine evaluates and upserts/resolves attention items.
type Engine struct {
	store             *store.Store
	log               *slog.Logger
	mu                sync.RWMutex
	longRunningAfter  time.Duration
	severityOverrides map[string]string
	onOpened          OnOpenedFunc
}

func New(st *store.Store, log *slog.Logger) *Engine {
	return NewWithConfig(st, log, 0)
}

func NewWithConfig(st *store.Store, log *slog.Logger, longRunningAfter time.Duration) *Engine {
	if log == nil {
		log = slog.Default()
	}
	if longRunningAfter <= 0 {
		longRunningAfter = 2 * time.Hour
	}
	e := &Engine{store: st, log: log, longRunningAfter: longRunningAfter, severityOverrides: map[string]string{}}
	if st != nil {
		if m, err := st.MapAttentionRuleOverrides(context.Background()); err == nil && m != nil {
			e.severityOverrides = m
		}
	}
	return e
}

// SetLongRunningAfter updates the long-running workflow threshold.
func (e *Engine) SetLongRunningAfter(d time.Duration) {
	if d <= 0 {
		d = 2 * time.Hour
	}
	e.mu.Lock()
	e.longRunningAfter = d
	e.mu.Unlock()
}

// SetOnOpened registers a callback for newly opened / reopened attention items.
func (e *Engine) SetOnOpened(fn OnOpenedFunc) {
	e.mu.Lock()
	e.onOpened = fn
	e.mu.Unlock()
}

// SetSeverityOverrides replaces in-memory rule_type → severity overrides.
func (e *Engine) SetSeverityOverrides(m map[string]string) {
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	e.mu.Lock()
	e.severityOverrides = cp
	e.mu.Unlock()
}

// ReloadSeverityOverrides loads overrides from the store.
func (e *Engine) ReloadSeverityOverrides(ctx context.Context) error {
	m, err := e.store.MapAttentionRuleOverrides(ctx)
	if err != nil {
		return err
	}
	e.SetSeverityOverrides(m)
	return nil
}

func (e *Engine) longRunningThreshold() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.longRunningAfter
}

func (e *Engine) severityFor(ruleType, defaultSeverity string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if sev, ok := e.severityOverrides[ruleType]; ok && sev != "" {
		return sev
	}
	return defaultSeverity
}

// KnownRuleTypes lists discrete PRD §10 rule keys (for settings UI / docs).
func KnownRuleTypes() []string {
	return []string{
		TypeFailedDefaultBranch,
		TypePRCIFailure,
		TypeDeployFailure,
		TypeRequiredCheckFailed,
		TypeAwaitingManual,
		TypeApprovedBlockedCI,
		TypeAwaitingReview,
		TypeApprovedBehind,
		TypeRunnerUnavailable,
		TypeLongRunning,
		TypeMergeConflict,
	}
}

// DefaultSeverityFor returns the built-in severity for a rule type.
func DefaultSeverityFor(ruleType string) string {
	switch ruleType {
	case TypeFailedDefaultBranch, TypePRCIFailure, TypeDeployFailure, TypeRequiredCheckFailed:
		return SeverityCritical
	case TypeApprovedBlockedCI, TypeAwaitingManual, TypeLongRunning, TypeApprovedBehind, TypeMergeConflict, TypeRunnerUnavailable:
		return SeverityWarning
	case TypeAwaitingReview:
		return SeverityWaiting
	default:
		return SeverityWarning
	}
}

func fingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (e *Engine) open(ctx context.Context, item models.AttentionItem) error {
	muted, err := e.store.IsAttentionMuted(ctx, store.MuteMatch{
		Fingerprint: item.Fingerprint,
		RuleType:    item.Type,
		RepoID:      item.RepoID,
		GlobalOnly:  true,
	})
	if err != nil {
		return err
	}
	if muted {
		return nil
	}
	item.Severity = e.severityFor(item.Type, item.Severity)
	var previouslyResolved bool
	var existed bool
	if prev, err := e.store.GetAttentionByFingerprint(ctx, item.Fingerprint); err == nil && prev != nil {
		existed = true
		previouslyResolved = prev.ResolvedAt != nil
	}
	out, err := e.store.UpsertAttention(ctx, item)
	if err != nil {
		return err
	}
	newlyOpened := !existed || previouslyResolved
	if newlyOpened && out != nil {
		e.mu.RLock()
		fn := e.onOpened
		e.mu.RUnlock()
		if fn != nil {
			fn(ctx, out)
		}
	}
	return nil
}

func (e *Engine) resolve(ctx context.Context, fp string) error {
	if err := e.store.ResolveAttentionByFingerprint(ctx, fp); err != nil {
		return err
	}
	// Clear "until resolved" mutes when the underlying condition clears.
	return e.store.ClearUntilResolvedMutes(ctx, fp)
}

func (e *Engine) EvaluateRun(ctx context.Context, instanceID int64, run *models.WorkflowRun) error {
	if run == nil {
		return nil
	}
	repo, err := e.store.GetRepositoryByID(ctx, run.RepoID)
	if err != nil {
		repo = &models.Repository{}
	}
	if err := e.evalFailedDefaultBranch(ctx, instanceID, run, repo); err != nil {
		return err
	}
	if err := e.evalDeployFailure(ctx, instanceID, run); err != nil {
		return err
	}
	if err := e.evalAwaitingManualRun(ctx, instanceID, run); err != nil {
		return err
	}
	if err := e.evalLongRunning(ctx, instanceID, run); err != nil {
		return err
	}
	if jobs, err := e.store.ListJobsByRunID(ctx, run.ID); err == nil {
		for i := range jobs {
			if err := e.EvaluateJob(ctx, instanceID, &jobs[i], run); err != nil {
				e.log.Debug("attention evaluate job", "err", err, "job_id", jobs[i].ID)
			}
		}
	}
	// Legacy fingerprints from pre-matrix engine.
	_ = e.resolve(ctx, fingerprint("failed_run", fmt.Sprintf("%d", run.RepoID), fmt.Sprintf("%d", run.ExternalID)))
	return nil
}

func (e *Engine) EvaluatePullRequest(ctx context.Context, instanceID int64, pr *models.PullRequest) error {
	if pr == nil {
		return nil
	}
	if err := e.evalPRCIFailure(ctx, instanceID, pr); err != nil {
		return err
	}
	if err := e.evalRequiredCheck(ctx, instanceID, pr); err != nil {
		return err
	}
	if err := e.evalApprovedBlockedCI(ctx, instanceID, pr); err != nil {
		return err
	}
	if err := e.evalAwaitingReview(ctx, instanceID, pr); err != nil {
		return err
	}
	if err := e.evalApprovedBehind(ctx, instanceID, pr); err != nil {
		return err
	}
	if err := e.evalMergeConflict(ctx, instanceID, pr); err != nil {
		return err
	}
	// Resolve legacy "every open PR" fingerprints.
	_ = e.resolve(ctx, fingerprint("stale_or_open_pr", fmt.Sprintf("%d", pr.RepoID), fmt.Sprintf("%d", pr.Number)))
	return nil
}

func (e *Engine) EvaluateJob(ctx context.Context, instanceID int64, job *models.Job, run *models.WorkflowRun) error {
	if job == nil {
		return nil
	}
	if err := e.evalAwaitingManualJob(ctx, instanceID, job, run); err != nil {
		return err
	}
	if err := e.evalRunnerUnavailable(ctx, instanceID, job, run); err != nil {
		return err
	}
	_ = e.resolve(ctx, fingerprint("failed_job", fmt.Sprintf("%d", job.RepoID), fmt.Sprintf("%d", job.ExternalID)))
	return nil
}

func (e *Engine) evalFailedDefaultBranch(ctx context.Context, instanceID int64, run *models.WorkflowRun, repo *models.Repository) error {
	fp := fingerprint(TypeFailedDefaultBranch, fmt.Sprintf("%d", run.RepoID), fmt.Sprintf("%d", run.ExternalID))
	failed := run.Conclusion == models.ConclusionFailure || run.Conclusion == models.ConclusionTimedOut
	onDefault := repo.DefaultBranch != "" && run.Branch == repo.DefaultBranch
	if failed && onDefault {
		meta, _ := json.Marshal(map[string]any{"run_id": run.ID, "branch": run.Branch, "conclusion": run.Conclusion})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: run.RepoID, Type: TypeFailedDefaultBranch,
			Severity: SeverityCritical, EntityType: "workflow_run", EntityID: run.ID,
			Title:        fmt.Sprintf("Failed default-branch workflow: %s", run.Name),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalDeployFailure(ctx context.Context, instanceID int64, run *models.WorkflowRun) error {
	fp := fingerprint(TypeDeployFailure, fmt.Sprintf("%d", run.RepoID), fmt.Sprintf("%d", run.ExternalID))
	failed := run.Conclusion == models.ConclusionFailure || run.Conclusion == models.ConclusionTimedOut
	if failed && looksLikeDeploy(run) {
		meta, _ := json.Marshal(map[string]any{"run_id": run.ID, "path": run.WorkflowPath, "event": run.Event})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: run.RepoID, Type: TypeDeployFailure,
			Severity: SeverityCritical, EntityType: "workflow_run", EntityID: run.ID,
			Title:        fmt.Sprintf("Deployment workflow failure: %s", run.Name),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func looksLikeDeploy(run *models.WorkflowRun) bool {
	blob := strings.ToLower(run.Name + " " + run.WorkflowPath + " " + run.Event)
	for _, tip := range []string{"deploy", "release", "production", "prod", "cd.yaml", "cd.yml"} {
		if strings.Contains(blob, tip) {
			return true
		}
	}
	return false
}

func (e *Engine) evalAwaitingManualRun(ctx context.Context, instanceID int64, run *models.WorkflowRun) error {
	fp := fingerprint(TypeAwaitingManual, "run", fmt.Sprintf("%d", run.RepoID), fmt.Sprintf("%d", run.ExternalID))
	if run.Status == models.StatusWaiting || run.Conclusion == models.ConclusionActionRequired {
		meta, _ := json.Marshal(map[string]any{"run_id": run.ID})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: run.RepoID, Type: TypeAwaitingManual,
			Severity: SeverityWarning, EntityType: "workflow_run", EntityID: run.ID,
			Title:        fmt.Sprintf("Workflow awaiting manual intervention: %s", run.Name),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalLongRunning(ctx context.Context, instanceID int64, run *models.WorkflowRun) error {
	fp := fingerprint(TypeLongRunning, fmt.Sprintf("%d", run.RepoID), fmt.Sprintf("%d", run.ExternalID))
	incomplete := run.Status != models.StatusCompleted && run.CompletedAt == nil
	if incomplete && run.StartedAt != nil && time.Since(run.StartedAt.UTC()) > e.longRunningThreshold() {
		meta, _ := json.Marshal(map[string]any{"run_id": run.ID, "started_at": run.StartedAt})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: run.RepoID, Type: TypeLongRunning,
			Severity: SeverityWarning, EntityType: "workflow_run", EntityID: run.ID,
			Title:        fmt.Sprintf("Long-running workflow: %s", run.Name),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalPRCIFailure(ctx context.Context, instanceID int64, pr *models.PullRequest) error {
	fp := fingerprint(TypePRCIFailure, fmt.Sprintf("%d", pr.RepoID), fmt.Sprintf("%d", pr.Number))
	if pr.State == "open" && pr.CIState == models.CIStateFailure {
		meta, _ := json.Marshal(map[string]any{"pr_id": pr.ID, "number": pr.Number})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: pr.RepoID, Type: TypePRCIFailure,
			Severity: SeverityCritical, EntityType: "pull_request", EntityID: pr.ID,
			Title:        fmt.Sprintf("PR #%d CI failure: %s", pr.Number, pr.Title),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalRequiredCheck(ctx context.Context, instanceID int64, pr *models.PullRequest) error {
	// Gitea payloads rarely expose required-check config; use mergeable_state=unstable
	// as a best-effort signal when review/CI pressure exists.
	fp := fingerprint(TypeRequiredCheckFailed, fmt.Sprintf("%d", pr.RepoID), fmt.Sprintf("%d", pr.Number))
	unstable := strings.EqualFold(pr.MergeableState, "unstable")
	if pr.State == "open" && unstable && (pr.CIState == models.CIStateFailure || pr.ReviewState != "") {
		meta, _ := json.Marshal(map[string]any{"pr_id": pr.ID, "mergeable_state": pr.MergeableState})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: pr.RepoID, Type: TypeRequiredCheckFailed,
			Severity: SeverityCritical, EntityType: "pull_request", EntityID: pr.ID,
			Title:        fmt.Sprintf("PR #%d required check failed: %s", pr.Number, pr.Title),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalApprovedBlockedCI(ctx context.Context, instanceID int64, pr *models.PullRequest) error {
	fp := fingerprint(TypeApprovedBlockedCI, fmt.Sprintf("%d", pr.RepoID), fmt.Sprintf("%d", pr.Number))
	approved := isApproved(pr.ReviewState)
	if pr.State == "open" && approved && pr.CIState == models.CIStateFailure {
		meta, _ := json.Marshal(map[string]any{"pr_id": pr.ID, "number": pr.Number})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: pr.RepoID, Type: TypeApprovedBlockedCI,
			Severity: SeverityWarning, EntityType: "pull_request", EntityID: pr.ID,
			Title:        fmt.Sprintf("Approved PR #%d blocked by CI: %s", pr.Number, pr.Title),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalAwaitingReview(ctx context.Context, instanceID int64, pr *models.PullRequest) error {
	fp := fingerprint(TypeAwaitingReview, fmt.Sprintf("%d", pr.RepoID), fmt.Sprintf("%d", pr.Number))
	ciOK := pr.CIState == models.CIStateSuccess || pr.CIState == ""
	needsReview := !isApproved(pr.ReviewState) && !pr.Draft
	if pr.State == "open" && ciOK && needsReview && pr.CIState == models.CIStateSuccess {
		meta, _ := json.Marshal(map[string]any{"pr_id": pr.ID, "number": pr.Number})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: pr.RepoID, Type: TypeAwaitingReview,
			Severity: SeverityWaiting, EntityType: "pull_request", EntityID: pr.ID,
			Title:        fmt.Sprintf("PR #%d awaiting review: %s", pr.Number, pr.Title),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalApprovedBehind(ctx context.Context, instanceID int64, pr *models.PullRequest) error {
	fp := fingerprint(TypeApprovedBehind, fmt.Sprintf("%d", pr.RepoID), fmt.Sprintf("%d", pr.Number))
	behind := strings.EqualFold(pr.MergeableState, "behind") || strings.EqualFold(pr.MergeableState, "dirty")
	if pr.State == "open" && isApproved(pr.ReviewState) && behind {
		meta, _ := json.Marshal(map[string]any{"pr_id": pr.ID, "mergeable_state": pr.MergeableState})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: pr.RepoID, Type: TypeApprovedBehind,
			Severity: SeverityWarning, EntityType: "pull_request", EntityID: pr.ID,
			Title:        fmt.Sprintf("Approved PR #%d behind target: %s", pr.Number, pr.Title),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalMergeConflict(ctx context.Context, instanceID int64, pr *models.PullRequest) error {
	fp := fingerprint(TypeMergeConflict, fmt.Sprintf("%d", pr.RepoID), fmt.Sprintf("%d", pr.Number))
	conflict := (pr.Mergeable != nil && !*pr.Mergeable) ||
		strings.EqualFold(pr.MergeableState, "dirty") ||
		strings.EqualFold(pr.MergeableState, "conflicting")
	if pr.State == "open" && conflict {
		meta, _ := json.Marshal(map[string]any{"pr_id": pr.ID, "mergeable_state": pr.MergeableState})
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: pr.RepoID, Type: TypeMergeConflict,
			Severity: SeverityWarning, EntityType: "pull_request", EntityID: pr.ID,
			Title:        fmt.Sprintf("Merge conflict on PR #%d: %s", pr.Number, pr.Title),
			MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalAwaitingManualJob(ctx context.Context, instanceID int64, job *models.Job, run *models.WorkflowRun) error {
	fp := fingerprint(TypeAwaitingManual, "job", fmt.Sprintf("%d", job.RepoID), fmt.Sprintf("%d", job.ExternalID))
	if job.Status == models.StatusWaiting || job.Conclusion == models.ConclusionActionRequired {
		meta, _ := json.Marshal(map[string]any{"job_id": job.ID, "run_id": job.RunID})
		title := fmt.Sprintf("Job awaiting manual intervention: %s", job.Name)
		if run != nil {
			title = fmt.Sprintf("Job awaiting manual intervention: %s (%s)", job.Name, run.RepoFull)
		}
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: job.RepoID, Type: TypeAwaitingManual,
			Severity: SeverityWarning, EntityType: "job", EntityID: job.ID,
			Title: title, MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

func (e *Engine) evalRunnerUnavailable(ctx context.Context, instanceID int64, job *models.Job, run *models.WorkflowRun) error {
	fp := fingerprint(TypeRunnerUnavailable, fmt.Sprintf("%d", job.RepoID), fmt.Sprintf("%d", job.ExternalID))
	queued := job.Status == models.StatusQueued || job.Status == models.StatusWaiting
	matched, reason := jobSignalsRunnerUnavailable(job)
	// Open only on a positive forge signal while the job is still queued/waiting.
	// Do not alert on "queued with no runner assigned" alone — that is normal.
	if queued && matched {
		meta, _ := json.Marshal(map[string]any{
			"job_id": job.ID, "run_id": job.RunID, "signal": reason,
			"upstream_conclusion": job.UpstreamConclusion, "message": job.Message,
		})
		title := fmt.Sprintf("Queued job waiting on unavailable runner: %s", job.Name)
		if run != nil && run.RepoFull != "" {
			title = fmt.Sprintf("Queued job waiting on unavailable runner: %s (%s)", job.Name, run.RepoFull)
		}
		return e.open(ctx, models.AttentionItem{
			InstanceID: instanceID, RepoID: job.RepoID, Type: TypeRunnerUnavailable,
			Severity: SeverityWarning, EntityType: "job", EntityID: job.ID,
			Title: title, MetadataJSON: string(meta), Fingerprint: fp,
		})
	}
	return e.resolve(ctx, fp)
}

// jobSignalsRunnerUnavailable reports a best-effort positive offline/unavailable
// signal from job conclusion, message, labels, steps, or name when present.
// Ordinary queued jobs with no such text return false (no fake always-on alerts).
func jobSignalsRunnerUnavailable(job *models.Job) (bool, string) {
	if job == nil {
		return false, ""
	}
	parts := []string{
		job.UpstreamConclusion,
		job.Conclusion,
		job.UpstreamStatus,
		job.Name,
		job.RunnerName,
		job.Message,
	}
	if job.LabelsJSON != nil {
		parts = append(parts, *job.LabelsJSON)
	}
	if job.StepsJSON != nil {
		parts = append(parts, *job.StepsJSON)
	}
	blob := strings.ToLower(strings.Join(parts, "\n"))
	if blob == "" {
		return false, ""
	}
	for _, phrase := range runnerUnavailablePhrases {
		if strings.Contains(blob, phrase) {
			return true, phrase
		}
	}
	return false, ""
}

// Phrases that indicate runner unavailability — not normal "waiting for a runner" queue text.
var runnerUnavailablePhrases = []string{
	"runner offline",
	"runner is offline",
	"runners are offline",
	"runner has been offline",
	"no runners available",
	"no runner available",
	"no matching runner",
	"no matching runners",
	"could not find a runner",
	"could not find any runner",
	"unable to find a runner",
	"unable to find any runner",
	"there are no runners",
	"no online runners",
	"no active runners",
	"runner was not found",
	"runners were not found",
	"the runner matching the labels was not found",
	"hosted runners are currently unavailable",
	"self-hosted runner offline",
	"failed to request a runner",
	"runner_unavailable",
	"runner-unavailable",
	"runner_system_failure",
	"stuck_or_timeout_failure",
}

func isApproved(reviewState string) bool {
	s := strings.ToLower(strings.TrimSpace(reviewState))
	return s == "approved" || s == "approve"
}

// Sweep re-evaluates open PRs and incomplete runs for attention drift.
func (e *Engine) Sweep(ctx context.Context, instanceID int64) error {
	const page = 200
	for offset := 0; ; offset += page {
		prs, total, err := e.store.ListPullRequests(ctx, store.ListPRsOpts{
			State: "open", Limit: page, Offset: offset, BootstrapAll: true,
		})
		if err != nil {
			return err
		}
		for i := range prs {
			if err := e.EvaluatePullRequest(ctx, instanceID, &prs[i]); err != nil {
				e.log.Debug("attention sweep pr", "err", err)
			}
		}
		if offset+len(prs) >= total || len(prs) == 0 {
			break
		}
	}
	for _, status := range []string{models.StatusRunning, models.StatusWaiting, models.StatusQueued} {
		for offset := 0; ; offset += page {
			runs, total, err := e.store.ListWorkflowRuns(ctx, store.ListRunsOpts{
				Status: status, Limit: page, Offset: offset, BootstrapAll: true,
			})
			if err != nil {
				return err
			}
			for i := range runs {
				if err := e.EvaluateRun(ctx, instanceID, &runs[i]); err != nil {
					e.log.Debug("attention sweep run", "err", err)
				}
			}
			if offset+len(runs) >= total || len(runs) == 0 {
				break
			}
		}
	}
	return nil
}

// RunPeriodicSweep ticks attention re-evaluation in the background for every
// instance ID returned by instanceIDsFn (multi-forge aware).
func (e *Engine) RunPeriodicSweep(ctx context.Context, interval time.Duration, instanceIDsFn func(context.Context) ([]int64, error)) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids, err := instanceIDsFn(ctx)
			if err != nil || len(ids) == 0 {
				continue
			}
			for _, id := range ids {
				if id == 0 {
					continue
				}
				if err := e.Sweep(ctx, id); err != nil {
					e.log.Warn("attention sweep failed", "instance_id", id, "err", err)
				}
			}
		}
	}
}
