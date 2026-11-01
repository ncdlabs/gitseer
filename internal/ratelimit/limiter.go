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
	mu      sync.Mutex
	window  time.Duration
	maxHits int
	hits    map[string][]time.Time
	trusted []*net.IPNet
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
//
// The TCP peer in r.RemoteAddr is treated as the rightmost hop (mirroring the
// go-chi/chi v5.4.0 ClientIPFromXFF fix for GHSA-j6qf-3m9c-vwj3): an untrusted
// peer is the client IP and forwarding headers are ignored, so a client that
// reaches the origin directly cannot spoof its IP by appending a trusted hop
// to X-Forwarded-For. When the peer is trusted, X-Forwarded-For is walked
// right-to-left skipping trusted entries and the first untrusted entry is the
// client; all-trusted or unparseable chains fall back to the peer (fail-closed
// to a verified address, keeping rate-limit keys unspoofable).
func ClientIP(r *http.Request, trusted []*net.IPNet) string {
	remote := remoteIP(r.RemoteAddr)
	if len(trusted) == 0 || !ipInNets(remote, trusted) {
		return remote
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		addrs := parseForwardedChain(r.Header.Values("X-Forwarded-For"))
		for i := len(addrs) - 1; i >= 0; i-- {
			if addrs[i] == nil {
				// Garbage in a trusted chain: fail closed to the verified peer.
				return remote
			}
			if !ipNetContainsIP(trusted, addrs[i]) {
				return addrs[i].String()
			}
		}
		// All entries trusted (or empty after parsing): the verified peer is
		// the only address we did not learn from the client.
		if len(addrs) > 0 {
			return remote
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if ip := net.ParseIP(strings.TrimSpace(strings.Split(xri, ",")[0])); ip != nil {
			if !ipNetContainsIP(trusted, ip) {
				return ip.String()
			}
			return remote
		}
	}
	return remote
}

// parseForwardedChain flattens one or more X-Forwarded-For header values into
// ordered IPs (left-to-right). Unparseable entries are nil to force fail-closed.
func parseForwardedChain(values []string) []net.IP {
	var out []net.IP
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			out = append(out, net.ParseIP(part))
		}
	}
	return out
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

// PeerIsLoopback reports whether the immediate TCP peer is a loopback address.
// Does not honor X-Forwarded-* (must not trust client headers for password exposure).
func PeerIsLoopback(r *http.Request) bool {
	ip := net.ParseIP(remoteIP(r.RemoteAddr))
	return ip != nil && ip.IsLoopback()
}

func ipInNets(ipStr string, nets []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ipNetContainsIP(nets, ip)
}

func ipNetContainsIP(nets []*net.IPNet, ip net.IP) bool {
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
