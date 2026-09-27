-- name: GetWorkflowRunByID :one
SELECT wr.id, wr.repo_id, wr.workflow_id, wr.external_id, wr.node_id, wr.name, wr.event, wr.branch, wr.commit_sha,
       wr.status, wr.conclusion, wr.upstream_status, wr.upstream_conclusion, wr.actor_login, wr.html_url,
       wr.workflow_path, wr.started_at, wr.completed_at, wr.run_attempt, r.owner, r.name, r.full_name,
       COALESCE(NULLIF(TRIM(i.forge_type), ''), 'gitea'), r.instance_id, COALESCE(i.name, '')
FROM workflow_runs wr
JOIN repositories r ON r.id = wr.repo_id
LEFT JOIN instances i ON i.id = r.instance_id
WHERE wr.id = sqlc.arg(id);

-- name: GetWorkflowRunByExternalID :one
SELECT wr.id, wr.repo_id, wr.workflow_id, wr.external_id, wr.node_id, wr.name, wr.event, wr.branch, wr.commit_sha,
       wr.status, wr.conclusion, wr.upstream_status, wr.upstream_conclusion, wr.actor_login, wr.html_url,
       wr.workflow_path, wr.started_at, wr.completed_at, wr.run_attempt, r.owner, r.name, r.full_name
FROM workflow_runs wr
JOIN repositories r ON r.id = wr.repo_id
WHERE wr.repo_id = sqlc.arg(repo_id) AND wr.external_id = sqlc.arg(external_id);

-- name: CountWorkflowRuns :one
SELECT COUNT(*) FROM workflow_runs;

-- name: ListInFlightWorkflowRunsByRepo :many
SELECT wr.id, wr.repo_id, wr.workflow_id, wr.external_id, wr.node_id, wr.name, wr.event, wr.branch, wr.commit_sha,
       wr.status, wr.conclusion, wr.upstream_status, wr.upstream_conclusion, wr.actor_login, wr.html_url,
       wr.workflow_path, wr.started_at, wr.completed_at, wr.run_attempt, r.owner, r.name, r.full_name
FROM workflow_runs wr
JOIN repositories r ON r.id = wr.repo_id
WHERE wr.repo_id = sqlc.arg(repo_id) AND wr.status IN ('queued', 'waiting', 'running')
ORDER BY wr.id DESC;
