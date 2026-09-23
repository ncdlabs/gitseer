// Package ratelimit provides a simple in-process per-IP token window.
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter is a coarse per-key rate limiter (no Redis).
type Limiter struct {
	mu       sync.Mutex
	window   time.Duration
	maxHits  int
	hits     map[string][]time.Time
	trusted  []*net.IPNet
}

func New(window time.Duration, maxHits int, trusted ...*net.IPNet) *Limiter {
	if window <= 0 {
		window = time.Minute
	}
	if maxHits <= 0 {
		maxHits = 30
	}
	return &Limiter{window: window, maxHits: maxHits, hits: map[string][]time.Time{}, trusted: trusted}
}

func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	arr := l.hits[key]
	kept := arr[:0]
	for _, t := range arr {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.maxHits {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// Middleware returns 429 when the client IP exceeds the limit.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(ClientIP(r, l.trusted)) {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIP returns the peer address, or a forwarded client IP only when the
// immediate peer is in trusted (configured reverse-proxy CIDRs).
func ClientIP(r *http.Request, trusted []*net.IPNet) string {
	remote := remoteIP(r.RemoteAddr)
	if len(trusted) == 0 || !ipInNets(remote, trusted) {
		return remote
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if ip := net.ParseIP(strings.TrimSpace(strings.Split(xri, ",")[0])); ip != nil {
			return ip.String()
		}
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip.String()
		}
	}
	return remote
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		if ip := net.ParseIP(remoteAddr); ip != nil {
			return ip.String()
		}
		return remoteAddr
	}
	return host
}

func ipInNets(ipStr string, nets []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}
