package bitbucket

import (
	"fmt"
	"net/url"
	"strings"
)

// BuildAuthorizeURL constructs a Bitbucket Cloud OAuth authorize URL with PKCE.
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
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	if codeChallenge != "" {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}
	return strings.TrimRight(web, "/") + "/site/oauth2/authorize?" + q.Encode(), nil
}

// TokenEndpoint returns the Bitbucket Cloud OAuth token URL.
func TokenEndpoint(baseURL string) (string, error) {
	web, err := OAuthWebBase(baseURL)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(web, "/") + "/site/oauth2/access_token", nil
}
