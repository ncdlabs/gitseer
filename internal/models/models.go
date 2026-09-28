// Package models holds GitSeer domain types (no raw forge payloads).
package models

import (
	"strings"
	"time"
)

// Forge type values persisted on instances.forge_type.
const (
	ForgeTypeGitea     = "gitea"
	ForgeTypeGitHub    = "github"
	ForgeTypeGitLab    = "gitlab"
	ForgeTypeBitbucket = "bitbucket"
	ForgeTypeForgejo   = "forgejo"
)

// SupportedForgeTypes lists forge_type values that have registered clients.
var SupportedForgeTypes = []string{
	ForgeTypeGitea,
	ForgeTypeGitHub,
	ForgeTypeGitLab,
	ForgeTypeBitbucket,
	ForgeTypeForgejo,
}

// IsSupportedForgeType reports whether ft is a registered forge implementation.
func IsSupportedForgeType(ft string) bool {
	switch strings.ToLower(strings.TrimSpace(ft)) {
	case ForgeTypeGitea, ForgeTypeGitHub, ForgeTypeGitLab, ForgeTypeBitbucket, ForgeTypeForgejo:
		return true
	default:
		return false
	}
}

// ForgeTokenLabel returns the provider's user-facing name for the sync API credential.
func ForgeTokenLabel(forgeType string) string {
	switch strings.ToLower(strings.TrimSpace(forgeType)) {
	case ForgeTypeGitHub, ForgeTypeGitLab:
		return "Personal Access Token"
	case ForgeTypeBitbucket:
		return "HTTP Access Token"
	case ForgeTypeGitea, ForgeTypeForgejo:
		return "Access Token"
	default:
		return "Access Token"
	}
}

type Instance struct {
	ID                      int64
	Name                    string
	ForgeType               string
	BaseURL                 string
	Version                 string
	CapabilitiesJSON        string
	SyncTokenCiphertext     string
	WebhookSecretCiphertext string
	OAuthClientID           string
	OAuthClientSecretCipher string
	ExternalURL             string
	AllowPrivateNetwork     bool
	AllowUnsignedWebhooks   bool
	WebhookVerifiedAt       *time.Time
	WebhookEnsureAt         *time.Time
	WebhookEnsureError      string
	WebhookVerifyToken      string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type InstanceInfo struct {
	Version string
}

type Capabilities struct {
	Version            string `json:"version"`
	ActionsAPI         bool   `json:"actions_api"`
	OAuthProvider      bool   `json:"oauth_provider"`
	SystemHooksAPI     bool   `json:"system_hooks_api"`
	JobLogsAPI         bool   `json:"job_logs_api"`
	RunnersAPI         bool   `json:"runners_api"`
	WorkflowRunWebhook bool   `json:"workflow_run_webhook"`
	WorkflowJobWebhook bool   `json:"workflow_job_webhook"`
	// RerunWorkflowAPI: forge exposes POST .../actions/runs/{id}/rerun.
	RerunWorkflowAPI bool `json:"rerun_workflow_api"`
	// CancelWorkflowAPI: forge exposes POST .../actions/runs/{id}/cancel.
	CancelWorkflowAPI bool `json:"cancel_workflow_api"`
}

type Organization struct {
	ID           int64      `json:"id"`
	InstanceID   int64      `json:"instance_id"`
	ExternalID   int64      `json:"external_id"`
	NodeID       string     `json:"node_id,omitempty"` // GitHub GraphQL node id (REST node_id); empty for Gitea
	Name         string     `json:"name"`
	FullName     string     `json:"full_name"`
	AvatarURL    string     `json:"avatar_url,omitempty"`
	SyncedAt     *time.Time `json:"synced_at,omitempty"`
	ForgeType    string     `json:"forge_type,omitempty"`
	InstanceName string     `json:"instance_name,omitempty"`
}

type Repository struct {
	ID            int64      `json:"id"`
	InstanceID    int64      `json:"instance_id"`
	OrgID         *int64     `json:"org_id,omitempty"`
	ExternalID    int64      `json:"external_id"`
	NodeID        string     `json:"node_id,omitempty"` // GitHub GraphQL node id; empty for Gitea
	Owner         string     `json:"owner"`
	Name          string     `json:"name"`
	FullName      string     `json:"full_name"`
	DefaultBranch string     `json:"default_branch"`
	Private       bool       `json:"private"`
	Archived      bool       `json:"archived"`
	Empty         bool       `json:"empty"`
	Fork          bool       `json:"fork"`
	HTMLURL       string     `json:"html_url"`
	ForgeType     string     `json:"forge_type,omitempty"` // gitea | github; from instances join on list
	InstanceName  string     `json:"instance_name,omitempty"`
	LastSyncedAt  *time.Time `json:"last_synced_at,omitempty"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type RepoRef struct {
	Owner string
	Name  string
}

type PullRequest struct {
	ID               int64      `json:"id"`
	RepoID           int64      `json:"repo_id"`
	ExternalID       int64      `json:"external_id"`
	NodeID           string     `json:"node_id,omitempty"` // GitHub GraphQL node id; empty for Gitea
	Number           int64      `json:"number"`
	Title            string     `json:"title"`
	BodyExcerpt      string     `json:"body_excerpt"`
	AuthorLogin      string     `json:"author_login"`
	AuthorExternalID *int64     `json:"author_external_id,omitempty"`
	SourceBranch     string     `json:"source_branch"`
	TargetBranch     string     `json:"target_branch"`
	HeadSHA          string     `json:"head_sha"`
	BaseSHA          string     `json:"base_sha"`
	State            string     `json:"state"`
	Draft            bool       `json:"draft"`
	Mergeable        *bool      `json:"mergeable,omitempty"`
	MergeableState   string     `json:"mergeable_state"`
	ReviewState      string     `json:"review_state"`
	CIState          string     `json:"ci_state"`
	HTMLURL          string     `json:"html_url"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
	MergedAt         *time.Time `json:"merged_at,omitempty"`
	RepoOwner        string     `json:"repo_owner"`
	RepoName         string     `json:"repo_name"`
	RepoFull         string     `json:"repo_full"`
	ForgeType        string     `json:"forge_type,omitempty"` // gitea | github; from instances join on list
	InstanceID       int64      `json:"instance_id,omitempty"`
	InstanceName     string     `json:"instance_name,omitempty"`
}

type Workflow struct {
	ID                int64
	RepoID            int64
	Path              string
	Name              string
	ExternalIDOrPath  string
	LastSeenCommitSHA string
}

type WorkflowGraph struct {
	ID        int64
	RepoID    int64
	Path      string
	CommitSHA string
	NodesJSON string
}

type WorkflowNode struct {
	JobKey      string   `json:"job_key"`
	Name        string   `json:"name"`
	Needs       []string `json:"needs"`
	RawNeeds    string   `json:"raw_needs,omitempty"`
	UnknownDeps bool     `json:"unknown_deps"`
}

type WorkflowRun struct {
	ID                 int64      `json:"id"`
	RepoID             int64      `json:"repo_id"`
	WorkflowID         *int64     `json:"workflow_id,omitempty"`
	ExternalID         int64      `json:"external_id"`
	NodeID             string     `json:"node_id,omitempty"` // GitHub GraphQL node id; empty for Gitea
	Name               string     `json:"name"`
	Event              string     `json:"event"`
	Branch             string     `json:"branch"`
	CommitSHA          string     `json:"commit_sha"`
	Status             string     `json:"status"`
	Conclusion         string     `json:"conclusion"`
	UpstreamStatus     string     `json:"upstream_status"`
	UpstreamConclusion string     `json:"upstream_conclusion"`
	ActorLogin         string     `json:"actor_login"`
	HTMLURL            string     `json:"html_url"`
	WorkflowPath       string     `json:"workflow_path"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	RunAttempt         int        `json:"run_attempt"`
	RepoOwner          string     `json:"repo_owner"`
	RepoName           string     `json:"repo_name"`
	RepoFull           string     `json:"repo_full"`
	ForgeType          string     `json:"forge_type,omitempty"` // gitea | github; from instances join on list
	InstanceID         int64      `json:"instance_id,omitempty"`
	InstanceName       string     `json:"instance_name,omitempty"`
}

type AttentionItem struct {
	ID           int64      `json:"id"`
	InstanceID   int64      `json:"instance_id"`
	RepoID       int64      `json:"repo_id"`
	Type         string     `json:"type"`
	Severity     string     `json:"severity"`
	EntityType   string     `json:"entity_type"`
	EntityID     int64      `json:"entity_id"`
	Title        string     `json:"title"`
	MetadataJSON string     `json:"metadata_json"`
	Fingerprint  string     `json:"fingerprint"`
	OpenedAt     time.Time  `json:"opened_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RepoOwner    string     `json:"repo_owner"`
	RepoName     string     `json:"repo_name"`
	RepoFull     string     `json:"repo_full"`
	HTMLURL      string     `json:"html_url,omitempty"`
	ForgeType    string     `json:"forge_type,omitempty"`    // gitea | github; from instances join on list
	InstanceName string     `json:"instance_name,omitempty"` // instances.name when joined
}

type Job struct {
	ID                 int64      `json:"id"`
	RunID              int64      `json:"run_id"`
	RepoID             int64      `json:"repo_id"`
	ExternalID         int64      `json:"external_id"`
	NodeID             string     `json:"node_id,omitempty"` // GitHub GraphQL node id; empty for Gitea
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	Conclusion         string     `json:"conclusion"`
	UpstreamStatus     string     `json:"upstream_status"`
	UpstreamConclusion string     `json:"upstream_conclusion"`
	RunnerID           *int64     `json:"runner_id,omitempty"`
	RunnerName         string     `json:"runner_name"`
	HTMLURL            string     `json:"html_url"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	StepsJSON          *string    `json:"steps_json,omitempty"`
	LabelsJSON         *string    `json:"labels_json,omitempty"` // forge runner labels / label bag when present
	Message            string     `json:"message,omitempty"`     // optional forge freeform job message
}

type User struct {
	ID                  int64
	InstanceID          *int64
	GiteaUserID         *int64
	GitHubUserID        *int64
	GitHubInstanceID    *int64
	GitLabUserID        *int64
	GitLabInstanceID    *int64
	BitbucketUserID     *int64
	BitbucketInstanceID *int64
	Login               string
	Email               string
	DisplayName         string
	AvatarURL           string
	IsBootstrapAdmin    bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Session struct {
	ID                     string
	UserID                 int64
	ExpiresAt              time.Time
	CreatedAt              time.Time
	IP                     string
	UserAgent              string
	BootstrapElevatedUntil *time.Time
}

type Summary struct {
	Repositories int    `json:"repositories"`
	OpenPRs      int    `json:"open_pull_requests"`
	Attention    int    `json:"attention_open"`
	FailedRuns   int    `json:"failed_runs"`
	RunningRuns  int    `json:"running_runs"`
	Days         int    `json:"days"`
	Since        string `json:"since,omitempty"`
	OrgID        int64  `json:"org_id,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Team         string `json:"team,omitempty"`
}

// RunnerUtilizationRow is a per-runner rollup from indexed jobs (and optional forge listing).
type RunnerUtilizationRow struct {
	RunnerName    string `json:"runner_name"`
	RunnerID      *int64 `json:"runner_id,omitempty"`
	BusyJobs      int    `json:"busy_jobs"`
	QueuedJobs    int    `json:"queued_jobs"`
	CompletedJobs int    `json:"completed_jobs"`
	FailedJobs    int    `json:"failed_jobs"`
	InstanceID    int64  `json:"instance_id,omitempty"`
	InstanceName  string `json:"instance_name,omitempty"`
	ForgeType     string `json:"forge_type,omitempty"`
}

// RunnerUtilizationReport is the dashboard/status runner rollup.
type RunnerUtilizationReport struct {
	Days              int                    `json:"days"`
	Since             string                 `json:"since,omitempty"`
	Source            string                 `json:"source"` // indexed_jobs | forge_live | mixed
	Degraded          bool                   `json:"degraded"`
	DegradedReason    string                 `json:"degraded_reason,omitempty"`
	RunnersAPICapable bool                   `json:"runners_api_capable"`
	LiveForgeRunners  int                    `json:"live_forge_runners,omitempty"`
	Items             []RunnerUtilizationRow `json:"items"`
}

// ForgeRunner is a live runner inventory row from a forge ListRunners call.
type ForgeRunner struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status,omitempty"` // online | offline | unknown
	Busy       bool   `json:"busy,omitempty"`
	LabelsJSON string `json:"labels_json,omitempty"`
	InstanceID int64  `json:"instance_id,omitempty"`
	ForgeType  string `json:"forge_type,omitempty"`
}

// FlakyJob is a heuristic flip detection for a job name within a workflow.
type FlakyJob struct {
	RepoID        int64      `json:"repo_id"`
	RepoFull      string     `json:"repo_full,omitempty"`
	WorkflowPath  string     `json:"workflow_path"`
	JobName       string     `json:"job_name"`
	FailureCount  int        `json:"failure_count"`
	SuccessCount  int        `json:"success_count"`
	FlipCount     int        `json:"flip_count"`
	LastFailedAt  *time.Time `json:"last_failed_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	SampleRunID   *int64     `json:"sample_run_id,omitempty"`
}

// ReleaseRun is a release/deploy-like workflow run for visibility surfaces.
type ReleaseRun struct {
	ID            int64      `json:"id"`
	RepoID        int64      `json:"repo_id"`
	RepoFull      string     `json:"repo_full"`
	Name          string     `json:"name"`
	WorkflowPath  string     `json:"workflow_path"`
	Event         string     `json:"event"`
	Branch        string     `json:"branch"`
	Status        string     `json:"status"`
	Conclusion    string     `json:"conclusion"`
	HTMLURL       string     `json:"html_url,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	ForgeType     string     `json:"forge_type,omitempty"`
	InstanceID    int64      `json:"instance_id,omitempty"`
	AttentionOpen bool       `json:"attention_open"`
}

// WallboardSnapshot is the read-only public wallboard payload.
type WallboardSnapshot struct {
	GeneratedAt         string          `json:"generated_at"`
	Summary             Summary         `json:"summary"`
	Attention           []AttentionItem `json:"attention"`
	AttentionBySeverity []CountBucket   `json:"attention_by_severity"`
}

// StatsReport is the authz-scoped dashboard time-series / breakdown payload.
type StatsReport struct {
	Days                int            `json:"days"`
	Since               string         `json:"since,omitempty"`
	RunsByDay           []DayRunBucket `json:"runs_by_day"`
	RunConclusions      []CountBucket  `json:"run_conclusions"`
	RunDuration         *DurationStats `json:"run_duration"`
	PRsByDay            []DayPRBucket  `json:"prs_by_day"`
	PRCIStates          []CountBucket  `json:"pr_ci_states"`
	AttentionBySeverity []CountBucket  `json:"attention_by_severity"`
	AttentionByType     []CountBucket  `json:"attention_by_type"`
}

// DayRunBucket is one calendar day of workflow-run conclusions (stacked).
type DayRunBucket struct {
	Day       string `json:"day"`
	Success   int    `json:"success"`
	Failure   int    `json:"failure"`
	Cancelled int    `json:"cancelled"`
	Other     int    `json:"other"`
}

// DayPRBucket is one calendar day of PR open/merge/close activity.
type DayPRBucket struct {
	Day    string `json:"day"`
	Opened int    `json:"opened"`
	Merged int    `json:"merged"`
	Closed int    `json:"closed"`
}

// CountBucket is a labeled count for bar/breakdown charts.
type CountBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// DurationStats holds percentile run durations in seconds (nil when no samples).
type DurationStats struct {
	P50Seconds  float64 `json:"p50_seconds"`
	P95Seconds  float64 `json:"p95_seconds"`
	SampleCount int     `json:"sample_count"`
}

// Repo health grades (computed rollup; not a forge status).
const (
	HealthGradeHealthy  = "healthy"
	HealthGradeDegraded = "degraded"
	HealthGradeCritical = "critical"
)

// RepoHealth is a per-repository operational rollup (attention, default-branch CI, stale PRs, fail rate).
type RepoHealth struct {
	RepoID                  int64   `json:"repo_id"`
	Score                   int     `json:"score"`
	Grade                   string  `json:"grade"` // healthy | degraded | critical
	OpenCriticalAttention   int     `json:"open_critical_attention"`
	FailingDefaultBranch    bool    `json:"failing_default_branch"`
	FailingDefaultBranchRun *int64  `json:"failing_default_branch_run_id,omitempty"`
	StaleOpenPRs            int     `json:"stale_open_prs"`
	CIFailRate              float64 `json:"ci_fail_rate"`
	CIRunsInWindow          int     `json:"ci_runs_in_window"`
	CIFailuresInWindow      int     `json:"ci_failures_in_window"`
	WindowDays              int     `json:"window_days"`
	StalePRDays             int     `json:"stale_pr_days"`
}

// FailureCluster aggregates repeated job failures for one repo over a window.
type FailureCluster struct {
	RepoID       int64      `json:"repo_id"`
	WorkflowPath string     `json:"workflow_path"`
	JobName      string     `json:"job_name"`
	FailureCount int        `json:"failure_count"`
	LastFailedAt *time.Time `json:"last_failed_at,omitempty"`
	SampleRunID  *int64     `json:"sample_run_id,omitempty"`
	SampleJobID  *int64     `json:"sample_job_id,omitempty"`
}

// Status vocabulary (PRD §31)
const (
	StatusQueued    = "queued"
	StatusWaiting   = "waiting"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusUnknown   = "unknown"

	ConclusionSuccess        = "success"
	ConclusionFailure        = "failure"
	ConclusionCancelled      = "cancelled"
	ConclusionSkipped        = "skipped"
	ConclusionNeutral        = "neutral"
	ConclusionTimedOut       = "timed_out"
	ConclusionActionRequired = "action_required"
	ConclusionUnknown        = "unknown"

	// CIState is the aggregated check/run status shown on pull requests.
	CIStateSuccess   = "success"
	CIStateFailure   = "failure"
	CIStatePending   = "pending"
	CIStateCancelled = "cancelled"
)
