// Package forge defines the forge adapter boundary. Only forge/gitea speaks Gitea protocol.
package forge

import (
	"context"
	"io"

	"github.com/ncdlabs/gitea-lens/internal/models"
)

type ListReposOpts struct {
	Page     int
	PageSize int
}

type PROpts struct {
	State    string // open, closed, all
	Page     int
	PageSize int
}

type RunOpts struct {
	Page     int
	PageSize int
}

type Page[T any] struct {
	Items   []T
	Page    int
	HasMore bool
}

// Forge is the adapter contract. API/sync consume only models.
type Forge interface {
	GetInstance(ctx context.Context) (*models.InstanceInfo, error)
	DetectCapabilities(ctx context.Context) (*models.Capabilities, error)

	ListOrganizations(ctx context.Context) ([]models.Organization, error)
	ListRepositories(ctx context.Context, opts ListReposOpts) (Page[models.Repository], error)
	GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error)

	ListPullRequests(ctx context.Context, repo models.RepoRef, opts PROpts) (Page[models.PullRequest], error)
	GetCombinedCommitStatus(ctx context.Context, repo models.RepoRef, ref string) (string, error)
	ListWorkflowRuns(ctx context.Context, repo models.RepoRef, opts RunOpts) (Page[models.WorkflowRun], error)
	ListJobs(ctx context.Context, repo models.RepoRef, runExternalID int64) ([]models.Job, error)
	GetJobLogs(ctx context.Context, repo models.RepoRef, jobExternalID int64) (io.ReadCloser, error)
	GetWorkflowYAML(ctx context.Context, repo models.RepoRef, path, ref string) ([]byte, error)

	ListAccessibleReposForUser(ctx context.Context, userToken string, opts ListReposOpts) (Page[models.Repository], error)
	GetAuthenticatedUser(ctx context.Context, userToken string) (*models.User, error)
}

// NormalizeStatus maps upstream execution status strings into Lens vocabulary.
func NormalizeStatus(upstream string) (status, conclusion string) {
	u := lower(upstream)
	switch u {
	case "queued", "pending", "waiting":
		if u == "waiting" {
			return models.StatusWaiting, models.ConclusionUnknown
		}
		return models.StatusQueued, models.ConclusionUnknown
	case "in_progress", "running":
		return models.StatusRunning, models.ConclusionUnknown
	case "completed", "success", "failure", "cancelled", "canceled", "skipped", "neutral", "timed_out", "action_required":
		status = models.StatusCompleted
		switch u {
		case "completed":
			conclusion = models.ConclusionUnknown
		case "success":
			conclusion = models.ConclusionSuccess
		case "failure":
			conclusion = models.ConclusionFailure
		case "cancelled", "canceled":
			conclusion = models.ConclusionCancelled
		case "skipped":
			conclusion = models.ConclusionSkipped
		case "neutral":
			conclusion = models.ConclusionNeutral
		case "timed_out":
			conclusion = models.ConclusionTimedOut
		case "action_required":
			conclusion = models.ConclusionActionRequired
		}
		return status, conclusion
	default:
		if u == "" {
			return models.StatusUnknown, models.ConclusionUnknown
		}
		return models.StatusUnknown, models.ConclusionUnknown
	}
}

// NormalizeConclusion maps upstream conclusion separately when status is completed.
func NormalizeConclusion(upstream string) string {
	_, c := NormalizeStatus(upstream)
	if c != models.ConclusionUnknown || lower(upstream) == "unknown" || upstream == "" {
		switch lower(upstream) {
		case "success":
			return models.ConclusionSuccess
		case "failure":
			return models.ConclusionFailure
		case "cancelled", "canceled":
			return models.ConclusionCancelled
		case "skipped":
			return models.ConclusionSkipped
		case "neutral":
			return models.ConclusionNeutral
		case "timed_out":
			return models.ConclusionTimedOut
		case "action_required":
			return models.ConclusionActionRequired
		}
	}
	return c
}

// NormalizeCIState maps a Gitea combined commit status into Lens PR ci_state.
func NormalizeCIState(upstream string) string {
	switch lower(upstream) {
	case "success":
		return models.CIStateSuccess
	case "failure", "error":
		return models.CIStateFailure
	case "pending", "warning":
		return models.CIStatePending
	case "cancelled", "canceled":
		return models.CIStateCancelled
	default:
		return ""
	}
}

// AggregateCIState rolls up workflow run status/conclusion into a single PR ci_state.
// Priority: failure > pending/running > success > cancelled > empty.
func AggregateCIState(runs []models.WorkflowRun) string {
	if len(runs) == 0 {
		return ""
	}
	hasPending, hasSuccess, hasCancelled := false, false, false
	for _, run := range runs {
		switch run.Conclusion {
		case models.ConclusionFailure, models.ConclusionTimedOut, models.ConclusionActionRequired:
			return models.CIStateFailure
		case models.ConclusionSuccess:
			hasSuccess = true
		case models.ConclusionCancelled, models.ConclusionSkipped, models.ConclusionNeutral:
			hasCancelled = true
		}
		switch run.Status {
		case models.StatusQueued, models.StatusWaiting, models.StatusRunning:
			hasPending = true
		}
	}
	if hasPending {
		return models.CIStatePending
	}
	if hasSuccess {
		return models.CIStateSuccess
	}
	if hasCancelled {
		return models.CIStateCancelled
	}
	return ""
}

func lower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
