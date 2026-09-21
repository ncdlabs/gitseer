package gitea

import "testing"

func TestNewOAuthAppPreview(t *testing.T) {
	p := NewOAuthAppPreview("https://lens.example.com/api/v1/auth/callback")
	if p.Name != DefaultOAuthAppName {
		t.Fatalf("name: got %q", p.Name)
	}
	if p.RedirectURI != "https://lens.example.com/api/v1/auth/callback" {
		t.Fatalf("redirect: got %q", p.RedirectURI)
	}
	if !p.ConfidentialClient {
		t.Fatal("expected confidential client")
	}
	if p.GiteaSettingsPath == "" || p.GiteaAdminAppsPath == "" {
		t.Fatal("expected gitea path hints")
	}
}
