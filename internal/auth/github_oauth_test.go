package auth

import (
	"testing"

	"github.com/ncdlabs/gitseer/internal/forge/github"
)

func TestGitHubOAuthRedirectURI(t *testing.T) {
	s := &Service{cfg: Config{ExternalURL: "https://gitseer.example.com/"}}
	got := s.GitHubRedirectURI()
	want := "https://gitseer.example.com/api/v1/auth/github/callback"
	if got != want {
		t.Fatalf("GitHubRedirectURI=%q want %q", got, want)
	}
}

func TestGitHubOAuthEnabled(t *testing.T) {
	s := &Service{cfg: Config{}}
	if s.GitHubOAuthEnabled() {
		t.Fatal("expected disabled")
	}
	s.cfg.GitHubBaseURL = "https://github.com"
	s.cfg.GitHubOAuthClientID = "client"
	s.cfg.ExternalURL = "https://gitseer.example.com"
	if !s.GitHubOAuthEnabled() {
		t.Fatal("expected enabled")
	}
}

func TestOAuthWebBase(t *testing.T) {
	cases := map[string]string{
		"https://github.com":     "https://github.com",
		"https://api.github.com": "https://github.com",
		"https://ghe.example/api/v3": "https://ghe.example",
	}
	for in, want := range cases {
		got, err := github.OAuthWebBase(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %q want %q", in, got, want)
		}
	}
}
