package workflows

import "strings"

// NormalizeWorkflowPath turns Gitea Actions pseudo-paths into a stable file id.
// Gitea 1.25 often returns path as "ci.yaml@refs/heads/main" rather than a repo path.
func NormalizeWorkflowPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if i := strings.IndexByte(path, '@'); i >= 0 {
		path = strings.TrimSpace(path[:i])
	}
	return path
}

// DisplayWorkflowName picks a human-readable workflow label when upstream name is empty.
func DisplayWorkflowName(name, path string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	n := NormalizeWorkflowPath(path)
	if n == "" {
		return ""
	}
	if i := strings.LastIndexByte(n, '/'); i >= 0 && i+1 < len(n) {
		return n[i+1:]
	}
	return n
}

// CandidateWorkflowPaths lists repo-relative paths to try when fetching workflow YAML.
func CandidateWorkflowPaths(path string) []string {
	n := NormalizeWorkflowPath(path)
	if n == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = strings.TrimPrefix(strings.TrimSpace(p), "/")
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	if strings.HasPrefix(n, ".gitea/workflows/") || strings.HasPrefix(n, ".github/workflows/") {
		add(n)
		return out
	}
	if strings.Contains(n, "/") {
		add(n)
	}
	base := n
	if i := strings.LastIndexByte(n, '/'); i >= 0 {
		base = n[i+1:]
	}
	if base != "" {
		add(".gitea/workflows/" + base)
		add(".github/workflows/" + base)
	}
	return out
}
