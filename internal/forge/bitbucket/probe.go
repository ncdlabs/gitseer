package bitbucket

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

// WebhookPreview is the manual webhook body an admin pastes into Bitbucket.
type WebhookPreview struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

// DefaultWebhookEvents are Bitbucket Cloud webhook events GitSeer applies.
var DefaultWebhookEvents = []string{
	"pullrequest:created",
	"pullrequest:updated",
	"pullrequest:fulfilled",
	"pullrequest:rejected",
	"repo:push",
}

// NewWebhookPreview builds a manual webhook preview.
func NewWebhookPreview(deliveryURL, secret string) *WebhookPreview {
	sec := secret
	if sec == "" {
		sec = "«generated on confirm»"
	}
	return &WebhookPreview{
		URL:    strings.TrimRight(deliveryURL, "/"),
		Secret: sec,
		Events: append([]string(nil), DefaultWebhookEvents...),
	}
}

// ProbeResult is the full Bitbucket test-connection outcome.
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

// ProbeConnection verifies reachability, auth, and repo read access.
func (c *Client) ProbeConnection(ctx context.Context, webhookDeliveryURL string) (*ProbeResult, error) {
	out := &ProbeResult{
		ForgeType:     models.ForgeTypeBitbucket,
		Checks:        make([]ProbeCheck, 0, 8),
		ManualWebhook: true,
	}
	reachLabel := fmt.Sprintf("Can reach %s", strings.TrimRight(c.baseURL.String(), "/"))

	user, err := c.GetAuthenticatedUser(ctx, "")
	if err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "reachability", Group: GroupConnectivity, Label: reachLabel, Status: ProbeFail, Detail: err.Error(),
		})
		return out, nil
	}
	out.Version = "cloud"
	out.Login = user.Login
	out.Checks = append(out.Checks,
		ProbeCheck{ID: "reachability", Group: GroupConnectivity, Label: reachLabel, Status: ProbeOK, Detail: "Bitbucket Cloud"},
		ProbeCheck{ID: "authenticate", Group: GroupConnectivity, Label: "Authenticate with token", Status: ProbeOK, Detail: "Signed in as " + user.Login},
	)

	repos, err := c.ListRepositories(ctx, forge.ListReposOpts{Page: 1, PageSize: 5})
	if err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_repos", Group: GroupPermissions, Label: "List accessible repositories", Status: ProbeFail, Detail: err.Error(),
		})
	} else {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_repos", Group: GroupPermissions, Label: "List accessible repositories", Status: ProbeOK,
			Detail: fmt.Sprintf("%d repo(s) on first page", len(repos.Items)),
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
