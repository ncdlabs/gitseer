-- name: GetUserByID :one
SELECT id, instance_id, gitea_user_id, github_user_id, github_instance_id,
       gitlab_user_id, gitlab_instance_id, bitbucket_user_id, bitbucket_instance_id,
       login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at
FROM users
WHERE id = sqlc.arg(id);

-- name: GetSessionByTokenHash :one
SELECT id, user_id, expires_at, created_at, ip, user_agent
FROM sessions
WHERE token_hash = sqlc.arg(token_hash);

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = sqlc.arg(id);

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at, ip, user_agent)
VALUES (
  sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at),
  sqlc.arg(created_at), sqlc.arg(ip), sqlc.arg(user_agent)
);
