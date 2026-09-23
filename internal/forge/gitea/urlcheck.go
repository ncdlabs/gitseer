package gitea

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// URLCheckCode classifies setup URL blur / normalize results.
type URLCheckCode string

const (
	URLCheckOK             URLCheckCode = ""
	URLCheckInvalid        URLCheckCode = "invalid"
	URLCheckUnreachable    URLCheckCode = "unreachable"
	URLCheckPrivateNetwork URLCheckCode = "private_network"
)

// URLCheckResult is the outcome of normalizing and probing a Gitea base URL.
type URLCheckResult struct {
	OK        bool         `json:"ok"`
	URL       string       `json:"gitea_url,omitempty"`
	Private   bool         `json:"private"`
	PrivateIP string       `json:"private_ip,omitempty"`
	Code      URLCheckCode `json:"code,omitempty"`
	Error     string       `json:"error,omitempty"`
}

// PrivateNetworkCheckboxMessage is shown next to Allow Private Network Addresses.
func PrivateNetworkCheckboxMessage(ip net.IP) string {
	if ip != nil {
		return fmt.Sprintf(
			"This is a private address (%s). To continue, check %q.",
			ip,
			PrivateNetworkOptionLabel,
		)
	}
	return fmt.Sprintf("This is a private address. To continue, check %q.", PrivateNetworkOptionLabel)
}

// CheckGiteaURL normalizes scheme (https then http when missing), verifies the
// instance answers /api/v1/version, then requires the private-network option for RFC1918/lab hosts.
func CheckGiteaURL(ctx context.Context, raw string, allowPrivate bool) URLCheckResult {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return URLCheckResult{Code: URLCheckInvalid, Error: "Gitea URL is required."}
	}

	cands := urlCandidates(raw)
	if len(cands) == 0 {
		return URLCheckResult{Code: URLCheckInvalid, Error: "Enter a valid Gitea host or URL."}
	}

	var lastErr error
	var chosen string
	var blockedURL string
	var blockedIP net.IP
	for _, cand := range cands {
		u, err := url.Parse(cand)
		if err != nil || u.Hostname() == "" {
			lastErr = fmt.Errorf("invalid url")
			continue
		}
		privIP, err := lookupPrivateIP(u.Hostname())
		if err != nil {
			lastErr = err
			continue
		}
		if privIP != nil && !allowPrivate {
			blockedURL = cand
			blockedIP = privIP
			continue
		}
		if err := probeVersion(ctx, cand, allowPrivate); err != nil {
			lastErr = err
			continue
		}
		chosen = cand
		break
	}
	if chosen == "" {
		if blockedIP != nil {
			return URLCheckResult{
				OK:        false,
				URL:       blockedURL,
				Private:   true,
				PrivateIP: blockedIP.String(),
				Code:      URLCheckPrivateNetwork,
				Error:     PrivateNetworkCheckboxMessage(blockedIP),
			}
		}
		msg := "Could not reach Gitea at this address over HTTPS or HTTP."
		if len(cands) == 1 {
			msg = "Could not reach Gitea at this URL."
		}
		if lastErr != nil {
			msg = fmt.Sprintf("%s (%s)", msg, truncate(lastErr.Error(), 120))
		}
		return URLCheckResult{Code: URLCheckUnreachable, Error: msg}
	}

	u, err := url.Parse(chosen)
	if err != nil || u.Hostname() == "" {
		return URLCheckResult{Code: URLCheckInvalid, Error: "Enter a valid Gitea host or URL."}
	}

	privIP, err := lookupPrivateIP(u.Hostname())
	if err != nil {
		return URLCheckResult{Code: URLCheckUnreachable, Error: fmt.Sprintf("Could not resolve %s.", u.Hostname()), URL: chosen}
	}
	if privIP != nil {
		return URLCheckResult{OK: true, URL: chosen, Private: true, PrivateIP: privIP.String()}
	}
	return URLCheckResult{OK: true, URL: chosen, Private: false}
}

func urlCandidates(raw string) []string {
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return []string{strings.TrimRight(raw, "/")}
	}
	raw = strings.TrimPrefix(raw, "//")
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return nil
	}
	// Reject values that already look like a broken scheme.
	if strings.Contains(raw, "://") {
		return nil
	}
	return []string{"https://" + raw, "http://" + raw}
}

func probeVersion(ctx context.Context, base string, allowPrivate bool) error {
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	versionURL := strings.TrimRight(base, "/") + "/api/v1/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	client := NewHTTPClient(allowPrivate, 8*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return nil
}

// lookupPrivateIP returns a private/lab IP if the host resolves to one; nil if all public.
func lookupPrivateIP(host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return ip, nil
		}
		return nil, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses")
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return ip, nil
		}
	}
	return nil, nil
}
