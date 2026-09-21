package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitea-lens/internal/attention"
	"github.com/ncdlabs/gitea-lens/internal/auth"
	"github.com/ncdlabs/gitea-lens/internal/authz"
	"github.com/ncdlabs/gitea-lens/internal/config"
	"github.com/ncdlabs/gitea-lens/internal/database"
	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/realtime"
	"github.com/ncdlabs/gitea-lens/internal/settings"
	"github.com/ncdlabs/gitea-lens/internal/store"
	"github.com/ncdlabs/gitea-lens/internal/sync"
	"github.com/ncdlabs/gitea-lens/internal/webhooks"
)

func setupAPI(t *testing.T) (*Handler, *store.Store, *auth.Service) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.NewWithDriver(db, "sqlite")
	cfg := config.Default()
	cfg.Auth.BootstrapPassword = "test-pass"
	cfg.Server.ExternalURL = "http://localhost:8090"
	cfg.Gitea.URL = "https://git.example.com"
	cfg.Gitea.Token = "super-secret-token"
	cfg.Gitea.WebhookSecret = "super-secret-hook"
	cfg.Auth.OAuthClientSecret = "super-secret-oauth"
	settingsMgr := settings.New(cfg, st)
	if err := settingsMgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	authsvc := auth.New(st, auth.Config{
		BootstrapPassword: cfg.Auth.BootstrapPassword,
		SessionTTL:        time.Hour,
		CookieSecure:      false,
		CookiePath:        "/",
		GiteaBaseURL:      cfg.Gitea.URL,
		OAuthClientID:     cfg.Auth.OAuthClientID,
		OAuthClientSecret: cfg.Auth.OAuthClientSecret,
		ExternalURL:       cfg.Server.ExternalURL,
	})
	hub := realtime.NewHub()
	att := attention.New(st, nil)
	syncer := sync.NewService(st, att, hub, cfg, nil)
	syncer.SetGiteaSource(settingsMgr)
	wh := webhooks.NewProcessor(st, att, hub, nil)
	h := New(cfg, st, authsvc, authz.New(st), syncer, wh, att, hub, settingsMgr, nil, "test")
	return h, st, authsvc
}

func TestListAttentionEmptyItemsArray(t *testing.T) {
	h, _, authsvc := setupAPI(t)
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
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/attention", nil)
	for _, c := range loginRec.Result().Cookies() {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []models.AttentionItem `json:"items"`
		Total int                    `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Items == nil {
		t.Fatal("expected items to be [] not null")
	}
	if payload.Total != 0 || len(payload.Items) != 0 {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestMeUnauthenticatedOK(t *testing.T) {
	h, _, _ := setupAPI(t)
	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Authenticated bool   `json:"authenticated"`
		CSRFToken     string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Authenticated {
		t.Fatal("expected authenticated=false")
	}
	if payload.CSRFToken == "" {
		t.Fatal("expected csrf_token")
	}
}

func TestListRepositoriesRequiresAuth(t *testing.T) {
	h, _, _ := setupAPI(t)
	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/repositories", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListRepositoriesOK(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()
	inst, err := st.UpsertInstanceByURL(ctx, "test", "https://git.example.com", "1.26.0", "{}")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 9, Owner: "acme", Name: "widgets", FullName: "acme/widgets", DefaultBranch: "main", Private: true,
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
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/repositories", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []models.Repository `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].FullName != "acme/widgets" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestOAuthLoginUnavailableWithoutConfig(t *testing.T) {
	h, _, _ := setupAPI(t)
	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestStatsRequiresAuth(t *testing.T) {
	h, _, _ := setupAPI(t)
	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats?days=7", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatsAllowlistAndOK(t *testing.T) {
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
	now := time.Now().UTC()
	started := now.Add(-2 * time.Minute)
	completed := now.Add(-time.Minute)
	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
		StartedAt: &started, CompletedAt: &completed,
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
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()

	bad := httptest.NewRequest(http.MethodGet, "/api/v1/stats?days=14", nil)
	for _, c := range cookies {
		bad.AddCookie(c)
	}
	badRec := httptest.NewRecorder()
	r.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("allowlist status=%d body=%s", badRec.Code, badRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats?days=7", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload models.StatsReport
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Days != 7 || payload.Since == "" {
		t.Fatalf("payload days/since = %+v", payload)
	}
	if len(payload.RunsByDay) == 0 {
		t.Fatal("expected zero-filled runs_by_day")
	}
	success := 0
	for _, b := range payload.RunsByDay {
		success += b.Success
	}
	if success != 1 {
		t.Fatalf("success runs=%d", success)
	}
}

func TestSettingsGetAndPut(t *testing.T) {
	h, _, authsvc := setupAPI(t)
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
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	var loginPayload struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginPayload)
	csrfToken := loginPayload.CSRFToken
	if csrfToken == "" {
		csrfToken = csrf
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	for _, c := range cookies {
		getReq.AddCookie(c)
	}
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var got struct {
		Editable bool            `json:"editable"`
		Settings settings.Values `json:"settings"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Editable {
		t.Fatal("expected editable for bootstrap admin")
	}

	body := settings.Values{
		InstanceName:              "Lens Lab",
		SyncHistoryDays:           21,
		AttentionLongRunningAfter: "90m",
		RetentionRunsDays:         45,
		RetentionWebhooksDays:     10,
		RetentionAttentionDays:    90,
	}
	raw, _ := json.Marshal(body)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(raw))
	for _, c := range cookies {
		putReq.AddCookie(c)
	}
	putReq.Header.Set(auth.CSRFHeaderName, csrfToken)
	putRec := httptest.NewRecorder()
	r.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", putRec.Code, putRec.Body.String())
	}
	var putOut struct {
		Settings settings.Values `json:"settings"`
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &putOut); err != nil {
		t.Fatal(err)
	}
	if putOut.Settings.InstanceName != "Lens Lab" || putOut.Settings.SyncHistoryDays != 21 {
		t.Fatalf("put settings=%+v", putOut.Settings)
	}
	if putOut.Settings.AttentionLongRunningAfter != "90m" {
		t.Fatalf("duration=%q", putOut.Settings.AttentionLongRunningAfter)
	}
}

func TestSettingsIntegrationSecretsNotLeaked(t *testing.T) {
	h, _, authsvc := setupAPI(t)
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
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	for _, c := range cookies {
		getReq.AddCookie(c)
	}
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	body := getRec.Body.String()
	for _, secret := range []string{"super-secret-token", "super-secret-hook", "super-secret-oauth"} {
		if strings.Contains(body, secret) {
			t.Fatalf("secret %q leaked in settings response", secret)
		}
	}
	var got struct {
		Integration settings.IntegrationPublic `json:"integration"`
		SetupCompleted bool                    `json:"setup_completed"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Integration.GiteaURL != "https://git.example.com" {
		t.Fatalf("integration url=%q", got.Integration.GiteaURL)
	}
	if !got.Integration.GiteaTokenConfigured || !got.Integration.GiteaWebhookSecretConfigured {
		t.Fatalf("expected configured flags: %+v", got.Integration)
	}
	if got.SetupCompleted {
		t.Fatal("setup should be incomplete")
	}
}
