package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
)

// DashboardScope optionally narrows summary/stats to an org/owner/team.
// Team filter: when team is set without owner, matches repositories whose owner
// equals the team slug (GitHub/Gitea org/team login alias). Forges do not expose
// durable team→repo membership in GitSeer inventory today, so this is the best
// available approximation — not a full team membership query.
type DashboardScope struct {
	OrgID int64
	Owner string
	Team  string
}

func (d DashboardScope) normalize() DashboardScope {
	d.Owner = strings.TrimSpace(d.Owner)
	d.Team = strings.TrimSpace(d.Team)
	if d.Owner == "" && d.Team != "" {
		d.Owner = d.Team
	}
	return d
}

func (d DashboardScope) repoFilterSQL() (clause string, args []any) {
	d = d.normalize()
	if d.OrgID > 0 {
		return " AND r.org_id = ?", []any{d.OrgID}
	}
	if d.Owner != "" {
		return " AND LOWER(r.owner) = LOWER(?)", []any{d.Owner}
	}
	return "", nil
}

// Summary returns authz-scoped dashboard counts (all accessible repos).
func (s *Store) Summary(ctx context.Context, userID int64, bootstrapAll bool, since *time.Time, snapshot bool) (*models.Summary, error) {
	return s.SummaryScoped(ctx, userID, bootstrapAll, since, snapshot, DashboardScope{})
}

// SummaryScoped is Summary with optional org/owner/team filter.
func (s *Store) SummaryScoped(ctx context.Context, userID int64, bootstrapAll bool, since *time.Time, snapshot bool, scope DashboardScope) (*models.Summary, error) {
	if err := requireListScope(userID, bootstrapAll); err != nil {
		return nil, err
	}
	scope = scope.normalize()
	if snapshot {
		since = nil
	}
	join := ""
	args := []any{}
	if userID > 0 && !bootstrapAll {
		join = "INNER JOIN user_repository_access ura ON ura.repo_id = r.id AND ura.user_id = ?"
		args = append(args, userID)
	}
	filter, fArgs := scope.repoFilterSQL()
	args = append(args, fArgs...)
	sum := &models.Summary{
		OrgID: scope.OrgID,
		Owner: scope.Owner,
		Team:  scope.Team,
	}
	if since != nil {
		sum.Since = formatTime(since.UTC())
	}
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM repositories r `+join+` WHERE r.deleted_at IS NULL`+filter, args...).Scan(&sum.Repositories); err != nil {
		return nil, err
	}

	prSQL := `
SELECT COUNT(*) FROM pull_requests pr JOIN repositories r ON r.id = pr.repo_id ` + join + `
WHERE r.deleted_at IS NULL AND pr.state = 'open'` + filter
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
WHERE r.deleted_at IS NULL AND a.resolved_at IS NULL` + filter
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
WHERE r.deleted_at IS NULL AND wr.conclusion = 'failure'` + filter
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
WHERE r.deleted_at IS NULL AND wr.status = 'running'` + filter
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

// ListAccessibleInstanceIDs returns distinct instance IDs for alive repos the user can access.
func (s *Store) ListAccessibleInstanceIDs(ctx context.Context, userID int64) ([]int64, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	rows, err := s.query(ctx, `
SELECT DISTINCT r.instance_id
FROM repositories r
INNER JOIN user_repository_access ura ON ura.repo_id = r.id AND ura.user_id = ?
WHERE r.deleted_at IS NULL
ORDER BY r.instance_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RunnerUtilization aggregates indexed jobs by runner name (authz-scoped).
// When no runner names are present, Degraded is set — forge runners APIs are not required.
func (s *Store) RunnerUtilization(ctx context.Context, userID int64, bootstrapAll bool, since time.Time, limit int) (*models.RunnerUtilizationReport, error) {
	if err := requireListScope(userID, bootstrapAll); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	join, args := statsAuthzJoin(userID, bootstrapAll)
	sinceStr := formatTime(since.UTC())
	out := &models.RunnerUtilizationReport{
		Source: "indexed_jobs",
		Since:  sinceStr,
		Items:  []models.RunnerUtilizationRow{},
	}

	// Capability probe: any instance with runners_api in capabilities JSON.
	capRows, err := s.query(ctx, `
SELECT COALESCE(capabilities_json, '') FROM instances`)
	if err != nil {
		return nil, err
	}
	for capRows.Next() {
		var raw string
		if err := capRows.Scan(&raw); err != nil {
			capRows.Close()
			return nil, err
		}
		var caps models.Capabilities
		if strings.TrimSpace(raw) != "" && json.Unmarshal([]byte(raw), &caps) == nil && caps.RunnersAPI {
			out.RunnersAPICapable = true
			break
		}
	}
	capRows.Close()
	if err := capRows.Err(); err != nil {
		return nil, err
	}

	q := `
SELECT COALESCE(NULLIF(TRIM(j.runner_name), ''), '(unnamed)'),
       MAX(j.runner_id),
       SUM(CASE WHEN j.status IN ('running', 'waiting') THEN 1 ELSE 0 END),
       SUM(CASE WHEN j.status = 'queued' THEN 1 ELSE 0 END),
       SUM(CASE WHEN j.status = 'completed' THEN 1 ELSE 0 END),
       SUM(CASE WHEN j.conclusion IN ('failure', 'timed_out') THEN 1 ELSE 0 END),
       MAX(r.instance_id),
       COALESCE(MAX(i.name), ''),
       COALESCE(NULLIF(TRIM(MAX(i.forge_type)), ''), 'gitea')
FROM jobs j
JOIN repositories r ON r.id = j.repo_id ` + join + `
LEFT JOIN instances i ON i.id = r.instance_id
WHERE r.deleted_at IS NULL
  AND COALESCE(j.completed_at, j.started_at, '') >= ?
GROUP BY COALESCE(NULLIF(TRIM(j.runner_name), ''), '(unnamed)')
ORDER BY SUM(CASE WHEN j.status IN ('running', 'waiting', 'queued') THEN 1 ELSE 0 END) DESC,
         SUM(CASE WHEN j.status = 'completed' THEN 1 ELSE 0 END) DESC,
         COALESCE(NULLIF(TRIM(j.runner_name), ''), '(unnamed)') ASC
LIMIT ?`
	qArgs := append(append([]any{}, args...), sinceStr, limit)
	rows, err := s.query(ctx, q, qArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	named := 0
	for rows.Next() {
		var row models.RunnerUtilizationRow
		var runnerID sqlNullInt64
		if err := rows.Scan(
			&row.RunnerName, &runnerID,
			&row.BusyJobs, &row.QueuedJobs, &row.CompletedJobs, &row.FailedJobs,
			&row.InstanceID, &row.InstanceName, &row.ForgeType,
		); err != nil {
			return nil, err
		}
		if runnerID.valid {
			v := runnerID.v
			row.RunnerID = &v
		}
		if row.RunnerName != "(unnamed)" {
			named++
		}
		out.Items = append(out.Items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if named == 0 {
		out.Degraded = true
		if out.RunnersAPICapable {
			out.DegradedReason = "Indexed jobs lack runner names; forge runners API is capable but live listing is not wired — utilization is approximate"
		} else {
			out.DegradedReason = "No runner names on indexed jobs and no forge reports runners_api; utilization degraded"
		}
	}
	return out, nil
}

type sqlNullInt64 struct {
	v     int64
	valid bool
}

func (n *sqlNullInt64) Scan(src any) error {
	if src == nil {
		n.valid = false
		n.v = 0
		return nil
	}
	switch v := src.(type) {
	case int64:
		n.v, n.valid = v, true
	case int:
		n.v, n.valid = int64(v), true
	case []byte:
		var x int64
		if _, err := fmt.Sscan(string(v), &x); err != nil {
			n.valid = false
			return nil
		}
		n.v, n.valid = x, true
	default:
		n.valid = false
	}
	return nil
}

// FlakyJobs detects jobs that both failed and succeeded in the window (flip heuristic).
func (s *Store) FlakyJobs(ctx context.Context, userID int64, bootstrapAll bool, repoID int64, since time.Time, limit int) ([]models.FlakyJob, error) {
	if err := requireListScope(userID, bootstrapAll); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	join, args := statsAuthzJoin(userID, bootstrapAll)
	sinceStr := formatTime(since.UTC())
	repoFilter := ""
	var repoArgs []any
	if repoID > 0 {
		repoFilter = " AND j.repo_id = ?"
		repoArgs = append(repoArgs, repoID)
	}
	// Aggregate fail/success counts per (repo, workflow, job). Flip ≈ min(fail, success)*2 heuristic.
	q := `
SELECT j.repo_id,
       COALESCE(r.full_name, r.owner || '/' || r.name),
       COALESCE(wr.workflow_path, ''),
       COALESCE(j.name, ''),
       SUM(CASE WHEN j.conclusion IN ('failure', 'timed_out') THEN 1 ELSE 0 END),
       SUM(CASE WHEN j.conclusion = 'success' THEN 1 ELSE 0 END),
       MAX(CASE WHEN j.conclusion IN ('failure', 'timed_out') THEN COALESCE(j.completed_at, '') ELSE '' END),
       MAX(CASE WHEN j.conclusion = 'success' THEN COALESCE(j.completed_at, '') ELSE '' END),
       MAX(wr.id)
FROM jobs j
JOIN workflow_runs wr ON wr.id = j.run_id
JOIN repositories r ON r.id = j.repo_id ` + join + `
WHERE r.deleted_at IS NULL
  AND j.status = 'completed'
  AND COALESCE(j.completed_at, wr.completed_at, '') >= ?
` + repoFilter + `
GROUP BY j.repo_id, COALESCE(r.full_name, r.owner || '/' || r.name), COALESCE(wr.workflow_path, ''), COALESCE(j.name, '')
HAVING SUM(CASE WHEN j.conclusion IN ('failure', 'timed_out') THEN 1 ELSE 0 END) > 0
   AND SUM(CASE WHEN j.conclusion = 'success' THEN 1 ELSE 0 END) > 0
ORDER BY CASE
  WHEN SUM(CASE WHEN j.conclusion IN ('failure', 'timed_out') THEN 1 ELSE 0 END)
     < SUM(CASE WHEN j.conclusion = 'success' THEN 1 ELSE 0 END)
  THEN SUM(CASE WHEN j.conclusion IN ('failure', 'timed_out') THEN 1 ELSE 0 END)
  ELSE SUM(CASE WHEN j.conclusion = 'success' THEN 1 ELSE 0 END)
END DESC,
SUM(CASE WHEN j.conclusion IN ('failure', 'timed_out') THEN 1 ELSE 0 END) DESC
LIMIT ?`
	qArgs := append(append([]any{}, args...), sinceStr)
	qArgs = append(qArgs, repoArgs...)
	qArgs = append(qArgs, limit)
	rows, err := s.query(ctx, q, qArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.FlakyJob
	for rows.Next() {
		var f models.FlakyJob
		var lastFail, lastOK string
		var runID int64
		if err := rows.Scan(&f.RepoID, &f.RepoFull, &f.WorkflowPath, &f.JobName,
			&f.FailureCount, &f.SuccessCount, &lastFail, &lastOK, &runID); err != nil {
			return nil, err
		}
		if f.FailureCount < f.SuccessCount {
			f.FlipCount = f.FailureCount * 2
		} else {
			f.FlipCount = f.SuccessCount * 2
		}
		if t, err := parseTime(lastFail); err == nil && !t.IsZero() {
			f.LastFailedAt = &t
		}
		if t, err := parseTime(lastOK); err == nil && !t.IsZero() {
			f.LastSuccessAt = &t
		}
		if runID > 0 {
			rid := runID
			f.SampleRunID = &rid
		}
		out = append(out, f)
	}
	if out == nil {
		out = []models.FlakyJob{}
	}
	return out, rows.Err()
}

var deployNameTips = []string{"deploy", "release", "production", "prod", "cd.yaml", "cd.yml"}

// LooksLikeDeploy reports whether a workflow run name/path/event looks deploy/release related.
func LooksLikeDeploy(name, workflowPath, event string) bool {
	blob := strings.ToLower(name + " " + workflowPath + " " + event)
	for _, tip := range deployNameTips {
		if strings.Contains(blob, tip) {
			return true
		}
	}
	return false
}

// ListReleaseRuns returns recent release/deploy-like workflow runs (authz-scoped).
func (s *Store) ListReleaseRuns(ctx context.Context, userID int64, bootstrapAll bool, since time.Time, limit int) ([]models.ReleaseRun, error) {
	if err := requireListScope(userID, bootstrapAll); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	join, args := statsAuthzJoin(userID, bootstrapAll)
	sinceStr := formatTime(since.UTC())
	// Broad SQL prefilter; exact tip match in Go.
	q := `
SELECT wr.id, wr.repo_id, COALESCE(r.full_name, r.owner || '/' || r.name),
       COALESCE(wr.name, ''), COALESCE(wr.workflow_path, ''), COALESCE(wr.event, ''),
       COALESCE(wr.branch, ''), COALESCE(wr.status, ''), COALESCE(wr.conclusion, ''),
       COALESCE(wr.html_url, ''), wr.started_at, wr.completed_at,
       COALESCE(NULLIF(TRIM(i.forge_type), ''), 'gitea'), r.instance_id,
       EXISTS (
         SELECT 1 FROM attention_items a
         WHERE a.entity_type = 'workflow_run' AND a.entity_id = wr.id AND a.resolved_at IS NULL
           AND a.type = 'deployment_workflow_failure'
       )
FROM workflow_runs wr
JOIN repositories r ON r.id = wr.repo_id ` + join + `
LEFT JOIN instances i ON i.id = r.instance_id
WHERE r.deleted_at IS NULL
  AND COALESCE(wr.completed_at, wr.started_at, '') >= ?
  AND (
    LOWER(COALESCE(wr.name, '') || ' ' || COALESCE(wr.workflow_path, '') || ' ' || COALESCE(wr.event, '')) LIKE '%deploy%'
    OR LOWER(COALESCE(wr.name, '') || ' ' || COALESCE(wr.workflow_path, '') || ' ' || COALESCE(wr.event, '')) LIKE '%release%'
    OR LOWER(COALESCE(wr.name, '') || ' ' || COALESCE(wr.workflow_path, '') || ' ' || COALESCE(wr.event, '')) LIKE '%production%'
    OR LOWER(COALESCE(wr.name, '') || ' ' || COALESCE(wr.workflow_path, '') || ' ' || COALESCE(wr.event, '')) LIKE '%prod%'
    OR LOWER(COALESCE(wr.workflow_path, '')) LIKE '%cd.y%'
  )
ORDER BY COALESCE(wr.completed_at, wr.started_at, '') DESC, wr.id DESC
LIMIT ?`
	// Fetch extra then filter tips precisely (prod matches production etc. already ok).
	fetch := limit * 3
	if fetch > 500 {
		fetch = 500
	}
	qArgs := append(append([]any{}, args...), sinceStr, fetch)
	rows, err := s.query(ctx, q, qArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ReleaseRun
	for rows.Next() {
		var rr models.ReleaseRun
		var started, completed sql.NullString
		var att int
		if err := rows.Scan(
			&rr.ID, &rr.RepoID, &rr.RepoFull, &rr.Name, &rr.WorkflowPath, &rr.Event,
			&rr.Branch, &rr.Status, &rr.Conclusion, &rr.HTMLURL, &started, &completed,
			&rr.ForgeType, &rr.InstanceID, &att,
		); err != nil {
			return nil, err
		}
		if !LooksLikeDeploy(rr.Name, rr.WorkflowPath, rr.Event) {
			continue
		}
		if started.Valid {
			if t, err := parseTime(started.String); err == nil && !t.IsZero() {
				rr.StartedAt = &t
			}
		}
		if completed.Valid {
			if t, err := parseTime(completed.String); err == nil && !t.IsZero() {
				rr.CompletedAt = &t
			}
		}
		rr.AttentionOpen = att != 0
		out = append(out, rr)
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		out = []models.ReleaseRun{}
	}
	return out, rows.Err()
}
