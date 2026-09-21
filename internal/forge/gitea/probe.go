package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ncdlabs/gitea-lens/internal/forge"
	"github.com/ncdlabs/gitea-lens/internal/models"
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

// ProbeResult is the full test-connection outcome.
type ProbeResult struct {
	OK             bool                 `json:"ok"`
	Version        string               `json:"version"`
	Login          string               `json:"login,omitempty"`
	IsAdmin        bool                 `json:"is_admin"`
	Checks         []ProbeCheck         `json:"checks"`
	Capabilities   *models.Capabilities `json:"capabilities,omitempty"`
	CanCreateHook  bool                 `json:"can_create_webhook"`
	WebhookPreview *WebhookPreview      `json:"webhook_preview,omitempty"`
}

func (c *Client) displayBaseURL() string {
	return strings.TrimRight(c.baseURL.String(), "/")
}

// ProbeConnection verifies reachability, auth, and permissions Lens needs now and for setup.
func (c *Client) ProbeConnection(ctx context.Context, webhookDeliveryURL string) (*ProbeResult, error) {
	out := &ProbeResult{Checks: make([]ProbeCheck, 0, 10)}
	reachLabel := fmt.Sprintf("Can reach %s", c.displayBaseURL())

	// Connectivity: reachability + version
	info, err := c.GetInstance(ctx)
	if err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "reachability", Group: GroupConnectivity, Label: reachLabel, Status: ProbeFail, Detail: err.Error(),
		})
		return out, nil
	}
	out.Version = info.Version
	out.Checks = append(out.Checks, ProbeCheck{
		ID: "reachability", Group: GroupConnectivity, Label: reachLabel, Status: ProbeOK, Detail: "Gitea " + info.Version,
	})

	// Connectivity: authenticate token
	user, admin, err := c.getTokenUser(ctx)
	if err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "authenticate", Group: GroupConnectivity, Label: "Authenticate with token", Status: ProbeFail, Detail: err.Error(),
		})
		return out, nil
	}
	out.Login = user.Login
	out.IsAdmin = admin
	out.Checks = append(out.Checks, ProbeCheck{
		ID: "authenticate", Group: GroupConnectivity, Label: "Authenticate with token", Status: ProbeOK,
		Detail: fmt.Sprintf("Signed in as %s", user.Login),
	})

	// Connectivity: Lens public URL (webhook delivery target)
	if webhookDeliveryURL == "" {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "webhook_url", Group: GroupConnectivity, Label: "Lens public URL", Status: ProbeWarn,
			Detail: "Lens public URL is unset; webhook delivery URL unknown",
		})
	} else {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "webhook_url", Group: GroupConnectivity, Label: "Lens public URL", Status: ProbeOK,
			Detail: webhookDeliveryURL,
		})
	}

	// Permissions: site administrator (system hooks + future admin APIs)
	if admin {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "admin", Group: GroupPermissions, Label: "Site administrator", Status: ProbeOK, Detail: "Token user is a site admin",
		})
	} else {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "admin", Group: GroupPermissions, Label: "Site administrator", Status: ProbeFail,
			Detail: "Token user is not a site admin; required for system webhooks and full Lens setup",
		})
	}

	// Permissions: list repositories (sync catalog)
	var sample *models.Repository
	repos, reposErr := c.ListRepositories(ctx, forge.ListReposOpts{Page: 1, PageSize: 1})
	if reposErr != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_repos", Group: GroupPermissions, Label: "List repositories", Status: ProbeFail, Detail: reposErr.Error(),
		})
	} else {
		detail := "Repository search OK"
		if len(repos.Items) == 0 {
			detail = "Token can search repositories (none visible yet)"
		} else {
			detail = fmt.Sprintf("Can read repositories (e.g. %s)", repos.Items[0].FullName)
			sample = &repos.Items[0]
		}
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_repos", Group: GroupPermissions, Label: "List repositories", Status: ProbeOK, Detail: detail,
		})
	}

	// Permissions: list organizations
	if err := c.probeOK(ctx, http.MethodGet, "/user/orgs", url.Values{"limit": {"1"}, "page": {"1"}}); err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_orgs", Group: GroupPermissions, Label: "List organizations", Status: ProbeFail, Detail: err.Error(),
		})
	} else {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "list_orgs", Group: GroupPermissions, Label: "List organizations", Status: ProbeOK, Detail: "Organization list OK",
		})
	}

	// Permissions: pull requests
	if sample == nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "pull_requests", Group: GroupPermissions, Label: "Read pull requests", Status: ProbeSkip,
			Detail: "No repository available to probe",
		})
	} else {
		ref := models.RepoRef{Owner: sample.Owner, Name: sample.Name}
		_, err := c.ListPullRequests(ctx, ref, forge.PROpts{State: "open", Page: 1, PageSize: 1})
		if err != nil {
			out.Checks = append(out.Checks, ProbeCheck{
				ID: "pull_requests", Group: GroupPermissions, Label: "Read pull requests", Status: ProbeFail, Detail: err.Error(),
			})
		} else {
			out.Checks = append(out.Checks, ProbeCheck{
				ID: "pull_requests", Group: GroupPermissions, Label: "Read pull requests", Status: ProbeOK,
				Detail: fmt.Sprintf("OK on %s", sample.FullName),
			})
		}
	}

	// Permissions: Actions runs (+ commit status when we have a default branch)
	caps := &models.Capabilities{
		Version:            out.Version,
		OAuthProvider:      true,
		WorkflowRunWebhook: true,
		WorkflowJobWebhook: true,
	}
	if sample == nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "actions", Group: GroupPermissions, Label: "Read Actions runs", Status: ProbeSkip,
			Detail: "No repository available to probe",
		})
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "commit_status", Group: GroupPermissions, Label: "Read commit status", Status: ProbeSkip,
			Detail: "No repository available to probe",
		})
	} else {
		ref := models.RepoRef{Owner: sample.Owner, Name: sample.Name}
		_, err := c.ListWorkflowRuns(ctx, ref, forge.RunOpts{Page: 1, PageSize: 1})
		if err != nil {
			detail := err.Error()
			st := ProbeFail
			if strings.Contains(detail, "not found") || strings.Contains(detail, "404") {
				st = ProbeWarn
				detail = "Actions API not available on sample repo (may be disabled)"
			}
			out.Checks = append(out.Checks, ProbeCheck{
				ID: "actions", Group: GroupPermissions, Label: "Read Actions runs", Status: st, Detail: detail,
			})
		} else {
			caps.ActionsAPI = true
			caps.JobLogsAPI = true
			out.Checks = append(out.Checks, ProbeCheck{
				ID: "actions", Group: GroupPermissions, Label: "Read Actions runs", Status: ProbeOK,
				Detail: fmt.Sprintf("OK on %s", sample.FullName),
			})
		}
		branch := sample.DefaultBranch
		if branch == "" {
			branch = "main"
		}
		_, err = c.GetCombinedCommitStatus(ctx, ref, branch)
		if err != nil {
			detail := err.Error()
			st := ProbeWarn
			if strings.Contains(detail, "403") || strings.Contains(strings.ToLower(detail), "forbidden") {
				st = ProbeFail
			}
			out.Checks = append(out.Checks, ProbeCheck{
				ID: "commit_status", Group: GroupPermissions, Label: "Read commit status", Status: st, Detail: detail,
			})
		} else {
			out.Checks = append(out.Checks, ProbeCheck{
				ID: "commit_status", Group: GroupPermissions, Label: "Read commit status", Status: ProbeOK,
				Detail: fmt.Sprintf("OK on %s@%s", sample.FullName, branch),
			})
		}
	}

	// Permissions: system webhooks API (create/list)
	if err := c.probeOK(ctx, http.MethodGet, "/admin/hooks", url.Values{"limit": {"1"}, "type": {"system"}}); err != nil {
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "system_hooks", Group: GroupPermissions, Label: "Manage system webhooks", Status: ProbeFail, Detail: err.Error(),
		})
		caps.SystemHooksAPI = false
	} else {
		caps.SystemHooksAPI = true
		out.CanCreateHook = true
		out.Checks = append(out.Checks, ProbeCheck{
			ID: "system_hooks", Group: GroupPermissions, Label: "Manage system webhooks", Status: ProbeOK,
			Detail: "Can list system hooks via admin API",
		})
	}

	out.Capabilities = caps
	if webhookDeliveryURL == "" {
		out.CanCreateHook = false
	}
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

func (c *Client) getTokenUser(ctx context.Context) (giteaUser, bool, error) {
	resp, err := c.do(ctx, http.MethodGet, "/user", nil, "")
	if err != nil {
		return giteaUser{}, false, err
	}
	var u giteaUser
	if err := decodeJSON(resp, &u); err != nil {
		return giteaUser{}, false, err
	}
	if u.Login == "" {
		return giteaUser{}, false, fmt.Errorf("token did not return a user")
	}
	return u, u.IsAdmin, nil
}

func (c *Client) probeOK(ctx context.Context, method, path string, query url.Values) error {
	resp, err := c.do(ctx, method, path, query, "")
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}
