-- +goose Up
CREATE TABLE attention_mutes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    rule_type TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL DEFAULT '',
    until_at TEXT,
    reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX attention_mutes_fingerprint ON attention_mutes (fingerprint);
CREATE INDEX attention_mutes_rule_repo ON attention_mutes (rule_type, repo_id);
CREATE INDEX attention_mutes_until ON attention_mutes (until_at);

CREATE TABLE attention_rule_overrides (
    rule_type TEXT PRIMARY KEY,
    severity TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- +goose Down
DROP TABLE IF EXISTS attention_rule_overrides;
DROP TABLE IF EXISTS attention_mutes;
