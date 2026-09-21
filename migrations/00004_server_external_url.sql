-- +goose Up
ALTER TABLE app_settings ADD COLUMN server_external_url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE app_settings DROP COLUMN server_external_url;
