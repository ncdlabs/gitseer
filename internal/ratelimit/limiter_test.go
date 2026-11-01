package ratelimit

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIPIgnoresSpoofedHeadersWithoutTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Real-IP", "198.51.100.1")
	req.Header.Set("X-Forwarded-For", "198.51.100.2")
	if got := ClientIP(req, nil); got != "203.0.113.10" {
		t.Fatalf("got %q", got)
	}
}

func TestClientIPHonorsHeadersFromTrustedProxy(t *testing.T) {
	_, n, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:443"
	req.Header.Set("X-Real-IP", "198.51.100.9")
	if got := ClientIP(req, []*net.IPNet{n}); got != "198.51.100.9" {
		t.Fatalf("got %q", got)
	}
}

func TestClientIPRejectsDirectSpoofEndingInTrustedHop(t *testing.T) {
	// GHSA-j6qf-3m9c-vwj3 PoC shape: direct (untrusted) peer appends a
	// trusted-looking hop and picks an allowlisted client IP.
	_, n, err := net.ParseCIDR("198.51.100.0/24")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:54417"
	req.Header.Set("X-Forwarded-For", "10.0.0.7, 198.51.100.42")
	if got := ClientIP(req, []*net.IPNet{n}); got == "10.0.0.7" {
		t.Fatal("vulnerable: returned attacker-supplied allowlisted IP")
	} else if got != "127.0.0.1" {
		t.Fatalf("want TCP peer 127.0.0.1, got %q", got)
	}
}

func TestClientIPStripsSpoofedLeftmostBehindTrustedProxy(t *testing.T) {
	// Trusted proxy appends the real client; attacker-prepended allowlisted
	// IP must not win (old code returned parts[0]).
	_, n, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:443"
	req.Header.Set("X-Forwarded-For", "10.0.0.7, 203.0.113.99")
	if got := ClientIP(req, []*net.IPNet{n}); got != "203.0.113.99" {
		t.Fatalf("got %q, want 203.0.113.99", got)
	}
}

func TestClientIPAllTrustedChainFallsBackToPeer(t *testing.T) {
	_, n, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:443"
	req.Header.Set("X-Forwarded-For", "10.0.0.7, 10.0.0.8")
	if got := ClientIP(req, []*net.IPNet{n}); got != "10.0.0.5" {
		t.Fatalf("got %q, want verified peer 10.0.0.5", got)
	}
	req.Header.Set("X-Real-IP", "10.0.0.7")
	req.Header.Del("X-Forwarded-For")
	if got := ClientIP(req, []*net.IPNet{n}); got != "10.0.0.5" {
		t.Fatalf("trusted X-Real-IP must not override peer, got %q", got)
	}
}

func TestClientIPGarbageChainFallsBackToPeer(t *testing.T) {
	_, n, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:443"
	req.Header.Set("X-Forwarded-For", "203.0.113.7, garbage")
	if got := ClientIP(req, []*net.IPNet{n}); got != "10.0.0.5" {
		t.Fatalf("got %q, want verified peer 10.0.0.5", got)
	}
}

func TestClientIPMergesMultipleXFFHeaders(t *testing.T) {
	_, n, err := net.ParseCIDR("198.51.100.0/24")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.5:443"
	req.Header.Add("X-Forwarded-For", "10.0.0.7")
	req.Header.Add("X-Forwarded-For", "203.0.113.9, 198.51.100.6")
	if got := ClientIP(req, []*net.IPNet{n}); got != "203.0.113.9" {
		t.Fatalf("got %q, want 203.0.113.9", got)
	}
}

func TestLimiterAllow(t *testing.T) {
	l := New(time.Minute, 2)
	if !l.Allow("a") || !l.Allow("a") || l.Allow("a") {
		t.Fatal("expected 2 allows then deny")
	}
}

func TestPeerIsLoopback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8090"
	if !PeerIsLoopback(req) {
		t.Fatal("expected loopback peer")
	}
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	if PeerIsLoopback(req) {
		t.Fatal("must not trust forwarded headers for peer loopback")
	}
}
