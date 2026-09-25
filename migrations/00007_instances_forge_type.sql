-- +goose Up
ALTER TABLE instances ADD COLUMN forge_type TEXT NOT NULL DEFAULT 'gitea';
ALTER TABLE instances ADD COLUMN allow_private_network INTEGER NOT NULL DEFAULT 0;
ALTER TABLE instances ADD COLUMN allow_unsigned_webhooks INTEGER NOT NULL DEFAULT 0;
CREATE INDEX instances_forge_type_idx ON instances (forge_type);

-- +goose Down
DROP INDEX IF EXISTS instances_forge_type_idx;
ALTER TABLE instances DROP COLUMN allow_unsigned_webhooks;
ALTER TABLE instances DROP COLUMN allow_private_network;
ALTER TABLE instances DROP COLUMN forge_type;
