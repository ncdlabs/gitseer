package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		{"https://github.com", "https://api.github.com"},
		{"https://www.github.com/", "https://api.github.com"},
		{"https://api.github.com", "https://api.github.com"},
		{"github.com", "https://api.github.com"},
		{"https://ghe.example.com", "https://ghe.example.com/api/v3"},
		{"https://ghe.example.com/", "https://ghe.example.com/api/v3"},
		{"https://ghe.example.com/api/v3", "https://ghe.example.com/api/v3"},
		{"https://ghe.example.com/api", "https://ghe.example.com/api/v3"},
		{"http://ghe.lab.local:8443", "http://ghe.lab.local:8443/api/v3"},
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

func TestValidateURLBlocksLoopback(t *testing.T) {
	err := validateURL(mustParse("http://127.0.0.1:3000/api/v3"), false)
	if err == nil {
		t.Fatal("expected block")
	}
	if !strings.Contains(err.Error(), "private network") {
		t.Fatalf("error=%q", err.Error())
	}
	if err := validateURL(mustParse("http://127.0.0.1:3000/api/v3"), true); err != nil {
		t.Fatal(err)
	}
}

func TestMapRepoNormalization(t *testing.T) {
	raw := `{
		"id": 99,
		"name": "gitseer",
		"full_name": "ncdlabs/gitseer",
		"private": true,
		"fork": false,
		"archived": false,
		"size": 12,
		"html_url": "https://github.com/ncdlabs/gitseer",
		"default_branch": "main",
		"owner": {"id": 1, "login": "ncdlabs", "avatar_url": ""}
	}`
	var gr ghRepo
	if err := json.Unmarshal([]byte(raw), &gr); err != nil {
		t.Fatal(err)
	}
	m := mapRepo(gr)
	if m.ExternalID != 99 || m.Owner != "ncdlabs" || m.Name != "gitseer" {
		t.Fatalf("repo=%+v", m)
	}
	if m.Empty || !m.Private {
		t.Fatalf("flags empty=%v private=%v", m.Empty, m.Private)
	}
}

func TestClientHTTptest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/meta", func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		w.Header().Set("X-GitHub-Enterprise-Version", "3.12.0")
		_ = json.NewEncoder(w).Encode(map[string]any{"installed_version": "3.12.0"})
	})
	mux.HandleFunc("/api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 7, "login": "octocat", "email": "o@example.com", "name": "Octo Cat", "avatar_url": "https://av",
		})
	})
	mux.HandleFunc("/api/v3/user/orgs", func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "login": "acme", "name": "Acme Inc", "avatar_url": ""},
		})
	})
	mux.HandleFunc("/api/v3/user/repos", func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id": 42, "name": "gitseer", "full_name": "acme/gitseer", "private": false,
				"fork": false, "archived": false, "size": 100,
				"html_url": "https://ghe/acme/gitseer", "default_branch": "main",
				"owner": map[string]any{"id": 1, "login": "acme"},
			},
		})
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 42, "name": "gitseer", "full_name": "acme/gitseer", "private": false,
			"fork": false, "archived": false, "size": 100,
			"html_url": "https://ghe/acme/gitseer", "default_branch": "main",
			"owner": map[string]any{"id": 1, "login": "acme"},
		})
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer/pulls", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id": 10, "number": 3, "title": "Fix", "body": "details", "state": "open",
				"draft": false, "mergeable": true, "mergeable_state": "clean",
				"html_url": "https://ghe/acme/gitseer/pull/3",
				"created_at": "2024-01-02T03:04:05Z", "updated_at": "2024-01-02T03:04:05Z",
				"user": map[string]any{"id": 7, "login": "octocat"},
				"head": map[string]any{"ref": "feature", "sha": "abc"},
				"base": map[string]any{"ref": "main", "sha": "def"},
			},
		})
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer/commits/abc/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "success", "total_count": 2})
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"workflow_runs": []map[string]any{
				{
					"id": 55, "name": "CI", "event": "push", "status": "completed", "conclusion": "success",
					"html_url": "https://ghe/acme/gitseer/actions/runs/55",
					"created_at": "2024-01-02T03:04:05Z", "updated_at": "2024-01-02T03:05:05Z",
					"run_attempt": 1, "head_branch": "main", "head_sha": "abc",
					"path": ".github/workflows/ci.yaml",
					"actor": map[string]any{"login": "octocat"},
				},
			},
		})
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer/actions/runs/55/jobs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"jobs": []map[string]any{
				{
					"id": 77, "run_id": 55, "name": "build", "status": "completed", "conclusion": "success",
					"html_url": "https://ghe/job/77",
					"started_at": "2024-01-02T03:04:05Z", "completed_at": "2024-01-02T03:05:05Z",
					"runner_name": "ubuntu", "steps": []map[string]any{{"name": "checkout", "status": "completed"}},
				},
			},
		})
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer/actions/runs/55/rerun", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer/actions/runs/55/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer/actions/jobs/77/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("log line\n"))
	})
	mux.HandleFunc("/api/v3/repos/acme/gitseer/contents/.github/workflows/ci.yaml", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github.raw" {
			t.Fatalf("accept=%q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte("name: CI\non: push\n"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, "test-token", true)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	info, err := client.GetInstance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "3.12.0" {
		t.Fatalf("version=%q", info.Version)
	}

	caps, err := client.DetectCapabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !caps.ActionsAPI || !caps.JobLogsAPI || !caps.RerunWorkflowAPI || !caps.CancelWorkflowAPI || caps.SystemHooksAPI {
		t.Fatalf("caps=%+v", caps)
	}

	orgs, err := client.ListOrganizations(ctx)
	if err != nil || len(orgs) != 1 || orgs[0].Name != "acme" {
		t.Fatalf("orgs=%v err=%v", orgs, err)
	}

	page, err := client.ListRepositories(ctx, forge.ListReposOpts{Page: 1, PageSize: 50})
	if err != nil || len(page.Items) != 1 || page.Items[0].FullName != "acme/gitseer" {
		t.Fatalf("repos=%v err=%v", page, err)
	}

	repo, err := client.GetRepository(ctx, "acme", "gitseer")
	if err != nil || repo.ExternalID != 42 {
		t.Fatalf("repo=%v err=%v", repo, err)
	}

	ref := models.RepoRef{Owner: "acme", Name: "gitseer"}
	prs, err := client.ListPullRequests(ctx, ref, forge.PROpts{State: "open"})
	if err != nil || len(prs.Items) != 1 || prs.Items[0].Number != 3 {
		t.Fatalf("prs=%v err=%v", prs, err)
	}

	ci, err := client.GetCombinedCommitStatus(ctx, ref, "abc")
	if err != nil || ci != models.CIStateSuccess {
		t.Fatalf("ci=%q err=%v", ci, err)
	}

	runs, err := client.ListWorkflowRuns(ctx, ref, forge.RunOpts{})
	if err != nil || len(runs.Items) != 1 || runs.Items[0].ExternalID != 55 {
		t.Fatalf("runs=%v err=%v", runs, err)
	}
	if runs.Items[0].WorkflowPath != ".github/workflows/ci.yaml" {
		t.Fatalf("path=%q", runs.Items[0].WorkflowPath)
	}

	jobs, err := client.ListJobs(ctx, ref, 55)
	if err != nil || len(jobs) != 1 || jobs[0].Name != "build" {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}

	if err := client.RerunWorkflowRun(ctx, ref, 55); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if err := client.CancelWorkflowRun(ctx, ref, 55); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	logs, err := client.GetJobLogs(ctx, ref, 77)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(logs)
	_ = logs.Close()
	if string(b) != "log line\n" {
		t.Fatalf("logs=%q", b)
	}

	yaml, err := client.GetWorkflowYAML(ctx, ref, "ci.yaml", "main")
	if err != nil || !strings.Contains(string(yaml), "name: CI") {
		t.Fatalf("yaml=%q err=%v", yaml, err)
	}

	user, err := client.GetAuthenticatedUser(ctx, "user-tok")
	if err != nil || user.Login != "octocat" || user.GiteaUserID == nil || *user.GiteaUserID != 7 {
		t.Fatalf("user=%+v err=%v", user, err)
	}

	upage, err := client.ListAccessibleReposForUser(ctx, "user-tok", forge.ListReposOpts{})
	if err != nil || len(upage.Items) != 1 {
		t.Fatalf("user repos=%v err=%v", upage, err)
	}
}

func TestForgeNewGitHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/meta" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	f, err := forge.New(forge.Options{
		ForgeType:           models.ForgeTypeGitHub,
		BaseURL:             srv.URL,
		Token:               "t",
		AllowPrivateNetwork: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.GetInstance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version == "" {
		t.Fatal("expected version")
	}
}

func assertAuth(t *testing.T, r *http.Request) {
	t.Helper()
	got := r.Header.Get("Authorization")
	if !strings.HasPrefix(got, "Bearer ") {
		t.Fatalf("authorization=%q", got)
	}
}

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
