-- name: GetPostgresDatabaseSize :one
SELECT pg_database_size(current_database()) AS bytes;
