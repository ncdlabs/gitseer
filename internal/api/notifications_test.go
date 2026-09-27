package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/auth"
	"github.com/ncdlabs/gitseer/internal/notify"
)

func TestNotificationSettingsAPI(t *testing.T) {
	h, _, authsvc := setupAPI(t)
	if !h.settings.EncryptionConfigured() {
		if err := h.settings.SetEncryptionKey("test-encryption-key-24chars!!"); err != nil {
			t.Fatal(err)
		}
	}
	h.SetNotify(notify.New(h.store, h.settings, h.settings, nil))

	r := chi.NewRouter()
	h.Routes(r)
	cookies, csrf := loginBootstrap(t, r, authsvc)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/settings", nil)
	for _, c := range cookies {
		getReq.AddCookie(c)
	}
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get %d: %s", getRec.Code, getRec.Body.String())
	}

	body := map[string]any{
		"enabled":         true,
		"min_severity":    "critical",
		"smtp_enabled":    true,
		"smtp_host":       "smtp.example.com",
		"smtp_from":       "gitseer@example.com",
		"smtp_to":         "ops@example.com",
		"smtp_password":   "s3cret",
		"webhook_enabled": true,
		"webhook_url":     "https://hooks.example.com/gitseer",
	}
	raw, _ := json.Marshal(body)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/settings", bytes.NewReader(raw))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set(auth.CSRFHeaderName, csrf)
	for _, c := range cookies {
		putReq.AddCookie(c)
	}
	putRec := httptest.NewRecorder()
	r.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put %d: %s", putRec.Code, putRec.Body.String())
	}
	var putPayload struct {
		Settings NotificationSettingsPublic `json:"settings"`
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &putPayload); err != nil {
		t.Fatal(err)
	}
	if !putPayload.Settings.Enabled || !putPayload.Settings.SMTPPasswordConfigured || !putPayload.Settings.WebhookURLConfigured {
		t.Fatalf("settings=%+v", putPayload.Settings)
	}
	if putPayload.Settings.SMTPHost != "smtp.example.com" {
		t.Fatalf("host=%s", putPayload.Settings.SMTPHost)
	}

	// Secrets must not round-trip in GET.
	get2 := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/settings", nil)
	for _, c := range cookies {
		get2.AddCookie(c)
	}
	get2Rec := httptest.NewRecorder()
	r.ServeHTTP(get2Rec, get2)
	if bytes.Contains(get2Rec.Body.Bytes(), []byte("s3cret")) || bytes.Contains(get2Rec.Body.Bytes(), []byte("hooks.example.com")) {
		t.Fatalf("secret leaked: %s", get2Rec.Body.String())
	}

	testReq := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/test", nil)
	testReq.Header.Set(auth.CSRFHeaderName, csrf)
	for _, c := range cookies {
		testReq.AddCookie(c)
	}
	testRec := httptest.NewRecorder()
	r.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("test %d: %s", testRec.Code, testRec.Body.String())
	}
	var testPayload struct {
		Queued int `json:"queued"`
	}
	_ = json.Unmarshal(testRec.Body.Bytes(), &testPayload)
	if testPayload.Queued < 1 {
		t.Fatalf("queued=%d", testPayload.Queued)
	}
}
