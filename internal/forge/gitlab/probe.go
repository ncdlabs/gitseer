package gitlab

import (
	"context"
	"fmt"
	"strings"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
)

// ProbeStatus is the outcome of one permission / connectivity check.
type ProbeStatus string

const (
	ProbeOK   ProbeStatus = "ok"
	ProbeFail ProbeStatus = "fail"
	ProbeWarn ProbeStatus = "warn"
	ProbeSkip ProbeStatus = "skip"
)

// ProbeCheck is one named check shown in the setup connection modal.
type ProbeCheck struct {
	ID     string      `json:"id"`
	Group  string      `json:"group"`
	Label  string      `json:"label"`
	Status ProbeStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

const (
	GroupConnectivity = "connectivity"
	GroupPermissions  = "permissions"
)

// WebhookPreview is the manual webhook body an admin pastes into GitLab.
type WebhookPreview struct {
	URL         string   `json:"url"`
	SecretToken string   `json:"secret_token"`
	Events      []string `json:"events"`
}

// DefaultWebhookEvents are GitLab hook events GitSeer applies.
var DefaultWebhookEvents = []string{
	"merge_request_events",
	"pipeline_events",
	"job_events",
	"push_events",
}

// NewWebhookPreview builds a manual webhook preview.
func NewWebhookPreview(deliveryURL, secret string) *WebhookPreview {
	sec := secret
	if sec == "" {
		sec = "«generated on confirm»"
	}
	return &WebhookPreview{
		URL:         strings.TrimRight(deliveryURL, "/"),
		SecretToken: sec,
		Events:      append([]string(nil), DefaultWebhookEvents...),
	}
}

// ProbeResult is the full GitLab test-connection outcome.
type ProbeResult struct {
	OK             bool                 `json:"ok"`
	ForgeType      string               `json:"forge_type"`
	Version        string               `json:"version"`
	Login          string               `json:"login,omitempty"`
	Checks         []ProbeCheck         `json:"checks"`
	Capabilities   *models.Capabilities `json:"capabilities,omitempty"`
	CanCreateHook  bool                 `json:"can_create_webhook"`
	CanCreateOAuth bool                 `json:"can_create_oauth"`
	WebhookPreview any                  `json:"webhook_preview,omitempty"`
	ManualWebhook  bool                 `json:"manual_webhook"`
}

// ProbeConnection verifies reachability, auth, and project read access.
func (c *Client) ProbeConnection(ctx context.Context, webhookDeliveryURL string) (*ProbeResult, error) {
	out := &ProbeResult{
		ForgeType:     models.ForgeTypeGitLab,
		Checks:        make([]ProbeCheck, 0, 8),
		ManualWebhook: true,
	}
	reachLabel := fmt.Sprintf("Can reach %s", strings.TrimRight(c.baseURL.String(), "/"))

	info, err := c.GetInstance(ctx)
	if err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "reachability", Group: GroupConnectivity, Label: reachLabel, Status: ProbeFail, Detail: err.Error(),
		})
		return out, nil
	}
	out.Version = info.Version
	out.Checks = append(out.Checks, ProbeCheck{
		ID: "reachability", Group: GroupConnectivity, Label: reachLabel, Status: ProbeOK, Detail: "GitLab " + info.Version,
	})

	user, err := c.GetAuthenticatedUser(ctx, "")
	if err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "authenticate", Group: GroupConnectivity, Label: "Authenticate with token", Status: ProbeFail, Detail: err.Error(),
		})
		return out, nil
	}
	out.Login = user.Login
	out.Checks = append(out.Checks, ProbeCheck{
		ID: "authenticate", Group: GroupConnectivity, Label: "Authenticate with token", Status: ProbeOK, Detail: "Signed in as " + user.Login,
	})

	repos, err := c.ListRepositories(ctx, forge.ListReposOpts{Page: 1, PageSize: 5})
	if err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_projects", Group: GroupPermissions, Label: "List accessible projects", Status: ProbeFail, Detail: err.Error(),
		})
	} else {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_projects", Group: GroupPermissions, Label: "List accessible projects", Status: ProbeOK,
			Detail: fmt.Sprintf("%d project(s) on first page", len(repos.Items)),
		})
	}

	caps, _ := c.DetectCapabilities(ctx)
	out.Capabilities = caps
	out.WebhookPreview = NewWebhookPreview(webhookDeliveryURL, "")
	out.OK = true
	for _, ch := range out.Checks {
		if ch.Status == ProbeFail {
			out.OK = false
			break
		}
	}
	return out, nil
}
