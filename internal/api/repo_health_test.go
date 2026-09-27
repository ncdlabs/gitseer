package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestRepositoryHealthAndFailureClusters(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()
	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.26", "{}")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "acme", Name: "widgets", FullName: "acme/widgets", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := time.Now().UTC().Add(-time.Hour)
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 50, Name: "CI", Branch: "main", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, WorkflowPath: ".gitea/workflows/ci.yaml",
		CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 501, Name: "test", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "failed_default_branch_workflow",
		Severity: "critical", EntityType: "workflow_run", EntityID: run.ID,
		Title: "Failed default-branch", Fingerprint: "fp-api-health",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	cookies, _ := loginBootstrap(t, r, authsvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/repositories", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listPayload struct {
		Items []struct {
			FullName string             `json:"full_name"`
			Health   *models.RepoHealth `json:"health"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatal(err)
	}
	if len(listPayload.Items) != 1 || listPayload.Items[0].Health == nil {
		t.Fatalf("list payload=%+v", listPayload)
	}
	if !listPayload.Items[0].Health.FailingDefaultBranch {
		t.Fatalf("health=%+v", listPayload.Items[0].Health)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/acme/widgets/health", nil)
	for _, c := range cookies {
		healthReq.AddCookie(c)
	}
	healthRec := httptest.NewRecorder()
	r.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", healthRec.Code, healthRec.Body.String())
	}

	clusterReq := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/acme/widgets/failure-clusters?days=7", nil)
	for _, c := range cookies {
		clusterReq.AddCookie(c)
	}
	clusterRec := httptest.NewRecorder()
	r.ServeHTTP(clusterRec, clusterReq)
	if clusterRec.Code != http.StatusOK {
		t.Fatalf("clusters status=%d body=%s", clusterRec.Code, clusterRec.Body.String())
	}
	var clusterPayload struct {
		Items []models.FailureCluster `json:"items"`
		Days  int                     `json:"days"`
	}
	if err := json.Unmarshal(clusterRec.Body.Bytes(), &clusterPayload); err != nil {
		t.Fatal(err)
	}
	if clusterPayload.Days != 7 || len(clusterPayload.Items) < 1 {
		t.Fatalf("clusters=%+v", clusterPayload)
	}
	if clusterPayload.Items[0].JobName != "test" {
		t.Fatalf("job=%s", clusterPayload.Items[0].JobName)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/acme/widgets", nil)
	for _, c := range cookies {
		detailReq.AddCookie(c)
	}
	detailRec := httptest.NewRecorder()
	r.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status=%d", detailRec.Code)
	}
	var detail struct {
		FullName string             `json:"full_name"`
		Health   *models.RepoHealth `json:"health"`
	}
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.FullName != "acme/widgets" || detail.Health == nil || detail.Health.OpenCriticalAttention != 1 {
		t.Fatalf("detail=%+v", detail)
	}
}
