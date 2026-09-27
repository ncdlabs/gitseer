package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Hook is a minimal GitHub org/repo hook response.
type Hook struct {
	ID     int64             `json:"id"`
	Name   string            `json:"name"`
	Active bool              `json:"active"`
	Events []string          `json:"events"`
	Config map[string]string `json:"config"`
}

type createHookBody struct {
	Name   string            `json:"name"`
	Active bool              `json:"active"`
	Events []string          `json:"events"`
	Config map[string]string `json:"config"`
}

// EnsureOrgWebhook creates or updates an organization webhook at deliveryURL.
// Requires a PAT with admin:org_hook (or equivalent) on the org.
// Returns the hook and whether it was newly created (false = updated existing).
func (c *Client) EnsureOrgWebhook(ctx context.Context, org, deliveryURL, secret string) (*Hook, bool, error) {
	org = strings.TrimSpace(org)
	deliveryURL = strings.TrimRight(strings.TrimSpace(deliveryURL), "/")
	if org == "" {
		return nil, false, fmt.Errorf("org is required")
	}
	if deliveryURL == "" {
		return nil, false, fmt.Errorf("webhook delivery url is required")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, false, fmt.Errorf("webhook secret is required")
	}

	existing, err := c.findOrgHookByURL(ctx, org, deliveryURL)
	if err != nil {
		return nil, false, err
	}
	body := createHookBody{
		Name:   "web",
		Active: true,
		Events: append([]string(nil), DefaultWebhookEvents...),
		Config: map[string]string{
			"url":          deliveryURL,
			"content_type": "json",
			"secret":       secret,
			"insecure_ssl": "0",
		},
	}
	if existing != nil {
		resp, err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/orgs/%s/hooks/%d", url.PathEscape(org), existing.ID), nil, body)
		if err != nil {
			return nil, false, err
		}
		var hook Hook
		if err := decodeJSON(resp, &hook); err != nil {
			return nil, false, err
		}
		return &hook, false, nil
	}
	resp, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/orgs/%s/hooks", url.PathEscape(org)), nil, body)
	if err != nil {
		return nil, false, err
	}
	var hook Hook
	if err := decodeJSON(resp, &hook); err != nil {
		return nil, false, err
	}
	return &hook, true, nil
}

// EnsureRepoWebhook creates or updates a repository webhook at deliveryURL.
// Requires admin on the repository (or repo hook scope).
func (c *Client) EnsureRepoWebhook(ctx context.Context, owner, repo, deliveryURL, secret string) (*Hook, bool, error) {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	deliveryURL = strings.TrimRight(strings.TrimSpace(deliveryURL), "/")
	if owner == "" || repo == "" {
		return nil, false, fmt.Errorf("owner and repo are required")
	}
	if deliveryURL == "" {
		return nil, false, fmt.Errorf("webhook delivery url is required")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, false, fmt.Errorf("webhook secret is required")
	}

	existing, err := c.findRepoHookByURL(ctx, owner, repo, deliveryURL)
	if err != nil {
		return nil, false, err
	}
	body := createHookBody{
		Name:   "web",
		Active: true,
		Events: append([]string(nil), DefaultWebhookEvents...),
		Config: map[string]string{
			"url":          deliveryURL,
			"content_type": "json",
			"secret":       secret,
			"insecure_ssl": "0",
		},
	}
	pathBase := fmt.Sprintf("/repos/%s/%s/hooks", url.PathEscape(owner), url.PathEscape(repo))
	if existing != nil {
		resp, err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("%s/%d", pathBase, existing.ID), nil, body)
		if err != nil {
			return nil, false, err
		}
		var hook Hook
		if err := decodeJSON(resp, &hook); err != nil {
			return nil, false, err
		}
		return &hook, false, nil
	}
	resp, err := c.doJSON(ctx, http.MethodPost, pathBase, nil, body)
	if err != nil {
		return nil, false, err
	}
	var hook Hook
	if err := decodeJSON(resp, &hook); err != nil {
		return nil, false, err
	}
	return &hook, true, nil
}

// CanManageOrgHooks reports whether the token can list hooks on org (admin:org_hook).
func (c *Client) CanManageOrgHooks(ctx context.Context, org string) bool {
	org = strings.TrimSpace(org)
	if org == "" {
		return false
	}
	_, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/orgs/%s/hooks", url.PathEscape(org)), url.Values{"per_page": {"1"}}, "")
	return err == nil
}

// ListMembershipOrgs returns login names of orgs the authenticated user belongs to.
func (c *Client) ListMembershipOrgs(ctx context.Context) ([]string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/user/orgs", url.Values{"per_page": {"100"}}, "")
	if err != nil {
		return nil, err
	}
	var orgs []struct {
		Login string `json:"login"`
	}
	if err := decodeJSON(resp, &orgs); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(orgs))
	for _, o := range orgs {
		if strings.TrimSpace(o.Login) != "" {
			out = append(out, o.Login)
		}
	}
	return out, nil
}

func (c *Client) findOrgHookByURL(ctx context.Context, org, deliveryURL string) (*Hook, error) {
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/orgs/%s/hooks", url.PathEscape(org)), url.Values{
		"per_page": {"100"},
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

func (c *Client) findRepoHookByURL(ctx context.Context, owner, repo, deliveryURL string) (*Hook, error) {
	resp, err := c.do(ctx, http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/hooks", url.PathEscape(owner), url.PathEscape(repo)),
		url.Values{"per_page": {"100"}}, "")
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
