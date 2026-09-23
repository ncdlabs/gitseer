-- +goose Up
CREATE UNIQUE INDEX users_instance_gitea_uid
  ON users (instance_id, gitea_user_id)
  WHERE gitea_user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS users_instance_gitea_uid;
