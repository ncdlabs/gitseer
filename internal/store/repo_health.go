package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
)

// Default windows for repo health / failure intelligence.
const (
	DefaultHealthWindowDays    = 7
	DefaultStalePRDays         = 14
	DefaultFailureClusterLimit = 25
)

// RepoHealthOpts controls health rollup windows.
type RepoHealthOpts struct {
	WindowDays  int // CI fail-rate lookback; default 7
	StalePRDays int // open PR age threshold; default 14
}

func normalizeRepoHealthOpts(opts RepoHealthOpts) RepoHealthOpts {
	if opts.WindowDays <= 0 {
		opts.WindowDays = DefaultHealthWindowDays
	}
	if opts.WindowDays > 90 {
		opts.WindowDays = 90
	}
	if opts.StalePRDays <= 0 {
		opts.StalePRDays = DefaultStalePRDays
	}
	if opts.StalePRDays > 365 {
		opts.StalePRDays = 365
	}
	return opts
}

// RepoHealthSummaries returns health rollups for the given repo IDs (batch).
// Missing IDs are omitted. Empty input returns an empty map.
func (s *Store) RepoHealthSummaries(ctx context.Context, repoIDs []int64, opts RepoHealthOpts) (map[int64]models.RepoHealth, error) {
	opts = normalizeRepoHealthOpts(opts)
	out := make(map[int64]models.RepoHealth, len(repoIDs))
	if len(repoIDs) == 0 {
		return out, nil
	}
	ids := uniquePositiveIDs(repoIDs)
	if len(ids) == 0 {
		return out, nil
	}

	for _, id := range ids {
		out[id] = models.RepoHealth{
			RepoID:      id,
			Score:       100,
			Grade:       models.HealthGradeHealthy,
			WindowDays:  opts.WindowDays,
			StalePRDays: opts.StalePRDays,
		}
	}

	ph, args := placeholders(ids)
	critSQL := `
SELECT a.repo_id, COUNT(*)
FROM attention_items a
WHERE a.resolved_at IS NULL AND a.severity = 'critical' AND a.repo_id IN (` + ph + `)
GROUP BY a.repo_id`
	rows, err := s.query(ctx, critSQL, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var repoID int64
		var n int
		if err := rows.Scan(&repoID, &n); err != nil {
			rows.Close()
			return nil, err
		}
		h := out[repoID]
		h.OpenCriticalAttention = n
		out[repoID] = h
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	// Latest completed default-branch run per repo, then mark failing.
	failSQL := `
SELECT wr.repo_id, wr.id, wr.conclusion
FROM workflow_runs wr
JOIN repositories r ON r.id = wr.repo_id
WHERE wr.repo_id IN (` + ph + `)
  AND r.default_branch != ''
  AND wr.branch = r.default_branch
  AND wr.status = ?
  AND wr.id = (
    SELECT wr2.id FROM workflow_runs wr2
    WHERE wr2.repo_id = wr.repo_id
      AND wr2.branch = r.default_branch
      AND wr2.status = ?
    ORDER BY COALESCE(wr2.completed_at, '') DESC, wr2.id DESC
    LIMIT 1
  )`
	failArgs := append(append([]any{}, args...), models.StatusCompleted, models.StatusCompleted)
	rows, err = s.query(ctx, failSQL, failArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var repoID, runID int64
		var conclusion string
		if err := rows.Scan(&repoID, &runID, &conclusion); err != nil {
			rows.Close()
			return nil, err
		}
		if conclusion != models.ConclusionFailure && conclusion != models.ConclusionTimedOut {
			continue
		}
		h := out[repoID]
		h.FailingDefaultBranch = true
		rid := runID
		h.FailingDefaultBranchRun = &rid
		out[repoID] = h
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	staleCutoff := formatTime(time.Now().UTC().Add(-time.Duration(opts.StalePRDays) * 24 * time.Hour))
	staleSQL := `
SELECT repo_id, COUNT(*)
FROM pull_requests
WHERE state = 'open' AND repo_id IN (` + ph + `)
  AND COALESCE(updated_at, created_at, '') != ''
  AND COALESCE(updated_at, created_at, '') < ?
GROUP BY repo_id`
	staleArgs := append(append([]any{}, args...), staleCutoff)
	rows, err = s.query(ctx, staleSQL, staleArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var repoID int64
		var n int
		if err := rows.Scan(&repoID, &n); err != nil {
			rows.Close()
			return nil, err
		}
		h := out[repoID]
		h.StaleOpenPRs = n
		out[repoID] = h
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	since := formatTime(time.Now().UTC().Add(-time.Duration(opts.WindowDays) * 24 * time.Hour))
	ciSQL := `
SELECT repo_id,
       COUNT(*),
       SUM(CASE WHEN conclusion IN ('failure', 'timed_out') THEN 1 ELSE 0 END)
FROM workflow_runs
WHERE repo_id IN (` + ph + `)
  AND status = ?
  AND COALESCE(completed_at, started_at, '') >= ?
GROUP BY repo_id`
	ciArgs := append(append([]any{}, args...), models.StatusCompleted, since)
	rows, err = s.query(ctx, ciSQL, ciArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var repoID int64
		var total, failures int
		if err := rows.Scan(&repoID, &total, &failures); err != nil {
			rows.Close()
			return nil, err
		}
		h := out[repoID]
		h.CIRunsInWindow = total
		h.CIFailuresInWindow = failures
		if total > 0 {
			h.CIFailRate = float64(failures) / float64(total)
		}
		out[repoID] = h
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for id, h := range out {
		out[id] = scoreRepoHealth(h)
	}
	return out, nil
}

// RepoHealthSummary returns health for one repository.
func (s *Store) RepoHealthSummary(ctx context.Context, repoID int64, opts RepoHealthOpts) (*models.RepoHealth, error) {
	if repoID <= 0 {
		return nil, fmt.Errorf("invalid repo id")
	}
	m, err := s.RepoHealthSummaries(ctx, []int64{repoID}, opts)
	if err != nil {
		return nil, err
	}
	h, ok := m[repoID]
	if !ok {
		empty := scoreRepoHealth(models.RepoHealth{
			RepoID:      repoID,
			Score:       100,
			Grade:       models.HealthGradeHealthy,
			WindowDays:  normalizeRepoHealthOpts(opts).WindowDays,
			StalePRDays: normalizeRepoHealthOpts(opts).StalePRDays,
		})
		return &empty, nil
	}
	return &h, nil
}

// FailureClusters aggregates failed jobs by (workflow_path, job_name) since the given time.
func (s *Store) FailureClusters(ctx context.Context, repoID int64, since time.Time, limit int) ([]models.FailureCluster, error) {
	if repoID <= 0 {
		return nil, fmt.Errorf("invalid repo id")
	}
	if limit <= 0 {
		limit = DefaultFailureClusterLimit
	}
	if limit > 100 {
		limit = 100
	}
	sinceStr := formatTime(since.UTC())
	rows, err := s.query(ctx, `
SELECT wr.repo_id, COALESCE(wr.workflow_path, ''), COALESCE(j.name, ''),
       COUNT(*),
       MAX(COALESCE(j.completed_at, wr.completed_at, '')),
       MAX(wr.id),
       MAX(j.id)
FROM jobs j
JOIN workflow_runs wr ON wr.id = j.run_id
WHERE j.repo_id = ?
  AND j.conclusion IN ('failure', 'timed_out')
  AND COALESCE(j.completed_at, wr.completed_at, '') >= ?
GROUP BY wr.repo_id, COALESCE(wr.workflow_path, ''), COALESCE(j.name, '')
ORDER BY COUNT(*) DESC, MAX(COALESCE(j.completed_at, wr.completed_at, '')) DESC
LIMIT ?`, repoID, sinceStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.FailureCluster
	for rows.Next() {
		var c models.FailureCluster
		var lastStr string
		var runID, jobID int64
		if err := rows.Scan(&c.RepoID, &c.WorkflowPath, &c.JobName, &c.FailureCount, &lastStr, &runID, &jobID); err != nil {
			return nil, err
		}
		if t, err := parseTime(lastStr); err == nil && !t.IsZero() {
			c.LastFailedAt = &t
		}
		if runID > 0 {
			rid := runID
			c.SampleRunID = &rid
		}
		if jobID > 0 {
			jid := jobID
			c.SampleJobID = &jid
		}
		out = append(out, c)
	}
	if out == nil {
		out = []models.FailureCluster{}
	}
	return out, rows.Err()
}

func scoreRepoHealth(h models.RepoHealth) models.RepoHealth {
	score := 100
	critPenalty := h.OpenCriticalAttention * 20
	if critPenalty > 60 {
		critPenalty = 60
	}
	score -= critPenalty
	if h.FailingDefaultBranch {
		score -= 30
	}
	stalePenalty := h.StaleOpenPRs * 5
	if stalePenalty > 20 {
		stalePenalty = 20
	}
	score -= stalePenalty
	if h.CIRunsInWindow > 0 {
		score -= int(40 * h.CIFailRate)
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	h.Score = score
	switch {
	case score >= 80:
		h.Grade = models.HealthGradeHealthy
	case score >= 50:
		h.Grade = models.HealthGradeDegraded
	default:
		h.Grade = models.HealthGradeCritical
	}
	return h
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func placeholders(ids []int64) (string, []any) {
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	return strings.Join(ph, ","), args
}
