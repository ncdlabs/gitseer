-- name: GetRepositoryByID :one
SELECT id, instance_id, org_id, external_id, node_id, owner, name, full_name, default_branch,
       private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
FROM repositories
WHERE id = sqlc.arg(id);

-- name: GetRepositoryByExternalID :one
SELECT id, instance_id, org_id, external_id, node_id, owner, name, full_name, default_branch,
       private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
FROM repositories
WHERE instance_id = sqlc.arg(instance_id) AND external_id = sqlc.arg(external_id);

-- name: ListRepositoriesByOwnerName :many
SELECT id, instance_id, org_id, external_id, node_id, owner, name, full_name, default_branch,
       private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
FROM repositories
WHERE owner = sqlc.arg(owner) AND name = sqlc.arg(name) AND deleted_at IS NULL
ORDER BY id ASC;

-- name: GetRepositoryByOwnerNameInInstance :one
SELECT id, instance_id, org_id, external_id, node_id, owner, name, full_name, default_branch,
       private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
FROM repositories
WHERE instance_id = sqlc.arg(instance_id)
  AND owner = sqlc.arg(owner)
  AND name = sqlc.arg(name)
  AND deleted_at IS NULL;

-- name: SoftDeleteRepository :exec
UPDATE repositories
SET deleted_at = sqlc.arg(deleted_at), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: DeleteAccessForRepo :exec
DELETE FROM user_repository_access WHERE repo_id = sqlc.arg(repo_id);
