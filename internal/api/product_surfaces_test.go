package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/auth"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestProductSurfacesAndWallboardAPI(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	r := chi.NewRouter()
	h.Routes(r)
	cookies := bootstrapSession(t, r, authsvc)

	ctx := t.Context()
	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.26", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	org, err := st.UpsertOrganization(ctx, inst.ID, models.Organization{
		ExternalID: 42, Name: "acme", FullName: "Acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "acme", Name: "widgets", FullName: "acme/widgets", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 9, Name: "Release", WorkflowPath: ".gitea/workflows/release.yaml",
		Event: "push", Branch: "main", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
		StartedAt: &now, CompletedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 1, Name: "build", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
		RunnerName: "box-a", CompletedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 2, Name: "build", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
		RunnerName: "box-a", CompletedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	if rec := get("/api/v1/runners/utilization?days=7"); rec.Code != http.StatusOK {
		t.Fatalf("runners status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := get("/api/v1/flaky-jobs?days=14"); rec.Code != http.StatusOK {
		t.Fatalf("flaky status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := get("/api/v1/releases?days=30"); rec.Code != http.StatusOK {
		t.Fatalf("releases status=%d body=%s", rec.Code, rec.Body.String())
	} else {
		var body struct {
			Items []models.ReleaseRun `json:"items"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if len(body.Items) != 1 {
			t.Fatalf("releases items=%+v", body.Items)
		}
	}
	if rec := get("/api/v1/summary?days=0&owner=acme"); rec.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := get("/api/v1/organizations?limit=100"); rec.Code != http.StatusOK {
		t.Fatalf("organizations status=%d body=%s", rec.Code, rec.Body.String())
	} else {
		var body struct {
			Items []models.Organization `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("organizations decode: %v body=%s", err, rec.Body.String())
		}
		if len(body.Items) != 1 || body.Items[0].ID != org.ID || body.Items[0].Name != "acme" {
			t.Fatalf("organizations items=%+v want acme id=%d", body.Items, org.ID)
		}
	}
	if rec := get(fmt.Sprintf("/api/v1/summary?days=0&org_id=%d", org.ID)); rec.Code != http.StatusOK {
		t.Fatalf("summary org_id status=%d body=%s", rec.Code, rec.Body.String())
	} else {
		var sum models.Summary
		if err := json.Unmarshal(rec.Body.Bytes(), &sum); err != nil {
			t.Fatalf("summary org_id decode: %v", err)
		}
		if sum.Repositories != 1 {
			t.Fatalf("summary org_id repositories=%d want 1", sum.Repositories)
		}
	}
	if rec := get("/api/v1/repositories/acme/widgets/flaky-jobs?days=14"); rec.Code != http.StatusOK {
		t.Fatalf("repo flaky status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Create wallboard token
	csrf := csrfFromCookies(cookies)
	if csrf == "" {
		t.Fatal("missing csrf cookie")
	}
	createBody, _ := json.Marshal(map[string]string{"name": "Lobby"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/wallboard/tokens", bytes.NewReader(createBody))
	for _, c := range cookies {
		createReq.AddCookie(c)
	}
	createReq.Header.Set(auth.CSRFHeaderName, csrf)
	createRec := httptest.NewRecorder()
	r.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create wallboard status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(createRec.Body.Bytes(), &created)
	if created.Secret == "" {
		t.Fatal("missing wallboard secret")
	}

	snapReq := httptest.NewRequest(http.MethodGet, "/api/v1/wallboard/snapshot", nil)
	snapReq.Header.Set("Authorization", "Bearer "+created.Secret)
	snapRec := httptest.NewRecorder()
	r.ServeHTTP(snapRec, snapReq)
	if snapRec.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", snapRec.Code, snapRec.Body.String())
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/wallboard/snapshot", nil)
	badRec := httptest.NewRecorder()
	r.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", badRec.Code)
	}
}
