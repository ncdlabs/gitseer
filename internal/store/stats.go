package store

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/models"
)

// StatsReport returns authz-scoped dashboard series for the [since, now] window.
// Day series are zero-filled; run_duration is nil when there are no completed samples.
func (s *Store) StatsReport(ctx context.Context, userID int64, bootstrapAll bool, since time.Time) (*models.StatsReport, error) {
	if err := requireListScope(userID, bootstrapAll); err != nil {
		return nil, err
	}
	join := ""
	args := []any{}
	if userID > 0 && !bootstrapAll {
		join = "INNER JOIN user_repository_access ura ON ura.repo_id = r.id AND ura.user_id = ?"
		args = append(args, userID)
	}
	now := time.Now().UTC()
	sinceUTC := since.UTC()
	sinceStr := formatTime(sinceUTC)
	days := dayKeys(sinceUTC, now)

	out := &models.StatsReport{
		Since:               sinceStr,
		RunsByDay:           make([]models.DayRunBucket, 0, len(days)),
		RunConclusions:      []models.CountBucket{},
		PRsByDay:            make([]models.DayPRBucket, 0, len(days)),
		PRCIStates:          []models.CountBucket{},
		AttentionBySeverity: []models.CountBucket{},
		AttentionByType:     []models.CountBucket{},
	}

	runByDay, err := s.statsRunsByDay(ctx, join, args, sinceStr)
	if err != nil {
		return nil, err
	}
	for _, d := range days {
		b := runByDay[d]
		b.Day = d
		out.RunsByDay = append(out.RunsByDay, b)
	}

	out.RunConclusions, err = s.statsCountBuckets(ctx, `
SELECT COALESCE(NULLIF(wr.conclusion, ''), 'unknown') AS k, COUNT(*)
FROM workflow_runs wr JOIN repositories r ON r.id = wr.repo_id `+join+`
WHERE r.deleted_at IS NULL
  AND COALESCE(wr.completed_at, wr.started_at) IS NOT NULL
  AND COALESCE(wr.completed_at, wr.started_at) >= ?
GROUP BY k
ORDER BY COUNT(*) DESC, k`, args, sinceStr)
	if err != nil {
		return nil, err
	}

	out.RunDuration, err = s.statsRunDuration(ctx, join, args, sinceStr)
	if err != nil {
		return nil, err
	}

	prOpened, err := s.statsDayCounts(ctx, `
SELECT substr(pr.created_at, 1, 10) AS day, COUNT(*)
FROM pull_requests pr JOIN repositories r ON r.id = pr.repo_id `+join+`
WHERE r.deleted_at IS NULL AND pr.created_at IS NOT NULL AND pr.created_at >= ?
GROUP BY day`, args, sinceStr)
	if err != nil {
		return nil, err
	}
	prMerged, err := s.statsDayCounts(ctx, `
SELECT substr(pr.merged_at, 1, 10) AS day, COUNT(*)
FROM pull_requests pr JOIN repositories r ON r.id = pr.repo_id `+join+`
WHERE r.deleted_at IS NULL AND pr.merged_at IS NOT NULL AND pr.merged_at >= ?
GROUP BY day`, args, sinceStr)
	if err != nil {
		return nil, err
	}
	prClosed, err := s.statsDayCounts(ctx, `
SELECT substr(pr.closed_at, 1, 10) AS day, COUNT(*)
FROM pull_requests pr JOIN repositories r ON r.id = pr.repo_id `+join+`
WHERE r.deleted_at IS NULL
  AND pr.closed_at IS NOT NULL AND pr.merged_at IS NULL AND pr.closed_at >= ?
GROUP BY day`, args, sinceStr)
	if err != nil {
		return nil, err
	}
	for _, d := range days {
		out.PRsByDay = append(out.PRsByDay, models.DayPRBucket{
			Day:    d,
			Opened: prOpened[d],
			Merged: prMerged[d],
			Closed: prClosed[d],
		})
	}

	out.PRCIStates, err = s.statsCountBuckets(ctx, `
SELECT COALESCE(NULLIF(pr.ci_state, ''), 'unknown') AS k, COUNT(*)
FROM pull_requests pr JOIN repositories r ON r.id = pr.repo_id `+join+`
WHERE r.deleted_at IS NULL AND pr.state = 'open'
GROUP BY k
ORDER BY COUNT(*) DESC, k`, args)
	if err != nil {
		return nil, err
	}

	out.AttentionBySeverity, err = s.statsCountBuckets(ctx, `
SELECT a.severity AS k, COUNT(*)
FROM attention_items a JOIN repositories r ON r.id = a.repo_id `+join+`
WHERE r.deleted_at IS NULL AND a.resolved_at IS NULL
GROUP BY k
ORDER BY COUNT(*) DESC, k`, args)
	if err != nil {
		return nil, err
	}

	out.AttentionByType, err = s.statsCountBuckets(ctx, `
SELECT a.type AS k, COUNT(*)
FROM attention_items a JOIN repositories r ON r.id = a.repo_id `+join+`
WHERE r.deleted_at IS NULL AND a.resolved_at IS NULL
GROUP BY k
ORDER BY COUNT(*) DESC, k`, args)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (s *Store) statsRunsByDay(ctx context.Context, join string, baseArgs []any, sinceStr string) (map[string]models.DayRunBucket, error) {
	q := `
SELECT substr(COALESCE(wr.completed_at, wr.started_at), 1, 10) AS day,
  SUM(CASE WHEN wr.conclusion = 'success' THEN 1 ELSE 0 END),
  SUM(CASE WHEN wr.conclusion = 'failure' THEN 1 ELSE 0 END),
  SUM(CASE WHEN wr.conclusion = 'cancelled' THEN 1 ELSE 0 END),
  SUM(CASE WHEN wr.conclusion NOT IN ('success', 'failure', 'cancelled') THEN 1 ELSE 0 END)
FROM workflow_runs wr JOIN repositories r ON r.id = wr.repo_id ` + join + `
WHERE r.deleted_at IS NULL
  AND COALESCE(wr.completed_at, wr.started_at) IS NOT NULL
  AND COALESCE(wr.completed_at, wr.started_at) >= ?
GROUP BY day`
	args := append(append([]any{}, baseArgs...), sinceStr)
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.DayRunBucket{}
	for rows.Next() {
		var day string
		var b models.DayRunBucket
		if err := rows.Scan(&day, &b.Success, &b.Failure, &b.Cancelled, &b.Other); err != nil {
			return nil, err
		}
		b.Day = day
		out[day] = b
	}
	return out, rows.Err()
}

func (s *Store) statsDayCounts(ctx context.Context, query string, baseArgs []any, sinceStr string) (map[string]int, error) {
	args := append(append([]any{}, baseArgs...), sinceStr)
	rows, err := s.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var day string
		var n int
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		out[day] = n
	}
	return out, rows.Err()
}

func (s *Store) statsCountBuckets(ctx context.Context, query string, baseArgs []any, extra ...any) ([]models.CountBucket, error) {
	args := append(append([]any{}, baseArgs...), extra...)
	rows, err := s.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CountBucket{}
	for rows.Next() {
		var b models.CountBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) statsRunDuration(ctx context.Context, join string, baseArgs []any, sinceStr string) (*models.DurationStats, error) {
	q := `
SELECT wr.started_at, wr.completed_at
FROM workflow_runs wr JOIN repositories r ON r.id = wr.repo_id ` + join + `
WHERE r.deleted_at IS NULL
  AND wr.started_at IS NOT NULL AND wr.completed_at IS NOT NULL
  AND wr.status = 'completed'
  AND wr.completed_at >= ?`
	args := append(append([]any{}, baseArgs...), sinceStr)
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var secs []float64
	for rows.Next() {
		var startedRaw, completedRaw string
		if err := rows.Scan(&startedRaw, &completedRaw); err != nil {
			return nil, err
		}
		started, err := parseTime(startedRaw)
		if err != nil {
			continue
		}
		completed, err := parseTime(completedRaw)
		if err != nil {
			continue
		}
		d := completed.Sub(started).Seconds()
		if d < 0 {
			continue
		}
		secs = append(secs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(secs) == 0 {
		return nil, nil
	}
	sort.Float64s(secs)
	return &models.DurationStats{
		P50Seconds:  percentile(secs, 0.50),
		P95Seconds:  percentile(secs, 0.95),
		SampleCount: len(secs),
	}, nil
}

// dayKeys returns inclusive UTC calendar days from since through until.
func dayKeys(since, until time.Time) []string {
	start := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(until.Year(), until.Month(), until.Day(), 0, 0, 0, 0, time.UTC)
	if end.Before(start) {
		return nil
	}
	n := int(end.Sub(start).Hours()/24) + 1
	keys := make([]string, 0, n)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		keys = append(keys, d.Format("2006-01-02"))
	}
	return keys
}

// percentile returns the linear-interpolated value at p in [0,1] for a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	w := idx - float64(lo)
	return sorted[lo]*(1-w) + sorted[hi]*w
}
