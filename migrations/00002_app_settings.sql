-- +goose Up
CREATE TABLE app_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    instance_name TEXT NOT NULL DEFAULT '',
    sync_history_days INTEGER NOT NULL DEFAULT 0,
    attention_long_running_after TEXT NOT NULL DEFAULT '',
    retention_runs_days INTEGER NOT NULL DEFAULT 0,
    retention_webhooks_days INTEGER NOT NULL DEFAULT 0,
    retention_attention_days INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- +goose Down
DROP TABLE IF EXISTS app_settings;
