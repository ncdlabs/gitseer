package settings

import "github.com/ncdlabs/gitseer/internal/forge/bitbucket"

func init() {
	tryNormalizeBitbucketAPI = bitbucket.NormalizeBaseURL
}
