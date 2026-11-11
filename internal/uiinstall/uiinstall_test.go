package uiinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPreservesAdminContent(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom")
	tmpl := filepath.Join(custom, "templates", "custom")
	if err := os.MkdirAll(tmpl, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(tmpl, "extra_links.tmpl")
	if err := os.WriteFile(existing, []byte("<a class=\"item\" href=\"/admin\">Admin</a>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(custom, "https://git.example.com/gitseer", 0); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "Admin") {
		t.Fatal("admin content overwritten")
	}
	if !strings.Contains(s, beginLinks) || !strings.Contains(s, "https://git.example.com/gitseer") {
		t.Fatalf("missing gitseer markers: %s", s)
	}
	if !strings.Contains(s, `target="_blank"`) || !strings.Contains(s, `rel="noopener noreferrer"`) {
		t.Fatalf("GitSeer link must open in a new tab: %s", s)
	}
	if err := Uninstall(custom); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Admin") {
		t.Fatal("admin content lost on uninstall")
	}
	if strings.Contains(string(b), beginLinks) {
		t.Fatal("markers not removed")
	}
}

func TestInstallIsHeaderOnly(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom")
	if err := Install(custom, "https://gitseer.example.com", 42); err != nil {
		t.Fatal(err)
	}
	// Header link must exist.
	b, err := os.ReadFile(filepath.Join(custom, "templates", "custom", ExtraLinksRel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), beginLinks) || !strings.Contains(string(b), "GitSeer") {
		t.Fatalf("header link missing: %s", string(b))
	}
	// No repo-tab file may be created.
	if _, err := os.Stat(filepath.Join(custom, "templates", "custom", ExtraTabsRel)); !os.IsNotExist(err) {
		t.Fatal("extra_tabs.tmpl must not be installed (header navbar only)")
	}
}

func TestInstallStripsLegacyRepoTabs(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom")
	tmpl := filepath.Join(custom, "templates", "custom")
	if err := os.MkdirAll(tmpl, 0o755); err != nil {
		t.Fatal(err)
	}
	// Legacy install wrote both files; re-install must leave header only.
	legacyTabs := beginTabs + "\n<a>GitSeer</a>\n" + endTabs + "\n"
	if err := os.WriteFile(filepath.Join(tmpl, ExtraTabsRel), []byte(legacyTabs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(custom, "https://gitseer.example.com", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmpl, ExtraTabsRel)); !os.IsNotExist(err) {
		rem, _ := os.ReadFile(filepath.Join(tmpl, ExtraTabsRel))
		if strings.Contains(string(rem), beginTabs) {
			t.Fatalf("legacy tab block not stripped: %s", string(rem))
		}
	}
}

func TestInstallIdempotentNoDoubleInsert(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom")
	if err := Install(custom, "https://gitseer.example.com", 0); err != nil {
		t.Fatal(err)
	}
	if err := Install(custom, "https://gitseer.example.com/v2", 0); err != nil {
		t.Fatal(err)
	}
	links := filepath.Join(custom, "templates", "custom", ExtraLinksRel)
	b, err := os.ReadFile(links)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, beginLinks) != 1 || strings.Count(s, endLinks) != 1 {
		t.Fatalf("expected single marker pair after re-install: %s", s)
	}
	if !strings.Contains(s, "https://gitseer.example.com/v2") {
		t.Fatalf("URL not updated: %s", s)
	}
	if strings.Count(s, "https://gitseer.example.com") != 1 {
		t.Fatalf("expected exactly one gitseer URL: %s", s)
	}
}

func TestInstallRejectsMalformedMarkers(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom")
	tmpl := filepath.Join(custom, "templates", "custom")
	if err := os.MkdirAll(tmpl, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmpl, ExtraLinksRel)
	if err := os.WriteFile(path, []byte(beginLinks+"\norphan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(custom, "https://gitseer.example.com", 0); err == nil {
		t.Fatal("expected error on begin without end")
	}
}

func TestInstallStripsDuplicateMarkers(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom")
	tmpl := filepath.Join(custom, "templates", "custom")
	if err := os.MkdirAll(tmpl, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmpl, ExtraLinksRel)
	dup := beginLinks + "\nold\n" + endLinks + "\n" + beginLinks + "\nalso\n" + endLinks + "\n"
	if err := os.WriteFile(path, []byte(dup), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(custom, "https://gitseer.example.com", 0); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, beginLinks) != 1 {
		t.Fatalf("expected one begin after install: %s", s)
	}
	if strings.Contains(s, "old") || strings.Contains(s, "also") {
		t.Fatalf("old blocks not stripped: %s", s)
	}
}

func TestSnippetFilesMatchesInstall(t *testing.T) {
	files, err := SnippetFiles("https://gitseer.example.com/", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files[ExtraLinksRel], beginLinks) {
		t.Fatal("links missing markers")
	}
	if _, ok := files[ExtraTabsRel]; ok {
		t.Fatal("header-only: no extra_tabs snippet expected")
	}
	concat, err := ConcatenatedSnippets("https://gitseer.example.com", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(concat, "===== extra_links.tmpl =====") {
		t.Fatalf("concat missing links header: %s", concat)
	}
	if strings.Contains(concat, "extra_tabs.tmpl") {
		t.Fatalf("concat must not include tabs: %s", concat)
	}
}
