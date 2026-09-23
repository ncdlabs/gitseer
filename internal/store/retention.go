package store

import (
	"context"
	"fmt"
	"time"
)

// MapExternalIDsToRepoIDs returns lens repo IDs for the given forge external IDs.
func (s *Store) MapExternalIDsToRepoIDs(ctx context.Context, instanceID int64, externalIDs []int64) ([]int64, error) {
	if len(externalIDs) == 0 {
		return nil, nil
	}
	ph := make([]string, len(externalIDs))
	args := []any{instanceID}
	for i, id := range externalIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	rows, err := s.query(ctx, fmt.Sprintf(`
SELECT id FROM repositories
WHERE instance_id = ? AND deleted_at IS NULL AND external_id IN (%s)`, joinComma(ph)), args...)
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

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += "," + parts[i]
	}
	return out
}

// PurgeRetention deletes aged rows per configured retention windows (days <= 0 skips).
func (s *Store) PurgeRetention(ctx context.Context, runsDays, webhooksDays, attentionDays int) (map[string]int64, error) {
	out := map[string]int64{}
	now := time.Now().UTC()
	if runsDays > 0 {
		cutoff := formatTime(now.AddDate(0, 0, -runsDays))
		res, err := s.exec(ctx, `DELETE FROM workflow_runs WHERE COALESCE(completed_at, started_at) < ?`, cutoff)
		if err != nil {
			return out, err
		}
		out["workflow_runs"], _ = res.RowsAffected()
	}
	if webhooksDays > 0 {
		cutoff := formatTime(now.AddDate(0, 0, -webhooksDays))
		// Never delete pending; keep processing younger than the reaper window so a
		// crash mid-claim can still be reset to pending.
		reaperCutoff := formatTime(now.Add(-DefaultWebhookReaperAge))
		res, err := s.exec(ctx, `
DELETE FROM webhook_events
WHERE received_at < ?
  AND status != 'pending'
  AND NOT (status = 'processing' AND (
    processing_started_at IS NULL OR processing_started_at >= ?
  ))`, cutoff, reaperCutoff)
		if err != nil {
			return out, err
		}
		out["webhook_events"], _ = res.RowsAffected()
	}
	if attentionDays > 0 {
		cutoff := formatTime(now.AddDate(0, 0, -attentionDays))
		res, err := s.exec(ctx, `DELETE FROM attention_items WHERE resolved_at IS NOT NULL AND resolved_at < ?`, cutoff)
		if err != nil {
			return out, err
		}
		out["attention_items"], _ = res.RowsAffected()
	}
	return out, nil
}
