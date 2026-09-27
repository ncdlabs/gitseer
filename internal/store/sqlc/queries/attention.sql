-- name: ResolveAttentionByFingerprint :exec
UPDATE attention_items
SET resolved_at = sqlc.arg(resolved_at), updated_at = sqlc.arg(updated_at)
WHERE fingerprint = sqlc.arg(fingerprint) AND resolved_at IS NULL;

-- name: ClearUntilResolvedMutes :exec
DELETE FROM attention_mutes
WHERE fingerprint = sqlc.arg(fingerprint) AND until_at IS NULL;

-- name: GetAttentionMuteByID :one
SELECT id, user_id, repo_id, rule_type, fingerprint, until_at, reason, created_at, created_by
FROM attention_mutes
WHERE id = sqlc.arg(id);

-- name: DeleteAttentionMuteByID :exec
DELETE FROM attention_mutes WHERE id = sqlc.arg(id);

-- name: ListAttentionRuleOverrides :many
SELECT rule_type, severity, updated_at
FROM attention_rule_overrides
ORDER BY rule_type;

-- name: DeleteAttentionRuleOverride :exec
DELETE FROM attention_rule_overrides WHERE rule_type = sqlc.arg(rule_type);
