-- +goose Up
CREATE TABLE instances (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL DEFAULT '',
    base_url TEXT NOT NULL UNIQUE,
    version TEXT NOT NULL DEFAULT '',
    capabilities_json TEXT NOT NULL DEFAULT '{}',
    sync_token_ciphertext TEXT NOT NULL DEFAULT '',
    webhook_secret_ciphertext TEXT NOT NULL DEFAULT '',
    oauth_client_id TEXT NOT NULL DEFAULT '',
    oauth_client_secret_ciphertext TEXT NOT NULL DEFAULT '',
    external_url TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE organizations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    external_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    synced_at TEXT,
    UNIQUE (instance_id, external_id)
);

CREATE TABLE repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    org_id INTEGER REFERENCES organizations(id) ON DELETE SET NULL,
    external_id INTEGER NOT NULL,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL,
    default_branch TEXT NOT NULL DEFAULT '',
    private INTEGER NOT NULL DEFAULT 0,
    archived INTEGER NOT NULL DEFAULT 0,
    empty INTEGER NOT NULL DEFAULT 0,
    fork INTEGER NOT NULL DEFAULT 0,
    html_url TEXT NOT NULL DEFAULT '',
    permissions_mirror_json TEXT NOT NULL DEFAULT '{}',
    last_synced_at TEXT,
    deleted_at TEXT,
    renamed_from TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (instance_id, external_id)
);

CREATE UNIQUE INDEX repositories_instance_owner_name_alive
    ON repositories (instance_id, owner, name)
    WHERE deleted_at IS NULL;

CREATE INDEX repositories_instance_id ON repositories (instance_id);

CREATE TABLE pull_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    external_id INTEGER NOT NULL,
    number INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    body_excerpt TEXT NOT NULL DEFAULT '',
    author_login TEXT NOT NULL DEFAULT '',
    author_external_id INTEGER,
    source_branch TEXT NOT NULL DEFAULT '',
    target_branch TEXT NOT NULL DEFAULT '',
    head_sha TEXT NOT NULL DEFAULT '',
    base_sha TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'open',
    draft INTEGER NOT NULL DEFAULT 0,
    mergeable INTEGER,
    mergeable_state TEXT NOT NULL DEFAULT '',
    review_state TEXT NOT NULL DEFAULT '',
    ci_state TEXT NOT NULL DEFAULT '',
    html_url TEXT NOT NULL DEFAULT '',
    created_at TEXT,
    updated_at TEXT,
    closed_at TEXT,
    merged_at TEXT,
    UNIQUE (repo_id, number)
);

CREATE INDEX pull_requests_repo_state ON pull_requests (repo_id, state);

CREATE TABLE workflows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    external_id_or_path TEXT NOT NULL DEFAULT '',
    last_seen_commit_sha TEXT NOT NULL DEFAULT '',
    UNIQUE (repo_id, path)
);

CREATE TABLE workflow_graphs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    nodes_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (repo_id, path, commit_sha)
);

CREATE TABLE workflow_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    workflow_id INTEGER REFERENCES workflows(id) ON DELETE SET NULL,
    external_id INTEGER NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    event TEXT NOT NULL DEFAULT '',
    branch TEXT NOT NULL DEFAULT '',
    commit_sha TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown',
    conclusion TEXT NOT NULL DEFAULT 'unknown',
    upstream_status TEXT NOT NULL DEFAULT '',
    upstream_conclusion TEXT NOT NULL DEFAULT '',
    actor_login TEXT NOT NULL DEFAULT '',
    html_url TEXT NOT NULL DEFAULT '',
    workflow_path TEXT NOT NULL DEFAULT '',
    started_at TEXT,
    completed_at TEXT,
    run_attempt INTEGER NOT NULL DEFAULT 1,
    UNIQUE (repo_id, external_id)
);

CREATE INDEX workflow_runs_repo_updated ON workflow_runs (repo_id, id DESC);

CREATE TABLE jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    external_id INTEGER NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown',
    conclusion TEXT NOT NULL DEFAULT 'unknown',
    upstream_status TEXT NOT NULL DEFAULT '',
    upstream_conclusion TEXT NOT NULL DEFAULT '',
    runner_id INTEGER,
    runner_name TEXT NOT NULL DEFAULT '',
    html_url TEXT NOT NULL DEFAULT '',
    started_at TEXT,
    completed_at TEXT,
    steps_json TEXT,
    UNIQUE (repo_id, external_id)
);

CREATE INDEX jobs_run_id ON jobs (run_id);

CREATE TABLE attention_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    severity TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    fingerprint TEXT NOT NULL UNIQUE,
    opened_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    resolved_at TEXT,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX attention_items_open ON attention_items (instance_id, resolved_at);

CREATE TABLE webhook_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    delivery_id TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    received_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    processed_at TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    error TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    UNIQUE (instance_id, delivery_id)
);

CREATE INDEX webhook_events_status ON webhook_events (status, id);

CREATE TABLE sync_state (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    scope TEXT NOT NULL,
    scope_id INTEGER NOT NULL DEFAULT 0,
    cursor_json TEXT NOT NULL DEFAULT '{}',
    last_success_at TEXT,
    last_error TEXT,
    phase TEXT NOT NULL DEFAULT '',
    UNIQUE (instance_id, scope, scope_id)
);

CREATE TABLE sync_leases (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    holder TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL,
    gitea_user_id INTEGER,
    login TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    is_bootstrap_admin INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);

CREATE INDEX sessions_user_id ON sessions (user_id);
CREATE INDEX sessions_expires_at ON sessions (expires_at);

CREATE TABLE oauth_states (
    state TEXT PRIMARY KEY,
    code_verifier TEXT NOT NULL,
    redirect_to TEXT NOT NULL DEFAULT '/',
    expires_at TEXT NOT NULL
);

CREATE TABLE user_repository_access (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    permission TEXT NOT NULL DEFAULT 'read',
    checked_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (user_id, repo_id)
);

CREATE INDEX user_repository_access_user ON user_repository_access (user_id);

CREATE TABLE user_tokens (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    access_token_ciphertext TEXT NOT NULL DEFAULT '',
    refresh_token_ciphertext TEXT NOT NULL DEFAULT '',
    expires_at TEXT
);

-- +goose Down
DROP TABLE IF EXISTS user_tokens;
DROP TABLE IF EXISTS user_repository_access;
DROP TABLE IF EXISTS oauth_states;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS sync_leases;
DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS webhook_events;
DROP TABLE IF EXISTS attention_items;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS workflow_runs;
DROP TABLE IF EXISTS workflow_graphs;
DROP TABLE IF EXISTS workflows;
DROP TABLE IF EXISTS pull_requests;
DROP INDEX IF EXISTS repositories_instance_owner_name_alive;
DROP INDEX IF EXISTS repositories_instance_id;
DROP TABLE IF EXISTS repositories;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS instances;
