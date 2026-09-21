// Package proxyprefix strips a configured path prefix for reverse-proxy subpath deploys.
package proxyprefix

import (
	"net/http"
	"strings"
)

// Middleware strips PathPrefix derived from server.external_url (e.g. "/lens").
// Client-supplied X-Forwarded-Prefix is ignored to avoid path confusion attacks.
func Middleware(configuredPrefix string) func(http.Handler) http.Handler {
	configuredPrefix = strings.TrimRight(configuredPrefix, "/")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			prefix := configuredPrefix
			if prefix == "" || prefix == "/" {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Path == prefix {
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				next.ServeHTTP(w, r2)
				return
			}
			if strings.HasPrefix(r.URL.Path, prefix+"/") {
				r2 := r.Clone(r.Context())
				r2.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
				if r2.URL.Path == "" {
					r2.URL.Path = "/"
				}
				next.ServeHTTP(w, r2)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
