package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// UserAlertPrefs is per-user browser / Web Push alert configuration.
type UserAlertPrefs struct {
	UserID          int64
	BrowserEnabled  bool
	PushEnabled     bool
	MinSeverity     string
	UpdatedAt       time.Time
}

// WebPushSubscription is one browser push endpoint for a user.
type WebPushSubscription struct {
	ID        int64
	UserID    int64
	Endpoint  string
	P256dh    string
	Auth      string
	UserAgent string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GetUserAlertPrefs returns prefs for a user, or defaults when no row exists.
func (s *Store) GetUserAlertPrefs(ctx context.Context, userID int64) (*UserAlertPrefs, error) {
	row := s.queryRow(ctx, `
SELECT user_id, browser_enabled, push_enabled, min_severity, updated_at
FROM user_alert_prefs WHERE user_id=?`, userID)
	var p UserAlertPrefs
	var browser, push int
	var updated string
	err := row.Scan(&p.UserID, &browser, &push, &p.MinSeverity, &updated)
	if err == sql.ErrNoRows {
		return &UserAlertPrefs{
			UserID:      userID,
			MinSeverity: "critical",
		}, nil
	}
	if err != nil {
		return nil, err
	}
	p.BrowserEnabled = browser != 0
	p.PushEnabled = push != 0
	if p.MinSeverity == "" {
		p.MinSeverity = "critical"
	}
	if t, e := time.Parse(time.RFC3339Nano, updated); e == nil {
		p.UpdatedAt = t
	}
	return &p, nil
}

// UpsertUserAlertPrefs creates or updates per-user alert prefs.
func (s *Store) UpsertUserAlertPrefs(ctx context.Context, in UserAlertPrefs) (*UserAlertPrefs, error) {
	if in.UserID <= 0 {
		return nil, fmt.Errorf("user_id required")
	}
	sev := strings.ToLower(strings.TrimSpace(in.MinSeverity))
	switch sev {
	case "critical", "warning", "waiting":
	default:
		sev = "critical"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	browser, push := 0, 0
	if in.BrowserEnabled {
		browser = 1
	}
	if in.PushEnabled {
		push = 1
	}
	_, err := s.exec(ctx, `
INSERT INTO user_alert_prefs (user_id, browser_enabled, push_enabled, min_severity, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
  browser_enabled=excluded.browser_enabled,
  push_enabled=excluded.push_enabled,
  min_severity=excluded.min_severity,
  updated_at=excluded.updated_at`,
		in.UserID, browser, push, sev, now)
	if err != nil {
		return nil, err
	}
	return s.GetUserAlertPrefs(ctx, in.UserID)
}

// UpsertWebPushSubscription registers or refreshes a push subscription for a user.
func (s *Store) UpsertWebPushSubscription(ctx context.Context, userID int64, endpoint, p256dh, auth, userAgent string) (*WebPushSubscription, error) {
	endpoint = strings.TrimSpace(endpoint)
	p256dh = strings.TrimSpace(p256dh)
	auth = strings.TrimSpace(auth)
	if userID <= 0 || endpoint == "" || p256dh == "" || auth == "" {
		return nil, fmt.Errorf("subscription fields required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.exec(ctx, `
INSERT INTO web_push_subscriptions (user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(endpoint) DO UPDATE SET
  user_id=excluded.user_id,
  p256dh=excluded.p256dh,
  auth=excluded.auth,
  user_agent=excluded.user_agent,
  updated_at=excluded.updated_at`,
		userID, endpoint, p256dh, auth, strings.TrimSpace(userAgent), now, now)
	if err != nil {
		return nil, err
	}
	return s.getWebPushSubscriptionByEndpoint(ctx, endpoint)
}

func (s *Store) getWebPushSubscriptionByEndpoint(ctx context.Context, endpoint string) (*WebPushSubscription, error) {
	row := s.queryRow(ctx, `
SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at
FROM web_push_subscriptions WHERE endpoint=?`, endpoint)
	return scanWebPushSubscription(row)
}

// ListWebPushSubscriptions returns all push endpoints for a user.
func (s *Store) ListWebPushSubscriptions(ctx context.Context, userID int64) ([]WebPushSubscription, error) {
	rows, err := s.query(ctx, `
SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at
FROM web_push_subscriptions WHERE user_id=? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebPushSubscription
	for rows.Next() {
		sub, err := scanWebPushSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sub)
	}
	return out, rows.Err()
}

// DeleteWebPushSubscriptionByEndpoint removes a stale or unsubscribed endpoint.
func (s *Store) DeleteWebPushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	_, err := s.exec(ctx, `DELETE FROM web_push_subscriptions WHERE endpoint=?`, strings.TrimSpace(endpoint))
	return err
}

// DeleteWebPushSubscriptionForUser removes an endpoint only if it belongs to the user.
func (s *Store) DeleteWebPushSubscriptionForUser(ctx context.Context, userID int64, endpoint string) error {
	_, err := s.exec(ctx, `DELETE FROM web_push_subscriptions WHERE user_id=? AND endpoint=?`, userID, strings.TrimSpace(endpoint))
	return err
}

// ListUserIDsForRepoAlert returns bootstrap admins plus users with ACL on the repo.
func (s *Store) ListUserIDsForRepoAlert(ctx context.Context, repoID int64) ([]int64, error) {
	rows, err := s.query(ctx, `
SELECT id FROM users WHERE is_bootstrap_admin != 0
UNION
SELECT user_id FROM user_repository_access WHERE repo_id=?`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	seen := map[int64]struct{}{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanWebPushSubscription(row scannable) (*WebPushSubscription, error) {
	var sub WebPushSubscription
	var created, updated string
	if err := row.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &sub.UserAgent, &created, &updated); err != nil {
		return nil, err
	}
	if t, e := time.Parse(time.RFC3339Nano, created); e == nil {
		sub.CreatedAt = t
	}
	if t, e := time.Parse(time.RFC3339Nano, updated); e == nil {
		sub.UpdatedAt = t
	}
	return &sub, nil
}
