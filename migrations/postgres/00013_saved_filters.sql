-- +goose Up
CREATE TABLE saved_filters (
    id BIGSERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    query_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')),
    updated_at TEXT NOT NULL DEFAULT (to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'))
);

CREATE INDEX saved_filters_user ON saved_filters (user_id);
CREATE UNIQUE INDEX saved_filters_user_name ON saved_filters (user_id, name);

-- +goose Down
DROP INDEX IF EXISTS saved_filters_user_name;
DROP INDEX IF EXISTS saved_filters_user;
DROP TABLE IF EXISTS saved_filters;
