-- +goose Up
ALTER TABLE webhook_events ADD COLUMN processing_started_at TEXT;

-- +goose Down
ALTER TABLE webhook_events DROP COLUMN processing_started_at;
