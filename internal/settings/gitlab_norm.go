package settings

import "github.com/ncdlabs/gitseer/internal/forge/gitlab"

func init() {
	tryNormalizeGitLabAPI = gitlab.NormalizeBaseURL
}
