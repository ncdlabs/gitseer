package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOAuthTokenExchanger_FormEncoding(t *testing.T) {
	var sawGrant, sawSecret bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		sawGrant = strings.Contains(s, "grant_type=authorization_code")
		sawSecret = strings.Contains(s, "client_secret=csec")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","refresh_token":"ref","expires_in":3600}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	e := newTokenExchanger(func() (string, error) { return srv.URL, nil }, srv.Client(), "cid", "csec")
	tok, err := e.exchange(context.Background(), "code123", "verifier456")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "tok" || tok.RefreshToken != "ref" {
		t.Fatalf("unexpected tok=%+v", tok)
	}
	if !sawGrant || !sawSecret {
		t.Fatalf("missing form fields: grant=%v secret=%v", sawGrant, sawSecret)
	}
}

func TestOAuthTokenExchanger_JSONEncoding(t *testing.T) {
	var sawGrant, sawSecret bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected JSON content-type, got %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		sawGrant = strings.Contains(s, `"grant_type":"refresh_token"`)
		sawSecret = strings.Contains(s, `"client_secret":"csec"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok2","refresh_token":"ref2"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	e := newTokenExchanger(func() (string, error) { return srv.URL, nil }, srv.Client(), "cid", "csec")
	e.style = tokenExchangeJSON
	tok, err := e.refresh(context.Background(), "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "tok2" {
		t.Fatalf("unexpected tok=%+v", tok)
	}
	if !sawGrant || !sawSecret {
		t.Fatalf("missing JSON fields: grant=%v secret=%v", sawGrant, sawSecret)
	}
}

func TestOAuthTokenExchanger_BasicAuth(t *testing.T) {
	var sawBasic bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := r.BasicAuth()
		sawBasic = ok
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok3"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	e := newTokenExchanger(func() (string, error) { return srv.URL, nil }, srv.Client(), "cid", "csec")
	e.basicAuth = true
	tok, err := e.exchange(context.Background(), "code", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "tok3" {
		t.Fatalf("unexpected tok=%+v", tok)
	}
	if !sawBasic {
		t.Fatal("expected basic auth")
	}
}

func TestOAuthTokenExchanger_RefreshTokenFallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Response omits refresh_token — exchanger should fall back to the input.
		_, _ = w.Write([]byte(`{"access_token":"new-tok"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	e := newTokenExchanger(func() (string, error) { return srv.URL, nil }, srv.Client(), "cid", "csec")
	tok, err := e.refresh(context.Background(), "original-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if tok.RefreshToken != "original-refresh" {
		t.Fatalf("expected refresh_token fallback, got %q", tok.RefreshToken)
	}
}

func TestOAuthTokenExchanger_ErrorOnMissingAccessToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	e := newTokenExchanger(func() (string, error) { return srv.URL, nil }, srv.Client(), "cid", "csec")
	_, err := e.exchange(context.Background(), "bad-code", "verifier")
	if err == nil {
		t.Fatal("expected error for missing access_token")
	}
}

func TestOAuthTokenExchanger_NilClient(t *testing.T) {
	e := newTokenExchanger(func() (string, error) { return "http://localhost", nil }, nil, "cid", "csec")
	_, err := e.exchange(context.Background(), "code", "verifier")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}
