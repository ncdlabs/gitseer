package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/auth"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestInboxAndSavedFiltersAPI(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := t.Context()

	inst, err := st.UpsertInstanceByURL(ctx, "t", "https://git.example.com", "1.26", "{}")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	pr, err := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 9, Number: 7, Title: "Bootstrap PR", AuthorLogin: "bootstrap",
		State: "open", CIState: models.CIStateFailure,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "pr_ci_failure", Severity: "critical",
		EntityType: "pull_request", EntityID: pr.ID, Title: "fail", Fingerprint: "api-inbox-fp",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)

	loginBody, _ := json.Marshal(map[string]string{"password": "test-pass"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap/login", bytes.NewReader(loginBody))
	probe := httptest.NewRecorder()
	csrf, err := authsvc.IssueCSRFToken(probe)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range probe.Result().Cookies() {
		loginReq.AddCookie(c)
	}
	loginReq.Header.Set(auth.CSRFHeaderName, csrf)
	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login %d: %s", loginRec.Code, loginRec.Body.String())
	}
	var loginPayload struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginPayload)
	cookies := loginRec.Result().Cookies()

	inboxReq := httptest.NewRequest(http.MethodGet, "/api/v1/inbox", nil)
	for _, c := range cookies {
		inboxReq.AddCookie(c)
	}
	inboxRec := httptest.NewRecorder()
	r.ServeHTTP(inboxRec, inboxReq)
	if inboxRec.Code != http.StatusOK {
		t.Fatalf("inbox %d: %s", inboxRec.Code, inboxRec.Body.String())
	}
	var inboxPayload struct {
		Total int `json:"total"`
		Items []struct {
			Reasons []string `json:"reasons"`
			Kind    string   `json:"kind"`
		} `json:"items"`
	}
	_ = json.Unmarshal(inboxRec.Body.Bytes(), &inboxPayload)
	if inboxPayload.Total < 1 || len(inboxPayload.Items) < 1 {
		t.Fatalf("expected inbox items: %+v", inboxPayload)
	}

	createBody := `{"name":"My Failures","query":{"reason":"failing_ci","page":"inbox"}}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/saved-filters", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set(auth.CSRFHeaderName, loginPayload.CSRFToken)
	for _, c := range cookies {
		createReq.AddCookie(c)
	}
	createRec := httptest.NewRecorder()
	r.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create filter %d: %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(createRec.Body.Bytes(), &created)
	if created.ID <= 0 || created.Name != "My Failures" {
		t.Fatalf("created=%+v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/saved-filters", nil)
	for _, c := range cookies {
		listReq.AddCookie(c)
	}
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list filters %d", listRec.Code)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/saved-filters/"+strconv.FormatInt(created.ID, 10), nil)
	delReq.Header.Set(auth.CSRFHeaderName, loginPayload.CSRFToken)
	for _, c := range cookies {
		delReq.AddCookie(c)
	}
	delRec := httptest.NewRecorder()
	r.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete %d: %s", delRec.Code, delRec.Body.String())
	}
}
