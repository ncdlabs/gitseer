package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
)

// InstanceSecrets is the write payload for per-instance credentials and SSRF/webhook flags.
// Empty ciphertext / OAuth secret fields leave existing stored values unchanged when the row exists,
// unless the matching Clear* flag is set.
type InstanceSecrets struct {
	Name                    string
	ForgeType               string // gitea | github; empty defaults to gitea
	BaseURL                 string
	Version                 string // optional; empty leaves version unchanged on update
	CapabilitiesJSON        string // optional; empty leaves capabilities unchanged on update
	SyncTokenCiphertext     string
	ClearSyncToken          bool
	WebhookSecretCiphertext string
	ClearWebhookSecret      bool
	OAuthClientID           string
	OAuthClientSecretCipher string
	ClearOAuthClientSecret  bool
	ExternalURL             string
	AllowPrivateNetwork     bool
	AllowUnsignedWebhooks   bool
}

// UpsertInstanceSecrets inserts or updates an instance by base_url, setting forge_type,
// encrypted credential columns, and allow_private_network / allow_unsigned_webhooks flags.
func (s *Store) UpsertInstanceSecrets(ctx context.Context, in InstanceSecrets) (*models.Instance, error) {
	ft := normalizeForgeType(in.ForgeType)
	baseURL := strings.TrimSpace(in.BaseURL)
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = ft
	}
	now := formatTime(time.Now().UTC())

	existing, err := s.GetInstanceByURL(ctx, baseURL)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
		existing = nil
	}

	tokenCipher := strings.TrimSpace(in.SyncTokenCiphertext)
	webhookCipher := strings.TrimSpace(in.WebhookSecretCiphertext)
	oauthCipher := strings.TrimSpace(in.OAuthClientSecretCipher)
	oauthID := strings.TrimSpace(in.OAuthClientID)
	extURL := strings.TrimSpace(in.ExternalURL)
	version := strings.TrimSpace(in.Version)
	caps := in.CapabilitiesJSON
	if caps == "" {
		caps = "{}"
	}

	if existing != nil {
		if in.ClearSyncToken {
			tokenCipher = ""
		} else if tokenCipher == "" {
			tokenCipher = existing.SyncTokenCiphertext
		}
		if in.ClearWebhookSecret {
			webhookCipher = ""
		} else if webhookCipher == "" {
			webhookCipher = existing.WebhookSecretCiphertext
		}
		if in.ClearOAuthClientSecret {
			oauthCipher = ""
		} else if oauthCipher == "" {
			oauthCipher = existing.OAuthClientSecretCipher
		}
		if oauthID == "" {
			oauthID = existing.OAuthClientID
		}
		if extURL == "" {
			extURL = existing.ExternalURL
		}
		if version == "" {
			version = existing.Version
		}
		if strings.TrimSpace(in.CapabilitiesJSON) == "" {
			caps = existing.CapabilitiesJSON
		}
	}

	_, err = s.exec(ctx, `
INSERT INTO instances (
  name, forge_type, base_url, version, capabilities_json,
  sync_token_ciphertext, webhook_secret_ciphertext,
  oauth_client_id, oauth_client_secret_ciphertext, external_url,
  allow_private_network, allow_unsigned_webhooks, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(base_url) DO UPDATE SET
  name=excluded.name,
  forge_type=excluded.forge_type,
  version=excluded.version,
  capabilities_json=excluded.capabilities_json,
  sync_token_ciphertext=excluded.sync_token_ciphertext,
  webhook_secret_ciphertext=excluded.webhook_secret_ciphertext,
  oauth_client_id=excluded.oauth_client_id,
  oauth_client_secret_ciphertext=excluded.oauth_client_secret_ciphertext,
  external_url=excluded.external_url,
  allow_private_network=excluded.allow_private_network,
  allow_unsigned_webhooks=excluded.allow_unsigned_webhooks,
  updated_at=excluded.updated_at
`, name, ft, baseURL, version, caps,
		tokenCipher, webhookCipher, oauthID, oauthCipher, extURL,
		boolToInt(in.AllowPrivateNetwork), boolToInt(in.AllowUnsignedWebhooks), now, now)
	if err != nil {
		return nil, err
	}
	return s.GetInstanceByURL(ctx, baseURL)
}

// ListInstances returns all forge instances ordered by id.
func (s *Store) ListInstances(ctx context.Context) ([]models.Instance, error) {
	rows, err := s.query(ctx, `SELECT`+instanceSelectCols+` FROM instances ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// InstanceHasSecrets reports whether sync token or webhook secret ciphertext is present.
func InstanceHasSecrets(inst *models.Instance) bool {
	if inst == nil {
		return false
	}
	return strings.TrimSpace(inst.SyncTokenCiphertext) != "" ||
		strings.TrimSpace(inst.WebhookSecretCiphertext) != ""
}
