package gitea

import (
	"encoding/json"
	"net"
	"net/url"
	"testing"
)

func TestMapRepoNormalization(t *testing.T) {
	raw := `{
		"id": 42,
		"name": "gitseer",
		"full_name": "ncdlabs/gitseer",
		"private": true,
		"fork": false,
		"empty": false,
		"archived": false,
		"html_url": "https://git.example.com/ncdlabs/gitseer",
		"default_branch": "main",
		"owner": {"id": 1, "login": "ncdlabs", "full_name": "ncdLabs", "avatar_url": ""}
	}`
	var gr giteaRepo
	if err := json.Unmarshal([]byte(raw), &gr); err != nil {
		t.Fatal(err)
	}
	m := mapRepo(gr)
	if m.ExternalID != 42 {
		t.Fatalf("external_id=%d", m.ExternalID)
	}
	if m.Owner != "ncdlabs" || m.Name != "gitseer" || m.FullName != "ncdlabs/gitseer" {
		t.Fatalf("identity=%s/%s (%s)", m.Owner, m.Name, m.FullName)
	}
	if !m.Private || m.DefaultBranch != "main" {
		t.Fatalf("flags private=%v branch=%s", m.Private, m.DefaultBranch)
	}
	if m.ID != 0 {
		t.Fatal("GitSeer ID must not come from Gitea id")
	}
}

func TestMapRepoOwnerFromFullName(t *testing.T) {
	m := mapRepo(giteaRepo{ID: 1, Name: "r", FullName: "alice/r"})
	if m.Owner != "alice" {
		t.Fatalf("owner=%q", m.Owner)
	}
}

func TestValidateURLBlocksLoopback(t *testing.T) {
	err := validateURL(mustParse("http://127.0.0.1:3000"), false)
	if err == nil {
		t.Fatal("expected block")
	}
	want := `This Gitea URL points to a private network address (127.0.0.1). Check "Allow Private Network Addresses" to allow GitSeer to connect.`
	if err.Error() != want {
		t.Fatalf("error=%q want=%q", err.Error(), want)
	}
	if err := validateURL(mustParse("http://127.0.0.1:3000"), true); err != nil {
		t.Fatal(err)
	}
}

func TestValidateURLFailsClosedOnBadDNS(t *testing.T) {
	err := validateURL(mustParse("http://this-host-should-not-resolve.invalid"), false)
	if err == nil {
		t.Fatal("expected dns failure to block")
	}
}

func TestIsBlockedIPPrivateV6(t *testing.T) {
	ip := net.ParseIP("fc00::1")
	if ip == nil || !isBlockedIP(ip) {
		t.Fatal("expected ULA blocked")
	}
}

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestMapRunGiteaActionsShape(t *testing.T) {
	// Gitea 1.25 Actions: started_at/completed_at/display_title; no name/created_at/updated_at.
	raw := `{
		"id": 12997,
		"display_title": "Set GA4 measurement ID",
		"path": "deploy.yaml@refs/heads/main",
		"event": "push",
		"run_attempt": 0,
		"run_number": 30,
		"head_sha": "abc",
		"head_branch": "main",
		"status": "completed",
		"conclusion": "failure",
		"started_at": "2026-09-28T21:43:32Z",
		"completed_at": "2026-09-28T21:45:31Z",
		"html_url": "https://git.example.com/o/r/actions/runs/30",
		"actor": {"login": "lou"}
	}`
	var gr giteaRun
	if err := json.Unmarshal([]byte(raw), &gr); err != nil {
		t.Fatal(err)
	}
	m := mapRun(gr)
	if m.ExternalID != 12997 {
		t.Fatalf("external_id=%d", m.ExternalID)
	}
	if m.Status != "completed" || m.Conclusion != "failure" {
		t.Fatalf("status=%q conclusion=%q", m.Status, m.Conclusion)
	}
	if m.Name != "deploy.yaml" {
		t.Fatalf("name=%q want deploy.yaml (path-derived; display_title is commit subject)", m.Name)
	}
	if m.WorkflowPath != "deploy.yaml" {
		t.Fatalf("path=%q", m.WorkflowPath)
	}
	if m.RunAttempt != 1 {
		t.Fatalf("run_attempt=%d want 1 (zero coerced)", m.RunAttempt)
	}
	if m.StartedAt == nil || m.CompletedAt == nil {
		t.Fatalf("timestamps started=%v completed=%v", m.StartedAt, m.CompletedAt)
	}
	if m.ActorLogin != "lou" {
		t.Fatalf("actor=%q", m.ActorLogin)
	}
}

