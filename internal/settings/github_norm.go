package settings

import "github.com/ncdlabs/gitseer/internal/forge/github"

func init() {
	tryNormalizeGitHubAPI = github.NormalizeBaseURL
}
