-- +goose Up
-- GitHub OAuth identity + per-instance user tokens (Stream 6).

ALTER TABLE users ADD COLUMN IF NOT EXISTS github_user_id INTEGER;
ALTER TABLE users ADD COLUMN IF NOT EXISTS github_instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS users_github_instance_uid
  ON users (github_instance_id, github_user_id)
  WHERE github_user_id IS NOT NULL;

ALTER TABLE oauth_states ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'gitea';
ALTER TABLE oauth_states ADD COLUMN IF NOT EXISTS instance_id INTEGER;
ALTER TABLE oauth_states ADD COLUMN IF NOT EXISTS link_user_id INTEGER;

CREATE TABLE IF NOT EXISTS user_tokens_v2 (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    access_token_ciphertext TEXT NOT NULL DEFAULT '',
    refresh_token_ciphertext TEXT NOT NULL DEFAULT '',
    expires_at TEXT,
    PRIMARY KEY (user_id, instance_id)
);

INSERT INTO user_tokens_v2 (user_id, instance_id, access_token_ciphertext, refresh_token_ciphertext, expires_at)
SELECT t.user_id, u.instance_id, t.access_token_ciphertext, t.refresh_token_ciphertext, t.expires_at
FROM user_tokens t
INNER JOIN users u ON u.id = t.user_id
WHERE u.instance_id IS NOT NULL
ON CONFLICT DO NOTHING;

INSERT INTO user_tokens_v2 (user_id, instance_id, access_token_ciphertext, refresh_token_ciphertext, expires_at)
SELECT t.user_id, i.id, t.access_token_ciphertext, t.refresh_token_ciphertext, t.expires_at
FROM user_tokens t
INNER JOIN users u ON u.id = t.user_id
CROSS JOIN LATERAL (
  SELECT id FROM instances
  WHERE forge_type = 'gitea' OR forge_type = '' OR forge_type IS NULL
  ORDER BY id ASC LIMIT 1
) i
WHERE u.instance_id IS NULL
ON CONFLICT DO NOTHING;

DROP TABLE IF EXISTS user_tokens;
ALTER TABLE user_tokens_v2 RENAME TO user_tokens;

-- +goose Down
CREATE TABLE user_tokens_legacy (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    access_token_ciphertext TEXT NOT NULL DEFAULT '',
    refresh_token_ciphertext TEXT NOT NULL DEFAULT '',
    expires_at TEXT
);

INSERT INTO user_tokens_legacy (user_id, access_token_ciphertext, refresh_token_ciphertext, expires_at)
SELECT DISTINCT ON (user_id) user_id, access_token_ciphertext, refresh_token_ciphertext, expires_at
FROM user_tokens
ORDER BY user_id, instance_id;

DROP TABLE IF EXISTS user_tokens;
ALTER TABLE user_tokens_legacy RENAME TO user_tokens;

ALTER TABLE oauth_states DROP COLUMN IF EXISTS link_user_id;
ALTER TABLE oauth_states DROP COLUMN IF EXISTS instance_id;
ALTER TABLE oauth_states DROP COLUMN IF EXISTS provider;

DROP INDEX IF EXISTS users_github_instance_uid;
ALTER TABLE users DROP COLUMN IF EXISTS github_instance_id;
ALTER TABLE users DROP COLUMN IF EXISTS github_user_id;
