-- +goose Up
CREATE TABLE saved_filters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    query_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX saved_filters_user ON saved_filters (user_id);
CREATE UNIQUE INDEX saved_filters_user_name ON saved_filters (user_id, name);

-- +goose Down
DROP INDEX IF EXISTS saved_filters_user_name;
DROP INDEX IF EXISTS saved_filters_user;
DROP TABLE IF EXISTS saved_filters;
