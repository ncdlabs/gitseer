package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/models"
)

func clampLimit(n, def, max int) int {
	if n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// ErrUnscopedList is returned when a list helper would otherwise return all rows
// without an authenticated user scope or explicit BootstrapAll.
var ErrUnscopedList = fmt.Errorf("unscoped list refused: user id required unless bootstrap_all")

func requireListScope(userID int64, bootstrapAll bool) error {
	if !bootstrapAll && userID <= 0 {
		return ErrUnscopedList
	}
	return nil
}

// likePattern builds a LIKE pattern with metacharacters stripped from user input.
func likePattern(q string) string {
	q = strings.TrimSpace(q)
	q = strings.ReplaceAll(q, `%`, "")
	q = strings.ReplaceAll(q, `_`, "")
	if q == "" {
		return ""
	}
	return "%" + q + "%"
}

func (s *Store) UpsertPullRequest(ctx context.Context, repoID int64, pr models.PullRequest) (*models.PullRequest, error) {
	if existing, err := s.GetPullRequestByNumber(ctx, repoID, pr.Number); err == nil {
		if !shouldApplyPR(*existing, pr) {
			return existing, nil
		}
	}
	_, err := s.exec(ctx, `
INSERT INTO pull_requests (
  repo_id, external_id, number, title, body_excerpt, author_login, author_external_id,
  source_branch, target_branch, head_sha, base_sha, state, draft, mergeable, mergeable_state,
  review_state, ci_state, html_url, created_at, updated_at, closed_at, merged_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_id, number) DO UPDATE SET
  external_id=excluded.external_id, title=excluded.title, body_excerpt=excluded.body_excerpt,
  author_login=excluded.author_login, author_external_id=excluded.author_external_id,
  source_branch=excluded.source_branch, target_branch=excluded.target_branch,
  head_sha=excluded.head_sha, base_sha=excluded.base_sha, state=excluded.state, draft=excluded.draft,
  mergeable=excluded.mergeable, mergeable_state=excluded.mergeable_state,
  review_state=excluded.review_state,
  ci_state=CASE
    WHEN excluded.ci_state != '' THEN excluded.ci_state
    WHEN excluded.head_sha != pull_requests.head_sha THEN ''
    ELSE pull_requests.ci_state
  END,
  html_url=excluded.html_url,
  created_at=COALESCE(excluded.created_at, pull_requests.created_at),
  updated_at=COALESCE(excluded.updated_at, pull_requests.updated_at),
  closed_at=COALESCE(excluded.closed_at, pull_requests.closed_at),
  merged_at=COALESCE(excluded.merged_at, pull_requests.merged_at)
`, repoID, pr.ExternalID, pr.Number, pr.Title, pr.BodyExcerpt, pr.AuthorLogin, nullInt64(pr.AuthorExternalID),
		pr.SourceBranch, pr.TargetBranch, pr.HeadSHA, pr.BaseSHA, pr.State, boolToInt(pr.Draft),
		nullBool(pr.Mergeable), pr.MergeableState, pr.ReviewState, pr.CIState, pr.HTMLURL,
		formatTimePtr(pr.CreatedAt), formatTimePtr(pr.UpdatedAt), formatTimePtr(pr.ClosedAt), formatTimePtr(pr.MergedAt))
	if err != nil {
		return nil, err
	}
	return s.GetPullRequestByNumber(ctx, repoID, pr.Number)
}

// shouldApplyPR rejects out-of-order webhook/sync updates with an older updated_at.
func shouldApplyPR(existing, incoming models.PullRequest) bool {
	if incoming.UpdatedAt == nil || existing.UpdatedAt == nil {
		return true
	}
	return !incoming.UpdatedAt.Before(*existing.UpdatedAt)
}

// SetPullRequestsCIStateByHeadSHA updates ci_state for open PRs whose head matches sha.
func (s *Store) SetPullRequestsCIStateByHeadSHA(ctx context.Context, repoID int64, headSHA, ciState string) error {
	if headSHA == "" {
		return nil
	}
	_, err := s.exec(ctx, `
UPDATE pull_requests SET ci_state=?
WHERE repo_id=? AND head_sha=? AND state='open'
`, ciState, repoID, headSHA)
	return err
}

// ListWorkflowRunsByCommitSHA returns runs for a repo commit (newest first, capped).
func (s *Store) ListWorkflowRunsByCommitSHA(ctx context.Context, repoID int64, commitSHA string) ([]models.WorkflowRun, error) {
	if commitSHA == "" {
		return nil, nil
	}
	rows, err := s.query(ctx, `
SELECT wr.id, wr.repo_id, wr.workflow_id, wr.external_id, wr.name, wr.event, wr.branch, wr.commit_sha,
       wr.status, wr.conclusion, wr.upstream_status, wr.upstream_conclusion, wr.actor_login, wr.html_url,
       wr.workflow_path, wr.started_at, wr.completed_at, wr.run_attempt, r.owner, r.name, r.full_name
FROM workflow_runs wr JOIN repositories r ON r.id = wr.repo_id
WHERE wr.repo_id=? AND wr.commit_sha=?
ORDER BY wr.id DESC
LIMIT 100`, repoID, commitSHA)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.WorkflowRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullBool(v *bool) any {
	if v == nil {
		return nil
	}
	return boolToInt(*v)
}

func (s *Store) GetPullRequestByNumber(ctx context.Context, repoID, number int64) (*models.PullRequest, error) {
	row := s.queryRow(ctx, `
SELECT pr.id, pr.repo_id, pr.external_id, pr.number, pr.title, pr.body_excerpt, pr.author_login, pr.author_external_id,
       pr.source_branch, pr.target_branch, pr.head_sha, pr.base_sha, pr.state, pr.draft, pr.mergeable, pr.mergeable_state,
       pr.review_state, pr.ci_state, pr.html_url, pr.created_at, pr.updated_at, pr.closed_at, pr.merged_at,
       r.owner, r.name, r.full_name
FROM pull_requests pr JOIN repositories r ON r.id = pr.repo_id
WHERE pr.repo_id=? AND pr.number=?`, repoID, number)
	return scanPR(row)
}

func scanPR(row scanner) (*models.PullRequest, error) {
	var pr models.PullRequest
	var authorID sql.NullInt64
	var draft int
	var mergeable sql.NullInt64
	var created, updated, closed, merged sql.NullString
	if err := row.Scan(
		&pr.ID, &pr.RepoID, &pr.ExternalID, &pr.Number, &pr.Title, &pr.BodyExcerpt, &pr.AuthorLogin, &authorID,
		&pr.SourceBranch, &pr.TargetBranch, &pr.HeadSHA, &pr.BaseSHA, &pr.State, &draft, &mergeable, &pr.MergeableState,
		&pr.ReviewState, &pr.CIState, &pr.HTMLURL, &created, &updated, &closed, &merged,
		&pr.RepoOwner, &pr.RepoName, &pr.RepoFull,
	); err != nil {
		return nil, err
	}
	if authorID.Valid {
		v := authorID.Int64
		pr.AuthorExternalID = &v
	}
	pr.Draft = draft != 0
	if mergeable.Valid {
		b := mergeable.Int64 != 0
		pr.Mergeable = &b
	}
	pr.CreatedAt = nullTime(created)
	pr.UpdatedAt = nullTime(updated)
	pr.ClosedAt = nullTime(closed)
	pr.MergedAt = nullTime(merged)
	return &pr, nil
}

type ListPRsOpts struct {
	UserID       int64
	BootstrapAll bool
	RepoID       int64
	State        string
	Query        string
	Limit        int
	Offset       int
}

func (s *Store) ListPullRequests(ctx context.Context, opts ListPRsOpts) ([]models.PullRequest, int, error) {
	if err := requireListScope(opts.UserID, opts.BootstrapAll); err != nil {
		return nil, 0, err
	}
	opts.Limit = clampLimit(opts.Limit, 50, 200)
	where := []string{"r.deleted_at IS NULL"}
	args := []any{}
	join := "JOIN repositories r ON r.id = pr.repo_id"
	if opts.UserID > 0 && !opts.BootstrapAll {
		join += " INNER JOIN user_repository_access ura ON ura.repo_id = r.id AND ura.user_id = ?"
		args = append(args, opts.UserID)
	}
	if opts.RepoID > 0 {
		where = append(where, "pr.repo_id = ?")
		args = append(args, opts.RepoID)
	}
	if opts.State != "" && opts.State != "all" {
		where = append(where, "pr.state = ?")
		args = append(args, opts.State)
	}
	if q := strings.TrimSpace(opts.Query); q != "" {
		if like := likePattern(q); like != "" {
			where = append(where, "(pr.title LIKE ? OR r.full_name LIKE ?)")
			args = append(args, like, like)
		}
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM pull_requests pr `+join+` WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, opts.Limit, opts.Offset)
	rows, err := s.query(ctx, `
SELECT pr.id, pr.repo_id, pr.external_id, pr.number, pr.title, pr.body_excerpt, pr.author_login, pr.author_external_id,
       pr.source_branch, pr.target_branch, pr.head_sha, pr.base_sha, pr.state, pr.draft, pr.mergeable, pr.mergeable_state,
       pr.review_state, pr.ci_state, pr.html_url, pr.created_at, pr.updated_at, pr.closed_at, pr.merged_at,
       r.owner, r.name, r.full_name
FROM pull_requests pr `+join+`
WHERE `+clause+`
ORDER BY COALESCE(pr.updated_at, pr.created_at) DESC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.PullRequest
	for rows.Next() {
		pr, err := scanPR(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *pr)
	}
	return out, total, rows.Err()
}

func (s *Store) UpsertWorkflowRun(ctx context.Context, repoID int64, run models.WorkflowRun) (*models.WorkflowRun, error) {
	if run.RunAttempt <= 0 {
		run.RunAttempt = 1
	}
	if existing, err := s.GetWorkflowRunByExternalID(ctx, repoID, run.ExternalID); err == nil {
		if !shouldApplyRun(*existing, run) {
			return existing, nil
		}
	}
	_, err := s.exec(ctx, `
INSERT INTO workflow_runs (
  repo_id, external_id, name, event, branch, commit_sha, status, conclusion,
  upstream_status, upstream_conclusion, actor_login, html_url, workflow_path,
  started_at, completed_at, run_attempt
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_id, external_id) DO UPDATE SET
  name=excluded.name, event=excluded.event, branch=excluded.branch, commit_sha=excluded.commit_sha,
  status=excluded.status, conclusion=excluded.conclusion, upstream_status=excluded.upstream_status,
  upstream_conclusion=excluded.upstream_conclusion, actor_login=excluded.actor_login, html_url=excluded.html_url,
  workflow_path=excluded.workflow_path,
  started_at=COALESCE(excluded.started_at, workflow_runs.started_at),
  completed_at=COALESCE(excluded.completed_at, workflow_runs.completed_at),
  run_attempt=excluded.run_attempt
`, repoID, run.ExternalID, run.Name, run.Event, run.Branch, run.CommitSHA, run.Status, run.Conclusion,
		run.UpstreamStatus, run.UpstreamConclusion, run.ActorLogin, run.HTMLURL, run.WorkflowPath,
		formatTimePtr(run.StartedAt), formatTimePtr(run.CompletedAt), run.RunAttempt)
	if err != nil {
		return nil, err
	}
	return s.GetWorkflowRunByExternalID(ctx, repoID, run.ExternalID)
}

// shouldApplyRun accepts a greater run_attempt, or the same attempt with non-regressing status/completion.
func shouldApplyRun(existing, incoming models.WorkflowRun) bool {
	inAttempt := incoming.RunAttempt
	if inAttempt <= 0 {
		inAttempt = 1
	}
	exAttempt := existing.RunAttempt
	if exAttempt <= 0 {
		exAttempt = 1
	}
	if inAttempt > exAttempt {
		return true
	}
	if inAttempt < exAttempt {
		return false
	}
	if runStatusRank(incoming.Status) < runStatusRank(existing.Status) {
		return false
	}
	if existing.CompletedAt != nil && incoming.CompletedAt == nil && incoming.Status != models.StatusCompleted {
		return false
	}
	return true
}

func runStatusRank(status string) int {
	switch status {
	case models.StatusQueued:
		return 1
	case models.StatusWaiting:
		return 2
	case models.StatusRunning:
		return 3
	case models.StatusCompleted:
		return 4
	default:
		return 0
	}
}

func (s *Store) GetWorkflowRunByExternalID(ctx context.Context, repoID, externalID int64) (*models.WorkflowRun, error) {
	row := s.queryRow(ctx, `
SELECT wr.id, wr.repo_id, wr.workflow_id, wr.external_id, wr.name, wr.event, wr.branch, wr.commit_sha,
       wr.status, wr.conclusion, wr.upstream_status, wr.upstream_conclusion, wr.actor_login, wr.html_url,
       wr.workflow_path, wr.started_at, wr.completed_at, wr.run_attempt, r.owner, r.name, r.full_name
FROM workflow_runs wr JOIN repositories r ON r.id = wr.repo_id
WHERE wr.repo_id=? AND wr.external_id=?`, repoID, externalID)
	return scanRun(row)
}

func (s *Store) GetWorkflowRunByID(ctx context.Context, id int64) (*models.WorkflowRun, error) {
	row := s.queryRow(ctx, `
SELECT wr.id, wr.repo_id, wr.workflow_id, wr.external_id, wr.name, wr.event, wr.branch, wr.commit_sha,
       wr.status, wr.conclusion, wr.upstream_status, wr.upstream_conclusion, wr.actor_login, wr.html_url,
       wr.workflow_path, wr.started_at, wr.completed_at, wr.run_attempt, r.owner, r.name, r.full_name
FROM workflow_runs wr JOIN repositories r ON r.id = wr.repo_id
WHERE wr.id=?`, id)
	return scanRun(row)
}

func scanRun(row scanner) (*models.WorkflowRun, error) {
	var run models.WorkflowRun
	var wfID sql.NullInt64
	var started, completed sql.NullString
	if err := row.Scan(
		&run.ID, &run.RepoID, &wfID, &run.ExternalID, &run.Name, &run.Event, &run.Branch, &run.CommitSHA,
		&run.Status, &run.Conclusion, &run.UpstreamStatus, &run.UpstreamConclusion, &run.ActorLogin, &run.HTMLURL,
		&run.WorkflowPath, &started, &completed, &run.RunAttempt, &run.RepoOwner, &run.RepoName, &run.RepoFull,
	); err != nil {
		return nil, err
	}
	if wfID.Valid {
		v := wfID.Int64
		run.WorkflowID = &v
	}
	run.StartedAt = nullTime(started)
	run.CompletedAt = nullTime(completed)
	return &run, nil
}

type ListRunsOpts struct {
	UserID       int64
	BootstrapAll bool
	Status       string   // single status; ignored when Statuses is non-empty
	Statuses     []string // multi-status IN (...); preferred over Status when set
	Conclusion   string
	Query        string
	Limit        int
	Offset       int
}

func (s *Store) ListWorkflowRuns(ctx context.Context, opts ListRunsOpts) ([]models.WorkflowRun, int, error) {
	if err := requireListScope(opts.UserID, opts.BootstrapAll); err != nil {
		return nil, 0, err
	}
	opts.Limit = clampLimit(opts.Limit, 50, 200)
	where := []string{"r.deleted_at IS NULL"}
	args := []any{}
	join := "JOIN repositories r ON r.id = wr.repo_id"
	if opts.UserID > 0 && !opts.BootstrapAll {
		join += " INNER JOIN user_repository_access ura ON ura.repo_id = r.id AND ura.user_id = ?"
		args = append(args, opts.UserID)
	}
	if len(opts.Statuses) > 0 {
		ph := make([]string, len(opts.Statuses))
		for i, st := range opts.Statuses {
			ph[i] = "?"
			args = append(args, st)
		}
		where = append(where, "wr.status IN ("+strings.Join(ph, ",")+")")
	} else if opts.Status != "" {
		where = append(where, "wr.status = ?")
		args = append(args, opts.Status)
	}
	if opts.Conclusion != "" {
		where = append(where, "wr.conclusion = ?")
		args = append(args, opts.Conclusion)
	}
	if q := strings.TrimSpace(opts.Query); q != "" {
		if like := likePattern(q); like != "" {
			where = append(where, "(wr.name LIKE ? OR r.full_name LIKE ? OR wr.branch LIKE ?)")
			args = append(args, like, like, like)
		}
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM workflow_runs wr `+join+` WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, opts.Limit, opts.Offset)
	rows, err := s.query(ctx, `
SELECT wr.id, wr.repo_id, wr.workflow_id, wr.external_id, wr.name, wr.event, wr.branch, wr.commit_sha,
       wr.status, wr.conclusion, wr.upstream_status, wr.upstream_conclusion, wr.actor_login, wr.html_url,
       wr.workflow_path, wr.started_at, wr.completed_at, wr.run_attempt, r.owner, r.name, r.full_name
FROM workflow_runs wr `+join+`
WHERE `+clause+`
ORDER BY wr.id DESC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.WorkflowRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *run)
	}
	return out, total, rows.Err()
}

func (s *Store) UpsertJob(ctx context.Context, repoID, runID int64, job models.Job) (*models.Job, error) {
	if existing, err := s.GetJobByExternalID(ctx, repoID, job.ExternalID); err == nil {
		if !shouldApplyJob(*existing, job) {
			return existing, nil
		}
	}
	_, err := s.exec(ctx, `
INSERT INTO jobs (
  run_id, repo_id, external_id, name, status, conclusion, upstream_status, upstream_conclusion,
  runner_id, runner_name, html_url, started_at, completed_at, steps_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_id, external_id) DO UPDATE SET
  run_id=excluded.run_id, name=excluded.name, status=excluded.status, conclusion=excluded.conclusion,
  upstream_status=excluded.upstream_status, upstream_conclusion=excluded.upstream_conclusion,
  runner_id=excluded.runner_id, runner_name=excluded.runner_name, html_url=excluded.html_url,
  started_at=COALESCE(excluded.started_at, jobs.started_at),
  completed_at=COALESCE(excluded.completed_at, jobs.completed_at),
  steps_json=COALESCE(excluded.steps_json, jobs.steps_json)
`, runID, repoID, job.ExternalID, job.Name, job.Status, job.Conclusion, job.UpstreamStatus, job.UpstreamConclusion,
		nullInt64(job.RunnerID), job.RunnerName, job.HTMLURL, formatTimePtr(job.StartedAt), formatTimePtr(job.CompletedAt), nullString(job.StepsJSON))
	if err != nil {
		return nil, err
	}
	return s.GetJobByExternalID(ctx, repoID, job.ExternalID)
}

// shouldApplyJob rejects out-of-order webhook/sync updates that would regress status or clear completion.
func shouldApplyJob(existing, incoming models.Job) bool {
	if runStatusRank(incoming.Status) < runStatusRank(existing.Status) {
		return false
	}
	if existing.CompletedAt != nil && incoming.CompletedAt == nil && incoming.Status != models.StatusCompleted {
		return false
	}
	return true
}

// CloseOpenPRsNotInSet marks DB-open PRs closed when their numbers are absent from the open set.
func (s *Store) CloseOpenPRsNotInSet(ctx context.Context, repoID int64, openNumbers []int64) (int64, error) {
	now := formatTime(time.Now().UTC())
	if len(openNumbers) == 0 {
		res, err := s.exec(ctx, `
UPDATE pull_requests SET state='closed', updated_at=COALESCE(updated_at, ?)
WHERE repo_id=? AND state='open'`, now, repoID)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	ph := make([]string, len(openNumbers))
	args := []any{now, repoID}
	for i, n := range openNumbers {
		ph[i] = "?"
		args = append(args, n)
	}
	res, err := s.exec(ctx, `
UPDATE pull_requests SET state='closed', updated_at=COALESCE(updated_at, ?)
WHERE repo_id=? AND state='open' AND number NOT IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func nullString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func (s *Store) GetJobByExternalID(ctx context.Context, repoID, externalID int64) (*models.Job, error) {
	row := s.queryRow(ctx, `
SELECT id, run_id, repo_id, external_id, name, status, conclusion, upstream_status, upstream_conclusion,
       runner_id, runner_name, html_url, started_at, completed_at, steps_json
FROM jobs WHERE repo_id=? AND external_id=?`, repoID, externalID)
	return scanJob(row)
}

func (s *Store) GetJobByID(ctx context.Context, id int64) (*models.Job, error) {
	row := s.queryRow(ctx, `
SELECT id, run_id, repo_id, external_id, name, status, conclusion, upstream_status, upstream_conclusion,
       runner_id, runner_name, html_url, started_at, completed_at, steps_json
FROM jobs WHERE id=?`, id)
	return scanJob(row)
}

func (s *Store) ListJobsByRunID(ctx context.Context, runID int64) ([]models.Job, error) {
	rows, err := s.query(ctx, `
SELECT id, run_id, repo_id, external_id, name, status, conclusion, upstream_status, upstream_conclusion,
       runner_id, runner_name, html_url, started_at, completed_at, steps_json
FROM jobs WHERE run_id=? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// ListJobsByRunIDs returns jobs grouped by run ID. Empty map when runIDs is empty.
func (s *Store) ListJobsByRunIDs(ctx context.Context, runIDs []int64) (map[int64][]models.Job, error) {
	out := make(map[int64][]models.Job)
	if len(runIDs) == 0 {
		return out, nil
	}
	ph := make([]string, len(runIDs))
	args := make([]any, len(runIDs))
	for i, id := range runIDs {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := s.query(ctx, `
SELECT id, run_id, repo_id, external_id, name, status, conclusion, upstream_status, upstream_conclusion,
       runner_id, runner_name, html_url, started_at, completed_at, steps_json
FROM jobs WHERE run_id IN (`+strings.Join(ph, ",")+`) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out[j.RunID] = append(out[j.RunID], *j)
	}
	return out, rows.Err()
}

func scanJob(row scanner) (*models.Job, error) {
	var j models.Job
	var runnerID sql.NullInt64
	var started, completed, steps sql.NullString
	if err := row.Scan(
		&j.ID, &j.RunID, &j.RepoID, &j.ExternalID, &j.Name, &j.Status, &j.Conclusion, &j.UpstreamStatus, &j.UpstreamConclusion,
		&runnerID, &j.RunnerName, &j.HTMLURL, &started, &completed, &steps,
	); err != nil {
		return nil, err
	}
	if runnerID.Valid {
		v := runnerID.Int64
		j.RunnerID = &v
	}
	j.StartedAt = nullTime(started)
	j.CompletedAt = nullTime(completed)
	if steps.Valid {
		s := steps.String
		j.StepsJSON = &s
	}
	return &j, nil
}

func (s *Store) UpsertWorkflowGraph(ctx context.Context, repoID int64, path, commitSHA, nodesJSON string) error {
	_, err := s.exec(ctx, `
INSERT INTO workflow_graphs (repo_id, path, commit_sha, nodes_json)
VALUES (?, ?, ?, ?)
ON CONFLICT(repo_id, path, commit_sha) DO UPDATE SET nodes_json=excluded.nodes_json
`, repoID, path, commitSHA, nodesJSON)
	return err
}

func (s *Store) GetWorkflowGraph(ctx context.Context, repoID int64, path, commitSHA string) (string, error) {
	var nodes string
	err := s.queryRow(ctx, `SELECT nodes_json FROM workflow_graphs WHERE repo_id=? AND path=? AND commit_sha=?`, repoID, path, commitSHA).Scan(&nodes)
	return nodes, err
}

func (s *Store) UpsertAttention(ctx context.Context, item models.AttentionItem) (*models.AttentionItem, error) {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO attention_items (
  instance_id, repo_id, type, severity, entity_type, entity_id, title, metadata_json, fingerprint, opened_at, resolved_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)
ON CONFLICT(fingerprint) DO UPDATE SET
  severity=excluded.severity, title=excluded.title, metadata_json=excluded.metadata_json,
  resolved_at=NULL, updated_at=excluded.updated_at
`, item.InstanceID, item.RepoID, item.Type, item.Severity, item.EntityType, item.EntityID, item.Title, item.MetadataJSON, item.Fingerprint, now, now)
	if err != nil {
		return nil, err
	}
	row := s.queryRow(ctx, `
SELECT a.id, a.instance_id, a.repo_id, a.type, a.severity, a.entity_type, a.entity_id, a.title, a.metadata_json,
       a.fingerprint, a.opened_at, a.resolved_at, a.updated_at, r.owner, r.name, r.full_name,
       `+attentionHTMLURLExpr+`
FROM attention_items a
JOIN repositories r ON r.id = a.repo_id
`+attentionEntityJoins+`
WHERE a.fingerprint=?`, item.Fingerprint)
	return scanAttention(row)
}

func (s *Store) ResolveAttentionByFingerprint(ctx context.Context, fingerprint string) error {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `UPDATE attention_items SET resolved_at=?, updated_at=? WHERE fingerprint=? AND resolved_at IS NULL`, now, now, fingerprint)
	return err
}

const attentionEntityJoins = `
LEFT JOIN pull_requests pr ON a.entity_type = 'pull_request' AND pr.id = a.entity_id
LEFT JOIN workflow_runs wr ON a.entity_type = 'workflow_run' AND wr.id = a.entity_id
LEFT JOIN jobs j ON a.entity_type = 'job' AND j.id = a.entity_id
LEFT JOIN workflow_runs wrj ON a.entity_type = 'job' AND wrj.id = j.run_id`

const attentionHTMLURLExpr = `CASE a.entity_type
  WHEN 'pull_request' THEN COALESCE(pr.html_url, '')
  WHEN 'workflow_run' THEN COALESCE(wr.html_url, '')
  WHEN 'job' THEN COALESCE(NULLIF(j.html_url, ''), wrj.html_url, '')
  ELSE ''
END`

func scanAttention(row scanner) (*models.AttentionItem, error) {
	var a models.AttentionItem
	var opened, resolved, updated sql.NullString
	var htmlURL sql.NullString
	if err := row.Scan(
		&a.ID, &a.InstanceID, &a.RepoID, &a.Type, &a.Severity, &a.EntityType, &a.EntityID, &a.Title, &a.MetadataJSON,
		&a.Fingerprint, &opened, &resolved, &updated, &a.RepoOwner, &a.RepoName, &a.RepoFull, &htmlURL,
	); err != nil {
		return nil, err
	}
	if t := nullTime(opened); t != nil {
		a.OpenedAt = *t
	}
	a.ResolvedAt = nullTime(resolved)
	if t := nullTime(updated); t != nil {
		a.UpdatedAt = *t
	}
	if htmlURL.Valid {
		a.HTMLURL = htmlURL.String
	}
	return &a, nil
}

type ListAttentionOpts struct {
	UserID       int64
	BootstrapAll bool
	Severity     string
	Type         string
	Query        string
	OpenOnly     bool
	Limit        int
	Offset       int
}

func (s *Store) ListAttention(ctx context.Context, opts ListAttentionOpts) ([]models.AttentionItem, int, error) {
	if err := requireListScope(opts.UserID, opts.BootstrapAll); err != nil {
		return nil, 0, err
	}
	opts.Limit = clampLimit(opts.Limit, 50, 200)
	where := []string{"r.deleted_at IS NULL"}
	args := []any{}
	join := "JOIN repositories r ON r.id = a.repo_id"
	if opts.UserID > 0 && !opts.BootstrapAll {
		join += " INNER JOIN user_repository_access ura ON ura.repo_id = r.id AND ura.user_id = ?"
		args = append(args, opts.UserID)
	}
	if opts.OpenOnly {
		where = append(where, "a.resolved_at IS NULL")
	}
	if opts.Severity != "" {
		where = append(where, "a.severity = ?")
		args = append(args, opts.Severity)
	}
	if opts.Type != "" {
		where = append(where, "a.type = ?")
		args = append(args, opts.Type)
	}
	if q := strings.TrimSpace(opts.Query); q != "" {
		if like := likePattern(q); like != "" {
			where = append(where, "(a.title LIKE ? OR a.type LIKE ? OR r.full_name LIKE ?)")
			args = append(args, like, like, like)
		}
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM attention_items a `+join+` WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, opts.Limit, opts.Offset)
	rows, err := s.query(ctx, `
SELECT a.id, a.instance_id, a.repo_id, a.type, a.severity, a.entity_type, a.entity_id, a.title, a.metadata_json,
       a.fingerprint, a.opened_at, a.resolved_at, a.updated_at, r.owner, r.name, r.full_name,
       `+attentionHTMLURLExpr+`
FROM attention_items a `+join+attentionEntityJoins+`
WHERE `+clause+`
ORDER BY CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 WHEN 'waiting' THEN 2 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END, a.opened_at DESC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.AttentionItem
	for rows.Next() {
		a, err := scanAttention(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *a)
	}
	return out, total, rows.Err()
}

// Summary returns dashboard counters scoped to repositories the caller can access.
// When since is non-nil, time-based metrics are limited to activity at or after that instant:
// open PRs by created_at, open attention by opened_at, failed runs by completed_at/started_at,
// running runs by started_at. Repositories remain a current inventory count.
// When snapshot is true (dashboard "Now"), counters reflect current open/running state with no
// lookback: open PRs, open attention, and running runs are unfiltered; failed runs count only
// failures that still have an open attention item (not historical failure volume).
func (s *Store) Summary(ctx context.Context, userID int64, bootstrapAll bool, since *time.Time, snapshot bool) (*models.Summary, error) {
	if err := requireListScope(userID, bootstrapAll); err != nil {
		return nil, err
	}
	if snapshot {
		since = nil
	}
	join := ""
	args := []any{}
	if userID > 0 && !bootstrapAll {
		join = "INNER JOIN user_repository_access ura ON ura.repo_id = r.id AND ura.user_id = ?"
		args = append(args, userID)
	}
	sum := &models.Summary{}
	if since != nil {
		sum.Since = formatTime(since.UTC())
	}
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM repositories r `+join+` WHERE r.deleted_at IS NULL`, args...).Scan(&sum.Repositories); err != nil {
		return nil, err
	}

	prSQL := `
SELECT COUNT(*) FROM pull_requests pr JOIN repositories r ON r.id = pr.repo_id ` + join + `
WHERE r.deleted_at IS NULL AND pr.state = 'open'`
	prArgs := append([]any{}, args...)
	if since != nil {
		prSQL += ` AND COALESCE(pr.created_at, pr.updated_at) >= ?`
		prArgs = append(prArgs, sum.Since)
	}
	if err := s.queryRow(ctx, prSQL, prArgs...).Scan(&sum.OpenPRs); err != nil {
		return nil, err
	}

	attSQL := `
SELECT COUNT(*) FROM attention_items a JOIN repositories r ON r.id = a.repo_id ` + join + `
WHERE r.deleted_at IS NULL AND a.resolved_at IS NULL`
	attArgs := append([]any{}, args...)
	if since != nil {
		attSQL += ` AND a.opened_at >= ?`
		attArgs = append(attArgs, sum.Since)
	}
	if err := s.queryRow(ctx, attSQL, attArgs...).Scan(&sum.Attention); err != nil {
		return nil, err
	}

	failSQL := `
SELECT COUNT(*) FROM workflow_runs wr JOIN repositories r ON r.id = wr.repo_id ` + join + `
WHERE r.deleted_at IS NULL AND wr.conclusion = 'failure'`
	failArgs := append([]any{}, args...)
	if snapshot {
		failSQL += `
AND EXISTS (
  SELECT 1 FROM attention_items a
  WHERE a.entity_type = 'workflow_run' AND a.entity_id = wr.id AND a.resolved_at IS NULL
)`
	} else if since != nil {
		failSQL += ` AND COALESCE(wr.completed_at, wr.started_at) >= ?`
		failArgs = append(failArgs, sum.Since)
	}
	if err := s.queryRow(ctx, failSQL, failArgs...).Scan(&sum.FailedRuns); err != nil {
		return nil, err
	}

	runSQL := `
SELECT COUNT(*) FROM workflow_runs wr JOIN repositories r ON r.id = wr.repo_id ` + join + `
WHERE r.deleted_at IS NULL AND wr.status = 'running'`
	runArgs := append([]any{}, args...)
	if since != nil {
		runSQL += ` AND COALESCE(wr.started_at, wr.completed_at) >= ?`
		runArgs = append(runArgs, sum.Since)
	}
	if err := s.queryRow(ctx, runSQL, runArgs...).Scan(&sum.RunningRuns); err != nil {
		return nil, err
	}
	return sum, nil
}

func (s *Store) CountWorkflowRuns(ctx context.Context) (int64, error) {
	var n int64
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM workflow_runs`).Scan(&n)
	return n, err
}

func (s *Store) UserCanAccessRepo(ctx context.Context, userID, repoID int64) (bool, error) {
	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM user_repository_access WHERE user_id=? AND repo_id=?`, userID, repoID).Scan(&n)
	return n > 0, err
}

func (s *Store) ReplaceUserRepoAccess(ctx context.Context, userID int64, repoIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.sql(`DELETE FROM user_repository_access WHERE user_id=?`), userID); err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())
	for _, rid := range repoIDs {
		if _, err := tx.ExecContext(ctx, s.sql(`INSERT INTO user_repository_access (user_id, repo_id, permission, checked_at) VALUES (?, ?, 'read', ?)`), userID, rid, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteAccessForRepo removes all ACL rows for a repository (forces next refresh to re-grant).
func (s *Store) DeleteAccessForRepo(ctx context.Context, repoID int64) error {
	_, err := s.exec(ctx, `DELETE FROM user_repository_access WHERE repo_id=?`, repoID)
	return err
}

// ListUsersWithTokenCiphers returns non-bootstrap users that have a stored OAuth token cipher.
func (s *Store) ListUsersWithTokenCiphers(ctx context.Context) ([]models.User, error) {
	rows, err := s.query(ctx, `
SELECT u.id, u.instance_id, u.gitea_user_id, u.login, u.email, u.display_name, u.avatar_url, u.is_bootstrap_admin, u.created_at, u.updated_at
FROM users u
INNER JOIN user_tokens t ON t.user_id = u.id
WHERE u.is_bootstrap_admin = 0 AND t.access_token_ciphertext != ''
ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (s *Store) GrantBootstrapAllAccess(ctx context.Context, userID, instanceID int64) error {
	repos, err := s.ListAllAliveRepos(ctx, instanceID)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(repos))
	for _, r := range repos {
		ids = append(ids, r.ID)
	}
	return s.ReplaceUserRepoAccess(ctx, userID, ids)
}

func (s *Store) Search(ctx context.Context, userID int64, bootstrapAll bool, q string, limit int) (map[string]any, error) {
	if err := requireListScope(userID, bootstrapAll); err != nil {
		return nil, err
	}
	limit = clampLimit(limit, 20, 200)
	q = strings.TrimSpace(q)
	repos, _, err := s.ListRepositories(ctx, ListRepositoriesOpts{UserID: userID, BootstrapAll: bootstrapAll, Query: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	prs, _, err := s.ListPullRequests(ctx, ListPRsOpts{UserID: userID, BootstrapAll: bootstrapAll, Query: q, Limit: limit, State: "open"})
	if err != nil {
		return nil, err
	}
	runs, _, err := s.ListWorkflowRuns(ctx, ListRunsOpts{UserID: userID, BootstrapAll: bootstrapAll, Query: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	return map[string]any{"repositories": repos, "pull_requests": prs, "workflow_runs": runs}, nil
}
