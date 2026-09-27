-- +goose Up
-- Incident webhook destination (extend notify) + public wallboard tokens.

ALTER TABLE notification_settings ADD COLUMN incident_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_settings ADD COLUMN incident_webhook_ciphertext TEXT NOT NULL DEFAULT '';

CREATE TABLE wallboard_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL DEFAULT '',
    created_by_user_id INTEGER,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used_at TEXT,
    revoked_at TEXT,
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX wallboard_tokens_active ON wallboard_tokens (revoked_at, id);

-- +goose Down
DROP INDEX IF EXISTS wallboard_tokens_active;
DROP TABLE IF EXISTS wallboard_tokens;
-- SQLite cannot DROP COLUMN portably in older versions; leave incident columns on down.
