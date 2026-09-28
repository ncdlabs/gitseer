package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestGetRunCachedGraph(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()
	inst, err := st.UpsertInstanceByURL(ctx, "test", "https://git.example.com", "1.26.0", "{}")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 9, Owner: "acme", Name: "widgets", FullName: "acme/widgets", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "ci", Status: models.StatusRunning,
		CommitSHA: "abc123", WorkflowPath: ".github/workflows/ci.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	nodes := []models.WorkflowNode{
		{JobKey: "build", Name: "Build", Needs: nil},
		{JobKey: "test", Name: "Test", Needs: []string{"build"}},
	}
	raw, _ := json.Marshal(nodes)
	if err := st.UpsertWorkflowGraph(ctx, repo.ID, ".github/workflows/ci.yaml", "abc123", string(raw)); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	cookies, _ := loginBootstrap(t, r, authsvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10), nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Graph      []models.WorkflowNode `json:"graph"`
		GraphError string                `json:"graph_error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.GraphError != "" {
		t.Fatalf("unexpected graph_error=%q", payload.GraphError)
	}
	if len(payload.Graph) != 2 {
		t.Fatalf("graph=%+v", payload.Graph)
	}
}

func TestGetRunEmptyCacheMissJobsFallback(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()
	inst, err := st.UpsertInstanceByURL(ctx, "test", "https://git.example.com", "1.26.0", "{}")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 9, Owner: "acme", Name: "widgets", FullName: "acme/widgets", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 2, Name: "ci", Status: models.StatusRunning,
		CommitSHA: "deadbeef", WorkflowPath: ".github/workflows/ci.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Poisoned empty cache from a prior bad Contents JSON parse.
	if err := st.UpsertWorkflowGraph(ctx, repo.ID, ".github/workflows/ci.yaml", "deadbeef", "[]"); err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 201, Name: "build", Status: models.StatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 202, Name: "test", Status: models.StatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	cookies, _ := loginBootstrap(t, r, authsvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10), nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Graph      []models.WorkflowNode `json:"graph"`
		GraphError string                `json:"graph_error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.GraphError == "" {
		t.Fatal("expected graph_error when YAML fetch fails")
	}
	if len(payload.Graph) != 2 {
		t.Fatalf("expected jobs fallback graph, got %+v", payload.Graph)
	}
	keys := map[string]bool{}
	for _, n := range payload.Graph {
		keys[n.JobKey] = true
		if len(n.Needs) != 0 {
			t.Fatalf("fallback nodes should have no needs: %+v", n)
		}
	}
	if !keys["build"] || !keys["test"] {
		t.Fatalf("keys=%v", keys)
	}
}

func TestGetRunGitHubYAMLFetchAndCache(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()

	yamlBody := "name: CI\non: push\njobs:\n  build:\n    name: Build\n    runs-on: ubuntu-latest\n  test:\n    name: Test\n    needs: build\n    runs-on: ubuntu-latest\n"
	var contentsHits int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/acme/widgets/contents/.github/workflows/ci.yaml", func(w http.ResponseWriter, r *http.Request) {
		contentsHits++
		if r.Header.Get("Accept") != "application/vnd.github.raw+json" {
			t.Fatalf("accept=%q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(yamlBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := h.settings.SetEncryptionKey("test-encryption-key-24chars!!"); err != nil {
		t.Fatal(err)
	}
	key, err := gitseercrypto.KeyFromString("test-encryption-key-24chars!!")
	if err != nil {
		t.Fatal(err)
	}
	authsvc.SetEncryptionKey(key)
	cipher, err := gitseercrypto.Encrypt(key, "gh-pat")
	if err != nil {
		t.Fatal(err)
	}

	inst, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                "github",
		ForgeType:           models.ForgeTypeGitHub,
		BaseURL:             srv.URL,
		SyncTokenCiphertext: cipher,
		AllowPrivateNetwork: true,
		SetFlags:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 9, Owner: "acme", Name: "widgets", FullName: "acme/widgets", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 55, Name: "CI", Status: models.StatusRunning,
		CommitSHA: "abc123def", WorkflowPath: ".github/workflows/ci.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Empty poisoned cache must be ignored.
	if err := st.UpsertWorkflowGraph(ctx, repo.ID, ".github/workflows/ci.yaml", "abc123def", "[]"); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	cookies, _ := loginBootstrap(t, r, authsvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10), nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Graph      []models.WorkflowNode `json:"graph"`
		GraphError string                `json:"graph_error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.GraphError != "" {
		t.Fatalf("graph_error=%q body=%s", payload.GraphError, rec.Body.String())
	}
	if contentsHits != 1 {
		t.Fatalf("contentsHits=%d", contentsHits)
	}
	if len(payload.Graph) != 2 {
		t.Fatalf("graph=%+v", payload.Graph)
	}
	byKey := map[string]models.WorkflowNode{}
	for _, n := range payload.Graph {
		byKey[n.JobKey] = n
	}
	if byKey["test"].Needs == nil || len(byKey["test"].Needs) != 1 || byKey["test"].Needs[0] != "build" {
		t.Fatalf("test needs=%v", byKey["test"].Needs)
	}

	cached, err := st.GetWorkflowGraph(ctx, repo.ID, ".github/workflows/ci.yaml", "abc123def")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(cached) == "" || cached == "[]" {
		t.Fatalf("expected non-empty cache, got %q", cached)
	}

	// Second request should hit cache only.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10), nil)
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status=%d", rec2.Code)
	}
	if contentsHits != 1 {
		t.Fatalf("expected cache hit, contentsHits=%d", contentsHits)
	}
}
