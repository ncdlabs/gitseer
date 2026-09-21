// Package models holds Lens domain types (no raw forge payloads).
package models

import "time"

type Instance struct {
	ID                       int64
	Name                     string
	BaseURL                  string
	Version                  string
	CapabilitiesJSON         string
	SyncTokenCiphertext      string
	WebhookSecretCiphertext  string
	OAuthClientID            string
	OAuthClientSecretCipher  string
	ExternalURL              string
	CreatedAt                time.Time
	UpdatedAt                time.Time
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
}

type Organization struct {
	ID         int64
	InstanceID int64
	ExternalID int64
	Name       string
	FullName   string
	AvatarURL  string
	SyncedAt   *time.Time
}

type Repository struct {
	ID            int64      `json:"id"`
	InstanceID    int64      `json:"instance_id"`
	OrgID         *int64     `json:"org_id,omitempty"`
	ExternalID    int64      `json:"external_id"`
	Owner         string     `json:"owner"`
	Name          string     `json:"name"`
	FullName      string     `json:"full_name"`
	DefaultBranch string     `json:"default_branch"`
	Private       bool       `json:"private"`
	Archived      bool       `json:"archived"`
	Empty         bool       `json:"empty"`
	Fork          bool       `json:"fork"`
	HTMLURL       string     `json:"html_url"`
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
}

type Job struct {
	ID                 int64      `json:"id"`
	RunID              int64      `json:"run_id"`
	RepoID             int64      `json:"repo_id"`
	ExternalID         int64      `json:"external_id"`
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
}

type User struct {
	ID               int64
	InstanceID       *int64
	GiteaUserID      *int64
	Login            string
	Email            string
	DisplayName      string
	AvatarURL        string
	IsBootstrapAdmin bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
	IP        string
	UserAgent string
}

type Summary struct {
	Repositories int    `json:"repositories"`
	OpenPRs      int    `json:"open_pull_requests"`
	Attention    int    `json:"attention_open"`
	FailedRuns   int    `json:"failed_runs"`
	RunningRuns  int    `json:"running_runs"`
	Days         int    `json:"days"`
	Since        string `json:"since,omitempty"`
}

// StatsReport is the authz-scoped dashboard time-series / breakdown payload.
type StatsReport struct {
	Days                 int            `json:"days"`
	Since                string         `json:"since,omitempty"`
	RunsByDay            []DayRunBucket `json:"runs_by_day"`
	RunConclusions       []CountBucket  `json:"run_conclusions"`
	RunDuration          *DurationStats `json:"run_duration"`
	PRsByDay             []DayPRBucket  `json:"prs_by_day"`
	PRCIStates           []CountBucket  `json:"pr_ci_states"`
	AttentionBySeverity  []CountBucket  `json:"attention_by_severity"`
	AttentionByType      []CountBucket  `json:"attention_by_type"`
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
