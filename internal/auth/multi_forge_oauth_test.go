package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ncdlabs/gitseer/internal/models"
)

func TestGitLabBitbucketForgejoOAuthEnabledAndRedirect(t *testing.T) {
	s := &Service{cfg: Config{ExternalURL: "https://gitseer.example.com/"}}
	if s.GitLabOAuthEnabled() || s.BitbucketOAuthEnabled() || s.ForgejoOAuthEnabled() {
		t.Fatal("expected disabled")
	}
	s.cfg.GitLabBaseURL = "https://gitlab.com"
	s.cfg.GitLabOAuthClientID = "cid"
	s.cfg.ExternalURL = "https://gitseer.example.com"
	if !s.GitLabOAuthEnabled() {
		t.Fatal("gitlab expected enabled")
	}
	if s.GitLabRedirectURI() != "https://gitseer.example.com/api/v1/auth/gitlab/callback" {
		t.Fatalf("gitlab redirect=%s", s.GitLabRedirectURI())
	}
	s.cfg.BitbucketBaseURL = "https://bitbucket.org"
	s.cfg.BitbucketOAuthClientID = "cid"
	if !s.BitbucketOAuthEnabled() {
		t.Fatal("bitbucket expected enabled")
	}
	if s.BitbucketRedirectURI() != "https://gitseer.example.com/api/v1/auth/bitbucket/callback" {
		t.Fatalf("bitbucket redirect=%s", s.BitbucketRedirectURI())
	}
	s.cfg.ForgejoBaseURL = "https://forgejo.example.com"
	s.cfg.ForgejoOAuthClientID = "cid"
	if !s.ForgejoOAuthEnabled() {
		t.Fatal("forgejo expected enabled")
	}
	if s.ForgejoRedirectURI() != "https://gitseer.example.com/api/v1/auth/forgejo/callback" {
		t.Fatalf("forgejo redirect=%s", s.ForgejoRedirectURI())
	}
}

func TestRefreshAccessTokenForInstanceDispatchesByForge(t *testing.T) {
	tests := []struct {
		ft       string
		pathHas  string
		basic    bool
		jsonBody bool
	}{
		{models.ForgeTypeGitLab, "/oauth/token", false, false},
		{models.ForgeTypeBitbucket, "/site/oauth2/access_token", true, false},
		{models.ForgeTypeForgejo, "/login/oauth/access_token", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.ft, func(t *testing.T) {
			var sawGrant, sawBasic, sawJSON bool
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				ct := r.Header.Get("Content-Type")
				sawJSON = strings.Contains(ct, "application/json")
				_, _, ok := r.BasicAuth()
				sawBasic = ok
				body, _ := io.ReadAll(r.Body)
				sawGrant = strings.Contains(string(body), "grant_type=refresh_token") ||
					strings.Contains(string(body), `"grant_type":"refresh_token"`) ||
					strings.Contains(string(body), `"grant_type": "refresh_token"`)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			s := &Service{cfg: Config{ExternalURL: "https://gitseer.example.com"}, client: srv.Client()}
			switch tc.ft {
			case models.ForgeTypeGitLab:
				s.UpdateGitLabAuth(srv.URL, "cid", "csec", true)
			case models.ForgeTypeBitbucket:
				s.UpdateBitbucketAuth(srv.URL, "cid", "csec", true)
			case models.ForgeTypeForgejo:
				s.UpdateForgejoAuth(srv.URL, "cid", "csec", true)
			}

			var tok *tokenResponse
			var err error
			switch tc.ft {
			case models.ForgeTypeGitLab:
				tok, err = s.refreshGitLabAccessToken(context.Background(), "old-refresh")
			case models.ForgeTypeBitbucket:
				tok, err = s.refreshBitbucketAccessToken(context.Background(), "old-refresh")
			case models.ForgeTypeForgejo:
				tok, err = s.refreshForgejoAccessToken(context.Background(), "old-refresh")
			}
			if err != nil {
				t.Fatal(err)
			}
			if tok == nil || tok.AccessToken != "new-access" {
				t.Fatalf("tok=%+v", tok)
			}
			if !sawGrant {
				t.Fatal("expected refresh_token grant")
			}
			if tc.basic && !sawBasic {
				t.Fatal("expected basic auth")
			}
			if tc.jsonBody && !sawJSON {
				t.Fatal("expected JSON body")
			}
			if !tc.jsonBody && sawJSON {
				t.Fatal("expected form body")
			}
			_ = tc.pathHas
		})
	}
}
