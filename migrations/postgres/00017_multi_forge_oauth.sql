-- +goose Up
-- GitLab / Bitbucket OAuth identity columns (Forgejo reuses gitea_user_id + instance_id).

ALTER TABLE users ADD COLUMN IF NOT EXISTS gitlab_user_id INTEGER;
ALTER TABLE users ADD COLUMN IF NOT EXISTS gitlab_instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS bitbucket_user_id INTEGER;
ALTER TABLE users ADD COLUMN IF NOT EXISTS bitbucket_instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS users_gitlab_instance_uid
  ON users (gitlab_instance_id, gitlab_user_id)
  WHERE gitlab_user_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS users_bitbucket_instance_uid
  ON users (bitbucket_instance_id, bitbucket_user_id)
  WHERE bitbucket_user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS users_bitbucket_instance_uid;
DROP INDEX IF EXISTS users_gitlab_instance_uid;
ALTER TABLE users DROP COLUMN IF EXISTS bitbucket_instance_id;
ALTER TABLE users DROP COLUMN IF EXISTS bitbucket_user_id;
ALTER TABLE users DROP COLUMN IF EXISTS gitlab_instance_id;
ALTER TABLE users DROP COLUMN IF EXISTS gitlab_user_id;
