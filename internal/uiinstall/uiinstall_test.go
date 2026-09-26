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

func TestInstallIncludesInstanceID(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom")
	if err := Install(custom, "https://gitseer.example.com", 42); err != nil {
		t.Fatal(err)
	}
	tabs := filepath.Join(custom, "templates", "custom", "extra_tabs.tmpl")
	b, err := os.ReadFile(tabs)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "instance_id=42") {
		t.Fatalf("missing instance_id query: %s", s)
	}
	if !strings.Contains(s, "/repositories/{{.Owner.Name}}/{{.Repository.Name}}?instance_id=42") {
		t.Fatalf("unexpected repo deep link: %s", s)
	}
}
