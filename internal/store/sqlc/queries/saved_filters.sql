-- name: ListSavedFilters :many
SELECT id, user_id, name, query_json, created_at, updated_at
FROM saved_filters
WHERE user_id = sqlc.arg(user_id)
ORDER BY LOWER(name) ASC, id ASC;

-- name: GetSavedFilter :one
SELECT id, user_id, name, query_json, created_at, updated_at
FROM saved_filters
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: InsertSavedFilter :one
INSERT INTO saved_filters (user_id, name, query_json, created_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(query_json), sqlc.arg(created_at), sqlc.arg(updated_at))
RETURNING id, user_id, name, query_json, created_at, updated_at;

-- name: UpdateSavedFilter :exec
UPDATE saved_filters SET name = sqlc.arg(name), query_json = sqlc.arg(query_json), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeleteSavedFilter :exec
DELETE FROM saved_filters WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);
