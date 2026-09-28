-- +goose Up
-- Bootstrap claim (username + password hash), keep-after-setup policy, session elevation.

ALTER TABLE app_settings ADD COLUMN bootstrap_username TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN bootstrap_password_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN bootstrap_keep_after_setup INTEGER NOT NULL DEFAULT 1;

ALTER TABLE sessions ADD COLUMN bootstrap_elevated_until TEXT;

-- +goose Down
-- SQLite cannot DROP COLUMN portably across older versions; leave columns on down.
-- Postgres down is in migrations/postgres/00019_bootstrap_claim.sql.
