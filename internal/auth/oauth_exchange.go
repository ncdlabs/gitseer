package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// tokenExchangeStyle selects how the token request body is encoded.
type tokenExchangeStyle int

const (
	// tokenExchangeForm encodes the request as application/x-www-form-urlencoded.
	tokenExchangeForm tokenExchangeStyle = iota
	// tokenExchangeJSON encodes the request as application/json.
	tokenExchangeJSON
)

// oauthTokenExchanger provides shared OAuth 2.0 token exchange and refresh for all forge providers.
// Each provider configures an endpoint function, an optional HTTP client, credentials, and encoding style.
type oauthTokenExchanger struct {
	endpointFn func() (string, error)
	client     *http.Client
	clientID   string
	secret     string
	// style selects form (default) or JSON encoding.
	style tokenExchangeStyle
	// basicAuth uses HTTP Basic auth instead of in-body client_secret.
	basicAuth bool
	// redirectURI is included in the exchange request (refresh requests may omit it).
	redirectURI string
}

// newTokenExchanger builds an exchanger for the given endpoint and credentials.
func newTokenExchanger(endpointFn func() (string, error), client *http.Client, clientID, secret string) *oauthTokenExchanger {
	return &oauthTokenExchanger{
		endpointFn: endpointFn,
		client:     client,
		clientID:   clientID,
		secret:     secret,
		style:      tokenExchangeForm,
	}
}

// exchange performs an Authorization Code + PKCE token exchange.
func (e *oauthTokenExchanger) exchange(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	vals := url.Values{}
	vals.Set("grant_type", "authorization_code")
	vals.Set("code", code)
	vals.Set("redirect_uri", e.redirectURI)
	vals.Set("code_verifier", verifier)
	return e.doTokenRequest(ctx, vals)
}

// refresh performs a refresh_token grant.
func (e *oauthTokenExchanger) refresh(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	vals := url.Values{}
	vals.Set("grant_type", "refresh_token")
	vals.Set("refresh_token", refreshToken)
	return e.doTokenRequest(ctx, vals)
}

func (e *oauthTokenExchanger) doTokenRequest(ctx context.Context, vals url.Values) (*tokenResponse, error) {
	if e.client == nil {
		return nil, fmt.Errorf("oauth http client not configured")
	}
	endpoint, err := e.endpointFn()
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if e.style == tokenExchangeJSON {
		if e.secret != "" {
			vals.Set("client_secret", e.secret)
		}
		// url.Values is map[string][]string; convert to map[string]string for clean JSON.
		obj := make(map[string]string, len(vals))
		for k, v := range vals {
			if len(v) > 0 {
				obj[k] = v[0]
			}
		}
		payload, _ := json.Marshal(obj)
		body = strings.NewReader(string(payload))
	} else {
		vals.Set("client_id", e.clientID)
		if e.secret != "" {
			vals.Set("client_secret", e.secret)
		}
		body = strings.NewReader(vals.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if e.style == tokenExchangeJSON {
		req.Header.Set("Content-Type", "application/json")
	}
	if e.basicAuth {
		req.SetBasicAuth(e.clientID, e.secret)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token request failed: %s: %s", resp.Status, truncate(string(raw), 200))
	}
	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = vals.Get("refresh_token")
	}
	return &tok, nil
}
