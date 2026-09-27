// Package forgejo implements forge.Forge as a thin adapter over the Gitea client
// (Forgejo is API-compatible with Gitea). Distinct forge_type and webhook path.
package forgejo

import (
	"context"
	"io"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/forge/gitea"
	"github.com/ncdlabs/gitseer/internal/models"
)

func init() {
	forge.Register(models.ForgeTypeForgejo, func(baseURL, token string, allowPrivateNetwork bool) (forge.Forge, error) {
		return New(baseURL, token, allowPrivateNetwork)
	})
}

// Client wraps a Gitea client while advertising forge_type=forgejo.
type Client struct {
	inner *gitea.Client
}

// New builds a Forgejo client (Gitea-compatible REST under the hood).
func New(baseURL, token string, allowPrivateNetwork bool) (*Client, error) {
	inner, err := gitea.New(baseURL, token, allowPrivateNetwork)
	if err != nil {
		return nil, err
	}
	return &Client{inner: inner}, nil
}

// Inner exposes the underlying Gitea client for hooks/OAuth helpers that are API-identical.
func (c *Client) Inner() *gitea.Client { return c.inner }

func (c *Client) GetInstance(ctx context.Context) (*models.InstanceInfo, error) {
	return c.inner.GetInstance(ctx)
}

func (c *Client) DetectCapabilities(ctx context.Context) (*models.Capabilities, error) {
	return c.inner.DetectCapabilities(ctx)
}

func (c *Client) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	return c.inner.ListOrganizations(ctx)
}

func (c *Client) ListRepositories(ctx context.Context, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return c.inner.ListRepositories(ctx, opts)
}

func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
	return c.inner.GetRepository(ctx, owner, repo)
}

func (c *Client) ListPullRequests(ctx context.Context, repo models.RepoRef, opts forge.PROpts) (forge.Page[models.PullRequest], error) {
	return c.inner.ListPullRequests(ctx, repo, opts)
}

func (c *Client) GetPullRequestReviewState(ctx context.Context, repo models.RepoRef, number int64) (string, error) {
	return c.inner.GetPullRequestReviewState(ctx, repo, number)
}

func (c *Client) GetCombinedCommitStatus(ctx context.Context, repo models.RepoRef, ref string) (string, error) {
	return c.inner.GetCombinedCommitStatus(ctx, repo, ref)
}

func (c *Client) ListWorkflowRuns(ctx context.Context, repo models.RepoRef, opts forge.RunOpts) (forge.Page[models.WorkflowRun], error) {
	return c.inner.ListWorkflowRuns(ctx, repo, opts)
}

func (c *Client) GetWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) (*models.WorkflowRun, error) {
	return c.inner.GetWorkflowRun(ctx, repo, runExternalID)
}

func (c *Client) ListJobs(ctx context.Context, repo models.RepoRef, runExternalID int64) ([]models.Job, error) {
	return c.inner.ListJobs(ctx, repo, runExternalID)
}

func (c *Client) GetJobLogs(ctx context.Context, repo models.RepoRef, jobExternalID int64) (io.ReadCloser, error) {
	return c.inner.GetJobLogs(ctx, repo, jobExternalID)
}

func (c *Client) GetWorkflowYAML(ctx context.Context, repo models.RepoRef, path, ref string) ([]byte, error) {
	return c.inner.GetWorkflowYAML(ctx, repo, path, ref)
}

func (c *Client) RerunWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) error {
	return c.inner.RerunWorkflowRun(ctx, repo, runExternalID)
}

func (c *Client) CancelWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) error {
	return c.inner.CancelWorkflowRun(ctx, repo, runExternalID)
}

func (c *Client) ListAccessibleReposForUser(ctx context.Context, userToken string, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return c.inner.ListAccessibleReposForUser(ctx, userToken, opts)
}

func (c *Client) GetAuthenticatedUser(ctx context.Context, userToken string) (*models.User, error) {
	return c.inner.GetAuthenticatedUser(ctx, userToken)
}

// ProbeConnection delegates to the Gitea probe (Forgejo is API-compatible).
func (c *Client) ProbeConnection(ctx context.Context, webhookDeliveryURL, oauthRedirectURI string) (*gitea.ProbeResult, error) {
	res, err := c.inner.ProbeConnection(ctx, webhookDeliveryURL, oauthRedirectURI)
	if res != nil {
		// Annotate checks detail when present; forge_type is not on ProbeResult for Gitea —
		// callers set forge_type from the instance.
		_ = models.ForgeTypeForgejo
	}
	return res, err
}

func (c *Client) ListRunners(ctx context.Context) ([]models.ForgeRunner, error) {
	return c.inner.ListRunners(ctx)
}

