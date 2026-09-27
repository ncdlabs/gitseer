package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/auth"
	"github.com/ncdlabs/gitseer/internal/models"
)

func bootstrapSession(t *testing.T, r chi.Router, authsvc *auth.Service) []*http.Cookie {
	t.Helper()
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
	return loginRec.Result().Cookies()
}

func csrfFromCookies(cookies []*http.Cookie) string {
	for _, c := range cookies {
		if c.Name == auth.CSRFCookieName {
			return c.Value
		}
	}
	return ""
}

func TestSystemStatusIncludesStorage(t *testing.T) {
	h, _, authsvc := setupAPI(t)
	h.cfg.Database.Driver = "sqlite"
	h.cfg.Database.Path = ""
	r := chi.NewRouter()
	h.Routes(r)
	cookies := bootstrapSession(t, r, authsvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	storage, ok := payload["storage"].(map[string]any)
	if !ok {
		t.Fatalf("missing storage: %+v", payload)
	}
	level, _ := storage["level"].(string)
	switch level {
	case "ok", "warn", "critical", "unknown":
	default:
		t.Fatalf("unexpected level: %v", storage["level"])
	}
	if _, ok := storage["warn_bytes"]; !ok {
		t.Fatal("missing warn_bytes")
	}
	if level != "unknown" {
		if _, ok := storage["bytes"]; !ok {
			t.Fatal("missing bytes")
		}
	}
}

func TestPurgeRetentionEndpoint(t *testing.T) {
	h, _, authsvc := setupAPI(t)
	r := chi.NewRouter()
	h.Routes(r)
	cookies := bootstrapSession(t, r, authsvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/purge-retention", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	csrf := csrfFromCookies(cookies)
	if csrf == "" {
		t.Fatal("missing csrf cookie")
	}
	req.Header.Set(auth.CSRFHeaderName, csrf)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK      bool             `json:"ok"`
		Stats   map[string]int64 `json:"stats"`
		Windows map[string]int   `json:"windows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK {
		t.Fatal("expected ok")
	}
	if payload.Windows["runs_days"] != 90 {
		t.Fatalf("windows=%+v", payload.Windows)
	}
}

func TestDownloadGiteaUISnippetsZip(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	inst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitea, "Lab", "https://git.example.com", "1.25", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	h.Routes(r)
	cookies := bootstrapSession(t, r, authsvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/"+strconv.FormatInt(inst.ID, 10)+"/gitea-ui-snippets?format=zip", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "zip") {
		t.Fatalf("content-type=%s", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		if strings.HasSuffix(f.Name, "extra_tabs.tmpl") {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(rc)
			_ = rc.Close()
			s := string(body)
			if !strings.Contains(s, "BEGIN GITSEER-TABS") {
				t.Fatalf("missing tabs marker: %s", s)
			}
			if !strings.Contains(s, "instance_id="+strconv.FormatInt(inst.ID, 10)) {
				t.Fatalf("missing instance_id: %s", s)
			}
		}
	}
	if !names["templates/custom/extra_links.tmpl"] || !names["templates/custom/extra_tabs.tmpl"] {
		t.Fatalf("zip entries=%v", names)
	}
}

func TestDownloadGiteaUISnippetsRejectsGitHub(t *testing.T) {
	h, st, authsvc := setupAPI(t)
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	inst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitHub, "GH", "https://api.github.com", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	h.Routes(r)
	cookies := bootstrapSession(t, r, authsvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/"+strconv.FormatInt(inst.ID, 10)+"/gitea-ui-snippets", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
}
