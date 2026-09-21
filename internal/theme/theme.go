// Package theme maps forge theme preferences onto Lens theme IDs.
package theme

import "strings"

// ID is a Lens UI theme identifier.
type ID string

const (
	Light    ID = "light"
	Dark     ID = "dark"
	System   ID = "system"
	Gruvbox  ID = "gruvbox"
	Terminal ID = "terminal"
)

// MapGitea maps a Gitea user theme name to a Lens theme.
// Only the built-in defaults (gitea-light, gitea-dark, gitea-auto) are recognized.
// Custom, colorblind, or unknown themes fail closed (ok=false).
func MapGitea(name string) (ID, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gitea-light":
		return Light, true
	case "gitea-dark":
		return Dark, true
	case "gitea-auto":
		return System, true
	default:
		return "", false
	}
}
