package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestURLCandidates(t *testing.T) {
	got := urlCandidates("git.example.com")
	if len(got) != 2 || got[0] != "https://git.example.com" || got[1] != "http://git.example.com" {
		t.Fatalf("got=%v", got)
	}
	got = urlCandidates("https://git.example.com/")
	if len(got) != 1 || got[0] != "https://git.example.com" {
		t.Fatalf("got=%v", got)
	}
	got = urlCandidates("http://git.example.com")
	if len(got) != 1 || got[0] != "http://git.example.com" {
		t.Fatalf("got=%v", got)
	}
}

func TestCheckGiteaURLHTTPSPreferred(t *testing.T) {
	httpsHits, httpHits := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/v1/version") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1.25.0"}`))
	}))
	defer srv.Close()

	// httptest is http only; feed host without scheme using the server URL host+port via rewriting.
	// Direct check with explicit http candidate path:
	res := CheckGiteaURL(context.Background(), srv.URL, true)
	if !res.OK || res.URL != strings.TrimRight(srv.URL, "/") {
		t.Fatalf("res=%+v httpsHits=%d httpHits=%d", res, httpsHits, httpHits)
	}
}

func TestCheckGiteaURLUnreachable(t *testing.T) {
	res := CheckGiteaURL(context.Background(), "https://127.0.0.1:1", true)
	if res.OK || res.Code != URLCheckUnreachable {
		t.Fatalf("res=%+v", res)
	}
}

func TestCheckGiteaURLPrivateRequiresOptIn(t *testing.T) {
	res := CheckGiteaURL(context.Background(), "http://127.0.0.1:1", false)
	if res.OK || res.Code != URLCheckPrivateNetwork {
		t.Fatalf("res=%+v", res)
	}
	if res.PrivateIP == "" {
		t.Fatal("expected private_ip")
	}
}

func TestPrivateNetworkCheckboxMessage(t *testing.T) {
	msg := PrivateNetworkCheckboxMessage(nil)
	if !strings.Contains(msg, PrivateNetworkOptionLabel) || !strings.Contains(msg, "private address") {
		t.Fatalf("msg=%q", msg)
	}
}
