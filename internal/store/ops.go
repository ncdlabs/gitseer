package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SyncStateRow is the latest sync_state for an instance scope.
type SyncStateRow struct {
	InstanceID    int64
	Scope         string
	Phase         string
	LastSuccessAt *time.Time
	LastError     string
}

// SyncLeaseRow is the current holder of a sync lease (typically instances.id).
type SyncLeaseRow struct {
	ID        int64
	Holder    string
	ExpiresAt time.Time
}

// WebhookStats aggregates webhook_events since a cutoff.
type WebhookStats struct {
	OK          int64
	Failed      int64
	Pending     int64
	Processing  int64
	Total       int64
	LastAt      *time.Time
	LastError   string
	LastStatus  string
	LastEvent   string
}

// GetSyncState returns sync_state for instance+scope (scope_id=0), or nil when missing.
func (s *Store) GetSyncState(ctx context.Context, instanceID int64, scope string) (*SyncStateRow, error) {
	if scope == "" {
		scope = "instance"
	}
	var phase, lastErr string
	var lastSuccess sql.NullString
	err := s.queryRow(ctx, `
SELECT phase, COALESCE(last_error, ''), last_success_at
FROM sync_state
WHERE instance_id = ? AND scope = ? AND scope_id = 0`, instanceID, scope).Scan(&phase, &lastErr, &lastSuccess)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row := &SyncStateRow{InstanceID: instanceID, Scope: scope, Phase: phase, LastError: lastErr}
	if lastSuccess.Valid && lastSuccess.String != "" {
		if t, perr := parseTime(lastSuccess.String); perr == nil {
			row.LastSuccessAt = &t
		}
	}
	return row, nil
}

// GetSyncLease returns the lease row for leaseID, or nil when missing.
func (s *Store) GetSyncLease(ctx context.Context, leaseID int64) (*SyncLeaseRow, error) {
	var holder, expires string
	err := s.queryRow(ctx, `SELECT holder, expires_at FROM sync_leases WHERE id = ?`, leaseID).Scan(&holder, &expires)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	exp, _ := parseTime(expires)
	return &SyncLeaseRow{ID: leaseID, Holder: holder, ExpiresAt: exp}, nil
}

// WebhookStatsSince returns counts and last delivery info for an instance since cutoff.
// When instanceID is 0, aggregates across all instances.
func (s *Store) WebhookStatsSince(ctx context.Context, instanceID int64, since time.Time) (*WebhookStats, error) {
	sinceStr := formatTime(since)
	stats := &WebhookStats{}

	type statusCount struct {
		Status string
		N      int64
	}
	var rows *sql.Rows
	var err error
	if instanceID > 0 {
		rows, err = s.query(ctx, `
SELECT status, COUNT(*) FROM webhook_events
WHERE instance_id = ? AND received_at >= ?
GROUP BY status`, instanceID, sinceStr)
	} else {
		rows, err = s.query(ctx, `
SELECT status, COUNT(*) FROM webhook_events
WHERE received_at >= ?
GROUP BY status`, sinceStr)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sc statusCount
		if err := rows.Scan(&sc.Status, &sc.N); err != nil {
			return nil, err
		}
		stats.Total += sc.N
		switch sc.Status {
		case "processed", "ok":
			stats.OK += sc.N
		case "error", "failed":
			stats.Failed += sc.N
		case "pending":
			stats.Pending += sc.N
		case "processing":
			stats.Processing += sc.N
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var lastAt, lastStatus, lastEvent, lastErr sql.NullString
	if instanceID > 0 {
		err = s.queryRow(ctx, `
SELECT received_at, status, event_type, COALESCE(error, '')
FROM webhook_events
WHERE instance_id = ?
ORDER BY received_at DESC, id DESC LIMIT 1`, instanceID).Scan(&lastAt, &lastStatus, &lastEvent, &lastErr)
	} else {
		err = s.queryRow(ctx, `
SELECT received_at, status, event_type, COALESCE(error, '')
FROM webhook_events
ORDER BY received_at DESC, id DESC LIMIT 1`).Scan(&lastAt, &lastStatus, &lastEvent, &lastErr)
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		if lastAt.Valid && lastAt.String != "" {
			if t, perr := parseTime(lastAt.String); perr == nil {
				stats.LastAt = &t
			}
		}
		if lastStatus.Valid {
			stats.LastStatus = lastStatus.String
		}
		if lastEvent.Valid {
			stats.LastEvent = lastEvent.String
		}
		if lastErr.Valid {
			stats.LastError = lastErr.String
		}
	}
	return stats, nil
}

// CountWebhookEventsByTypeSince counts events whose type matches any of the prefixes since cutoff.
func (s *Store) CountWebhookEventsByTypeSince(ctx context.Context, instanceID int64, since time.Time, typePrefixes []string) (int64, error) {
	if len(typePrefixes) == 0 {
		return 0, nil
	}
	sinceStr := formatTime(since)
	var total int64
	for _, prefix := range typePrefixes {
		var n int64
		var err error
		pat := prefix + "%"
		if instanceID > 0 {
			err = s.queryRow(ctx, `
SELECT COUNT(*) FROM webhook_events
WHERE instance_id = ? AND received_at >= ? AND event_type LIKE ?`, instanceID, sinceStr, pat).Scan(&n)
		} else {
			err = s.queryRow(ctx, `
SELECT COUNT(*) FROM webhook_events
WHERE received_at >= ? AND event_type LIKE ?`, sinceStr, pat).Scan(&n)
		}
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// CountInFlightWorkflowRuns returns queued/waiting/running workflow runs (optionally per instance).
func (s *Store) CountInFlightWorkflowRuns(ctx context.Context, instanceID int64) (int64, error) {
	var n int64
	var err error
	if instanceID > 0 {
		err = s.queryRow(ctx, `
SELECT COUNT(*) FROM workflow_runs wr
JOIN repositories r ON r.id = wr.repo_id
WHERE r.instance_id = ? AND wr.status IN ('queued', 'waiting', 'running')`, instanceID).Scan(&n)
	} else {
		err = s.queryRow(ctx, `
SELECT COUNT(*) FROM workflow_runs
WHERE status IN ('queued', 'waiting', 'running')`).Scan(&n)
	}
	return n, err
}

// SetWebhookEnsureMeta records the last ensure-webhook attempt.
func (s *Store) SetWebhookEnsureMeta(ctx context.Context, instanceID int64, at time.Time, ensureErr string) error {
	_, err := s.exec(ctx, `
UPDATE instances SET webhook_ensure_at = ?, webhook_ensure_error = ?, updated_at = ?
WHERE id = ?`, formatTime(at), ensureErr, formatTime(time.Now().UTC()), instanceID)
	return err
}

// SetWebhookVerifyPending stores a pending verification token (clears verified_at until delivery).
func (s *Store) SetWebhookVerifyPending(ctx context.Context, instanceID int64, token string) error {
	if token == "" {
		return fmt.Errorf("verify token required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
UPDATE instances SET webhook_verify_token = ?, updated_at = ?
WHERE id = ?`, token, now, instanceID)
	return err
}

// MarkWebhookVerifiedIfPending clears a pending verify token and sets webhook_verified_at.
// Returns true when a pending token was consumed.
func (s *Store) MarkWebhookVerifiedIfPending(ctx context.Context, instanceID int64) (bool, error) {
	now := formatTime(time.Now().UTC())
	res, err := s.exec(ctx, `
UPDATE instances SET
  webhook_verified_at = ?,
  webhook_verify_token = '',
  updated_at = ?
WHERE id = ? AND TRIM(COALESCE(webhook_verify_token, '')) != ''`, now, now, instanceID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkWebhookVerifiedForce sets webhook_verified_at without requiring a pending token
// (operator-triggered confirm after sending a forge ping).
func (s *Store) MarkWebhookVerifiedForce(ctx context.Context, instanceID int64) error {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
UPDATE instances SET
  webhook_verified_at = ?,
  webhook_verify_token = '',
  updated_at = ?
WHERE id = ?`, now, now, instanceID)
	return err
}

// ProbeDecryptableSecret returns one stored ciphertext suitable for a decrypt health probe,
// preferring sync token then webhook secret then oauth client secret across instances.
func (s *Store) ProbeDecryptableSecret(ctx context.Context) (ciphertext string, err error) {
	var cipher sql.NullString
	err = s.queryRow(ctx, `
SELECT CASE
  WHEN TRIM(COALESCE(sync_token_ciphertext, '')) != '' THEN sync_token_ciphertext
  WHEN TRIM(COALESCE(webhook_secret_ciphertext, '')) != '' THEN webhook_secret_ciphertext
  WHEN TRIM(COALESCE(oauth_client_secret_ciphertext, '')) != '' THEN oauth_client_secret_ciphertext
  ELSE ''
END
FROM instances
WHERE TRIM(COALESCE(sync_token_ciphertext, '')) != ''
   OR TRIM(COALESCE(webhook_secret_ciphertext, '')) != ''
   OR TRIM(COALESCE(oauth_client_secret_ciphertext, '')) != ''
ORDER BY id ASC LIMIT 1`).Scan(&cipher)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if cipher.Valid {
		return cipher.String, nil
	}
	return "", nil
}
