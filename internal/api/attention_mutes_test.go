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

func TestMuteAttentionAndRuleOverrides(t *testing.T) {
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
	item, err := st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "pr_ci_failure", Severity: "critical",
		EntityType: "pull_request", EntityID: 1, Title: "fail", Fingerprint: "mute-api-fp",
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

	muteBody := `{"until":"24h","reason":"flaky"}`
	muteReq := httptest.NewRequest(http.MethodPost, "/api/v1/attention/"+strconv.FormatInt(item.ID, 10)+"/mute", bytes.NewBufferString(muteBody))
	muteReq.Header.Set("Content-Type", "application/json")
	muteReq.Header.Set(auth.CSRFHeaderName, loginPayload.CSRFToken)
	for _, c := range loginRec.Result().Cookies() {
		muteReq.AddCookie(c)
	}
	muteRec := httptest.NewRecorder()
	r.ServeHTTP(muteRec, muteReq)
	if muteRec.Code != http.StatusOK {
		t.Fatalf("mute %d: %s", muteRec.Code, muteRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/attention", nil)
	for _, c := range loginRec.Result().Cookies() {
		listReq.AddCookie(c)
	}
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list %d", listRec.Code)
	}
	var listPayload struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(listRec.Body.Bytes(), &listPayload)
	if listPayload.Total != 0 {
		t.Fatalf("expected muted item hidden, total=%d", listPayload.Total)
	}

	putBody := `{"overrides":[{"rule_type":"long_running_workflow","severity":"critical"}]}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/attention/rule-overrides", bytes.NewBufferString(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set(auth.CSRFHeaderName, loginPayload.CSRFToken)
	for _, c := range loginRec.Result().Cookies() {
		putReq.AddCookie(c)
	}
	putRec := httptest.NewRecorder()
	r.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("overrides %d: %s", putRec.Code, putRec.Body.String())
	}
}
