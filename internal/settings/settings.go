// Package settings holds runtime GitSeer configuration overrides persisted in SQLite/Postgres.
package settings

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ncdlabs/gitseer/internal/config"
	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

// Values are the user-editable GitSeer settings (non-secret).
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
	URL                   string
	Token                 string
	WebhookSecret         string
	AllowPrivateNetwork   bool
	AllowUnsignedWebhooks bool
	OAuthClientID         string
	OAuthClientSecret     string
}

// GitHubIntegration is the effective GitHub connection (service PAT + optional OAuth App).
type GitHubIntegration struct {
	URL                   string
	Token                 string
	WebhookSecret         string
	AllowPrivateNetwork   bool
	AllowUnsignedWebhooks bool
	OAuthClientID         string
	OAuthClientSecret     string
}

// IntegrationPublic is the API-safe view (never includes secret values).
// Flat gitea_* / github_* fields; OAuth client fields remain Gitea-only.
type IntegrationPublic struct {
	GiteaURL                     string `json:"gitea_url"`
	GiteaTokenConfigured         bool   `json:"gitea_token_configured"`
	GiteaWebhookSecretConfigured bool   `json:"gitea_webhook_secret_configured"`
	GiteaAllowPrivateNetwork     bool   `json:"gitea_allow_private_network"`
	GiteaAllowUnsignedWebhooks   bool   `json:"gitea_allow_unsigned_webhooks"`
	OAuthClientID                string `json:"oauth_client_id"`
	OAuthClientSecretConfigured  bool   `json:"oauth_client_secret_configured"`

	GitHubURL                     string `json:"github_url"`
	GitHubTokenConfigured         bool   `json:"github_token_configured"`
	GitHubWebhookSecretConfigured bool   `json:"github_webhook_secret_configured"`
	GitHubAllowPrivateNetwork     bool   `json:"github_allow_private_network"`
	GitHubAllowUnsignedWebhooks   bool   `json:"github_allow_unsigned_webhooks"`
}

// IntegrationPatch is a PUT body for integration fields.
// Empty secret strings leave the stored secret unchanged; clear_* flags clear them.
// GitHub fields are applied only when ApplyGitHub is true (Settings Integration form);
// setup Gitea handlers leave ApplyGitHub false so GitHub state is preserved.
type IntegrationPatch struct {
	GiteaURL                   string `json:"gitea_url"`
	GiteaToken                 string `json:"gitea_token"`
	GiteaWebhookSecret         string `json:"gitea_webhook_secret"`
	ClearGiteaToken            bool   `json:"clear_gitea_token"`
	ClearGiteaWebhookSecret    bool   `json:"clear_gitea_webhook_secret"`
	GiteaAllowPrivateNetwork   bool   `json:"gitea_allow_private_network"`
	GiteaAllowUnsignedWebhooks bool   `json:"gitea_allow_unsigned_webhooks"`
	OAuthClientID              string `json:"oauth_client_id"`
	OAuthClientSecret          string `json:"oauth_client_secret"`
	ClearOAuthClientSecret     bool   `json:"clear_oauth_client_secret"`

	ApplyGitHub                 bool   `json:"apply_github"`
	GitHubURL                   string `json:"github_url"`
	GitHubToken                 string `json:"github_token"`
	GitHubWebhookSecret         string `json:"github_webhook_secret"`
	ClearGitHubToken            bool   `json:"clear_github_token"`
	ClearGitHubWebhookSecret    bool   `json:"clear_github_webhook_secret"`
	GitHubAllowPrivateNetwork   bool   `json:"github_allow_private_network"`
	GitHubAllowUnsignedWebhooks bool   `json:"github_allow_unsigned_webhooks"`
}

// OnIntegrationChange is invoked after a successful integration update (live apply).
type OnIntegrationChange func(Integration)

// OnGitHubChange is invoked after GitHub integration/instance changes (OAuth live apply).
type OnGitHubChange func(GitHubIntegration)

// OnMultiForgeOAuthChange is invoked when instance OAuth credentials may have changed
// (GitLab / Bitbucket / Forgejo). Callers re-read Auth*Integration helpers.
type OnMultiForgeOAuthChange func()

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

	baseGitHub GitHubIntegration
	github     GitHubIntegration
	// Last known GitHub instance base URL (normalized) for upsert after URL changes.
	githubInstanceURL string

	encKey     []byte
	encSource  string
	encKeyPath string
	st         *store.Store
	onInteg    OnIntegrationChange
	onGitHub   OnGitHubChange
	onMultiOAuth OnMultiForgeOAuthChange
	onExt      OnExternalURLChange
	onEnc      OnEncryptionChange
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
		base.InstanceName = "GitSeer"
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
	baseGitHub := GitHubIntegration{
		URL:                   strings.TrimSpace(cfg.GitHub.URL),
		Token:                 cfg.GitHub.Token,
		WebhookSecret:         cfg.GitHub.WebhookSecret,
		AllowPrivateNetwork:   cfg.GitHub.AllowPrivateNetwork,
		AllowUnsignedWebhooks: cfg.GitHub.AllowUnsignedWebhooks,
	}
	encKey, encSource := loadKeyFromConfig(cfg)
	encKeyPath := DefaultEncryptionKeyPath(cfg)
	if encKey == nil {
		if k, ok := loadKeyFromFile(encKeyPath); ok {
			encKey = k
			encSource = EncryptionSourceFile
		}
	}
	return &Manager{
		base: base, cur: base,
		baseInteg: baseInteg, integ: baseInteg,
		baseGitHub: baseGitHub, github: baseGitHub,
		encKey: encKey, encSource: encSource, encKeyPath: encKeyPath, st: st,
	}
}

// SetOnIntegrationChange registers a live-apply callback (optional).
func (m *Manager) SetOnIntegrationChange(fn OnIntegrationChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onInteg = fn
}

// SetOnGitHubChange registers a live-apply callback for GitHub OAuth credentials.
func (m *Manager) SetOnGitHubChange(fn OnGitHubChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onGitHub = fn
}

// SetOnMultiForgeOAuthChange registers a live-apply callback for GitLab/Bitbucket/Forgejo OAuth.
func (m *Manager) SetOnMultiForgeOAuthChange(fn OnMultiForgeOAuthChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMultiOAuth = fn
}

// SetOnExternalURLChange registers a live-apply callback for the public GitSeer URL.
func (m *Manager) SetOnExternalURLChange(fn OnExternalURLChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onExt = fn
}

// Load merges persisted overrides from the database onto defaults.
// When a Gitea URL is configured but no matching instances row holds secrets,
// seeds/upserts a gitea instance from app_settings (one-time migrate path).
// GitHub is loaded from instances (and config defaults); seeded when config has a URL.
func (m *Manager) Load(ctx context.Context) error {
	row, err := m.st.GetAppSettings(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.cur = merge(m.base, row)
	integ, err := m.mergeIntegrationLocked(row)
	if err != nil {
		m.mu.Unlock()
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
	seedName := strings.TrimSpace(m.cur.InstanceName)
	seedInteg := m.integ
	tokenCipher := m.dbTokenCipher
	webhookCipher := m.dbWebhookCipher
	oauthCipher := m.dbOAuthCipher
	oauthID := m.dbOAuthID
	extURL := m.dbExternalURL
	if extURL == "" {
		extURL = m.cur.ServerExternalURL
	}
	if extURL == "" {
		extURL = m.base.ServerExternalURL
	}
	baseGitHub := m.baseGitHub
	m.mu.Unlock()

	if err := m.seedGiteaInstanceFromSettings(ctx, seedName, seedInteg, tokenCipher, webhookCipher, oauthID, oauthCipher, extURL); err != nil {
		return fmt.Errorf("seed gitea instance from settings: %w", err)
	}
	if err := m.loadAndSeedGitHub(ctx, seedName, baseGitHub, extURL); err != nil {
		return fmt.Errorf("load github integration: %w", err)
	}
	return nil
}

// seedGiteaInstanceFromSettings upserts a gitea instances row when a URL is configured
// and the matching instance does not yet hold sync/webhook secret ciphertext.
func (m *Manager) seedGiteaInstanceFromSettings(
	ctx context.Context,
	name string,
	integ Integration,
	tokenCipher, webhookCipher, oauthID, oauthCipher, externalURL string,
) error {
	baseURL := strings.TrimSpace(integ.URL)
	if baseURL == "" {
		return nil
	}

	inst, err := m.st.GetInstanceByForgeAndURL(ctx, models.ForgeTypeGitea, baseURL)
	if err != nil {
		return err
	}
	if store.InstanceHasSecrets(inst) {
		return nil
	}
	// Also treat a same-URL row (legacy sync upsert, forge_type may already be gitea) as seeded
	// when it already has secrets — GetInstanceByForgeAndURL misses forge_type mismatches.
	if inst == nil {
		byURL, err := m.st.GetInstanceByURL(ctx, baseURL)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if store.InstanceHasSecrets(byURL) {
			return nil
		}
	}

	if tokenCipher == "" && strings.TrimSpace(integ.Token) != "" {
		if sealed, sealErr := m.seal(strings.TrimSpace(integ.Token)); sealErr == nil {
			tokenCipher = sealed
		}
	}
	if webhookCipher == "" && strings.TrimSpace(integ.WebhookSecret) != "" {
		if sealed, sealErr := m.seal(strings.TrimSpace(integ.WebhookSecret)); sealErr == nil {
			webhookCipher = sealed
		}
	}
	if oauthCipher == "" && strings.TrimSpace(integ.OAuthClientSecret) != "" {
		if sealed, sealErr := m.seal(strings.TrimSpace(integ.OAuthClientSecret)); sealErr == nil {
			oauthCipher = sealed
		}
	}
	if oauthID == "" {
		oauthID = strings.TrimSpace(integ.OAuthClientID)
	}
	if name == "" {
		name = "Gitea"
	}

	_, err = m.st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                    name,
		ForgeType:               models.ForgeTypeGitea,
		BaseURL:                 baseURL,
		SyncTokenCiphertext:     tokenCipher,
		WebhookSecretCiphertext: webhookCipher,
		OAuthClientID:           oauthID,
		OAuthClientSecretCipher: oauthCipher,
		ExternalURL:             strings.TrimSpace(externalURL),
		AllowPrivateNetwork:     integ.AllowPrivateNetwork,
		AllowUnsignedWebhooks:   integ.AllowUnsignedWebhooks,
	})
	return err
}

// loadAndSeedGitHub merges config defaults with the first matching github instance row,
// and seeds an instances row when config has a GitHub URL but the instance lacks secrets.
func (m *Manager) loadAndSeedGitHub(ctx context.Context, name string, base GitHubIntegration, externalURL string) error {
	out := base
	instURL := ""

	list, err := m.st.ListInstances(ctx)
	if err != nil {
		return err
	}
	var selected *models.Instance
	wantURL := strings.TrimSpace(base.URL)
	if wantURL != "" {
		if norm, nerr := normalizeGitHubURL(wantURL); nerr == nil {
			wantURL = norm
			out.URL = norm
		}
	}
	for i := range list {
		if list[i].ForgeType != models.ForgeTypeGitHub {
			continue
		}
		if wantURL != "" && list[i].BaseURL == wantURL {
			selected = &list[i]
			break
		}
		if selected == nil {
			selected = &list[i]
		}
	}
	if selected != nil {
		instURL = selected.BaseURL
		out.URL = selected.BaseURL
		out.AllowPrivateNetwork = selected.AllowPrivateNetwork
		out.AllowUnsignedWebhooks = selected.AllowUnsignedWebhooks
		if selected.SyncTokenCiphertext != "" {
			tok, err := m.open(selected.SyncTokenCiphertext)
			if err != nil {
				return fmt.Errorf("decrypt github token: %w", err)
			}
			out.Token = tok
		}
		if selected.WebhookSecretCiphertext != "" {
			sec, err := m.open(selected.WebhookSecretCiphertext)
			if err != nil {
				return fmt.Errorf("decrypt github webhook secret: %w", err)
			}
			out.WebhookSecret = sec
		}
	}

	m.mu.Lock()
	m.github = out
	m.githubInstanceURL = instURL
	m.mu.Unlock()

	if strings.TrimSpace(out.URL) == "" {
		return nil
	}
	if store.InstanceHasSecrets(selected) {
		return nil
	}
	return m.upsertGitHubInstance(ctx, name, out, "", "", externalURL)
}

func normalizeGitHubURL(raw string) (string, error) {
	// Late import avoided: duplicate minimal trim; callers that need full normalize
	// use forge/github.NormalizeBaseURL via upsert path.
	return strings.TrimRight(strings.TrimSpace(raw), "/"), nil
}

func (m *Manager) upsertGitHubInstance(ctx context.Context, name string, gh GitHubIntegration, tokenCipher, webhookCipher, externalURL string) error {
	baseURL := strings.TrimSpace(gh.URL)
	if baseURL == "" {
		return nil
	}
	// Prefer canonical API root when the github package is available via a light helper.
	if norm, err := tryNormalizeGitHubAPI(baseURL); err == nil {
		baseURL = norm
	}
	if name == "" {
		name = "GitHub"
	}
	if tokenCipher == "" && strings.TrimSpace(gh.Token) != "" {
		sealed, err := m.seal(strings.TrimSpace(gh.Token))
		if err != nil {
			return err
		}
		tokenCipher = sealed
	}
	if webhookCipher == "" && strings.TrimSpace(gh.WebhookSecret) != "" {
		sealed, err := m.seal(strings.TrimSpace(gh.WebhookSecret))
		if err != nil {
			return err
		}
		webhookCipher = sealed
	}
	inst, err := m.st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                    name,
		ForgeType:               models.ForgeTypeGitHub,
		BaseURL:                 baseURL,
		SyncTokenCiphertext:     tokenCipher,
		WebhookSecretCiphertext: webhookCipher,
		ExternalURL:             strings.TrimSpace(externalURL),
		AllowPrivateNetwork:     gh.AllowPrivateNetwork,
		AllowUnsignedWebhooks:   gh.AllowUnsignedWebhooks,
	})
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.github.URL = baseURL
	if inst != nil {
		m.githubInstanceURL = inst.BaseURL
	} else {
		m.githubInstanceURL = baseURL
	}
	m.mu.Unlock()
	return nil
}

// tryNormalizeGitHubAPI is set from an init in github_norm.go when linked;
// default keeps the trimmed URL.
var tryNormalizeGitHubAPI = func(raw string) (string, error) {
	return strings.TrimRight(strings.TrimSpace(raw), "/"), nil
}

// tryNormalizeGitLabAPI is set from gitlab_norm.go init.
var tryNormalizeGitLabAPI = func(raw string) (string, error) {
	return strings.TrimRight(strings.TrimSpace(raw), "/"), nil
}

// tryNormalizeBitbucketAPI is set from bitbucket_norm.go init.
var tryNormalizeBitbucketAPI = func(raw string) (string, error) {
	return strings.TrimRight(strings.TrimSpace(raw), "/"), nil
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

// ExternalURL returns the effective public GitSeer URL (DB override or config).
func (m *Manager) ExternalURL() string {
	return strings.TrimRight(strings.TrimSpace(m.Get().ServerExternalURL), "/")
}

// Integration returns the effective Gitea/OAuth connection (includes secrets).
func (m *Manager) Integration() Integration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.integ
}

// GitHub returns the effective GitHub connection (includes secrets).
func (m *Manager) GitHub() GitHubIntegration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.github
}

// IntegrationPublic returns the API-safe integration view.
func (m *Manager) IntegrationPublic() IntegrationPublic {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return publicFrom(m.integ, m.github)
}

// AnyForgeConfigured reports whether at least one forge has URL + token in the
// effective in-memory Gitea/GitHub integration (from config file/env and DB).
// Prefer HasUsableForge when DB instances (GitLab/Bitbucket/Forgejo) must count.
func (m *Manager) AnyForgeConfigured() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	giteaOK := strings.TrimSpace(m.integ.URL) != "" && strings.TrimSpace(m.integ.Token) != ""
	githubOK := strings.TrimSpace(m.github.URL) != "" && strings.TrimSpace(m.github.Token) != ""
	return giteaOK || githubOK
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
// Always updates Gitea (app_settings + instances upsert when URL set).
// Updates GitHub only when patch.ApplyGitHub is true (instances upsert when URL set).
func (m *Manager) UpdateIntegration(ctx context.Context, patch IntegrationPatch) (IntegrationPublic, error) {
	m.mu.Lock()
	cur := m.integ
	dbToken := m.dbTokenCipher
	dbWebhook := m.dbWebhookCipher
	dbOAuth := m.dbOAuthCipher
	onChange := m.onInteg
	curGitHub := m.github
	instanceName := strings.TrimSpace(m.cur.InstanceName)
	extURL := m.dbExternalURL
	if extURL == "" {
		extURL = m.cur.ServerExternalURL
	}
	if extURL == "" {
		extURL = m.base.ServerExternalURL
	}
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

	nextGitHub := curGitHub
	if patch.ApplyGitHub {
		nextGitHub = curGitHub
		rawURL := strings.TrimSpace(patch.GitHubURL)
		if rawURL != "" {
			if norm, err := tryNormalizeGitHubAPI(rawURL); err == nil {
				nextGitHub.URL = norm
			} else {
				nextGitHub.URL = rawURL
			}
		} else {
			nextGitHub.URL = ""
		}
		nextGitHub.AllowPrivateNetwork = patch.GitHubAllowPrivateNetwork
		nextGitHub.AllowUnsignedWebhooks = patch.GitHubAllowUnsignedWebhooks
		if patch.ClearGitHubToken {
			nextGitHub.Token = ""
		} else if strings.TrimSpace(patch.GitHubToken) != "" {
			nextGitHub.Token = strings.TrimSpace(patch.GitHubToken)
		}
		if patch.ClearGitHubWebhookSecret {
			nextGitHub.WebhookSecret = ""
		} else if strings.TrimSpace(patch.GitHubWebhookSecret) != "" {
			nextGitHub.WebhookSecret = strings.TrimSpace(patch.GitHubWebhookSecret)
		}
		if err := ValidateGitHubIntegration(nextGitHub); err != nil {
			return IntegrationPublic{}, err
		}
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

	// Persist Gitea instance secrets when a URL is configured.
	if strings.TrimSpace(next.URL) != "" {
		name := instanceName
		if name == "" {
			name = "Gitea"
		}
		if _, err := m.st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
			Name:                    name,
			ForgeType:               models.ForgeTypeGitea,
			BaseURL:                 next.URL,
			SyncTokenCiphertext:     tokenCipher,
			ClearSyncToken:          patch.ClearGiteaToken,
			WebhookSecretCiphertext: webhookCipher,
			ClearWebhookSecret:      patch.ClearGiteaWebhookSecret,
			OAuthClientID:           next.OAuthClientID,
			OAuthClientSecretCipher: oauthCipher,
			ClearOAuthClientSecret:  patch.ClearOAuthClientSecret,
			ExternalURL:             strings.TrimSpace(extURL),
			AllowPrivateNetwork:     next.AllowPrivateNetwork,
			AllowUnsignedWebhooks:   next.AllowUnsignedWebhooks,
		}); err != nil {
			return IntegrationPublic{}, fmt.Errorf("upsert gitea instance secrets: %w", err)
		}
	}

	if patch.ApplyGitHub {
		var ghTokenCipher, ghWebhookCipher string
		if patch.ClearGitHubToken {
			ghTokenCipher = ""
		} else if strings.TrimSpace(patch.GitHubToken) != "" {
			ghTokenCipher, err = m.seal(strings.TrimSpace(patch.GitHubToken))
			if err != nil {
				return IntegrationPublic{}, err
			}
		}
		if patch.ClearGitHubWebhookSecret {
			ghWebhookCipher = ""
		} else if strings.TrimSpace(patch.GitHubWebhookSecret) != "" {
			ghWebhookCipher, err = m.seal(strings.TrimSpace(patch.GitHubWebhookSecret))
			if err != nil {
				return IntegrationPublic{}, err
			}
		}
		if strings.TrimSpace(nextGitHub.URL) != "" {
			name := instanceName
			if name == "" {
				name = "GitHub"
			}
			baseURL := nextGitHub.URL
			if _, err := m.st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
				Name:                    name,
				ForgeType:               models.ForgeTypeGitHub,
				BaseURL:                 baseURL,
				SyncTokenCiphertext:     ghTokenCipher,
				ClearSyncToken:          patch.ClearGitHubToken,
				WebhookSecretCiphertext: ghWebhookCipher,
				ClearWebhookSecret:      patch.ClearGitHubWebhookSecret,
				ExternalURL:             strings.TrimSpace(extURL),
				AllowPrivateNetwork:     nextGitHub.AllowPrivateNetwork,
				AllowUnsignedWebhooks:   nextGitHub.AllowUnsignedWebhooks,
			}); err != nil {
				return IntegrationPublic{}, fmt.Errorf("upsert github instance secrets: %w", err)
			}
			m.mu.Lock()
			m.githubInstanceURL = baseURL
			m.mu.Unlock()
		} else {
			m.mu.Lock()
			m.githubInstanceURL = ""
			m.mu.Unlock()
		}
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
	if patch.ApplyGitHub {
		m.github = nextGitHub
	}
	pub := publicFrom(m.integ, m.github)
	m.mu.Unlock()

	if onChange != nil {
		onChange(next)
	}
	return pub, nil
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

// ValidateGitHubIntegration checks GitHub integration invariants.
func ValidateGitHubIntegration(i GitHubIntegration) error {
	url := strings.TrimSpace(i.URL)
	if url != "" && strings.TrimSpace(i.WebhookSecret) == "" && !i.AllowUnsignedWebhooks {
		return fmt.Errorf("github_webhook_secret is required when github_url is set (or enable github_allow_unsigned_webhooks)")
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
		return "", fmt.Errorf("GITSEER_ENCRYPTION_KEY is required to store secrets in the database")
	}
	return gitseercrypto.Encrypt(m.encKey, plaintext)
}

func (m *Manager) open(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if len(m.encKey) != 32 {
		return "", fmt.Errorf("GITSEER_ENCRYPTION_KEY is required to decrypt stored secrets")
	}
	pt, err := gitseercrypto.Decrypt(m.encKey, stored)
	if err != nil {
		return "", fmt.Errorf("decrypt failed (re-enter the secret if it was stored before encryption was enabled): %w", err)
	}
	return pt, nil
}

// OpenSecret decrypts a stored ciphertext. Requires GITSEER_ENCRYPTION_KEY.
// Used by sync and webhooks for per-instance credentials.
func (m *Manager) OpenSecret(stored string) (string, error) {
	return m.open(stored)
}

func publicFrom(i Integration, gh GitHubIntegration) IntegrationPublic {
	return IntegrationPublic{
		GiteaURL:                      i.URL,
		GiteaTokenConfigured:          i.Token != "",
		GiteaWebhookSecretConfigured:  i.WebhookSecret != "",
		GiteaAllowPrivateNetwork:      i.AllowPrivateNetwork,
		GiteaAllowUnsignedWebhooks:    i.AllowUnsignedWebhooks,
		OAuthClientID:                 i.OAuthClientID,
		OAuthClientSecretConfigured:   i.OAuthClientSecret != "",
		GitHubURL:                     gh.URL,
		GitHubTokenConfigured:         gh.Token != "",
		GitHubWebhookSecretConfigured: gh.WebhookSecret != "",
		GitHubAllowPrivateNetwork:     gh.AllowPrivateNetwork,
		GitHubAllowUnsignedWebhooks:   gh.AllowUnsignedWebhooks,
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
