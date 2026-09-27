package bitbucket

import (
	"context"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
)

// ListRunners is unsupported for Bitbucket Cloud Pipelines (no public runners inventory API
// equivalent to GitLab/GitHub Actions runners for typical OAuth/PAT scopes).
func (c *Client) ListRunners(ctx context.Context) ([]models.ForgeRunner, error) {
	_ = ctx
	return nil, forge.ErrUnsupported
}
