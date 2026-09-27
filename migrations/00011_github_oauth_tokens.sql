-- +goose Up
-- GitHub OAuth identity + per-instance user tokens (Stream 6).

ALTER TABLE users ADD COLUMN github_user_id INTEGER;
ALTER TABLE users ADD COLUMN github_instance_id INTEGER REFERENCES instances(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX users_github_instance_uid
  ON users (github_instance_id, github_user_id)
  WHERE github_user_id IS NOT NULL;

ALTER TABLE oauth_states ADD COLUMN provider TEXT NOT NULL DEFAULT 'gitea';
ALTER TABLE oauth_states ADD COLUMN instance_id INTEGER;
ALTER TABLE oauth_states ADD COLUMN link_user_id INTEGER;

CREATE TABLE user_tokens_v2 (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    instance_id INTEGER NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    access_token_ciphertext TEXT NOT NULL DEFAULT '',
    refresh_token_ciphertext TEXT NOT NULL DEFAULT '',
    expires_at TEXT,
    PRIMARY KEY (user_id, instance_id)
);

-- Preserve existing Gitea tokens under the user's instance_id when available.
INSERT INTO user_tokens_v2 (user_id, instance_id, access_token_ciphertext, refresh_token_ciphertext, expires_at)
SELECT t.user_id, u.instance_id, t.access_token_ciphertext, t.refresh_token_ciphertext, t.expires_at
FROM user_tokens t
INNER JOIN users u ON u.id = t.user_id
WHERE u.instance_id IS NOT NULL;

INSERT INTO user_tokens_v2 (user_id, instance_id, access_token_ciphertext, refresh_token_ciphertext, expires_at)
SELECT t.user_id, i.id, t.access_token_ciphertext, t.refresh_token_ciphertext, t.expires_at
FROM user_tokens t
INNER JOIN users u ON u.id = t.user_id
INNER JOIN (
  SELECT id FROM instances
  WHERE forge_type = 'gitea' OR forge_type = '' OR forge_type IS NULL
  ORDER BY id ASC LIMIT 1
) i
WHERE u.instance_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM user_tokens_v2 v WHERE v.user_id = t.user_id AND v.instance_id = i.id);

DROP TABLE user_tokens;
ALTER TABLE user_tokens_v2 RENAME TO user_tokens;

-- +goose Down
CREATE TABLE user_tokens_legacy (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    access_token_ciphertext TEXT NOT NULL DEFAULT '',
    refresh_token_ciphertext TEXT NOT NULL DEFAULT '',
    expires_at TEXT
);

INSERT INTO user_tokens_legacy (user_id, access_token_ciphertext, refresh_token_ciphertext, expires_at)
SELECT user_id, access_token_ciphertext, refresh_token_ciphertext, expires_at
FROM user_tokens
GROUP BY user_id;

DROP TABLE user_tokens;
ALTER TABLE user_tokens_legacy RENAME TO user_tokens;

-- SQLite cannot DROP COLUMN on older goose paths reliably for all columns; recreate oauth_states.
CREATE TABLE oauth_states_legacy (
    state TEXT PRIMARY KEY,
    code_verifier TEXT NOT NULL,
    redirect_to TEXT NOT NULL DEFAULT '/',
    expires_at TEXT NOT NULL
);
INSERT INTO oauth_states_legacy (state, code_verifier, redirect_to, expires_at)
SELECT state, code_verifier, redirect_to, expires_at FROM oauth_states;
DROP TABLE oauth_states;
ALTER TABLE oauth_states_legacy RENAME TO oauth_states;

DROP INDEX IF EXISTS users_github_instance_uid;
ALTER TABLE users DROP COLUMN github_instance_id;
ALTER TABLE users DROP COLUMN github_user_id;
