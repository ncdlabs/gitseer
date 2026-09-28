package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeIndexInjectsBaseMeta(t *testing.T) {
	h := Handler("/gitseer")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if strings.Contains(got, "<script>window.__GITSEER_BASE__") {
		t.Fatal("inline base script must not be injected (CSP)")
	}
	want := `<meta name="gitseer-base" content="/gitseer">`
	if !strings.Contains(got, want) {
		t.Fatalf("missing base meta; body:\n%s", got)
	}
	if !strings.Contains(got, `href="/gitseer/assets/`) && strings.Contains(got, `href="/assets/`) {
		t.Fatal("expected asset hrefs rewritten for base path")
	}
}

func TestServeIndexEmptyBase(t *testing.T) {
	h := Handler("")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	got := string(body)
	if !strings.Contains(got, `<meta name="gitseer-base" content="">`) {
		t.Fatalf("expected empty base meta; body:\n%s", got)
	}
	if strings.Contains(got, "<script>window.__GITSEER_BASE__") {
		t.Fatal("inline base script must not be injected")
	}
}
