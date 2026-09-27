-- name: ListWallboardTokensAll :many
SELECT id, name, token_prefix, created_by_user_id, created_at, last_used_at, revoked_at
FROM wallboard_tokens
ORDER BY created_at DESC, id DESC;

-- name: ListWallboardTokensActive :many
SELECT id, name, token_prefix, created_by_user_id, created_at, last_used_at, revoked_at
FROM wallboard_tokens
WHERE revoked_at IS NULL
ORDER BY created_at DESC, id DESC;

-- name: GetWallboardTokenByID :one
SELECT id, name, token_prefix, created_by_user_id, created_at, last_used_at, revoked_at
FROM wallboard_tokens
WHERE id = sqlc.arg(id);

-- name: RevokeWallboardToken :exec
UPDATE wallboard_tokens SET revoked_at = sqlc.arg(revoked_at)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;
