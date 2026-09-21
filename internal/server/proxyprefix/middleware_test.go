package proxyprefix

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripConfiguredPrefix(t *testing.T) {
	var got string
	h := Middleware("/lens")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	req := httptest.NewRequest(http.MethodGet, "/lens/api/v1/summary", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "/api/v1/summary" {
		t.Fatalf("got %q", got)
	}
}

func TestIgnoresForwardedPrefix(t *testing.T) {
	var got string
	h := Middleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
	}))
	req := httptest.NewRequest(http.MethodGet, "/lens/health/live", nil)
	req.Header.Set("X-Forwarded-Prefix", "/lens")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "/lens/health/live" {
		t.Fatalf("expected client prefix ignored, got %q", got)
	}
}
