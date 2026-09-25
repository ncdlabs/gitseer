// Package all blank-imports forge implementations so forge.New constructors register.
package all

import (
	_ "github.com/ncdlabs/gitseer/internal/forge/gitea"
	_ "github.com/ncdlabs/gitseer/internal/forge/github"
)
