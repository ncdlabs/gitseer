package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
)

// Notification channel and kind constants.
const (
	NotifyChannelSMTP     = "smtp"
	NotifyChannelSlack    = "slack"
	NotifyChannelDiscord  = "discord"
	NotifyChannelWebhook  = "webhook"
	NotifyChannelIncident = "incident"

	NotifyKindImmediate = "immediate"
	NotifyKindDigest    = "digest"
	NotifyKindTest      = "test"

	NotifyStatusPending    = "pending"
	NotifyStatusProcessing = "processing"
	NotifyStatusSent       = "sent"
	NotifyStatusError      = "error"
	NotifyStatusDead       = "dead"

	MaxNotifyAttempts = 5
)

// NotificationSettings is the single-row outbound notification config (secrets as ciphertext).
type NotificationSettings struct {
	Enabled           bool
	MinSeverity       string
	ImmediateEnabled  bool
	DigestEnabled     bool
	DigestHourUTC     int
	LastDigestAt      *time.Time

	SMTPEnabled            bool
	SMTPHost               string
	SMTPPort               int
	SMTPTLSMode            string
	SMTPFrom               string
	SMTPTo                 string
	SMTPUsername           string
	SMTPPasswordCiphertext string

	SlackEnabled           bool
	SlackWebhookCiphertext string

	DiscordEnabled           bool
	DiscordWebhookCiphertext string

	WebhookEnabled      bool
	WebhookURLCiphertext string

	IncidentEnabled           bool
	IncidentWebhookCiphertext string

	UpdatedAt time.Time
}

// NotificationOutboxRow is a queued delivery attempt.
type NotificationOutboxRow struct {
	ID                   int64
	Channel              string
	Kind                 string
	PayloadJSON          string
	Status               string
	Attempts             int
	NextAttemptAt        *time.Time
	LastError            string
	DedupeKey            string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	SentAt               *time.Time
	ProcessingStartedAt  *time.Time
}

// GetNotificationSettings returns the singleton settings row (creates defaults if missing).
func (s *Store) GetNotificationSettings(ctx context.Context) (*NotificationSettings, error) {
	row := s.queryRow(ctx, `
SELECT enabled, min_severity, immediate_enabled, digest_enabled, digest_hour_utc, last_digest_at,
       smtp_enabled, smtp_host, smtp_port, smtp_tls_mode, smtp_from, smtp_to, smtp_username, smtp_password_ciphertext,
       slack_enabled, slack_webhook_ciphertext,
       discord_enabled, discord_webhook_ciphertext,
       webhook_enabled, webhook_url_ciphertext,
       incident_enabled, incident_webhook_ciphertext,
       updated_at
FROM notification_settings WHERE id=1`)
	ns, err := scanNotificationSettings(row)
	if err == sql.ErrNoRows {
		if _, err := s.exec(ctx, `INSERT INTO notification_settings (id) VALUES (1)`); err != nil && !isUniqueViolation(err) {
			return nil, err
		}
		return s.GetNotificationSettings(ctx)
	}
	return ns, err
}

// UpsertNotificationSettings writes the singleton settings row.
func (s *Store) UpsertNotificationSettings(ctx context.Context, in NotificationSettings) error {
	now := formatTime(time.Now().UTC())
	minSev := strings.TrimSpace(strings.ToLower(in.MinSeverity))
	if minSev == "" {
		minSev = "critical"
	}
	tlsMode := strings.TrimSpace(strings.ToLower(in.SMTPTLSMode))
	if tlsMode == "" {
		tlsMode = "starttls"
	}
	port := in.SMTPPort
	if port <= 0 {
		port = 587
	}
	hour := in.DigestHourUTC
	if hour < 0 || hour > 23 {
		hour = 14
	}
	_, err := s.exec(ctx, `
INSERT INTO notification_settings (
  id, enabled, min_severity, immediate_enabled, digest_enabled, digest_hour_utc, last_digest_at,
  smtp_enabled, smtp_host, smtp_port, smtp_tls_mode, smtp_from, smtp_to, smtp_username, smtp_password_ciphertext,
  slack_enabled, slack_webhook_ciphertext,
  discord_enabled, discord_webhook_ciphertext,
  webhook_enabled, webhook_url_ciphertext,
  incident_enabled, incident_webhook_ciphertext,
  updated_at
) VALUES (
  1, ?, ?, ?, ?, ?, ?,
  ?, ?, ?, ?, ?, ?, ?, ?,
  ?, ?,
  ?, ?,
  ?, ?,
  ?, ?,
  ?
)
ON CONFLICT(id) DO UPDATE SET
  enabled=excluded.enabled,
  min_severity=excluded.min_severity,
  immediate_enabled=excluded.immediate_enabled,
  digest_enabled=excluded.digest_enabled,
  digest_hour_utc=excluded.digest_hour_utc,
  last_digest_at=excluded.last_digest_at,
  smtp_enabled=excluded.smtp_enabled,
  smtp_host=excluded.smtp_host,
  smtp_port=excluded.smtp_port,
  smtp_tls_mode=excluded.smtp_tls_mode,
  smtp_from=excluded.smtp_from,
  smtp_to=excluded.smtp_to,
  smtp_username=excluded.smtp_username,
  smtp_password_ciphertext=excluded.smtp_password_ciphertext,
  slack_enabled=excluded.slack_enabled,
  slack_webhook_ciphertext=excluded.slack_webhook_ciphertext,
  discord_enabled=excluded.discord_enabled,
  discord_webhook_ciphertext=excluded.discord_webhook_ciphertext,
  webhook_enabled=excluded.webhook_enabled,
  webhook_url_ciphertext=excluded.webhook_url_ciphertext,
  incident_enabled=excluded.incident_enabled,
  incident_webhook_ciphertext=excluded.incident_webhook_ciphertext,
  updated_at=excluded.updated_at
`,
		boolToInt(in.Enabled), minSev, boolToInt(in.ImmediateEnabled), boolToInt(in.DigestEnabled), hour, formatTimePtr(in.LastDigestAt),
		boolToInt(in.SMTPEnabled), strings.TrimSpace(in.SMTPHost), port, tlsMode, strings.TrimSpace(in.SMTPFrom), strings.TrimSpace(in.SMTPTo), strings.TrimSpace(in.SMTPUsername), in.SMTPPasswordCiphertext,
		boolToInt(in.SlackEnabled), in.SlackWebhookCiphertext,
		boolToInt(in.DiscordEnabled), in.DiscordWebhookCiphertext,
		boolToInt(in.WebhookEnabled), in.WebhookURLCiphertext,
		boolToInt(in.IncidentEnabled), in.IncidentWebhookCiphertext,
		now,
	)
	return err
}

// MarkNotificationDigestSent records last_digest_at.
func (s *Store) MarkNotificationDigestSent(ctx context.Context, at time.Time) error {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
UPDATE notification_settings SET last_digest_at=?, updated_at=? WHERE id=1`,
		formatTime(at.UTC()), now)
	return err
}

func scanNotificationSettings(row scanner) (*NotificationSettings, error) {
	var ns NotificationSettings
	var enabled, imm, dig, smtpEn, slackEn, discEn, whEn, incEn int
	var lastDigest, updated sql.NullString
	var minSev, host, tlsMode, from, to, user, smtpCipher, slackCipher, discCipher, whCipher, incCipher sql.NullString
	var port sql.NullInt64
	var hour sql.NullInt64
	if err := row.Scan(
		&enabled, &minSev, &imm, &dig, &hour, &lastDigest,
		&smtpEn, &host, &port, &tlsMode, &from, &to, &user, &smtpCipher,
		&slackEn, &slackCipher,
		&discEn, &discCipher,
		&whEn, &whCipher,
		&incEn, &incCipher,
		&updated,
	); err != nil {
		return nil, err
	}
	ns.Enabled = enabled != 0
	ns.MinSeverity = minSev.String
	if ns.MinSeverity == "" {
		ns.MinSeverity = "critical"
	}
	ns.ImmediateEnabled = imm != 0
	ns.DigestEnabled = dig != 0
	if hour.Valid {
		ns.DigestHourUTC = int(hour.Int64)
	} else {
		ns.DigestHourUTC = 14
	}
	ns.LastDigestAt = nullTime(lastDigest)
	ns.SMTPEnabled = smtpEn != 0
	ns.SMTPHost = host.String
	if port.Valid {
		ns.SMTPPort = int(port.Int64)
	} else {
		ns.SMTPPort = 587
	}
	ns.SMTPTLSMode = tlsMode.String
	if ns.SMTPTLSMode == "" {
		ns.SMTPTLSMode = "starttls"
	}
	ns.SMTPFrom = from.String
	ns.SMTPTo = to.String
	ns.SMTPUsername = user.String
	ns.SMTPPasswordCiphertext = smtpCipher.String
	ns.SlackEnabled = slackEn != 0
	ns.SlackWebhookCiphertext = slackCipher.String
	ns.DiscordEnabled = discEn != 0
	ns.DiscordWebhookCiphertext = discCipher.String
	ns.WebhookEnabled = whEn != 0
	ns.WebhookURLCiphertext = whCipher.String
	ns.IncidentEnabled = incEn != 0
	ns.IncidentWebhookCiphertext = incCipher.String
	if t := nullTime(updated); t != nil {
		ns.UpdatedAt = *t
	}
	return &ns, nil
}

// GetAttentionByFingerprint returns an attention item by fingerprint (including resolved).
func (s *Store) GetAttentionByFingerprint(ctx context.Context, fingerprint string) (*models.AttentionItem, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil, sql.ErrNoRows
	}
	row := s.queryRow(ctx, `
SELECT a.id, a.instance_id, a.repo_id, a.type, a.severity, a.entity_type, a.entity_id, a.title, a.metadata_json,
       a.fingerprint, a.opened_at, a.resolved_at, a.updated_at, r.owner, r.name, r.full_name,
       `+attentionHTMLURLExpr+`,
       COALESCE(NULLIF(TRIM(i.forge_type), ''), 'gitea'), COALESCE(i.name, '')
FROM attention_items a
JOIN repositories r ON r.id = a.repo_id
LEFT JOIN instances i ON i.id = a.instance_id
`+attentionEntityJoins+`
WHERE a.fingerprint=?`, fingerprint)
	return scanAttention(row)
}

// ListOpenAttentionForNotify returns open attention items at or above minSeverity (bootstrap scope).
func (s *Store) ListOpenAttentionForNotify(ctx context.Context, minSeverity string, limit int) ([]models.AttentionItem, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	minRank := severityRank(minSeverity)
	rows, err := s.query(ctx, `
SELECT a.id, a.instance_id, a.repo_id, a.type, a.severity, a.entity_type, a.entity_id, a.title, a.metadata_json,
       a.fingerprint, a.opened_at, a.resolved_at, a.updated_at, r.owner, r.name, r.full_name,
       `+attentionHTMLURLExpr+`,
       COALESCE(NULLIF(TRIM(i.forge_type), ''), 'gitea'), COALESCE(i.name, '')
FROM attention_items a
JOIN repositories r ON r.id = a.repo_id
LEFT JOIN instances i ON i.id = a.instance_id
`+attentionEntityJoins+`
WHERE a.resolved_at IS NULL AND r.deleted_at IS NULL
ORDER BY a.opened_at DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AttentionItem
	for rows.Next() {
		item, err := scanAttention(rows)
		if err != nil {
			return nil, err
		}
		if severityRank(item.Severity) < minRank {
			continue
		}
		out = append(out, *item)
	}
	if out == nil {
		out = []models.AttentionItem{}
	}
	return out, rows.Err()
}

func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 3
	case "warning":
		return 2
	case "waiting":
		return 1
	default:
		return 0
	}
}

// SeverityMeetsMin reports whether itemSeverity is at least minSeverity.
func SeverityMeetsMin(itemSeverity, minSeverity string) bool {
	return severityRank(itemSeverity) >= severityRank(minSeverity)
}

// EnqueueNotification inserts an outbox row. Duplicate dedupe_key is ignored (returns nil, nil).
func (s *Store) EnqueueNotification(ctx context.Context, channel, kind, payloadJSON, dedupeKey string) (*NotificationOutboxRow, error) {
	channel = strings.TrimSpace(channel)
	kind = strings.TrimSpace(kind)
	if channel == "" || kind == "" {
		return nil, fmt.Errorf("channel and kind required")
	}
	if payloadJSON == "" {
		payloadJSON = "{}"
	}
	now := formatTime(time.Now().UTC())
	var id int64
	err := s.queryRow(ctx, `
INSERT INTO notification_outbox (channel, kind, payload_json, status, attempts, next_attempt_at, dedupe_key, created_at, updated_at)
VALUES (?, ?, ?, 'pending', 0, ?, ?, ?, ?)
RETURNING id
`, channel, kind, payloadJSON, now, strings.TrimSpace(dedupeKey), now, now).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, nil
		}
		return nil, err
	}
	return s.GetNotificationOutbox(ctx, id)
}

// GetNotificationOutbox loads one outbox row.
func (s *Store) GetNotificationOutbox(ctx context.Context, id int64) (*NotificationOutboxRow, error) {
	row := s.queryRow(ctx, `
SELECT id, channel, kind, payload_json, status, attempts, next_attempt_at, last_error, dedupe_key,
       created_at, updated_at, sent_at, processing_started_at
FROM notification_outbox WHERE id=?`, id)
	return scanOutbox(row)
}

// ClaimNotificationOutbox claims pending/retryable rows for delivery.
func (s *Store) ClaimNotificationOutbox(ctx context.Context, limit int) ([]NotificationOutboxRow, error) {
	if limit <= 0 {
		limit = 10
	}
	now := time.Now().UTC()
	nowStr := formatTime(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	selectSQL := `
SELECT id, channel, kind, payload_json, status, attempts, next_attempt_at, last_error, dedupe_key,
       created_at, updated_at, sent_at, processing_started_at
FROM notification_outbox
WHERE (status='pending' OR status='error')
  AND attempts < ?
  AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
ORDER BY id ASC
LIMIT ?`
	if s.driver == "postgres" {
		selectSQL += `
FOR UPDATE SKIP LOCKED`
	}
	rows, err := tx.QueryContext(ctx, s.sql(selectSQL), MaxNotifyAttempts, nowStr, limit)
	if err != nil {
		return nil, err
	}
	var candidates []NotificationOutboxRow
	for rows.Next() {
		row, err := scanOutbox(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, *row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var out []NotificationOutboxRow
	for _, e := range candidates {
		res, err := tx.ExecContext(ctx, s.sql(`
UPDATE notification_outbox SET status='processing', processing_started_at=?, updated_at=?
WHERE id=? AND (status='pending' OR status='error')`), nowStr, nowStr, e.ID)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			e.Status = NotifyStatusProcessing
			e.ProcessingStartedAt = &now
			e.UpdatedAt = now
			out = append(out, e)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// MarkNotificationSent marks an outbox row delivered.
func (s *Store) MarkNotificationSent(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
UPDATE notification_outbox SET status='sent', sent_at=?, updated_at=?, processing_started_at=NULL, last_error=''
WHERE id=?`, now, now, id)
	return err
}

// MarkNotificationFailed records a delivery failure with backoff (or dead after max attempts).
func (s *Store) MarkNotificationFailed(ctx context.Context, id int64, attempts int, errMsg string) error {
	now := time.Now().UTC()
	nowStr := formatTime(now)
	status := NotifyStatusError
	var next any
	if attempts >= MaxNotifyAttempts {
		status = NotifyStatusDead
		next = nil
	} else {
		backoff := time.Duration(1<<min(attempts, 4)) * time.Minute
		next = formatTime(now.Add(backoff))
	}
	_, err := s.exec(ctx, `
UPDATE notification_outbox SET status=?, attempts=?, next_attempt_at=?, last_error=?, updated_at=?, processing_started_at=NULL
WHERE id=?`, status, attempts, next, truncateErr(errMsg), nowStr, id)
	return err
}

// ReapStaleProcessingNotifications resets stuck processing rows.
func (s *Store) ReapStaleProcessingNotifications(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		olderThan = 5 * time.Minute
	}
	cutoff := formatTime(time.Now().UTC().Add(-olderThan))
	res, err := s.exec(ctx, `
UPDATE notification_outbox SET status='pending', processing_started_at=NULL
WHERE status='processing'
  AND processing_started_at IS NOT NULL
  AND processing_started_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func truncateErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}

func scanOutbox(row scanner) (*NotificationOutboxRow, error) {
	var e NotificationOutboxRow
	var next, created, updated, sent, proc sql.NullString
	if err := row.Scan(
		&e.ID, &e.Channel, &e.Kind, &e.PayloadJSON, &e.Status, &e.Attempts, &next, &e.LastError, &e.DedupeKey,
		&created, &updated, &sent, &proc,
	); err != nil {
		return nil, err
	}
	e.NextAttemptAt = nullTime(next)
	if t := nullTime(created); t != nil {
		e.CreatedAt = *t
	}
	if t := nullTime(updated); t != nil {
		e.UpdatedAt = *t
	}
	e.SentAt = nullTime(sent)
	e.ProcessingStartedAt = nullTime(proc)
	return &e, nil
}
