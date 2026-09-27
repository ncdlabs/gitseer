package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
)

// AttentionMute is a snooze/mute preventing attention reopen or list display.
// user_id NULL = global (affects evaluate for everyone). Non-null = personal list filter only.
// until_at NULL = until the underlying condition clears (cleared on resolve).
type AttentionMute struct {
	ID          int64      `json:"id"`
	UserID      *int64     `json:"user_id,omitempty"`
	RepoID      *int64     `json:"repo_id,omitempty"`
	RuleType    string     `json:"rule_type"`
	Fingerprint string     `json:"fingerprint"`
	UntilAt     *time.Time `json:"until_at,omitempty"`
	Reason      string     `json:"reason"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   *int64     `json:"created_by,omitempty"`
}

// AttentionRuleOverride maps a rule type to a custom severity.
type AttentionRuleOverride struct {
	RuleType  string    `json:"rule_type"`
	Severity  string    `json:"severity"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MuteMatch describes how to test whether an attention item is muted.
type MuteMatch struct {
	Fingerprint string
	RuleType    string
	RepoID      int64
	// UserID when >0 also matches personal mutes for that user.
	// GlobalOnly restricts to user_id IS NULL (evaluate path).
	UserID     int64
	GlobalOnly bool
}

func (s *Store) InsertAttentionMute(ctx context.Context, m AttentionMute) (*AttentionMute, error) {
	now := formatTime(time.Now().UTC())
	var until any
	if m.UntilAt != nil {
		until = formatTime(m.UntilAt.UTC())
	}
	var userID, repoID, createdBy any
	if m.UserID != nil {
		userID = *m.UserID
	}
	if m.RepoID != nil {
		repoID = *m.RepoID
	}
	if m.CreatedBy != nil {
		createdBy = *m.CreatedBy
	}
	var id int64
	err := s.queryRow(ctx, `
INSERT INTO attention_mutes (user_id, repo_id, rule_type, fingerprint, until_at, reason, created_at, created_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id
`, userID, repoID, strings.TrimSpace(m.RuleType), strings.TrimSpace(m.Fingerprint), until, strings.TrimSpace(m.Reason), now, createdBy).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetAttentionMute(ctx, id)
}

func (s *Store) GetAttentionMute(ctx context.Context, id int64) (*AttentionMute, error) {
	row := s.queryRow(ctx, `
SELECT id, user_id, repo_id, rule_type, fingerprint, until_at, reason, created_at, created_by
FROM attention_mutes WHERE id=?`, id)
	return scanAttentionMute(row)
}

func (s *Store) DeleteAttentionMute(ctx context.Context, id int64) error {
	_, err := s.exec(ctx, `DELETE FROM attention_mutes WHERE id=?`, id)
	return err
}

// DeleteAttentionMutesByFingerprint removes mutes for a fingerprint.
// userID > 0 deletes only that user's personal mutes.
// userID == 0 deletes all mutes for the fingerprint (admin unmute).
// globalOnly (with userID==0) deletes only global (user_id IS NULL) mutes.
func (s *Store) DeleteAttentionMutesByFingerprint(ctx context.Context, fingerprint string, userID int64, globalOnly bool) error {
	fp := strings.TrimSpace(fingerprint)
	if fp == "" {
		return fmt.Errorf("fingerprint required")
	}
	if userID > 0 {
		_, err := s.exec(ctx, `DELETE FROM attention_mutes WHERE fingerprint=? AND user_id=?`, fp, userID)
		return err
	}
	if globalOnly {
		_, err := s.exec(ctx, `DELETE FROM attention_mutes WHERE fingerprint=? AND user_id IS NULL`, fp)
		return err
	}
	_, err := s.exec(ctx, `DELETE FROM attention_mutes WHERE fingerprint=?`, fp)
	return err
}

func (s *Store) ClearUntilResolvedMutes(ctx context.Context, fingerprint string) error {
	fp := strings.TrimSpace(fingerprint)
	if fp == "" {
		return nil
	}
	return s.clearUntilResolvedMutesSQLC(ctx, fp)
}

// IsAttentionMuted reports whether a matching active mute exists.
func (s *Store) IsAttentionMuted(ctx context.Context, match MuteMatch) (bool, error) {
	now := formatTime(time.Now().UTC())
	where := []string{"(until_at IS NULL OR until_at > ?)"}
	args := []any{now}

	fp := strings.TrimSpace(match.Fingerprint)
	rule := strings.TrimSpace(match.RuleType)
	parts := []string{}
	if fp != "" {
		parts = append(parts, "(fingerprint != '' AND fingerprint = ?)")
		args = append(args, fp)
	}
	if rule != "" {
		parts = append(parts, `(fingerprint = '' AND rule_type = ? AND (repo_id IS NULL OR repo_id = ?))`)
		args = append(args, rule, match.RepoID)
	}
	if len(parts) == 0 {
		return false, nil
	}
	where = append(where, "("+strings.Join(parts, " OR ")+")")

	if match.GlobalOnly {
		where = append(where, "user_id IS NULL")
	} else if match.UserID > 0 {
		where = append(where, "(user_id IS NULL OR user_id = ?)")
		args = append(args, match.UserID)
	} else {
		where = append(where, "user_id IS NULL")
	}

	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM attention_mutes WHERE `+strings.Join(where, " AND "), args...).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Store) GetAttentionByID(ctx context.Context, id int64) (*models.AttentionItem, error) {
	row := s.queryRow(ctx, `
SELECT a.id, a.instance_id, a.repo_id, a.type, a.severity, a.entity_type, a.entity_id, a.title, a.metadata_json,
       a.fingerprint, a.opened_at, a.resolved_at, a.updated_at, r.owner, r.name, r.full_name,
       `+attentionHTMLURLExpr+`,
       COALESCE(NULLIF(TRIM(i.forge_type), ''), 'gitea'), COALESCE(i.name, '')
FROM attention_items a
JOIN repositories r ON r.id = a.repo_id
LEFT JOIN instances i ON i.id = a.instance_id
`+attentionEntityJoins+`
WHERE a.id=?`, id)
	return scanAttention(row)
}

func (s *Store) ListAttentionRuleOverrides(ctx context.Context) ([]AttentionRuleOverride, error) {
	rows, err := s.query(ctx, `
SELECT rule_type, severity, updated_at FROM attention_rule_overrides ORDER BY rule_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AttentionRuleOverride
	for rows.Next() {
		var o AttentionRuleOverride
		var updated sql.NullString
		if err := rows.Scan(&o.RuleType, &o.Severity, &updated); err != nil {
			return nil, err
		}
		if t := nullTime(updated); t != nil {
			o.UpdatedAt = *t
		}
		out = append(out, o)
	}
	if out == nil {
		out = []AttentionRuleOverride{}
	}
	return out, rows.Err()
}

// MapAttentionRuleOverrides returns rule_type → severity.
func (s *Store) MapAttentionRuleOverrides(ctx context.Context) (map[string]string, error) {
	list, err := s.ListAttentionRuleOverrides(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(list))
	for _, o := range list {
		out[o.RuleType] = o.Severity
	}
	return out, nil
}

func (s *Store) UpsertAttentionRuleOverride(ctx context.Context, ruleType, severity string) error {
	ruleType = strings.TrimSpace(ruleType)
	severity = strings.TrimSpace(severity)
	if ruleType == "" || severity == "" {
		return fmt.Errorf("rule_type and severity required")
	}
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO attention_rule_overrides (rule_type, severity, updated_at) VALUES (?, ?, ?)
ON CONFLICT(rule_type) DO UPDATE SET severity=excluded.severity, updated_at=excluded.updated_at
`, ruleType, severity, now)
	return err
}

func (s *Store) DeleteAttentionRuleOverride(ctx context.Context, ruleType string) error {
	_, err := s.exec(ctx, `DELETE FROM attention_rule_overrides WHERE rule_type=?`, strings.TrimSpace(ruleType))
	return err
}

// ReplaceAttentionRuleOverrides replaces all overrides with the given set (empty clears).
func (s *Store) ReplaceAttentionRuleOverrides(ctx context.Context, overrides []AttentionRuleOverride) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.sql(`DELETE FROM attention_rule_overrides`)); err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())
	for _, o := range overrides {
		rt := strings.TrimSpace(o.RuleType)
		sev := strings.TrimSpace(o.Severity)
		if rt == "" || sev == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, s.sql(`
INSERT INTO attention_rule_overrides (rule_type, severity, updated_at) VALUES (?, ?, ?)
`), rt, sev, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanAttentionMute(row scanner) (*AttentionMute, error) {
	var m AttentionMute
	var userID, repoID, createdBy sql.NullInt64
	var until, created sql.NullString
	if err := row.Scan(
		&m.ID, &userID, &repoID, &m.RuleType, &m.Fingerprint, &until, &m.Reason, &created, &createdBy,
	); err != nil {
		return nil, err
	}
	if userID.Valid {
		v := userID.Int64
		m.UserID = &v
	}
	if repoID.Valid {
		v := repoID.Int64
		m.RepoID = &v
	}
	if createdBy.Valid {
		v := createdBy.Int64
		m.CreatedBy = &v
	}
	m.UntilAt = nullTime(until)
	if t := nullTime(created); t != nil {
		m.CreatedAt = *t
	}
	return &m, nil
}
