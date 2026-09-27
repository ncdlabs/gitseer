-- name: GetJobByID :one
SELECT id, run_id, repo_id, external_id, node_id, name, status, conclusion, upstream_status, upstream_conclusion,
       runner_id, runner_name, html_url, started_at, completed_at, steps_json, labels_json, message
FROM jobs
WHERE id = sqlc.arg(id);

-- name: GetJobByExternalID :one
SELECT id, run_id, repo_id, external_id, node_id, name, status, conclusion, upstream_status, upstream_conclusion,
       runner_id, runner_name, html_url, started_at, completed_at, steps_json, labels_json, message
FROM jobs
WHERE repo_id = sqlc.arg(repo_id) AND external_id = sqlc.arg(external_id);

-- name: ListJobsByRunID :many
SELECT id, run_id, repo_id, external_id, node_id, name, status, conclusion, upstream_status, upstream_conclusion,
       runner_id, runner_name, html_url, started_at, completed_at, steps_json, labels_json, message
FROM jobs
WHERE run_id = sqlc.arg(run_id)
ORDER BY id;

-- name: ListJobsByRunIDs :many
SELECT id, run_id, repo_id, external_id, node_id, name, status, conclusion, upstream_status, upstream_conclusion,
       runner_id, runner_name, html_url, started_at, completed_at, steps_json, labels_json, message
FROM jobs
WHERE run_id IN (sqlc.slice(run_ids))
ORDER BY id;
