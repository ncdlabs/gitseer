-- +goose Up
-- Outbound notification settings + delivery outbox (Stream 7).

CREATE TABLE IF NOT EXISTS notification_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0,
    min_severity TEXT NOT NULL DEFAULT 'critical',
    immediate_enabled INTEGER NOT NULL DEFAULT 1,
    digest_enabled INTEGER NOT NULL DEFAULT 0,
    digest_hour_utc INTEGER NOT NULL DEFAULT 14,
    last_digest_at TEXT,

    smtp_enabled INTEGER NOT NULL DEFAULT 0,
    smtp_host TEXT NOT NULL DEFAULT '',
    smtp_port INTEGER NOT NULL DEFAULT 587,
    smtp_tls_mode TEXT NOT NULL DEFAULT 'starttls',
    smtp_from TEXT NOT NULL DEFAULT '',
    smtp_to TEXT NOT NULL DEFAULT '',
    smtp_username TEXT NOT NULL DEFAULT '',
    smtp_password_ciphertext TEXT NOT NULL DEFAULT '',

    slack_enabled INTEGER NOT NULL DEFAULT 0,
    slack_webhook_ciphertext TEXT NOT NULL DEFAULT '',

    discord_enabled INTEGER NOT NULL DEFAULT 0,
    discord_webhook_ciphertext TEXT NOT NULL DEFAULT '',

    webhook_enabled INTEGER NOT NULL DEFAULT 0,
    webhook_url_ciphertext TEXT NOT NULL DEFAULT '',

    updated_at TEXT NOT NULL DEFAULT (to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'))
);

INSERT INTO notification_settings (id) VALUES (1)
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS notification_outbox (
    id BIGSERIAL PRIMARY KEY,
    channel TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    dedupe_key TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')),
    updated_at TEXT NOT NULL DEFAULT (to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')),
    sent_at TEXT,
    processing_started_at TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS notification_outbox_dedupe
  ON notification_outbox (dedupe_key)
  WHERE dedupe_key != '';

CREATE INDEX IF NOT EXISTS notification_outbox_claim
  ON notification_outbox (status, next_attempt_at, id);

-- +goose Down
DROP INDEX IF EXISTS notification_outbox_claim;
DROP INDEX IF EXISTS notification_outbox_dedupe;
DROP TABLE IF EXISTS notification_outbox;
DROP TABLE IF EXISTS notification_settings;
