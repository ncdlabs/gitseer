package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/auth"
	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func loginBootstrap(t *testing.T, r chi.Router, authsvc *auth.Service) (cookies []*http.Cookie, csrf string) {
	t.Helper()
	loginBody, _ := json.Marshal(map[string]string{"password": "test-pass"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap/login", bytes.NewReader(loginBody))
	probe := httptest.NewRecorder()
	preCSRF, err := authsvc.IssueCSRFToken(probe)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range probe.Result().Cookies() {
		loginReq.AddCookie(c)
	}
	loginReq.Header.Set(auth.CSRFHeaderName, preCSRF)
	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	cookies = loginRec.Result().Cookies()
	csrf = preCSRF
	for _, c := range cookies {
		if c.Name == auth.CSRFCookieName && c.Value != "" {
			csrf = c.Value
		}
	}
	var loginPayload struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginPayload)
	if loginPayload.CSRFToken != "" {
		csrf = loginPayload.CSRFToken
	}
	return cookies, csrf
}

func sessionCookieForUser(t *testing.T, st *store.Store, userID int64) *http.Cookie {
	t.Helper()
	token := "test-session-token-" + strconv.FormatInt(userID, 10)
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	if err := st.CreateSession(context.Background(), "sess-"+strconv.FormatInt(userID, 10), hash, userID, time.Now().UTC().Add(time.Hour), "", ""); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: auth.CookieName, Value: token}
}

func TestRerunCancelBootstrapAdminUsesServicePAT(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()

	var gotRerunAuth, gotCancel bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs/100/rerun", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		gotRerunAuth = strings.Contains(r.Header.Get("Authorization"), "super-secret-token")
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs/42/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		gotCancel = true
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	inst, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                "lab",
		ForgeType:           models.ForgeTypeGitea,
		BaseURL:             srv.URL,
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
	done, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 100, Name: "ci-done", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 42, Name: "ci-active", Status: models.StatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	cookies, csrf := loginBootstrap(t, r, authsvc)

	post := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		req.Header.Set(auth.CSRFHeaderName, csrf)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	rec := post("/api/v1/workflow-runs/" + strconv.FormatInt(done.ID, 10) + "/rerun")
	if rec.Code != http.StatusOK {
		t.Fatalf("rerun status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["used_service_pat"] != true {
		t.Fatalf("expected used_service_pat: %+v", payload)
	}
	if !gotRerunAuth {
		t.Fatal("expected forge rerun with service token")
	}

	rec = post("/api/v1/workflow-runs/" + strconv.FormatInt(active.ID, 10) + "/cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !gotCancel {
		t.Fatal("expected forge cancel call")
	}

	rec = post("/api/v1/workflow-runs/" + strconv.FormatInt(done.ID, 10) + "/cancel")
	if rec.Code != http.StatusConflict {
		t.Fatalf("cancel completed status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRerunRequiresCSRF(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()
	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", "{}")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "a", Name: "b", FullName: "a/b",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
	})
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	cookies, _ := loginBootstrap(t, r, authsvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10)+"/rerun", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRerunOAuthUserRequiresUserToken(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs/7/rerun", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("forge must not be called without user token")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	inst, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                "lab",
		ForgeType:           models.ForgeTypeGitea,
		BaseURL:             srv.URL,
		AllowPrivateNetwork: true,
		SetFlags:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 9, Owner: "acme", Name: "widgets", FullName: "acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 7, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
	})
	if err != nil {
		t.Fatal(err)
	}

	instID := inst.ID
	oauthUser, err := st.UpsertGiteaUser(ctx, &instID, models.User{
		Login: "alice", DisplayName: "Alice", GiteaUserID: ptrInt64(99),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceUserRepoAccess(ctx, oauthUser.ID, []int64{repo.ID}); err != nil {
		t.Fatal(err)
	}

	sessCookie := sessionCookieForUser(t, st, oauthUser.ID)
	probe := httptest.NewRecorder()
	csrf, err := authsvc.IssueCSRFToken(probe)
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10)+"/rerun", nil)
	req.AddCookie(sessCookie)
	for _, c := range probe.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set(auth.CSRFHeaderName, csrf)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "user token") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestRerunOAuthUserUsesUserToken(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()

	const userTok = "user-oauth-token"
	var sawUserTok bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs/8/rerun", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), userTok) {
			sawUserTok = true
		}
		if strings.Contains(r.Header.Get("Authorization"), "super-secret-token") {
			t.Fatal("must not use service PAT for OAuth user")
		}
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	key, err := gitseercrypto.KeyFromString("test-encryption-key-24chars!!")
	if err != nil {
		t.Fatal(err)
	}
	authsvc.SetEncryptionKey(key)
	cipher, err := gitseercrypto.Encrypt(key, userTok)
	if err != nil {
		t.Fatal(err)
	}

	inst, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                "lab",
		ForgeType:           models.ForgeTypeGitea,
		BaseURL:             srv.URL,
		AllowPrivateNetwork: true,
		SetFlags:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 9, Owner: "acme", Name: "widgets", FullName: "acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 8, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
	})
	if err != nil {
		t.Fatal(err)
	}

	instID := inst.ID
	oauthUser, err := st.UpsertGiteaUser(ctx, &instID, models.User{
		Login: "bob", DisplayName: "Bob", GiteaUserID: ptrInt64(100),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveUserToken(ctx, oauthUser.ID, inst.ID, cipher, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceUserRepoAccess(ctx, oauthUser.ID, []int64{repo.ID}); err != nil {
		t.Fatal(err)
	}

	sessCookie := sessionCookieForUser(t, st, oauthUser.ID)
	probe := httptest.NewRecorder()
	csrf, err := authsvc.IssueCSRFToken(probe)
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10)+"/rerun", nil)
	req.AddCookie(sessCookie)
	for _, c := range probe.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set(auth.CSRFHeaderName, csrf)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if payload["used_service_pat"] == true {
		t.Fatalf("OAuth user must not report service PAT: %+v", payload)
	}
	if !sawUserTok {
		t.Fatal("expected user token on forge call")
	}
}

func TestRerunGitLabNonAdminDoesNotFallBackToGiteaToken(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()

	glInst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitLab, "gl", "https://gitlab.com", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, glInst.ID, models.Repository{
		ExternalID: 1, Owner: "acme", Name: "widgets", FullName: "acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 11, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
	})
	if err != nil {
		t.Fatal(err)
	}

	giteaInst, err := st.UpsertInstanceByURL(ctx, "gitea", "https://git.example.com", "1.25", "{}")
	if err != nil {
		t.Fatal(err)
	}
	giteaID := giteaInst.ID
	oauthUser, err := st.UpsertGiteaUser(ctx, &giteaID, models.User{
		Login: "dave", DisplayName: "Dave", GiteaUserID: ptrInt64(202),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceUserRepoAccess(ctx, oauthUser.ID, []int64{repo.ID}); err != nil {
		t.Fatal(err)
	}
	// Store a Gitea user token only — must not be used against GitLab.
	key, err := gitseercrypto.KeyFromString("test-encryption-key-24chars!!")
	if err != nil {
		t.Fatal(err)
	}
	authsvc.SetEncryptionKey(key)
	cipher, err := gitseercrypto.Encrypt(key, "gitea-only-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveUserToken(ctx, oauthUser.ID, giteaInst.ID, cipher, "", nil); err != nil {
		t.Fatal(err)
	}

	sessCookie := sessionCookieForUser(t, st, oauthUser.ID)
	probe := httptest.NewRecorder()
	csrf, err := authsvc.IssueCSRFToken(probe)
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10)+"/rerun", nil)
	req.AddCookie(sessCookie)
	for _, c := range probe.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set(auth.CSRFHeaderName, csrf)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "forge user token") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestCancelGitHubNonAdminForbidden(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()

	inst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitHub, "gh", "https://api.github.com", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "acme", Name: "widgets", FullName: "acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 9, Name: "ci", Status: models.StatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}

	giteaInst, err := st.UpsertInstanceByURL(ctx, "gitea", "https://git.example.com", "1.25", "{}")
	if err != nil {
		t.Fatal(err)
	}
	instID := giteaInst.ID
	oauthUser, err := st.UpsertGiteaUser(ctx, &instID, models.User{
		Login: "carol", DisplayName: "Carol", GiteaUserID: ptrInt64(101),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceUserRepoAccess(ctx, oauthUser.ID, []int64{repo.ID}); err != nil {
		t.Fatal(err)
	}

	sessCookie := sessionCookieForUser(t, st, oauthUser.ID)
	probe := httptest.NewRecorder()
	csrf, err := authsvc.IssueCSRFToken(probe)
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10)+"/cancel", nil)
	req.AddCookie(sessCookie)
	for _, c := range probe.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set(auth.CSRFHeaderName, csrf)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "GitHub") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestBootstrapWriteOpsFailClosedWithoutSyncTokenForGitLab(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := context.Background()

	glInst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitLab, "gl", "https://gitlab.example.com", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, glInst.ID, models.Repository{
		ExternalID: 1, Owner: "acme", Name: "widgets", FullName: "acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 11, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
	})
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	h.Routes(r)
	cookies, csrf := loginBootstrap(t, r, authsvc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-runs/"+strconv.FormatInt(run.ID, 10)+"/rerun", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req.Header.Set(auth.CSRFHeaderName, csrf)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "sync token") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func ptrInt64(n int64) *int64 { return &n }
