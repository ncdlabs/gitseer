package bitbucket

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PrivateNetworkOptionLabel is the UI checkbox label referenced in user-facing errors.
const PrivateNetworkOptionLabel = "Allow Private Network Addresses"

// NormalizeBaseURL returns the canonical Bitbucket Cloud API root.
func NormalizeBaseURL(raw string) (string, error) {
	u, err := normalizeAPIBase(raw)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// CheckBitbucketURL validates scheme/host/SSRF without authenticating.
func CheckBitbucketURL(raw string, allowPrivate bool) (ok bool, normalized string, err error) {
	u, err := normalizeAPIBase(raw)
	if err != nil {
		return false, "", err
	}
	if err := validateURL(u, allowPrivate); err != nil {
		return false, "", err
	}
	return true, strings.TrimRight(u.String(), "/"), nil
}

func normalizeAPIBase(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("bitbucket url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + strings.TrimPrefix(raw, "//")
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse bitbucket url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("bitbucket url scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("bitbucket url host is required")
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "bitbucket.org", "www.bitbucket.org", "api.bitbucket.org":
		out, err := url.Parse("https://api.bitbucket.org/2.0")
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	path := strings.TrimRight(u.Path, "/")
	switch {
	case path == "" || path == "/":
		u.Path = "/2.0"
	case strings.HasSuffix(path, "/2.0"):
		u.Path = path
	case strings.HasSuffix(path, "/rest/api"):
		u.Path = path + "/1.0" // Data Center — limited; keep path for dial
	default:
		if !strings.Contains(path, "2.0") && !strings.Contains(path, "rest") {
			u.Path = path + "/2.0"
		}
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

// OAuthWebBase returns the browser origin for Bitbucket Cloud OAuth.
func OAuthWebBase(raw string) (string, error) {
	u, err := normalizeAPIBase(raw)
	if err != nil {
		return "", err
	}
	host := strings.ToLower(u.Hostname())
	if host == "api.bitbucket.org" {
		return "https://bitbucket.org", nil
	}
	out := *u
	path := strings.TrimRight(out.Path, "/")
	path = strings.TrimSuffix(path, "/2.0")
	out.Path = path
	out.RawQuery = ""
	out.Fragment = ""
	return strings.TrimRight(out.String(), "/"), nil
}

// NewHTTPClient returns an HTTP client with redirect and dial-time private-network guards.
func NewHTTPClient(allowPrivate bool, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validateURL(req.URL, allowPrivate)
		},
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil {
					host = address
					port = ""
				}
				ips, err := resolveDialIPs(host, allowPrivate)
				if err != nil {
					return nil, err
				}
				var firstErr error
				for _, ip := range ips {
					addr := ip.String()
					if port != "" {
						addr = net.JoinHostPort(ip.String(), port)
					}
					conn, derr := dialer.DialContext(ctx, network, addr)
					if derr == nil {
						return conn, nil
					}
					if firstErr == nil {
						firstErr = derr
					}
				}
				if firstErr == nil {
					firstErr = fmt.Errorf("no addresses to dial for %s", host)
				}
				return nil, firstErr
			},
		},
	}
}

func resolveDialIPs(host string, allowPrivate bool) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !allowPrivate && isBlockedIP(ip) {
			return nil, privateAddressError(host, ip)
		}
		return []net.IP{ip}, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("dns lookup for %s: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("dns lookup for %s returned no addresses", host)
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if !allowPrivate && isBlockedIP(ip) {
			return nil, privateAddressError(host, ip)
		}
		out = append(out, ip)
	}
	return out, nil
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

func privateAddressError(_ string, ip net.IP) error {
	return fmt.Errorf(
		"This Bitbucket URL points to a private network address (%s). Check %q to allow GitSeer to connect.",
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
