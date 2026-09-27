package github

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// PrivateNetworkOptionLabel is the UI checkbox label referenced in user-facing errors.
const PrivateNetworkOptionLabel = "Allow Private Network Addresses"

// NormalizeBaseURL returns the canonical API root string for storage and client construction.
func NormalizeBaseURL(raw string) (string, error) {
	u, err := normalizeAPIBase(raw)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// CheckGitHubURL validates a GitHub / GHE URL (scheme, host, SSRF) without authenticating.
func CheckGitHubURL(raw string, allowPrivate bool) (ok bool, normalized string, err error) {
	u, err := normalizeAPIBase(raw)
	if err != nil {
		return false, "", err
	}
	if err := validateURL(u, allowPrivate); err != nil {
		return false, "", err
	}
	return true, strings.TrimRight(u.String(), "/"), nil
}

// normalizeAPIBase turns a user-facing GitHub / GHE URL into the REST API root.
//
// Decisions:
//   - github.com / www.github.com → https://api.github.com (host swap; no /api/v3 path)
//   - api.github.com → https://api.github.com
//   - anything else (GHE): keep host/scheme; ensure path ends with /api/v3
func normalizeAPIBase(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("github url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + strings.TrimPrefix(raw, "//")
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse github url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("github url scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("github url host is required")
	}

	host := strings.ToLower(u.Hostname())
	switch host {
	case "github.com", "www.github.com", "api.github.com":
		out, err := url.Parse("https://api.github.com")
		if err != nil {
			return nil, err
		}
		return out, nil
	}

	path := strings.TrimRight(u.Path, "/")
	switch {
	case path == "" || path == "/":
		u.Path = "/api/v3"
	case strings.HasSuffix(path, "/api/v3"):
		u.Path = path
	case strings.HasSuffix(path, "/api"):
		u.Path = path + "/v3"
	default:
		u.Path = path + "/api/v3"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

func validateURL(u *url.URL, allowPrivate bool) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("blocked scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}
	return validateHost(u.String(), host, allowPrivate)
}

func validateHost(displayURL, host string, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return privateAddressError(displayURL, ip)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("dns lookup for %s: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("dns lookup for %s returned no addresses", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return privateAddressError(displayURL, ip)
		}
	}
	return nil
}

func privateAddressError(_displayURL string, ip net.IP) error {
	return fmt.Errorf(
		"This GitHub URL points to a private network address (%s). Check %q to allow GitSeer to connect.",
		ip,
		PrivateNetworkOptionLabel,
	)
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

// OAuthWebBase returns the host used for /login/oauth/* (github.com or GHE web root).
// API bases like https://api.github.com map back to https://github.com.
func OAuthWebBase(raw string) (string, error) {
	u, err := normalizeAPIBase(raw)
	if err != nil {
		return "", err
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "api.github.com":
		return "https://github.com", nil
	}
	// GHE: strip /api/v3 path for the browser OAuth endpoints.
	out := *u
	path := strings.TrimRight(out.Path, "/")
	path = strings.TrimSuffix(path, "/api/v3")
	path = strings.TrimSuffix(path, "/api")
	out.Path = path
	out.RawQuery = ""
	out.Fragment = ""
	return strings.TrimRight(out.String(), "/"), nil
}
