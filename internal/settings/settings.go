// Package settings holds runtime Lens configuration overrides persisted in SQLite/Postgres.
package settings

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/config"
	lenscrypto "github.com/ncdlabs/gitea-lens/internal/crypto"
	"github.com/ncdlabs/gitea-lens/internal/store"
)

// Values are the user-editable Lens settings (non-secret).
type Values struct {
	InstanceName              string `json:"instance_name"`
	SyncHistoryDays           int    `json:"sync_history_days"`
	AttentionLongRunningAfter string `json:"attention_long_running_after"`
	RetentionRunsDays         int    `json:"retention_runs_days"`
	RetentionWebhooksDays     int    `json:"retention_webhooks_days"`
	RetentionAttentionDays    int    `json:"retention_attention_days"`
	ServerExternalURL         string `json:"server_external_url"`
}

// Integration is the effective Gitea/OAuth connection (secrets in memory only).
type Integration struct {
	URL                     string
	Token                   string
	WebhookSecret           string
	AllowPrivateNetwork     bool
	AllowUnsignedWebhooks   bool
	OAuthClientID           string
	OAuthClientSecret       string
}

// IntegrationPublic is the API-safe view (never includes secret values).
type IntegrationPublic struct {
	GiteaURL                       string `json:"gitea_url"`
	GiteaTokenConfigured           bool   `json:"gitea_token_configured"`
	GiteaWebhookSecretConfigured   bool   `json:"gitea_webhook_secret_configured"`
	GiteaAllowPrivateNetwork       bool   `json:"gitea_allow_private_network"`
	GiteaAllowUnsignedWebhooks     bool   `json:"gitea_allow_unsigned_webhooks"`
	OAuthClientID                  string `json:"oauth_client_id"`
	OAuthClientSecretConfigured    bool   `json:"oauth_client_secret_configured"`
}

// IntegrationPatch is a PUT body for integration fields.
// Empty secret strings leave the stored secret unchanged; clear_* flags clear them.
type IntegrationPatch struct {
	GiteaURL                     string `json:"gitea_url"`
	GiteaToken                   string `json:"gitea_token"`
	GiteaWebhookSecret           string `json:"gitea_webhook_secret"`
	ClearGiteaToken              bool   `json:"clear_gitea_token"`
	ClearGiteaWebhookSecret      bool   `json:"clear_gitea_webhook_secret"`
	GiteaAllowPrivateNetwork     bool   `json:"gitea_allow_private_network"`
	GiteaAllowUnsignedWebhooks   bool   `json:"gitea_allow_unsigned_webhooks"`
	OAuthClientID                string `json:"oauth_client_id"`
	OAuthClientSecret            string `json:"oauth_client_secret"`
	ClearOAuthClientSecret       bool   `json:"clear_oauth_client_secret"`
}

// OnIntegrationChange is invoked after a successful integration update (live apply).
type OnIntegrationChange func(Integration)

// OnExternalURLChange is invoked after server_external_url changes (live apply).
type OnExternalURLChange func(externalURL string)

// Manager merges file/env defaults with DB overrides and applies live updates.
type Manager struct {
	mu   sync.RWMutex
	base Values
	cur  Values

	baseInteg Integration
	integ     Integration
	// Raw DB ciphers / override markers (empty cipher = fall back to config on next Load).
	dbURL           string
	dbTokenCipher   string
	dbWebhookCipher string
	dbOAuthID       string
	dbOAuthCipher   string
	dbAllowPrivate  sql.NullInt64
	dbAllowUnsigned sql.NullInt64
	dbExternalURL   string
	setupCompleted  bool

	encKey  []byte
	st      *store.Store
	onInteg OnIntegrationChange
	onExt   OnExternalURLChange
}

// New builds a manager seeded from config defaults (before DB load).
func New(cfg config.Config, st *store.Store) *Manager {
	base := Values{
		InstanceName:              strings.TrimSpace(cfg.UI.InstanceName),
		SyncHistoryDays:           cfg.Sync.HistoryDays,
		AttentionLongRunningAfter: formatDuration(cfg.Attention.LongRunningAfter),
		RetentionRunsDays:         cfg.Retention.RunsDays,
		RetentionWebhooksDays:     cfg.Retention.WebhooksDays,
		RetentionAttentionDays:    cfg.Retention.AttentionDays,
		ServerExternalURL:         strings.TrimSpace(cfg.Server.ExternalURL),
	}
	if base.InstanceName == "" {
		base.InstanceName = "Gitea Lens"
	}
	if base.AttentionLongRunningAfter == "" {
		base.AttentionLongRunningAfter = "2h"
	}
	baseInteg := Integration{
		URL:                   strings.TrimSpace(cfg.Gitea.URL),
		Token:                 cfg.Gitea.Token,
		WebhookSecret:         cfg.Gitea.WebhookSecret,
		AllowPrivateNetwork:   cfg.Gitea.AllowPrivateNetwork,
		AllowUnsignedWebhooks: cfg.Gitea.AllowUnsignedWebhooks,
		OAuthClientID:         strings.TrimSpace(cfg.Auth.OAuthClientID),
		OAuthClientSecret:     cfg.Auth.OAuthClientSecret,
	}
	var encKey []byte
	if cfg.Auth.EncryptionKey != "" {
		k, err := lenscrypto.KeyFromString(cfg.Auth.EncryptionKey)
		if err != nil {
			// Invalid keys must not silently disable encryption (fail closed at Load/Validate).
			encKey = nil
		} else {
			encKey = k
		}
	}
	return &Manager{
		base: base, cur: base,
		baseInteg: baseInteg, integ: baseInteg,
		encKey: encKey, st: st,
	}
}

// SetOnIntegrationChange registers a live-apply callback (optional).
func (m *Manager) SetOnIntegrationChange(fn OnIntegrationChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onInteg = fn
}

// SetOnExternalURLChange registers a live-apply callback for the public Lens URL.
func (m *Manager) SetOnExternalURLChange(fn OnExternalURLChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onExt = fn
}

// Load merges persisted overrides from the database onto defaults.
func (m *Manager) Load(ctx context.Context) error {
	row, err := m.st.GetAppSettings(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cur = merge(m.base, row)
	integ, err := m.mergeIntegrationLocked(row)
	if err != nil {
		return err
	}
	m.integ = integ
	if row != nil {
		m.dbURL = row.GiteaURL
		m.dbTokenCipher = row.GiteaTokenCipher
		m.dbWebhookCipher = row.GiteaWebhookSecretCipher
		m.dbOAuthID = row.OAuthClientID
		m.dbOAuthCipher = row.OAuthClientSecretCipher
		m.dbAllowPrivate = row.GiteaAllowPrivateNetwork
		m.dbAllowUnsigned = row.GiteaAllowUnsignedWebhooks
		m.dbExternalURL = row.ServerExternalURL
		m.setupCompleted = row.SetupCompleted
	} else {
		m.dbURL = ""
		m.dbTokenCipher = ""
		m.dbWebhookCipher = ""
		m.dbOAuthID = ""
		m.dbOAuthCipher = ""
		m.dbAllowPrivate = sql.NullInt64{}
		m.dbAllowUnsigned = sql.NullInt64{}
		m.dbExternalURL = ""
		m.setupCompleted = false
	}
	return nil
}

// Get returns the effective settings.
func (m *Manager) Get() Values {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := m.cur
	if strings.TrimSpace(out.ServerExternalURL) == "" {
		out.ServerExternalURL = m.base.ServerExternalURL
	}
	return out
}

// ExternalURL returns the effective public Lens URL (DB override or config).
func (m *Manager) ExternalURL() string {
	return strings.TrimRight(strings.TrimSpace(m.Get().ServerExternalURL), "/")
}

// Integration returns the effective Gitea/OAuth connection (includes secrets).
func (m *Manager) Integration() Integration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.integ
}

// IntegrationPublic returns the API-safe integration view.
func (m *Manager) IntegrationPublic() IntegrationPublic {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return publicFrom(m.integ)
}

// SetupCompleted reports whether the setup wizard has been finished.
func (m *Manager) SetupCompleted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.setupCompleted
}

// GiteaConnection implements sync.GiteaSource.
func (m *Manager) GiteaConnection() (url, token string, allowPrivate bool) {
	i := m.Integration()
	return i.URL, i.Token, i.AllowPrivateNetwork
}

// Update validates, persists, and applies new non-integration settings.
func (m *Manager) Update(ctx context.Context, next Values) (Values, error) {
	cleaned, err := Validate(next)
	if err != nil {
		return Values{}, err
	}
	if err := m.st.UpsertAppSettingsValues(ctx, store.AppSettings{
		InstanceName:              cleaned.InstanceName,
		SyncHistoryDays:           cleaned.SyncHistoryDays,
		AttentionLongRunningAfter: cleaned.AttentionLongRunningAfter,
		RetentionRunsDays:         cleaned.RetentionRunsDays,
		RetentionWebhooksDays:     cleaned.RetentionWebhooksDays,
		RetentionAttentionDays:    cleaned.RetentionAttentionDays,
		ServerExternalURL:         cleaned.ServerExternalURL,
	}); err != nil {
		return Values{}, err
	}
	m.mu.Lock()
	m.cur = cleaned
	m.dbExternalURL = cleaned.ServerExternalURL
	onExt := m.onExt
	effective := cleaned.ServerExternalURL
	if effective == "" {
		effective = m.base.ServerExternalURL
	}
	m.mu.Unlock()
	if onExt != nil {
		onExt(effective)
	}
	return m.Get(), nil
}

// UpdateIntegration validates, encrypts secrets, persists, and live-applies integration settings.
func (m *Manager) UpdateIntegration(ctx context.Context, patch IntegrationPatch) (IntegrationPublic, error) {
	m.mu.Lock()
	cur := m.integ
	dbToken := m.dbTokenCipher
	dbWebhook := m.dbWebhookCipher
	dbOAuth := m.dbOAuthCipher
	onChange := m.onInteg
	m.mu.Unlock()

	next := cur
	next.URL = strings.TrimSpace(patch.GiteaURL)
	next.AllowPrivateNetwork = patch.GiteaAllowPrivateNetwork
	next.AllowUnsignedWebhooks = patch.GiteaAllowUnsignedWebhooks
	next.OAuthClientID = strings.TrimSpace(patch.OAuthClientID)

	if patch.ClearGiteaToken {
		next.Token = ""
		dbToken = ""
	} else if strings.TrimSpace(patch.GiteaToken) != "" {
		next.Token = strings.TrimSpace(patch.GiteaToken)
	}
	if patch.ClearGiteaWebhookSecret {
		next.WebhookSecret = ""
		dbWebhook = ""
	} else if strings.TrimSpace(patch.GiteaWebhookSecret) != "" {
		next.WebhookSecret = strings.TrimSpace(patch.GiteaWebhookSecret)
	}
	if patch.ClearOAuthClientSecret {
		next.OAuthClientSecret = ""
		dbOAuth = ""
	} else if strings.TrimSpace(patch.OAuthClientSecret) != "" {
		next.OAuthClientSecret = strings.TrimSpace(patch.OAuthClientSecret)
	}

	if err := ValidateIntegration(next); err != nil {
		return IntegrationPublic{}, err
	}

	// Empty secret on PUT = leave DB cipher unchanged (may still be empty → config fallback).
	tokenCipher := dbToken
	webhookCipher := dbWebhook
	oauthCipher := dbOAuth
	var err error
	if patch.ClearGiteaToken {
		tokenCipher = ""
	} else if strings.TrimSpace(patch.GiteaToken) != "" {
		tokenCipher, err = m.seal(strings.TrimSpace(patch.GiteaToken))
		if err != nil {
			return IntegrationPublic{}, err
		}
	}
	if patch.ClearGiteaWebhookSecret {
		webhookCipher = ""
	} else if strings.TrimSpace(patch.GiteaWebhookSecret) != "" {
		webhookCipher, err = m.seal(strings.TrimSpace(patch.GiteaWebhookSecret))
		if err != nil {
			return IntegrationPublic{}, err
		}
	}
	if patch.ClearOAuthClientSecret {
		oauthCipher = ""
	} else if strings.TrimSpace(patch.OAuthClientSecret) != "" {
		oauthCipher, err = m.seal(strings.TrimSpace(patch.OAuthClientSecret))
		if err != nil {
			return IntegrationPublic{}, err
		}
	}

	allowPrivate := sql.NullInt64{Valid: true, Int64: boolToInt(next.AllowPrivateNetwork)}
	allowUnsigned := sql.NullInt64{Valid: true, Int64: boolToInt(next.AllowUnsignedWebhooks)}

	row := store.AppSettings{
		GiteaURL:                   next.URL,
		GiteaTokenCipher:           tokenCipher,
		GiteaWebhookSecretCipher:   webhookCipher,
		GiteaAllowPrivateNetwork:   allowPrivate,
		GiteaAllowUnsignedWebhooks: allowUnsigned,
		OAuthClientID:              next.OAuthClientID,
		OAuthClientSecretCipher:    oauthCipher,
	}
	if err := m.st.UpsertAppSettingsIntegration(ctx, row); err != nil {
		return IntegrationPublic{}, err
	}

	m.mu.Lock()
	m.integ = next
	m.dbURL = next.URL
	m.dbTokenCipher = tokenCipher
	m.dbWebhookCipher = webhookCipher
	m.dbOAuthID = next.OAuthClientID
	m.dbOAuthCipher = oauthCipher
	m.dbAllowPrivate = allowPrivate
	m.dbAllowUnsigned = allowUnsigned
	m.mu.Unlock()

	if onChange != nil {
		onChange(next)
	}
	return publicFrom(next), nil
}

// SetSetupCompleted persists and applies the setup wizard completion flag.
func (m *Manager) SetSetupCompleted(ctx context.Context, completed bool) error {
	if err := m.st.SetSetupCompleted(ctx, completed); err != nil {
		return err
	}
	m.mu.Lock()
	m.setupCompleted = completed
	m.mu.Unlock()
	return nil
}

// LongRunningAfter parses the effective duration (falls back to 2h).
func (m *Manager) LongRunningAfter() time.Duration {
	v := m.Get()
	d, err := time.ParseDuration(v.AttentionLongRunningAfter)
	if err != nil || d <= 0 {
		return 2 * time.Hour
	}
	return d
}

// Retention returns the effective retention windows.
func (m *Manager) Retention() config.RetentionConfig {
	v := m.Get()
	return config.RetentionConfig{
		RunsDays:      v.RetentionRunsDays,
		WebhooksDays:  v.RetentionWebhooksDays,
		AttentionDays: v.RetentionAttentionDays,
	}
}

// Validate normalizes and checks editable settings.
func Validate(v Values) (Values, error) {
	out := Values{
		InstanceName:              strings.TrimSpace(v.InstanceName),
		SyncHistoryDays:           v.SyncHistoryDays,
		AttentionLongRunningAfter: strings.TrimSpace(v.AttentionLongRunningAfter),
		RetentionRunsDays:         v.RetentionRunsDays,
		RetentionWebhooksDays:     v.RetentionWebhooksDays,
		RetentionAttentionDays:    v.RetentionAttentionDays,
		ServerExternalURL:         strings.TrimRight(strings.TrimSpace(v.ServerExternalURL), "/"),
	}
	if out.InstanceName == "" {
		return Values{}, fmt.Errorf("instance_name is required")
	}
	if len(out.InstanceName) > 100 {
		return Values{}, fmt.Errorf("instance_name must be at most 100 characters")
	}
	if out.SyncHistoryDays < 1 || out.SyncHistoryDays > 365 {
		return Values{}, fmt.Errorf("sync_history_days must be between 1 and 365")
	}
	d, err := time.ParseDuration(out.AttentionLongRunningAfter)
	if err != nil {
		return Values{}, fmt.Errorf("attention_long_running_after must be a duration like 2h")
	}
	if d < 15*time.Minute || d > 7*24*time.Hour {
		return Values{}, fmt.Errorf("attention_long_running_after must be between 15m and 168h")
	}
	out.AttentionLongRunningAfter = formatDuration(d)
	for _, pair := range []struct {
		name string
		val  int
	}{
		{"retention_runs_days", out.RetentionRunsDays},
		{"retention_webhooks_days", out.RetentionWebhooksDays},
		{"retention_attention_days", out.RetentionAttentionDays},
	} {
		if pair.val < 0 || pair.val > 3650 {
			return Values{}, fmt.Errorf("%s must be between 0 and 3650 (0 disables purge)", pair.name)
		}
	}
	if out.ServerExternalURL != "" {
		u, err := url.Parse(out.ServerExternalURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Values{}, fmt.Errorf("server_external_url must be a valid http:// or https:// URL")
		}
	}
	return out, nil
}

// ValidateIntegration checks Gitea integration invariants.
func ValidateIntegration(i Integration) error {
	url := strings.TrimSpace(i.URL)
	if url != "" && strings.TrimSpace(i.WebhookSecret) == "" && !i.AllowUnsignedWebhooks {
		return fmt.Errorf("gitea_webhook_secret is required when gitea_url is set (or enable gitea_allow_unsigned_webhooks)")
	}
	return nil
}

func (m *Manager) mergeIntegrationLocked(row *store.AppSettings) (Integration, error) {
	out := m.baseInteg
	if row == nil {
		return out, nil
	}
	if strings.TrimSpace(row.GiteaURL) != "" {
		out.URL = strings.TrimSpace(row.GiteaURL)
	}
	if row.GiteaTokenCipher != "" {
		tok, err := m.open(row.GiteaTokenCipher)
		if err != nil {
			return Integration{}, fmt.Errorf("decrypt gitea token: %w", err)
		}
		out.Token = tok
	}
	if row.GiteaWebhookSecretCipher != "" {
		sec, err := m.open(row.GiteaWebhookSecretCipher)
		if err != nil {
			return Integration{}, fmt.Errorf("decrypt webhook secret: %w", err)
		}
		out.WebhookSecret = sec
	}
	if row.GiteaAllowPrivateNetwork.Valid {
		out.AllowPrivateNetwork = row.GiteaAllowPrivateNetwork.Int64 != 0
	}
	if row.GiteaAllowUnsignedWebhooks.Valid {
		out.AllowUnsignedWebhooks = row.GiteaAllowUnsignedWebhooks.Int64 != 0
	}
	if strings.TrimSpace(row.OAuthClientID) != "" {
		out.OAuthClientID = strings.TrimSpace(row.OAuthClientID)
	}
	if row.OAuthClientSecretCipher != "" {
		sec, err := m.open(row.OAuthClientSecretCipher)
		if err != nil {
			return Integration{}, fmt.Errorf("decrypt oauth client secret: %w", err)
		}
		out.OAuthClientSecret = sec
	}
	return out, nil
}

func (m *Manager) seal(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(m.encKey) != 32 {
		return "", fmt.Errorf("LENS_ENCRYPTION_KEY is required to store secrets in the database")
	}
	return lenscrypto.Encrypt(m.encKey, plaintext)
}

func (m *Manager) open(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if len(m.encKey) != 32 {
		if lenscrypto.LooksLikeCiphertext(stored) {
			return "", fmt.Errorf("LENS_ENCRYPTION_KEY is required to decrypt stored secrets")
		}
		// No key: treat as legacy plaintext row (pre-encryption installs).
		return stored, nil
	}
	pt, err := lenscrypto.Decrypt(m.encKey, stored)
	if err != nil {
		return "", fmt.Errorf("decrypt failed (re-enter the secret if it was stored before encryption was enabled): %w", err)
	}
	return pt, nil
}

func publicFrom(i Integration) IntegrationPublic {
	return IntegrationPublic{
		GiteaURL:                     i.URL,
		GiteaTokenConfigured:         i.Token != "",
		GiteaWebhookSecretConfigured: i.WebhookSecret != "",
		GiteaAllowPrivateNetwork:     i.AllowPrivateNetwork,
		GiteaAllowUnsignedWebhooks:   i.AllowUnsignedWebhooks,
		OAuthClientID:                i.OAuthClientID,
		OAuthClientSecretConfigured:  i.OAuthClientSecret != "",
	}
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func merge(base Values, row *store.AppSettings) Values {
	if row == nil {
		return base
	}
	// Upsert writes the full settings document; treat a present row as authoritative.
	out := Values{
		InstanceName:              strings.TrimSpace(row.InstanceName),
		SyncHistoryDays:           row.SyncHistoryDays,
		AttentionLongRunningAfter: strings.TrimSpace(row.AttentionLongRunningAfter),
		RetentionRunsDays:         row.RetentionRunsDays,
		RetentionWebhooksDays:     row.RetentionWebhooksDays,
		RetentionAttentionDays:    row.RetentionAttentionDays,
		ServerExternalURL:         strings.TrimSpace(row.ServerExternalURL),
	}
	if out.InstanceName == "" {
		out.InstanceName = base.InstanceName
	}
	if out.SyncHistoryDays <= 0 {
		out.SyncHistoryDays = base.SyncHistoryDays
	}
	if out.AttentionLongRunningAfter == "" {
		out.AttentionLongRunningAfter = base.AttentionLongRunningAfter
	}
	if out.ServerExternalURL == "" {
		out.ServerExternalURL = base.ServerExternalURL
	}
	return out
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return d.String()
}
