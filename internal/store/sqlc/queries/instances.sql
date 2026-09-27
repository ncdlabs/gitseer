-- name: GetInstanceByID :one
SELECT
  id, name, forge_type, base_url, version, capabilities_json,
  sync_token_ciphertext, webhook_secret_ciphertext,
  oauth_client_id, oauth_client_secret_ciphertext, external_url,
  allow_private_network, allow_unsigned_webhooks,
  webhook_verified_at, webhook_ensure_at, webhook_ensure_error, webhook_verify_token,
  created_at, updated_at
FROM instances
WHERE id = sqlc.arg(id);

-- name: GetInstanceByURL :one
SELECT
  id, name, forge_type, base_url, version, capabilities_json,
  sync_token_ciphertext, webhook_secret_ciphertext,
  oauth_client_id, oauth_client_secret_ciphertext, external_url,
  allow_private_network, allow_unsigned_webhooks,
  webhook_verified_at, webhook_ensure_at, webhook_ensure_error, webhook_verify_token,
  created_at, updated_at
FROM instances
WHERE base_url = sqlc.arg(base_url);

-- name: GetInstanceByForgeAndURL :one
SELECT
  id, name, forge_type, base_url, version, capabilities_json,
  sync_token_ciphertext, webhook_secret_ciphertext,
  oauth_client_id, oauth_client_secret_ciphertext, external_url,
  allow_private_network, allow_unsigned_webhooks,
  webhook_verified_at, webhook_ensure_at, webhook_ensure_error, webhook_verify_token,
  created_at, updated_at
FROM instances
WHERE forge_type = sqlc.arg(forge_type) AND base_url = sqlc.arg(base_url);

-- name: ListInstances :many
SELECT
  id, name, forge_type, base_url, version, capabilities_json,
  sync_token_ciphertext, webhook_secret_ciphertext,
  oauth_client_id, oauth_client_secret_ciphertext, external_url,
  allow_private_network, allow_unsigned_webhooks,
  webhook_verified_at, webhook_ensure_at, webhook_ensure_error, webhook_verify_token,
  created_at, updated_at
FROM instances
ORDER BY id ASC;

-- name: GetPrimaryInstance :one
SELECT
  id, name, forge_type, base_url, version, capabilities_json,
  sync_token_ciphertext, webhook_secret_ciphertext,
  oauth_client_id, oauth_client_secret_ciphertext, external_url,
  allow_private_network, allow_unsigned_webhooks,
  webhook_verified_at, webhook_ensure_at, webhook_ensure_error, webhook_verify_token,
  created_at, updated_at
FROM instances
ORDER BY id ASC
LIMIT 1;

-- name: DeleteInstanceByID :exec
DELETE FROM instances WHERE id = sqlc.arg(id);
