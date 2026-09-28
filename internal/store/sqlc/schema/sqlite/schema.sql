CREATE TABLE app_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    instance_name TEXT NOT NULL DEFAULT '',
    sync_history_days INTEGER NOT NULL DEFAULT 0,
    attention_long_running_after TEXT NOT NULL DEFAULT '',
    retention_runs_days INTEGER NOT NULL DEFAULT 0,
    retention_webhooks_days INTEGER NOT NULL DEFAULT 0,
    retention_attention_days INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    gitea_url TEXT NOT NULL DEFAULT '', gitea_token_cipher TEXT NOT NULL DEFAULT '', gitea_webhook_secret_cipher TEXT NOT NULL DEFAULT '', gitea_allow_private_network INTEGER, gitea_allow_unsigned_webhooks INTEGER, oauth_client_id TEXT NOT NULL DEFAULT '', oauth_client_secret_cipher TEXT NOT NULL DEFAULT '', setup_completed INTEGER NOT NULL DEFAULT 0, server_external_url TEXT NOT NULL DEFAULT '',
    bootstrap_username TEXT NOT NULL DEFAULT '', bootstrap_password_hash TEXT NOT NULL DEFAULT '', bootstrap_keep_after_setup INTEGER NOT NULL DEFAULT 1);

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

CREATE TABLE attention_rule_overrides (
    rule_type TEXT PRIMARY KEY,
    severity TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

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
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    forge_type TEXT NOT NULL DEFAULT 'gitea', allow_private_network INTEGER NOT NULL DEFAULT 0, allow_unsigned_webhooks INTEGER NOT NULL DEFAULT 0, webhook_verified_at TEXT, webhook_ensure_at TEXT, webhook_ensure_error TEXT, webhook_verify_token TEXT);

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
    steps_json TEXT, node_id TEXT NOT NULL DEFAULT '', labels_json TEXT, message TEXT NOT NULL DEFAULT '',
    UNIQUE (repo_id, external_id)
);

CREATE TABLE notification_outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    dedupe_key TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    sent_at TEXT,
    processing_started_at TEXT
);

CREATE TABLE notification_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0,
    min_severity TEXT NOT NULL DEFAULT 'critical',
    immediate_enabled INTEGER NOT NULL DEFAULT 1,
    digest_enabled INTEGER NOT NULL DEFAULT 0,
    digest_hour_utc INTEGER NOT NULL DEFAULT 14,
    last_digest_at TEXT,

    smtp_enabled INTEGER NOT NULL DEFAULT 0,
    smtp_host TEXT NOT NULL DEFAULT '',
    smtp_port INTEGER NOT NULL DEFAULT 587,
    smtp_tls_mode TEXT NOT NULL DEFAULT 'starttls',
    smtp_from TEXT NOT NULL DEFAULT '',
    smtp_to TEXT NOT NULL DEFAULT '',
    smtp_username TEXT NOT NULL DEFAULT '',
    smtp_password_ciphertext TEXT NOT NULL DEFAULT '',

    slack_enabled INTEGER NOT NULL DEFAULT 0,
    slack_webhook_ciphertext TEXT NOT NULL DEFAULT '',

    discord_enabled INTEGER NOT NULL DEFAULT 0,
    discord_webhook_ciphertext TEXT NOT NULL DEFAULT '',

    webhook_enabled INTEGER NOT NULL DEFAULT 0,
    webhook_url_ciphertext TEXT NOT NULL DEFAULT '',

    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    incident_enabled INTEGER NOT NULL DEFAULT 0, incident_webhook_ciphertext TEXT NOT NULL DEFAULT '');

CREATE TABLE oauth_states (
    state TEXT PRIMARY KEY,
    code_verifier TEXT NOT NULL,
    redirect_to TEXT NOT NULL DEFAULT '/',
    expires_at TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'gitea', instance_id INTEGER, link_user_id INTEGER);

CREATE TABLE organizations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    external_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    full_name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    synced_at TEXT, node_id TEXT NOT NULL DEFAULT '',
    UNIQUE (instance_id, external_id)
);

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
    merged_at TEXT, node_id TEXT NOT NULL DEFAULT '',
    UNIQUE (repo_id, number)
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
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')), node_id TEXT NOT NULL DEFAULT '',
    UNIQUE (instance_id, external_id)
);

CREATE TABLE saved_filters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    query_json TEXT NOT NULL DEFAULT '{}',
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
    user_agent TEXT NOT NULL DEFAULT '',
    bootstrap_elevated_until TEXT
);

CREATE TABLE "sync_leases" (
    id INTEGER PRIMARY KEY,
    holder TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

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

CREATE TABLE user_repository_access (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    permission TEXT NOT NULL DEFAULT 'read',
    checked_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (user_id, repo_id)
);

CREATE TABLE "user_tokens" (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    access_token_ciphertext TEXT NOT NULL DEFAULT '',
    refresh_token_ciphertext TEXT NOT NULL DEFAULT '',
    expires_at TEXT,
    PRIMARY KEY (user_id, instance_id)
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
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    github_user_id INTEGER, github_instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL, gitlab_user_id INTEGER, gitlab_instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL, bitbucket_user_id INTEGER, bitbucket_instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL);

CREATE TABLE wallboard_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL DEFAULT '',
    created_by_user_id INTEGER,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used_at TEXT,
    revoked_at TEXT,
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL
);

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
    attempts INTEGER NOT NULL DEFAULT 0, processing_started_at TEXT,
    UNIQUE (instance_id, delivery_id)
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
    run_attempt INTEGER NOT NULL DEFAULT 1, node_id TEXT NOT NULL DEFAULT '',
    UNIQUE (repo_id, external_id)
);

CREATE TABLE workflows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    external_id_or_path TEXT NOT NULL DEFAULT '',
    last_seen_commit_sha TEXT NOT NULL DEFAULT '',
    UNIQUE (repo_id, path)
);

CREATE INDEX attention_items_open ON attention_items (instance_id, resolved_at);

CREATE INDEX attention_mutes_fingerprint ON attention_mutes (fingerprint);

CREATE INDEX attention_mutes_rule_repo ON attention_mutes (rule_type, repo_id);

CREATE INDEX attention_mutes_until ON attention_mutes (until_at);

CREATE INDEX instances_forge_type_idx ON instances (forge_type);

CREATE INDEX jobs_run_id ON jobs (run_id);

CREATE INDEX notification_outbox_claim
  ON notification_outbox (status, next_attempt_at, id);

CREATE UNIQUE INDEX notification_outbox_dedupe
  ON notification_outbox (dedupe_key)
  WHERE dedupe_key != '';

CREATE INDEX pull_requests_repo_state ON pull_requests (repo_id, state);

CREATE INDEX repositories_instance_id ON repositories (instance_id);

CREATE UNIQUE INDEX repositories_instance_owner_name_alive
    ON repositories (instance_id, owner, name)
    WHERE deleted_at IS NULL;

CREATE INDEX saved_filters_user ON saved_filters (user_id);

CREATE UNIQUE INDEX saved_filters_user_name ON saved_filters (user_id, name);

CREATE INDEX sessions_expires_at ON sessions (expires_at);

CREATE INDEX sessions_user_id ON sessions (user_id);

CREATE INDEX user_repository_access_user ON user_repository_access (user_id);

CREATE UNIQUE INDEX users_github_instance_uid
  ON users (github_instance_id, github_user_id)
  WHERE github_user_id IS NOT NULL;
CREATE UNIQUE INDEX users_gitlab_instance_uid
  ON users (gitlab_instance_id, gitlab_user_id)
  WHERE gitlab_user_id IS NOT NULL;
CREATE UNIQUE INDEX users_bitbucket_instance_uid
  ON users (bitbucket_instance_id, bitbucket_user_id)
  WHERE bitbucket_user_id IS NOT NULL;

CREATE UNIQUE INDEX users_instance_gitea_uid
  ON users (instance_id, gitea_user_id)
  WHERE gitea_user_id IS NOT NULL;

CREATE INDEX wallboard_tokens_active ON wallboard_tokens (revoked_at, id);

CREATE INDEX webhook_events_status ON webhook_events (status, id);

CREATE INDEX workflow_runs_repo_updated ON workflow_runs (repo_id, id DESC);

