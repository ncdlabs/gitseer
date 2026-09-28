-- +goose Up
-- Per-user browser/OS alert prefs + Web Push subscriptions (self-hosted VAPID).

CREATE TABLE user_alert_prefs (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    browser_enabled INTEGER NOT NULL DEFAULT 0,
    push_enabled INTEGER NOT NULL DEFAULT 0,
    min_severity TEXT NOT NULL DEFAULT 'critical',
    updated_at TEXT NOT NULL DEFAULT (to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'))
);

CREATE TABLE web_push_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')),
    updated_at TEXT NOT NULL DEFAULT (to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'))
);

CREATE UNIQUE INDEX web_push_subscriptions_endpoint ON web_push_subscriptions (endpoint);
CREATE INDEX web_push_subscriptions_user ON web_push_subscriptions (user_id);

-- +goose Down
DROP INDEX IF EXISTS web_push_subscriptions_user;
DROP INDEX IF EXISTS web_push_subscriptions_endpoint;
DROP TABLE IF EXISTS web_push_subscriptions;
DROP TABLE IF EXISTS user_alert_prefs;
