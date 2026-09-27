-- +goose Up
ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS incident_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS incident_webhook_ciphertext TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS wallboard_tokens (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL DEFAULT '',
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS wallboard_tokens_active ON wallboard_tokens (revoked_at, id);

-- +goose Down
DROP INDEX IF EXISTS wallboard_tokens_active;
DROP TABLE IF EXISTS wallboard_tokens;
ALTER TABLE notification_settings DROP COLUMN IF EXISTS incident_webhook_ciphertext;
ALTER TABLE notification_settings DROP COLUMN IF EXISTS incident_enabled;
