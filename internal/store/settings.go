package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AppSettings is the single-row persisted Lens settings override.
type AppSettings struct {
	InstanceName              string
	SyncHistoryDays           int
	AttentionLongRunningAfter string
	RetentionRunsDays         int
	RetentionWebhooksDays     int
	RetentionAttentionDays    int
	ServerExternalURL         string

	GiteaURL                     string
	GiteaTokenCipher             string
	GiteaWebhookSecretCipher     string
	GiteaAllowPrivateNetwork     sql.NullInt64 // NULL = unset (fall back to config)
	GiteaAllowUnsignedWebhooks   sql.NullInt64
	OAuthClientID                string
	OAuthClientSecretCipher      string
	SetupCompleted               bool

	UpdatedAt time.Time
}

// GetAppSettings returns the settings row, or nil when unset.
func (s *Store) GetAppSettings(ctx context.Context) (*AppSettings, error) {
	var row AppSettings
	var updated string
	var setup int
	err := s.queryRow(ctx, `
SELECT instance_name, sync_history_days, attention_long_running_after,
       retention_runs_days, retention_webhooks_days, retention_attention_days,
       server_external_url,
       gitea_url, gitea_token_cipher, gitea_webhook_secret_cipher,
       gitea_allow_private_network, gitea_allow_unsigned_webhooks,
       oauth_client_id, oauth_client_secret_cipher, setup_completed, updated_at
FROM app_settings WHERE id = 1`).Scan(
		&row.InstanceName,
		&row.SyncHistoryDays,
		&row.AttentionLongRunningAfter,
		&row.RetentionRunsDays,
		&row.RetentionWebhooksDays,
		&row.RetentionAttentionDays,
		&row.ServerExternalURL,
		&row.GiteaURL,
		&row.GiteaTokenCipher,
		&row.GiteaWebhookSecretCipher,
		&row.GiteaAllowPrivateNetwork,
		&row.GiteaAllowUnsignedWebhooks,
		&row.OAuthClientID,
		&row.OAuthClientSecretCipher,
		&setup,
		&updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.SetupCompleted = setup != 0
	if t, err := parseTime(updated); err == nil {
		row.UpdatedAt = t
	}
	return &row, nil
}

// UpsertAppSettings writes the single settings row (values + integration).
func (s *Store) UpsertAppSettings(ctx context.Context, in AppSettings) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	setup := 0
	if in.SetupCompleted {
		setup = 1
	}
	_, err := s.exec(ctx, `
INSERT INTO app_settings (
  id, instance_name, sync_history_days, attention_long_running_after,
  retention_runs_days, retention_webhooks_days, retention_attention_days,
  server_external_url,
  gitea_url, gitea_token_cipher, gitea_webhook_secret_cipher,
  gitea_allow_private_network, gitea_allow_unsigned_webhooks,
  oauth_client_id, oauth_client_secret_cipher, setup_completed, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  instance_name=excluded.instance_name,
  sync_history_days=excluded.sync_history_days,
  attention_long_running_after=excluded.attention_long_running_after,
  retention_runs_days=excluded.retention_runs_days,
  retention_webhooks_days=excluded.retention_webhooks_days,
  retention_attention_days=excluded.retention_attention_days,
  server_external_url=excluded.server_external_url,
  gitea_url=excluded.gitea_url,
  gitea_token_cipher=excluded.gitea_token_cipher,
  gitea_webhook_secret_cipher=excluded.gitea_webhook_secret_cipher,
  gitea_allow_private_network=excluded.gitea_allow_private_network,
  gitea_allow_unsigned_webhooks=excluded.gitea_allow_unsigned_webhooks,
  oauth_client_id=excluded.oauth_client_id,
  oauth_client_secret_cipher=excluded.oauth_client_secret_cipher,
  setup_completed=excluded.setup_completed,
  updated_at=excluded.updated_at
`, in.InstanceName, in.SyncHistoryDays, in.AttentionLongRunningAfter,
		in.RetentionRunsDays, in.RetentionWebhooksDays, in.RetentionAttentionDays,
		in.ServerExternalURL,
		in.GiteaURL, in.GiteaTokenCipher, in.GiteaWebhookSecretCipher,
		nullInt64Arg(in.GiteaAllowPrivateNetwork), nullInt64Arg(in.GiteaAllowUnsignedWebhooks),
		in.OAuthClientID, in.OAuthClientSecretCipher, setup, now)
	return err
}

// UpsertAppSettingsValues updates only non-integration settings columns.
func (s *Store) UpsertAppSettingsValues(ctx context.Context, in AppSettings) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.exec(ctx, `
INSERT INTO app_settings (
  id, instance_name, sync_history_days, attention_long_running_after,
  retention_runs_days, retention_webhooks_days, retention_attention_days,
  server_external_url, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  instance_name=excluded.instance_name,
  sync_history_days=excluded.sync_history_days,
  attention_long_running_after=excluded.attention_long_running_after,
  retention_runs_days=excluded.retention_runs_days,
  retention_webhooks_days=excluded.retention_webhooks_days,
  retention_attention_days=excluded.retention_attention_days,
  server_external_url=excluded.server_external_url,
  updated_at=excluded.updated_at
`, in.InstanceName, in.SyncHistoryDays, in.AttentionLongRunningAfter,
		in.RetentionRunsDays, in.RetentionWebhooksDays, in.RetentionAttentionDays,
		in.ServerExternalURL, now)
	return err
}

// UpsertAppSettingsIntegration updates only Gitea/OAuth integration columns.
func (s *Store) UpsertAppSettingsIntegration(ctx context.Context, in AppSettings) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.exec(ctx, `
INSERT INTO app_settings (
  id, gitea_url, gitea_token_cipher, gitea_webhook_secret_cipher,
  gitea_allow_private_network, gitea_allow_unsigned_webhooks,
  oauth_client_id, oauth_client_secret_cipher, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  gitea_url=excluded.gitea_url,
  gitea_token_cipher=excluded.gitea_token_cipher,
  gitea_webhook_secret_cipher=excluded.gitea_webhook_secret_cipher,
  gitea_allow_private_network=excluded.gitea_allow_private_network,
  gitea_allow_unsigned_webhooks=excluded.gitea_allow_unsigned_webhooks,
  oauth_client_id=excluded.oauth_client_id,
  oauth_client_secret_cipher=excluded.oauth_client_secret_cipher,
  updated_at=excluded.updated_at
`, in.GiteaURL, in.GiteaTokenCipher, in.GiteaWebhookSecretCipher,
		nullInt64Arg(in.GiteaAllowPrivateNetwork), nullInt64Arg(in.GiteaAllowUnsignedWebhooks),
		in.OAuthClientID, in.OAuthClientSecretCipher, now)
	return err
}

// SetSetupCompleted sets the one-time setup wizard completion flag.
func (s *Store) SetSetupCompleted(ctx context.Context, completed bool) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	v := 0
	if completed {
		v = 1
	}
	_, err := s.exec(ctx, `
INSERT INTO app_settings (id, setup_completed, updated_at) VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  setup_completed=excluded.setup_completed,
  updated_at=excluded.updated_at
`, v, now)
	return err
}

func nullInt64Arg(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}
