package github

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
	Group  string      `json:"group"` // connectivity | permissions
	Label  string      `json:"label"`
	Status ProbeStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

const (
	GroupConnectivity = "connectivity"
	GroupPermissions  = "permissions"
)

// WebhookPreview is the manual webhook body an admin pastes into GitHub.
type WebhookPreview struct {
	Active bool              `json:"active"`
	Events []string          `json:"events"`
	Config map[string]string `json:"config"`
}

// DefaultWebhookEvents are the GitHub events GitSeer applies for immediacy.
var DefaultWebhookEvents = []string{
	"pull_request",
	"pull_request_review",
	"workflow_run",
	"workflow_job",
	"repository",
	"check_run",
	"check_suite",
	"status",
}

// NewWebhookPreview builds a manual webhook preview (no auto-create on GitHub).
func NewWebhookPreview(deliveryURL, secret string) *WebhookPreview {
	sec := secret
	if sec == "" {
		sec = "«generated on confirm»"
	}
	return &WebhookPreview{
		Active: true,
		Events: append([]string(nil), DefaultWebhookEvents...),
		Config: map[string]string{
			"url":          strings.TrimRight(deliveryURL, "/"),
			"content_type": "json",
			"secret":       sec,
			"insecure_ssl": "0",
		},
	}
}

// ProbeResult is the full GitHub test-connection outcome.
type ProbeResult struct {
	OK             bool                 `json:"ok"`
	ForgeType      string               `json:"forge_type"`
	Version        string               `json:"version"`
	Login          string               `json:"login,omitempty"`
	Checks         []ProbeCheck         `json:"checks"`
	Capabilities   *models.Capabilities `json:"capabilities,omitempty"`
	CanCreateHook  bool                 `json:"can_create_webhook"` // true when PAT can manage at least one org hook
	CanCreateOAuth bool                 `json:"can_create_oauth"`   // always false this slice
	WebhookPreview *WebhookPreview      `json:"webhook_preview,omitempty"`
	ManualWebhook  bool                 `json:"manual_webhook"`
}

// ProbeConnection verifies reachability, auth, and repo read access for a GitHub PAT.
// Org webhook auto-create is available when the PAT has admin:org_hook on a membership org.
func (c *Client) ProbeConnection(ctx context.Context, webhookDeliveryURL string) (*ProbeResult, error) {
	out := &ProbeResult{
		ForgeType:     models.ForgeTypeGitHub,
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
		ID: "reachability", Group: GroupConnectivity, Label: reachLabel, Status: ProbeOK, Detail: "GitHub " + info.Version,
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
		ID: "authenticate", Group: GroupConnectivity, Label: "Authenticate with token", Status: ProbeOK,
		Detail: fmt.Sprintf("Signed in as %s", user.Login),
	})

	repos, reposErr := c.ListRepositories(ctx, forge.ListReposOpts{Page: 1, PageSize: 1})
	if reposErr != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_repos", Group: GroupPermissions, Label: "List repositories", Status: ProbeFail, Detail: reposErr.Error(),
		})
	} else {
		detail := "Token can list repositories (none visible yet)"
		if len(repos.Items) > 0 {
			detail = fmt.Sprintf("Can read repositories (e.g. %s)", repos.Items[0].FullName)
		}
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_repos", Group: GroupPermissions, Label: "List repositories", Status: ProbeOK, Detail: detail,
		})
	}

	caps, _ := c.DetectCapabilities(ctx)
	if caps == nil {
		caps = &models.Capabilities{Version: out.Version}
	}
	out.Capabilities = caps
	if caps.ActionsAPI {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "actions", Group: GroupPermissions, Label: "Read Actions runs", Status: ProbeOK, Detail: "Actions API available",
		})
	} else {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "actions", Group: GroupPermissions, Label: "Read Actions runs", Status: ProbeWarn,
			Detail: "Actions API not confirmed on a sample repo (may be disabled or empty)",
		})
	}

	orgs, orgsErr := c.ListMembershipOrgs(ctx)
	canHook := false
	hookDetail := "GitHub has no system-hooks API; add a webhook manually in each org/repo (or use Ensure Webhook with admin:org_hook)"
	if orgsErr == nil {
		for _, org := range orgs {
			if c.CanManageOrgHooks(ctx, org) {
				canHook = true
				hookDetail = fmt.Sprintf("Can manage org hooks (e.g. %s)", org)
				break
			}
		}
	}
	out.CanCreateHook = canHook
	out.ManualWebhook = !canHook
	hookStatus := ProbeSkip
	if canHook {
		hookStatus = ProbeOK
	}
	out.Checks = append(out.Checks, ProbeCheck{
		ID: "system_hooks", Group: GroupPermissions, Label: "Manage org webhooks", Status: hookStatus,
		Detail: hookDetail,
	})
	out.Checks = append(out.Checks, ProbeCheck{
		ID: "oauth_apps", Group: GroupPermissions, Label: "Manage OAuth applications", Status: ProbeSkip,
		Detail: "Configure a GitHub OAuth App (Authorization Code + PKCE) on the instance for per-user login; sync still uses the service PAT",
	})

	out.OK = allRequiredOK(out.Checks)
	if webhookDeliveryURL != "" {
		out.WebhookPreview = NewWebhookPreview(webhookDeliveryURL, "")
	}
	return out, nil
}

func allRequiredOK(checks []ProbeCheck) bool {
	for _, c := range checks {
		if c.Status == ProbeFail {
			return false
		}
	}
	return true
}
