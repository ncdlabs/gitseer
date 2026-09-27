// Package uiinstall installs/uninstalls Gitea custom template snippets with marker-safe edits.
package uiinstall

import (
	"flag"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	beginLinks = "<!-- BEGIN GITSEER -->"
	endLinks   = "<!-- END GITSEER -->"
	beginTabs  = "<!-- BEGIN GITSEER-TABS -->"
	endTabs    = "<!-- END GITSEER-TABS -->"
)

func InstallCmd(args []string) {
	fs := flag.NewFlagSet("install-ui", flag.ExitOnError)
	customPath := fs.String("custom-path", "", "path to Gitea custom/ directory")
	gitseerURL := fs.String("gitseer-url", "", "public GitSeer base URL (e.g. https://git.example.com/gitseer)")
	instanceID := fs.Int64("instance-id", 0, "GitSeer forge instance id for deep links (optional)")
	_ = fs.Parse(args)
	if *customPath == "" || *gitseerURL == "" {
		fmt.Fprintln(os.Stderr, "usage: gitseer install-ui --custom-path DIR --gitseer-url URL [--instance-id N]")
		os.Exit(2)
	}
	if err := Install(*customPath, strings.TrimRight(*gitseerURL, "/"), *instanceID); err != nil {
		fmt.Fprintf(os.Stderr, "install-ui: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("GitSeer UI snippets installed (marker-safe).")
}

func UninstallCmd(args []string) {
	fs := flag.NewFlagSet("uninstall-ui", flag.ExitOnError)
	customPath := fs.String("custom-path", "", "path to Gitea custom/ directory")
	_ = fs.Parse(args)
	if *customPath == "" {
		fmt.Fprintln(os.Stderr, "usage: gitseer uninstall-ui --custom-path DIR")
		os.Exit(2)
	}
	if err := Uninstall(*customPath); err != nil {
		fmt.Fprintf(os.Stderr, "uninstall-ui: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("GitSeer UI snippets removed.")
}

// Template relative paths under Gitea custom/templates/custom/.
const (
	ExtraLinksRel = "extra_links.tmpl"
	ExtraTabsRel  = "extra_tabs.tmpl"
)

// SnippetFiles returns marker-wrapped template bodies keyed by relative path
// (extra_links.tmpl, extra_tabs.tmpl). Same content Install writes; safe for download.
func SnippetFiles(gitseerURL string, instanceID int64) (map[string]string, error) {
	safeURL, err := sanitizeGitSeerURL(gitseerURL)
	if err != nil {
		return nil, err
	}
	href := html.EscapeString(safeURL)
	repoPath := fmt.Sprintf("%s/repositories/{{.Owner.Name}}/{{.Repository.Name}}", href)
	if instanceID > 0 {
		repoPath = fmt.Sprintf("%s?instance_id=%d", repoPath, instanceID)
	}
	linksSnippet := fmt.Sprintf(`%s
<a class="item" href="%s" target="_blank" rel="noopener noreferrer">GitSeer</a>
%s
`, beginLinks, href, endLinks)
	tabsSnippet := fmt.Sprintf(`%s
<a class="item" href="%s" target="_blank" rel="noopener noreferrer">GitSeer</a>
%s
`, beginTabs, repoPath, endTabs)
	return map[string]string{
		ExtraLinksRel: linksSnippet,
		ExtraTabsRel:  tabsSnippet,
	}, nil
}

// ConcatenatedSnippets returns a single text document with file headers for each snippet.
func ConcatenatedSnippets(gitseerURL string, instanceID int64) (string, error) {
	files, err := SnippetFiles(gitseerURL, instanceID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	order := []string{ExtraLinksRel, ExtraTabsRel}
	for i, name := range order {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("===== ")
		b.WriteString(name)
		b.WriteString(" =====\n")
		b.WriteString(files[name])
		if !strings.HasSuffix(files[name], "\n") {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

func Install(customPath, gitseerURL string, instanceID int64) error {
	files, err := SnippetFiles(gitseerURL, instanceID)
	if err != nil {
		return err
	}
	templates := filepath.Join(customPath, "templates", "custom")
	if err := os.MkdirAll(templates, 0o755); err != nil {
		return err
	}
	if err := upsertMarkedFile(filepath.Join(templates, ExtraLinksRel), beginLinks, endLinks, files[ExtraLinksRel]); err != nil {
		return err
	}
	return upsertMarkedFile(filepath.Join(templates, ExtraTabsRel), beginTabs, endTabs, files[ExtraTabsRel])
}

func Uninstall(customPath string) error {
	templates := filepath.Join(customPath, "templates", "custom")
	if err := stripMarkedFile(filepath.Join(templates, "extra_links.tmpl"), beginLinks, endLinks); err != nil {
		return err
	}
	return stripMarkedFile(filepath.Join(templates, "extra_tabs.tmpl"), beginTabs, endTabs)
}

func upsertMarkedFile(path, begin, end, snippet string) error {
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return err
	}
	// Strip every marked block first so re-install / restructured templates cannot
	// double-insert when markers are duplicated or partially left behind.
	cleaned, err := removeAllBetween(existing, begin, end)
	if err != nil {
		return err
	}
	out := cleaned
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += strings.TrimRight(snippet, "\n") + "\n"
	return os.WriteFile(path, []byte(out), 0o644)
}

func stripMarkedFile(path, begin, end string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	updated, err := removeAllBetween(string(b), begin, end)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(updated)
	if trimmed == "" {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

// removeAllBetween removes every begin…end region (idempotent). Orphan begin/end
// pairs (begin without matching end, or unequal counts) fail closed.
func removeAllBetween(content, begin, end string) (string, error) {
	begins := strings.Count(content, begin)
	ends := strings.Count(content, end)
	if begins != ends {
		return "", fmt.Errorf("malformed markers: found %d %q and %d %q (run uninstall-ui or fix the template)", begins, begin, ends, end)
	}
	for begins > 0 {
		start := strings.Index(content, begin)
		stop := strings.Index(content[start:], end)
		if start < 0 || stop < 0 {
			return "", fmt.Errorf("malformed markers in file")
		}
		stop = start + stop + len(end)
		content = content[:start] + content[stop:]
		begins--
	}
	return content, nil
}

func sanitizeGitSeerURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid gitseer-url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("gitseer-url must be http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("gitseer-url host is required")
	}
	u.Fragment = ""
	u.RawQuery = ""
	return strings.TrimRight(u.String(), "/"), nil
}
