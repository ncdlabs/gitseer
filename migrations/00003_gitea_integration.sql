-- +goose Up
ALTER TABLE app_settings ADD COLUMN gitea_url TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN gitea_token_cipher TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN gitea_webhook_secret_cipher TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN gitea_allow_private_network INTEGER;
ALTER TABLE app_settings ADD COLUMN gitea_allow_unsigned_webhooks INTEGER;
ALTER TABLE app_settings ADD COLUMN oauth_client_id TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN oauth_client_secret_cipher TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN setup_completed INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE app_settings DROP COLUMN gitea_url;
ALTER TABLE app_settings DROP COLUMN gitea_token_cipher;
ALTER TABLE app_settings DROP COLUMN gitea_webhook_secret_cipher;
ALTER TABLE app_settings DROP COLUMN gitea_allow_private_network;
ALTER TABLE app_settings DROP COLUMN gitea_allow_unsigned_webhooks;
ALTER TABLE app_settings DROP COLUMN oauth_client_id;
ALTER TABLE app_settings DROP COLUMN oauth_client_secret_cipher;
ALTER TABLE app_settings DROP COLUMN setup_completed;
