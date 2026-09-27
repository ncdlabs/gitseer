package store

import (
	"context"
	"database/sql"
	"fmt"
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
	// SetFlags is true when AllowPrivateNetwork / AllowUnsignedWebhooks should be written.
	// When false on update-by-id, existing flag values are preserved.
	SetFlags bool
}

// ErrInstanceURLForgeConflict is returned when base_url is already owned by a different forge_type.
var ErrInstanceURLForgeConflict = fmt.Errorf("base_url already in use by a different forge type")

// ErrInstanceURLInUse is returned when inserting an instance whose base_url already exists.
var ErrInstanceURLInUse = fmt.Errorf("base_url already in use")

// UpsertInstanceSecrets inserts or updates an instance by base_url for the same forge_type.
// It refuses to change forge_type when the URL already belongs to another forge (no silent clobber).
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
		existingFT := normalizeForgeType(existing.ForgeType)
		if existingFT != ft {
			return nil, fmt.Errorf("%w: existing=%s requested=%s", ErrInstanceURLForgeConflict, existingFT, ft)
		}
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
		// Keep forge_type immutable on URL upsert (same-type update only).
		ft = existingFT
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
WHERE instances.forge_type = excluded.forge_type
`, name, ft, baseURL, version, caps,
		tokenCipher, webhookCipher, oauthID, oauthCipher, extURL,
		boolToInt(in.AllowPrivateNetwork), boolToInt(in.AllowUnsignedWebhooks), now, now)
	if err != nil {
		return nil, err
	}
	out, err := s.GetInstanceByURL(ctx, baseURL)
	if err != nil {
		return nil, err
	}
	if out != nil && normalizeForgeType(out.ForgeType) != ft {
		return nil, fmt.Errorf("%w: existing=%s requested=%s", ErrInstanceURLForgeConflict, out.ForgeType, ft)
	}
	return out, nil
}

// InsertInstanceSecrets inserts a new instance. Fails if base_url is already taken (no upsert).
func (s *Store) InsertInstanceSecrets(ctx context.Context, in InstanceSecrets) (*models.Instance, error) {
	ft := normalizeForgeType(in.ForgeType)
	baseURL := strings.TrimSpace(in.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = ft
	}
	now := formatTime(time.Now().UTC())
	caps := in.CapabilitiesJSON
	if caps == "" {
		caps = "{}"
	}
	_, err := s.exec(ctx, `
INSERT INTO instances (
  name, forge_type, base_url, version, capabilities_json,
  sync_token_ciphertext, webhook_secret_ciphertext,
  oauth_client_id, oauth_client_secret_ciphertext, external_url,
  allow_private_network, allow_unsigned_webhooks, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, name, ft, baseURL, strings.TrimSpace(in.Version), caps,
		strings.TrimSpace(in.SyncTokenCiphertext), strings.TrimSpace(in.WebhookSecretCiphertext),
		strings.TrimSpace(in.OAuthClientID), strings.TrimSpace(in.OAuthClientSecretCipher),
		strings.TrimSpace(in.ExternalURL),
		boolToInt(in.AllowPrivateNetwork), boolToInt(in.AllowUnsignedWebhooks), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrInstanceURLInUse
		}
		return nil, err
	}
	return s.GetInstanceByURL(ctx, baseURL)
}

// ListInstances returns all forge instances ordered by id.
func (s *Store) ListInstances(ctx context.Context) ([]models.Instance, error) {
	return s.listInstancesSQLC(ctx)
}

// UpdateInstanceSecretsByID updates an existing instance by id (including base_url).
// Empty ciphertext / OAuth secret fields leave existing values unless Clear* is set.
// Changing base_url to one already owned by another row returns an error.
func (s *Store) UpdateInstanceSecretsByID(ctx context.Context, id int64, in InstanceSecrets) (*models.Instance, error) {
	existing, err := s.GetInstanceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	existingFT := normalizeForgeType(existing.ForgeType)
	ft := existingFT
	if strings.TrimSpace(in.ForgeType) != "" {
		requested := normalizeForgeType(in.ForgeType)
		if requested != existingFT {
			return nil, fmt.Errorf("forge_type cannot be changed after create (existing=%s requested=%s)", existingFT, requested)
		}
	}
	baseURL := strings.TrimSpace(in.BaseURL)
	if baseURL == "" {
		baseURL = existing.BaseURL
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = existing.Name
	}
	if name == "" {
		name = ft
	}

	if baseURL != existing.BaseURL {
		other, err := s.GetInstanceByURL(ctx, baseURL)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if other != nil && other.ID != id {
			return nil, fmt.Errorf("base_url already in use")
		}
	}

	tokenCipher := strings.TrimSpace(in.SyncTokenCiphertext)
	webhookCipher := strings.TrimSpace(in.WebhookSecretCiphertext)
	oauthCipher := strings.TrimSpace(in.OAuthClientSecretCipher)
	oauthID := strings.TrimSpace(in.OAuthClientID)
	extURL := strings.TrimSpace(in.ExternalURL)
	version := strings.TrimSpace(in.Version)
	caps := in.CapabilitiesJSON

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
	if strings.TrimSpace(caps) == "" {
		caps = existing.CapabilitiesJSON
	}
	if caps == "" {
		caps = "{}"
	}

	allowPrivate := existing.AllowPrivateNetwork
	allowUnsigned := existing.AllowUnsignedWebhooks
	if in.SetFlags {
		allowPrivate = in.AllowPrivateNetwork
		allowUnsigned = in.AllowUnsignedWebhooks
	}

	now := formatTime(time.Now().UTC())
	_, err = s.exec(ctx, `
UPDATE instances SET
  name = ?, forge_type = ?, base_url = ?, version = ?, capabilities_json = ?,
  sync_token_ciphertext = ?, webhook_secret_ciphertext = ?,
  oauth_client_id = ?, oauth_client_secret_ciphertext = ?, external_url = ?,
  allow_private_network = ?, allow_unsigned_webhooks = ?, updated_at = ?
WHERE id = ?
`, name, ft, baseURL, version, caps,
		tokenCipher, webhookCipher, oauthID, oauthCipher, extURL,
		boolToInt(allowPrivate), boolToInt(allowUnsigned), now, id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrInstanceURLInUse
		}
		return nil, err
	}
	return s.GetInstanceByID(ctx, id)
}

// DeleteInstance removes an instance row (dependent rows cascade via FK).
func (s *Store) DeleteInstance(ctx context.Context, id int64) error {
	res, err := s.exec(ctx, `DELETE FROM instances WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// InstanceHasSecrets reports whether sync token or webhook secret ciphertext is present.
func InstanceHasSecrets(inst *models.Instance) bool {
	if inst == nil {
		return false
	}
	return strings.TrimSpace(inst.SyncTokenCiphertext) != "" ||
		strings.TrimSpace(inst.WebhookSecretCiphertext) != ""
}
