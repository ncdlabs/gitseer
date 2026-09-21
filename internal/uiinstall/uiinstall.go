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
	beginLinks = "<!-- BEGIN GITEA-LENS -->"
	endLinks   = "<!-- END GITEA-LENS -->"
	beginTabs  = "<!-- BEGIN GITEA-LENS-TABS -->"
	endTabs    = "<!-- END GITEA-LENS-TABS -->"
)

func InstallCmd(args []string) {
	fs := flag.NewFlagSet("install-ui", flag.ExitOnError)
	customPath := fs.String("custom-path", "", "path to Gitea custom/ directory")
	lensURL := fs.String("lens-url", "", "public Lens base URL (e.g. https://git.example.com/lens)")
	_ = fs.Parse(args)
	if *customPath == "" || *lensURL == "" {
		fmt.Fprintln(os.Stderr, "usage: lens install-ui --custom-path DIR --lens-url URL")
		os.Exit(2)
	}
	if err := Install(*customPath, strings.TrimRight(*lensURL, "/")); err != nil {
		fmt.Fprintf(os.Stderr, "install-ui: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Gitea Lens UI snippets installed (marker-safe).")
}

func UninstallCmd(args []string) {
	fs := flag.NewFlagSet("uninstall-ui", flag.ExitOnError)
	customPath := fs.String("custom-path", "", "path to Gitea custom/ directory")
	_ = fs.Parse(args)
	if *customPath == "" {
		fmt.Fprintln(os.Stderr, "usage: lens uninstall-ui --custom-path DIR")
		os.Exit(2)
	}
	if err := Uninstall(*customPath); err != nil {
		fmt.Fprintf(os.Stderr, "uninstall-ui: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Gitea Lens UI snippets removed.")
}

func Install(customPath, lensURL string) error {
	safeURL, err := sanitizeLensURL(lensURL)
	if err != nil {
		return err
	}
	templates := filepath.Join(customPath, "templates", "custom")
	if err := os.MkdirAll(templates, 0o755); err != nil {
		return err
	}
	href := html.EscapeString(safeURL)
	linksSnippet := fmt.Sprintf(`%s
<a class="item" href="%s" target="_blank" rel="noopener noreferrer">Lens</a>
%s
`, beginLinks, href, endLinks)
	tabsSnippet := fmt.Sprintf(`%s
<a class="item" href="%s/repositories/{{.Owner.Name}}/{{.Repository.Name}}" target="_blank" rel="noopener noreferrer">Lens</a>
%s
`, beginTabs, href, endTabs)

	if err := upsertMarkedFile(filepath.Join(templates, "extra_links.tmpl"), beginLinks, endLinks, linksSnippet); err != nil {
		return err
	}
	return upsertMarkedFile(filepath.Join(templates, "extra_tabs.tmpl"), beginTabs, endTabs, tabsSnippet)
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
	if strings.Contains(existing, begin) && strings.Contains(existing, end) {
		updated, err := replaceBetween(existing, begin, end, snippet)
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(updated), 0o644)
	}
	// Preserve any admin content; append marked block.
	out := existing
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += snippet
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
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
	updated, err := removeBetween(string(b), begin, end)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(updated)
	if trimmed == "" {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func replaceBetween(content, begin, end, replacement string) (string, error) {
	start := strings.Index(content, begin)
	stop := strings.Index(content, end)
	if start < 0 || stop < 0 || stop < start {
		return "", fmt.Errorf("malformed markers in file")
	}
	stop += len(end)
	return content[:start] + strings.TrimRight(replacement, "\n") + content[stop:], nil
}

func removeBetween(content, begin, end string) (string, error) {
	start := strings.Index(content, begin)
	stop := strings.Index(content, end)
	if start < 0 || stop < 0 {
		return content, nil
	}
	if stop < start {
		return "", fmt.Errorf("malformed markers in file")
	}
	stop += len(end)
	out := content[:start] + content[stop:]
	return out, nil
}

func sanitizeLensURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid lens-url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("lens-url must be http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("lens-url host is required")
	}
	u.Fragment = ""
	u.RawQuery = ""
	return strings.TrimRight(u.String(), "/"), nil
}
