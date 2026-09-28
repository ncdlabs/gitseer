-- +goose Up
-- Bootstrap claim (username + password hash), keep-after-setup policy, session elevation.

ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS bootstrap_username TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS bootstrap_password_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS bootstrap_keep_after_setup INTEGER NOT NULL DEFAULT 1;

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS bootstrap_elevated_until TEXT;

-- +goose Down
ALTER TABLE sessions DROP COLUMN IF EXISTS bootstrap_elevated_until;
ALTER TABLE app_settings DROP COLUMN IF EXISTS bootstrap_keep_after_setup;
ALTER TABLE app_settings DROP COLUMN IF EXISTS bootstrap_password_hash;
ALTER TABLE app_settings DROP COLUMN IF EXISTS bootstrap_username;
