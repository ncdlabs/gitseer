package settings

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

// InstancePublic is the API-safe view of a forge instance (no secret values).
type InstancePublic struct {
	ID                          int64     `json:"id"`
	Name                        string    `json:"name"`
	ForgeType                   string    `json:"forge_type"`
	BaseURL                     string    `json:"base_url"`
	Version                     string    `json:"version,omitempty"`
	TokenConfigured             bool      `json:"token_configured"`
	WebhookSecretConfigured     bool      `json:"webhook_secret_configured"`
	OAuthClientID               string    `json:"oauth_client_id,omitempty"`
	OAuthClientSecretConfigured bool      `json:"oauth_client_secret_configured"`
	AllowPrivateNetwork         bool      `json:"allow_private_network"`
	AllowUnsignedWebhooks       bool      `json:"allow_unsigned_webhooks"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

// InstancePatch is the create/update body for /api/v1/instances.
// Empty secret strings leave stored secrets unchanged on update; clear_* flags clear them.
// Pointer bools omit unchanged SSRF/unsigned flags on partial updates.
type InstancePatch struct {
	ForgeType              string `json:"forge_type"`
	Name                   string `json:"name"`
	BaseURL                string `json:"base_url"`
	Token                  string `json:"token"`
	WebhookSecret          string `json:"webhook_secret"`
	ClearToken             bool   `json:"clear_token"`
	ClearWebhookSecret     bool   `json:"clear_webhook_secret"`
	OAuthClientID          string `json:"oauth_client_id"`
	OAuthClientSecret      string `json:"oauth_client_secret"`
	ClearOAuthClientSecret bool   `json:"clear_oauth_client_secret"`
	AllowPrivateNetwork    *bool  `json:"allow_private_network"`
	AllowUnsignedWebhooks  *bool  `json:"allow_unsigned_webhooks"`
}

// ListInstancesPublic returns all instances without secret material.
func (m *Manager) ListInstancesPublic(ctx context.Context) ([]InstancePublic, error) {
	list, err := m.st.ListInstances(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]InstancePublic, 0, len(list))
	for i := range list {
		out = append(out, instancePublicFrom(&list[i]))
	}
	return out, nil
}

// CreateInstance inserts a new forge instance with encrypted secrets.
func (m *Manager) CreateInstance(ctx context.Context, patch InstancePatch) (InstancePublic, error) {
	ft, baseURL, name, err := normalizeInstancePatch(patch, true)
	if err != nil {
		return InstancePublic{}, err
	}
	existing, err := m.st.GetInstanceByURL(ctx, baseURL)
	if err != nil && err != sql.ErrNoRows {
		return InstancePublic{}, err
	}
	if existing != nil {
		return InstancePublic{}, fmt.Errorf("base_url already in use")
	}

	tokenCipher, webhookCipher, oauthCipher, err := m.sealInstanceSecrets(patch)
	if err != nil {
		return InstancePublic{}, err
	}
	allowPrivate, allowUnsigned := false, false
	if patch.AllowPrivateNetwork != nil {
		allowPrivate = *patch.AllowPrivateNetwork
	}
	if patch.AllowUnsignedWebhooks != nil {
		allowUnsigned = *patch.AllowUnsignedWebhooks
	}
	if err := validateInstanceSecrets(ft, baseURL, tokenCipher != "", webhookCipher != "", allowUnsigned); err != nil {
		return InstancePublic{}, err
	}

	extURL := m.effectiveExternalURL()
	inst, err := m.st.InsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                    name,
		ForgeType:               ft,
		BaseURL:                 baseURL,
		SyncTokenCiphertext:     tokenCipher,
		WebhookSecretCiphertext: webhookCipher,
		OAuthClientID:           strings.TrimSpace(patch.OAuthClientID),
		OAuthClientSecretCipher: oauthCipher,
		ExternalURL:             extURL,
		AllowPrivateNetwork:     allowPrivate,
		AllowUnsignedWebhooks:   allowUnsigned,
		SetFlags:                true,
	})
	if err != nil {
		return InstancePublic{}, err
	}
	if err := m.refreshSnapshotsFromInstances(ctx); err != nil {
		return InstancePublic{}, err
	}
	return instancePublicFrom(inst), nil
}

// UpdateInstance updates an existing instance by id (leave-blank secrets).
func (m *Manager) UpdateInstance(ctx context.Context, id int64, patch InstancePatch) (InstancePublic, error) {
	existing, err := m.st.GetInstanceByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return InstancePublic{}, fmt.Errorf("instance not found")
		}
		return InstancePublic{}, err
	}

	ft := normalizeSupportedForgeType(existing.ForgeType)
	if strings.TrimSpace(patch.ForgeType) != "" {
		requested := normalizeSupportedForgeType(patch.ForgeType)
		if requested == "" {
			return InstancePublic{}, fmt.Errorf("forge_type must be gitea or github")
		}
		if requested != ft {
			return InstancePublic{}, fmt.Errorf("forge_type cannot be changed after create")
		}
	}
	if ft == "" {
		return InstancePublic{}, fmt.Errorf("forge_type must be gitea or github")
	}
	baseURL := strings.TrimSpace(patch.BaseURL)
	if baseURL == "" {
		baseURL = existing.BaseURL
	}
	if ft == models.ForgeTypeGitHub {
		if norm, nerr := tryNormalizeGitHubAPI(baseURL); nerr == nil {
			baseURL = norm
		}
	} else {
		baseURL = strings.TrimRight(baseURL, "/")
	}
	name := strings.TrimSpace(patch.Name)
	if name == "" {
		name = existing.Name
	}
	if name == "" {
		name = defaultInstanceName(ft)
	}

	tokenCipher, webhookCipher, oauthCipher, err := m.sealInstanceSecrets(patch)
	if err != nil {
		return InstancePublic{}, err
	}

	willHaveToken := strings.TrimSpace(existing.SyncTokenCiphertext) != ""
	if patch.ClearToken {
		willHaveToken = false
	} else if tokenCipher != "" {
		willHaveToken = true
	}
	willHaveWebhook := strings.TrimSpace(existing.WebhookSecretCiphertext) != ""
	if patch.ClearWebhookSecret {
		willHaveWebhook = false
	} else if webhookCipher != "" {
		willHaveWebhook = true
	}
	setFlags := patch.AllowPrivateNetwork != nil || patch.AllowUnsignedWebhooks != nil
	allowPrivate := existing.AllowPrivateNetwork
	allowUnsigned := existing.AllowUnsignedWebhooks
	if patch.AllowPrivateNetwork != nil {
		allowPrivate = *patch.AllowPrivateNetwork
	}
	if patch.AllowUnsignedWebhooks != nil {
		allowUnsigned = *patch.AllowUnsignedWebhooks
	}
	if err := validateInstanceSecrets(ft, baseURL, willHaveToken, willHaveWebhook, allowUnsigned); err != nil {
		return InstancePublic{}, err
	}

	oauthID := strings.TrimSpace(patch.OAuthClientID)
	inst, err := m.st.UpdateInstanceSecretsByID(ctx, id, store.InstanceSecrets{
		Name:                    name,
		ForgeType:               ft,
		BaseURL:                 baseURL,
		SyncTokenCiphertext:     tokenCipher,
		ClearSyncToken:          patch.ClearToken,
		WebhookSecretCiphertext: webhookCipher,
		ClearWebhookSecret:      patch.ClearWebhookSecret,
		OAuthClientID:           oauthID,
		OAuthClientSecretCipher: oauthCipher,
		ClearOAuthClientSecret:  patch.ClearOAuthClientSecret,
		ExternalURL:             m.effectiveExternalURL(),
		AllowPrivateNetwork:     allowPrivate,
		AllowUnsignedWebhooks:   allowUnsigned,
		SetFlags:                setFlags,
	})
	if err != nil {
		if strings.Contains(err.Error(), "base_url already in use") {
			return InstancePublic{}, err
		}
		return InstancePublic{}, err
	}
	if err := m.refreshSnapshotsFromInstances(ctx); err != nil {
		return InstancePublic{}, err
	}
	return instancePublicFrom(inst), nil
}

// DeleteInstance removes an instance and refreshes first-gitea / first-github snapshots.
func (m *Manager) DeleteInstance(ctx context.Context, id int64) error {
	if err := m.st.DeleteInstance(ctx, id); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("instance not found")
		}
		return err
	}
	return m.refreshSnapshotsFromInstances(ctx)
}

// AuthGiteaIntegration returns the Gitea connection used for OAuth live-apply:
// primary Gitea (OAuth configured preferred), else Integration() snapshot.
func (m *Manager) AuthGiteaIntegration(ctx context.Context) Integration {
	inst, err := m.st.GetPrimaryGiteaInstance(ctx)
	if err != nil || inst == nil {
		return m.Integration()
	}
	integ, err := m.integrationFromGiteaInstance(inst)
	if err != nil {
		return m.Integration()
	}
	return integ
}

// refreshSnapshotsFromInstances sets Integration/GitHub from the first instance of
// each forge type (by id) for setup-path compatibility, then live-applies primary
// Gitea OAuth credentials to the auth callback.
func (m *Manager) refreshSnapshotsFromInstances(ctx context.Context) error {
	list, err := m.st.ListInstances(ctx)
	if err != nil {
		return err
	}

	var firstGitea, firstGitHub *models.Instance
	for i := range list {
		inst := &list[i]
		switch inst.ForgeType {
		case models.ForgeTypeGitHub:
			if firstGitHub == nil {
				firstGitHub = inst
			}
		default:
			if firstGitea == nil {
				firstGitea = inst
			}
		}
	}

	var nextInteg Integration
	if firstGitea != nil {
		nextInteg, err = m.integrationFromGiteaInstance(firstGitea)
		if err != nil {
			return err
		}
	} else {
		m.mu.RLock()
		nextInteg = m.baseInteg
		m.mu.RUnlock()
	}

	var nextGitHub GitHubIntegration
	if firstGitHub != nil {
		nextGitHub, err = m.githubFromInstance(firstGitHub)
		if err != nil {
			return err
		}
	} else {
		m.mu.RLock()
		nextGitHub = m.baseGitHub
		m.mu.RUnlock()
	}

	authInteg := nextInteg
	if prim, perr := m.st.GetPrimaryGiteaInstance(ctx); perr == nil && prim != nil {
		if ai, aerr := m.integrationFromGiteaInstance(prim); aerr == nil {
			authInteg = ai
		}
	}

	m.mu.Lock()
	m.integ = nextInteg
	if firstGitea != nil {
		m.dbURL = firstGitea.BaseURL
		m.dbTokenCipher = firstGitea.SyncTokenCiphertext
		m.dbWebhookCipher = firstGitea.WebhookSecretCiphertext
		m.dbOAuthID = firstGitea.OAuthClientID
		m.dbOAuthCipher = firstGitea.OAuthClientSecretCipher
		m.dbAllowPrivate = sql.NullInt64{Valid: true, Int64: boolToInt(firstGitea.AllowPrivateNetwork)}
		m.dbAllowUnsigned = sql.NullInt64{Valid: true, Int64: boolToInt(firstGitea.AllowUnsignedWebhooks)}
	} else {
		m.dbURL = ""
		m.dbTokenCipher = ""
		m.dbWebhookCipher = ""
		m.dbOAuthID = ""
		m.dbOAuthCipher = ""
		m.dbAllowPrivate = sql.NullInt64{}
		m.dbAllowUnsigned = sql.NullInt64{}
	}
	m.github = nextGitHub
	if firstGitHub != nil {
		m.githubInstanceURL = firstGitHub.BaseURL
	} else {
		m.githubInstanceURL = ""
	}
	onChange := m.onInteg
	m.mu.Unlock()

	if onChange != nil {
		onChange(authInteg)
	}
	return nil
}

func (m *Manager) integrationFromGiteaInstance(inst *models.Instance) (Integration, error) {
	out := Integration{
		URL:                   inst.BaseURL,
		AllowPrivateNetwork:   inst.AllowPrivateNetwork,
		AllowUnsignedWebhooks: inst.AllowUnsignedWebhooks,
		OAuthClientID:         strings.TrimSpace(inst.OAuthClientID),
	}
	if inst.SyncTokenCiphertext != "" {
		tok, err := m.open(inst.SyncTokenCiphertext)
		if err != nil {
			return Integration{}, fmt.Errorf("decrypt sync token: %w", err)
		}
		out.Token = tok
	}
	if inst.WebhookSecretCiphertext != "" {
		sec, err := m.open(inst.WebhookSecretCiphertext)
		if err != nil {
			return Integration{}, fmt.Errorf("decrypt webhook secret: %w", err)
		}
		out.WebhookSecret = sec
	}
	if inst.OAuthClientSecretCipher != "" {
		sec, err := m.open(inst.OAuthClientSecretCipher)
		if err != nil {
			return Integration{}, fmt.Errorf("decrypt oauth client secret: %w", err)
		}
		out.OAuthClientSecret = sec
	}
	return out, nil
}

func (m *Manager) githubFromInstance(inst *models.Instance) (GitHubIntegration, error) {
	out := GitHubIntegration{
		URL:                   inst.BaseURL,
		AllowPrivateNetwork:   inst.AllowPrivateNetwork,
		AllowUnsignedWebhooks: inst.AllowUnsignedWebhooks,
	}
	if inst.SyncTokenCiphertext != "" {
		tok, err := m.open(inst.SyncTokenCiphertext)
		if err != nil {
			return GitHubIntegration{}, fmt.Errorf("decrypt github token: %w", err)
		}
		out.Token = tok
	}
	if inst.WebhookSecretCiphertext != "" {
		sec, err := m.open(inst.WebhookSecretCiphertext)
		if err != nil {
			return GitHubIntegration{}, fmt.Errorf("decrypt github webhook secret: %w", err)
		}
		out.WebhookSecret = sec
	}
	return out, nil
}

func (m *Manager) sealInstanceSecrets(patch InstancePatch) (tokenCipher, webhookCipher, oauthCipher string, err error) {
	if strings.TrimSpace(patch.Token) != "" {
		tokenCipher, err = m.seal(strings.TrimSpace(patch.Token))
		if err != nil {
			return "", "", "", err
		}
	}
	if strings.TrimSpace(patch.WebhookSecret) != "" {
		webhookCipher, err = m.seal(strings.TrimSpace(patch.WebhookSecret))
		if err != nil {
			return "", "", "", err
		}
	}
	if strings.TrimSpace(patch.OAuthClientSecret) != "" {
		oauthCipher, err = m.seal(strings.TrimSpace(patch.OAuthClientSecret))
		if err != nil {
			return "", "", "", err
		}
	}
	return tokenCipher, webhookCipher, oauthCipher, nil
}

func (m *Manager) effectiveExternalURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ext := m.dbExternalURL
	if ext == "" {
		ext = m.cur.ServerExternalURL
	}
	if ext == "" {
		ext = m.base.ServerExternalURL
	}
	return strings.TrimSpace(ext)
}

func normalizeInstancePatch(patch InstancePatch, creating bool) (ft, baseURL, name string, err error) {
	ft = normalizeSupportedForgeType(patch.ForgeType)
	if ft == "" {
		return "", "", "", fmt.Errorf("forge_type must be gitea or github")
	}
	baseURL = strings.TrimSpace(patch.BaseURL)
	if baseURL == "" {
		return "", "", "", fmt.Errorf("base_url is required")
	}
	if ft == models.ForgeTypeGitHub {
		if norm, nerr := tryNormalizeGitHubAPI(baseURL); nerr == nil {
			baseURL = norm
		}
	} else {
		baseURL = strings.TrimRight(baseURL, "/")
	}
	name = strings.TrimSpace(patch.Name)
	if name == "" {
		name = defaultInstanceName(ft)
	}
	_ = creating
	return ft, baseURL, name, nil
}

func normalizeSupportedForgeType(ft string) string {
	switch strings.ToLower(strings.TrimSpace(ft)) {
	case models.ForgeTypeGitea:
		return models.ForgeTypeGitea
	case models.ForgeTypeGitHub:
		return models.ForgeTypeGitHub
	case "gitlab", "bitbucket":
		return ""
	default:
		if strings.TrimSpace(ft) == "" {
			return ""
		}
		return ""
	}
}

func defaultInstanceName(ft string) string {
	switch ft {
	case models.ForgeTypeGitHub:
		return "GitHub"
	default:
		return "Gitea"
	}
}

func validateInstanceSecrets(ft, baseURL string, hasToken, hasWebhook, allowUnsigned bool) error {
	_ = hasToken
	_ = ft
	if baseURL == "" {
		return fmt.Errorf("base_url is required")
	}
	if !hasWebhook && !allowUnsigned {
		return fmt.Errorf("webhook_secret is required when base_url is set (or enable allow_unsigned_webhooks)")
	}
	return nil
}

func instancePublicFrom(inst *models.Instance) InstancePublic {
	if inst == nil {
		return InstancePublic{}
	}
	ft := inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	out := InstancePublic{
		ID:                          inst.ID,
		Name:                        inst.Name,
		ForgeType:                   ft,
		BaseURL:                     inst.BaseURL,
		Version:                     inst.Version,
		TokenConfigured:             strings.TrimSpace(inst.SyncTokenCiphertext) != "",
		WebhookSecretConfigured:     strings.TrimSpace(inst.WebhookSecretCiphertext) != "",
		OAuthClientID:               strings.TrimSpace(inst.OAuthClientID),
		OAuthClientSecretConfigured: strings.TrimSpace(inst.OAuthClientSecretCipher) != "",
		AllowPrivateNetwork:         inst.AllowPrivateNetwork,
		AllowUnsignedWebhooks:       inst.AllowUnsignedWebhooks,
		CreatedAt:                   inst.CreatedAt,
		UpdatedAt:                   inst.UpdatedAt,
	}
	if ft != models.ForgeTypeGitea {
		out.OAuthClientID = ""
		out.OAuthClientSecretConfigured = false
	}
	return out
}
