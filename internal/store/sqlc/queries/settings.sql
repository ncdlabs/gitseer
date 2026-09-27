-- name: GetAppSettings :one
SELECT
  instance_name, sync_history_days, attention_long_running_after,
  retention_runs_days, retention_webhooks_days, retention_attention_days,
  server_external_url,
  gitea_url, gitea_token_cipher, gitea_webhook_secret_cipher,
  gitea_allow_private_network, gitea_allow_unsigned_webhooks,
  oauth_client_id, oauth_client_secret_cipher, setup_completed, updated_at
FROM app_settings
WHERE id = 1;

-- name: GetWorkflowGraph :one
SELECT nodes_json FROM workflow_graphs
WHERE repo_id = sqlc.arg(repo_id) AND path = sqlc.arg(path) AND commit_sha = sqlc.arg(commit_sha);

-- name: UpsertWorkflowGraph :exec
INSERT INTO workflow_graphs (repo_id, path, commit_sha, nodes_json)
VALUES (sqlc.arg(repo_id), sqlc.arg(path), sqlc.arg(commit_sha), sqlc.arg(nodes_json))
ON CONFLICT(repo_id, path, commit_sha) DO UPDATE SET nodes_json = excluded.nodes_json;
