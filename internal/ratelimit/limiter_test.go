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
