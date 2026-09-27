package bitbucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestNormalizeAPIBase(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://bitbucket.org", "https://api.bitbucket.org/2.0"},
		{"https://api.bitbucket.org", "https://api.bitbucket.org/2.0"},
		{"bitbucket.org", "https://api.bitbucket.org/2.0"},
	}
	for _, tc := range cases {
		u, err := normalizeAPIBase(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		got := strings.TrimRight(u.String(), "/")
		if got != tc.want {
			t.Fatalf("%q → %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestStableID(t *testing.T) {
	a := stableID("{11111111-1111-1111-1111-111111111111}")
	b := stableID("11111111-1111-1111-1111-111111111111")
	if a != b || a <= 0 {
		t.Fatalf("stableID mismatch a=%d b=%d", a, b)
	}
	// Guard against regressing to the pre-fix OAuth h×31 hash (must stay FNV-64a).
	const uuid = "11111111-1111-1111-1111-111111111111"
	var legacy uint64
	for i := 0; i < len(uuid); i++ {
		legacy = legacy*31 + uint64(uuid[i])
	}
	legacyID := int64(legacy & 0x7fffffffffffffff)
	if a == legacyID {
		t.Fatalf("StableID unexpectedly matches legacy h×31 hash (%d)", legacyID)
	}
}

func TestClientHTTptest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing auth")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uuid": "{u1}", "username": "octocat", "display_name": "Octo", "nickname": "octocat",
			"links": map[string]any{"avatar": map[string]any{"href": "https://av"}},
		})
	})
	mux.HandleFunc("/2.0/workspaces", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{"uuid": "{w1}", "slug": "acme", "name": "Acme"}},
		})
	})
	mux.HandleFunc("/2.0/repositories", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"uuid": "{r1}", "name": "gitseer", "full_name": "acme/gitseer", "slug": "gitseer",
				"is_private": true, "mainbranch": map[string]any{"name": "main"},
				"workspace": map[string]any{"slug": "acme"},
				"links": map[string]any{"html": map[string]any{"href": "https://bitbucket.org/acme/gitseer"}},
			}},
		})
	})
	mux.HandleFunc("/2.0/repositories/acme/gitseer", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uuid": "{r1}", "name": "gitseer", "full_name": "acme/gitseer", "slug": "gitseer",
			"is_private": true, "mainbranch": map[string]any{"name": "main"},
			"workspace": map[string]any{"slug": "acme"},
		})
	})
	mux.HandleFunc("/2.0/repositories/acme/gitseer/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"id": 3, "title": "Fix", "description": "d", "state": "OPEN",
				"created_on": "2024-01-02T03:04:05Z", "updated_on": "2024-01-02T03:04:05Z",
				"author": map[string]any{"uuid": "{u1}", "username": "octocat"},
				"source": map[string]any{"branch": map[string]any{"name": "feature"}, "commit": map[string]any{"hash": "abc"}},
				"destination": map[string]any{"branch": map[string]any{"name": "main"}},
				"links": map[string]any{"html": map[string]any{"href": "https://bitbucket.org/acme/gitseer/pull-requests/3"}},
			}},
		})
	})
	mux.HandleFunc("/2.0/repositories/acme/gitseer/pipelines/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{{
				"uuid": "{p1}", "build_number": 55,
				"state": map[string]any{"name": "COMPLETED", "result": map[string]any{"name": "SUCCESSFUL"}},
				"target": map[string]any{"ref_name": "main", "commit": map[string]any{"hash": "abc"}},
				"created_on": "2024-01-02T03:04:05Z", "completed_on": "2024-01-02T03:05:05Z",
			}},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, "tok", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	info, err := client.GetInstance(ctx)
	if err != nil || info.Version != "cloud" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	user, err := client.GetAuthenticatedUser(ctx, "")
	if err != nil || user.Login != "octocat" {
		t.Fatalf("user=%+v err=%v", user, err)
	}
	orgs, err := client.ListOrganizations(ctx)
	if err != nil || len(orgs) != 1 {
		t.Fatalf("orgs=%+v err=%v", orgs, err)
	}
	repos, err := client.ListRepositories(ctx, forge.ListReposOpts{Page: 1})
	if err != nil || len(repos.Items) != 1 || repos.Items[0].FullName != "acme/gitseer" {
		t.Fatalf("repos=%+v err=%v", repos, err)
	}
	ref := models.RepoRef{Owner: "acme", Name: "gitseer"}
	prs, err := client.ListPullRequests(ctx, ref, forge.PROpts{State: "open"})
	if err != nil || len(prs.Items) != 1 || prs.Items[0].Number != 3 {
		t.Fatalf("prs=%+v err=%v", prs, err)
	}
	runs, err := client.ListWorkflowRuns(ctx, ref, forge.RunOpts{})
	if err != nil || len(runs.Items) != 1 || runs.Items[0].ExternalID != 55 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	if err := client.RerunWorkflowRun(ctx, ref, 55); err == nil || !forge.IsUnsupported(err) {
		t.Fatalf("expected unsupported rerun, got %v", err)
	}
}
