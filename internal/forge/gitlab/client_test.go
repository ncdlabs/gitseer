package gitlab

import (
	"context"
	"encoding/json"
	"io"
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
		{"https://gitlab.com", "https://gitlab.com/api/v4"},
		{"https://gitlab.com/", "https://gitlab.com/api/v4"},
		{"https://gitlab.com/api/v4", "https://gitlab.com/api/v4"},
		{"gitlab.example.com", "https://gitlab.example.com/api/v4"},
		{"https://gitlab.lab.local:8443", "https://gitlab.lab.local:8443/api/v4"},
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

func TestClientHTTptest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		path := r.URL.EscapedPath()
		switch {
		case path == "/api/v4/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "16.11.0"})
		case path == "/api/v4/user":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 7, "username": "octocat", "email": "o@example.com", "name": "Octo Cat", "avatar_url": "https://av",
			})
		case path == "/api/v4/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "path": "acme", "name": "Acme", "full_path": "acme", "avatar_url": ""},
			})
		case path == "/api/v4/projects":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 42, "name": "gitseer", "path_with_namespace": "acme/gitseer",
					"default_branch": "main", "visibility": "private", "archived": false, "empty_repo": false,
					"web_url":   "https://gitlab.example/acme/gitseer",
					"namespace": map[string]any{"id": 1, "path": "acme", "kind": "group"},
				},
			})
		case strings.HasSuffix(path, "/merge_requests"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 10, "iid": 3, "title": "Fix", "description": "details", "state": "opened",
					"draft": false, "web_url": "https://gitlab.example/acme/gitseer/-/merge_requests/3",
					"created_at": "2024-01-02T03:04:05Z", "updated_at": "2024-01-02T03:04:05Z",
					"source_branch": "feature", "target_branch": "main", "sha": "abc",
					"author": map[string]any{"id": 7, "username": "octocat"},
				},
			})
		case strings.HasSuffix(path, "/pipelines/55/jobs"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 77, "name": "build", "status": "success", "stage": "build",
					"web_url":    "https://gitlab.example/j/77",
					"started_at": "2024-01-02T03:04:05Z", "finished_at": "2024-01-02T03:05:05Z",
					"pipeline": map[string]any{"id": 55},
				},
			})
		case strings.HasSuffix(path, "/pipelines/55/retry"):
			if r.Method != http.MethodPost {
				t.Fatalf("method=%s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 56})
		case strings.HasSuffix(path, "/pipelines/55/cancel"):
			if r.Method != http.MethodPost {
				t.Fatalf("method=%s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 55, "status": "canceled"})
		case strings.HasSuffix(path, "/pipelines/55"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 55, "status": "success", "source": "push", "ref": "main", "sha": "abc",
				"web_url": "https://gitlab.example/p/55", "created_at": "2024-01-02T03:04:05Z", "updated_at": "2024-01-02T03:05:05Z",
			})
		case strings.HasSuffix(path, "/pipelines"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id": 55, "iid": 1, "status": "success", "source": "push", "ref": "main", "sha": "abc",
					"web_url":    "https://gitlab.example/acme/gitseer/-/pipelines/55",
					"created_at": "2024-01-02T03:04:05Z", "updated_at": "2024-01-02T03:05:05Z",
					"name": "CI", "user": map[string]any{"username": "octocat"},
				},
			})
		case strings.HasSuffix(path, "/retry"):
			if r.Method != http.MethodPost {
				t.Fatalf("method=%s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 56})
		case strings.HasSuffix(path, "/cancel"):
			if r.Method != http.MethodPost {
				t.Fatalf("method=%s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 55, "status": "canceled"})
		case strings.HasSuffix(path, "/trace"):
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("log line\n"))
		case strings.Contains(path, "/repository/files/"):
			_, _ = w.Write([]byte("stages:\n  - build\n"))
		case strings.HasPrefix(path, "/api/v4/projects/") && !strings.Contains(strings.TrimPrefix(path, "/api/v4/projects/"), "/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 42, "name": "gitseer", "path_with_namespace": "acme/gitseer",
				"default_branch": "main", "visibility": "private", "web_url": "https://gitlab.example/acme/gitseer",
				"namespace": map[string]any{"path": "acme"},
			})
		default:
			http.NotFound(w, r)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, "test-token", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	info, err := client.GetInstance(ctx)
	if err != nil || info.Version != "16.11.0" {
		t.Fatalf("GetInstance: %+v err=%v", info, err)
	}
	user, err := client.GetAuthenticatedUser(ctx, "")
	if err != nil || user.Login != "octocat" {
		t.Fatalf("user=%+v err=%v", user, err)
	}
	orgs, err := client.ListOrganizations(ctx)
	if err != nil || len(orgs) != 1 || orgs[0].Name != "acme" {
		t.Fatalf("orgs=%+v err=%v", orgs, err)
	}
	repos, err := client.ListRepositories(ctx, forge.ListReposOpts{Page: 1, PageSize: 10})
	if err != nil || len(repos.Items) != 1 || repos.Items[0].FullName != "acme/gitseer" {
		t.Fatalf("repos=%+v err=%v", repos, err)
	}
	ref := models.RepoRef{Owner: "acme", Name: "gitseer"}
	prs, err := client.ListPullRequests(ctx, ref, forge.PROpts{State: "open"})
	if err != nil || len(prs.Items) != 1 || prs.Items[0].Number != 3 || prs.Items[0].State != "open" {
		t.Fatalf("prs=%+v err=%v", prs, err)
	}
	runs, err := client.ListWorkflowRuns(ctx, ref, forge.RunOpts{})
	if err != nil || len(runs.Items) != 1 || runs.Items[0].ExternalID != 55 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	run, err := client.GetWorkflowRun(ctx, ref, 55)
	if err != nil || run.Conclusion != models.ConclusionSuccess {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	jobs, err := client.ListJobs(ctx, ref, 55)
	if err != nil || len(jobs) != 1 || jobs[0].Name != "build" {
		t.Fatalf("jobs=%+v err=%v", jobs, err)
	}
	logs, err := client.GetJobLogs(ctx, ref, 77)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(logs)
	logs.Close()
	if string(raw) != "log line\n" {
		t.Fatalf("logs=%q", raw)
	}
	if err := client.RerunWorkflowRun(ctx, ref, 55); err != nil {
		t.Fatal(err)
	}
	if err := client.CancelWorkflowRun(ctx, ref, 55); err != nil {
		t.Fatal(err)
	}
	yaml, err := client.GetWorkflowYAML(ctx, ref, ".gitlab-ci.yml", "main")
	if err != nil || !strings.Contains(string(yaml), "stages") {
		t.Fatalf("yaml=%q err=%v", yaml, err)
	}
}

func assertAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("PRIVATE-TOKEN") == "" && r.Header.Get("Authorization") == "" {
		t.Fatalf("missing auth headers")
	}
}
