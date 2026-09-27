-- +goose Up
ALTER TABLE instances ADD COLUMN IF NOT EXISTS webhook_verified_at TEXT;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS webhook_ensure_at TEXT;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS webhook_ensure_error TEXT;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS webhook_verify_token TEXT;

-- +goose Down
ALTER TABLE instances DROP COLUMN IF EXISTS webhook_verify_token;
ALTER TABLE instances DROP COLUMN IF EXISTS webhook_ensure_error;
ALTER TABLE instances DROP COLUMN IF EXISTS webhook_ensure_at;
ALTER TABLE instances DROP COLUMN IF EXISTS webhook_verified_at;
