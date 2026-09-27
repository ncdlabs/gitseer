package gitlab

import (
	"fmt"
	"net/url"
	"strings"
)

// BuildAuthorizeURL constructs a GitLab OAuth authorize URL with PKCE.
func BuildAuthorizeURL(baseURL, clientID, redirectURI, state, codeChallenge string) (string, error) {
	web, err := OAuthWebBase(baseURL)
	if err != nil {
		return "", err
	}
	clientID = strings.TrimSpace(clientID)
	redirectURI = strings.TrimSpace(redirectURI)
	if clientID == "" || redirectURI == "" {
		return "", fmt.Errorf("oauth client_id and redirect_uri are required")
	}
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	q.Set("scope", "read_api read_user read_repository")
	if codeChallenge != "" {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}
	return strings.TrimRight(web, "/") + "/oauth/authorize?" + q.Encode(), nil
}

// TokenEndpoint returns the GitLab OAuth token URL.
func TokenEndpoint(baseURL string) (string, error) {
	web, err := OAuthWebBase(baseURL)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(web, "/") + "/oauth/token", nil
}
