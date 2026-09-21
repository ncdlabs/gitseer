package gitea

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// DefaultWebhookEvents are the Gitea events Lens applies today.
var DefaultWebhookEvents = []string{
	"pull_request",
	"workflow_run",
	"workflow_job",
	"repository",
}

// WebhookPreview is the system hook Lens will create (or that the admin can paste).
type WebhookPreview struct {
	Type             string            `json:"type"`
	Active           bool              `json:"active"`
	Events           []string          `json:"events"`
	Config           map[string]string `json:"config"`
	AuthorizationHdr string            `json:"-"`
}

// NewWebhookPreview builds the create-hook body shown in the setup wizard.
// secret may be empty for a preview before generation ("«generated on confirm»").
func NewWebhookPreview(deliveryURL, secret string) *WebhookPreview {
	sec := secret
	if sec == "" {
		sec = "«generated on confirm»"
	}
	return &WebhookPreview{
		Type:   "gitea",
		Active: true,
		Events: append([]string(nil), DefaultWebhookEvents...),
		Config: map[string]string{
			"url":               strings.TrimRight(deliveryURL, "/"),
			"content_type":      "json",
			"secret":            sec,
			"is_system_webhook": "true",
		},
	}
}

// CreateHookOption is the Gitea admin create-hook body.
type CreateHookOption struct {
	Type   string            `json:"type"`
	Active bool              `json:"active"`
	Events []string          `json:"events"`
	Config map[string]string `json:"config"`
}

// Hook is a minimal Gitea hook response.
type Hook struct {
	ID     int64             `json:"id"`
	Type   string            `json:"type"`
	Active bool              `json:"active"`
	Events []string          `json:"events"`
	Config map[string]string `json:"config"`
}

// GenerateWebhookSecret returns a 32-byte hex secret for HMAC.
func GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnsureSystemWebhook creates or updates a Lens system webhook at deliveryURL.
func (c *Client) EnsureSystemWebhook(ctx context.Context, deliveryURL, secret string) (*Hook, bool, error) {
	deliveryURL = strings.TrimRight(strings.TrimSpace(deliveryURL), "/")
	if deliveryURL == "" {
		return nil, false, fmt.Errorf("webhook delivery url is required")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, false, fmt.Errorf("webhook secret is required")
	}

	existing, err := c.findSystemHookByURL(ctx, deliveryURL)
	if err != nil {
		return nil, false, err
	}

	body := CreateHookOption{
		Type:   "gitea",
		Active: true,
		Events: append([]string(nil), DefaultWebhookEvents...),
		Config: map[string]string{
			"url":               deliveryURL,
			"content_type":      "json",
			"secret":            secret,
			"is_system_webhook": "true",
		},
	}

	if existing != nil {
		resp, err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/admin/hooks/%d", existing.ID), nil, body)
		if err != nil {
			return nil, false, err
		}
		var hook Hook
		if err := decodeJSON(resp, &hook); err != nil {
			return nil, false, err
		}
		return &hook, false, nil
	}

	resp, err := c.doJSON(ctx, http.MethodPost, "/admin/hooks", nil, body)
	if err != nil {
		return nil, false, err
	}
	var hook Hook
	if err := decodeJSON(resp, &hook); err != nil {
		return nil, false, err
	}
	return &hook, true, nil
}

func (c *Client) findSystemHookByURL(ctx context.Context, deliveryURL string) (*Hook, error) {
	resp, err := c.do(ctx, http.MethodGet, "/admin/hooks", url.Values{
		"limit": {"50"},
		"type":  {"system"},
	}, "")
	if err != nil {
		return nil, err
	}
	var hooks []Hook
	if err := decodeJSON(resp, &hooks); err != nil {
		return nil, err
	}
	want := strings.TrimRight(deliveryURL, "/")
	for i := range hooks {
		cfgURL := strings.TrimRight(hooks[i].Config["url"], "/")
		if cfgURL == want {
			return &hooks[i], nil
		}
	}
	return nil, nil
}
