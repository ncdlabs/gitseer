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
		"name": "lens",
		"full_name": "ncdlabs/lens",
		"private": true,
		"fork": false,
		"empty": false,
		"archived": false,
		"html_url": "https://git.example.com/ncdlabs/lens",
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
	if m.Owner != "ncdlabs" || m.Name != "lens" || m.FullName != "ncdlabs/lens" {
		t.Fatalf("identity=%s/%s (%s)", m.Owner, m.Name, m.FullName)
	}
	if !m.Private || m.DefaultBranch != "main" {
		t.Fatalf("flags private=%v branch=%s", m.Private, m.DefaultBranch)
	}
	if m.ID != 0 {
		t.Fatal("Lens ID must not come from Gitea id")
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
	want := `This Gitea URL points to a private network address (127.0.0.1). Check "Allow Private Network Addresses" to allow Lens to connect.`
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
