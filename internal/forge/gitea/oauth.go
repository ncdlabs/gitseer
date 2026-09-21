package gitea

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// DefaultOAuthAppName is the application name Lens creates on Gitea.
const DefaultOAuthAppName = "Gitea Lens"

// OAuthAppPreview is the create-app summary shown in the setup wizard.
type OAuthAppPreview struct {
	Name               string `json:"name"`
	RedirectURI        string `json:"redirect_uri"`
	ConfidentialClient bool   `json:"confidential_client"`
	GiteaSettingsPath  string `json:"gitea_settings_path"`
	GiteaAdminAppsPath string `json:"gitea_admin_apps_path"`
}

// NewOAuthAppPreview builds the OAuth app summary for setup UI.
func NewOAuthAppPreview(redirectURI string) *OAuthAppPreview {
	return &OAuthAppPreview{
		Name:               DefaultOAuthAppName,
		RedirectURI:        strings.TrimSpace(redirectURI),
		ConfidentialClient: true,
		GiteaSettingsPath:  "/user/settings/applications",
		GiteaAdminAppsPath: "/admin/applications",
	}
}

// CreateOAuth2ApplicationOptions is the Gitea create/update OAuth2 app body.
type CreateOAuth2ApplicationOptions struct {
	Name               string   `json:"name"`
	RedirectURIs       []string `json:"redirect_uris"`
	ConfidentialClient bool     `json:"confidential_client"`
}

// OAuth2Application is a minimal Gitea OAuth2 application response.
type OAuth2Application struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	ClientID           string   `json:"client_id"`
	ClientSecret       string   `json:"client_secret"`
	RedirectURIs       []string `json:"redirect_uris"`
	ConfidentialClient bool     `json:"confidential_client"`
}

// EnsureOAuthApplication creates or updates a Lens OAuth2 application on Gitea.
// Updating regenerates the client secret (Gitea API behavior) so Lens can store it.
func (c *Client) EnsureOAuthApplication(ctx context.Context, name, redirectURI string) (*OAuth2Application, bool, error) {
	name = strings.TrimSpace(name)
	redirectURI = strings.TrimSpace(redirectURI)
	if name == "" {
		name = DefaultOAuthAppName
	}
	if redirectURI == "" {
		return nil, false, fmt.Errorf("oauth redirect uri is required")
	}

	existing, err := c.findOAuthApp(ctx, name, redirectURI)
	if err != nil {
		return nil, false, err
	}

	body := CreateOAuth2ApplicationOptions{
		Name:               name,
		RedirectURIs:       []string{redirectURI},
		ConfidentialClient: true,
	}

	if existing != nil {
		resp, err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/user/applications/oauth2/%d", existing.ID), nil, body)
		if err != nil {
			return nil, false, err
		}
		var app OAuth2Application
		if err := decodeJSON(resp, &app); err != nil {
			return nil, false, err
		}
		if strings.TrimSpace(app.ClientSecret) == "" {
			return nil, false, fmt.Errorf("gitea did not return a client secret on update")
		}
		return &app, false, nil
	}

	resp, err := c.doJSON(ctx, http.MethodPost, "/user/applications/oauth2", nil, body)
	if err != nil {
		return nil, false, err
	}
	var app OAuth2Application
	if err := decodeJSON(resp, &app); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(app.ClientID) == "" || strings.TrimSpace(app.ClientSecret) == "" {
		return nil, false, fmt.Errorf("gitea did not return oauth client credentials")
	}
	return &app, true, nil
}

func (c *Client) findOAuthApp(ctx context.Context, name, redirectURI string) (*OAuth2Application, error) {
	resp, err := c.do(ctx, http.MethodGet, "/user/applications/oauth2", nil, "")
	if err != nil {
		return nil, err
	}
	var apps []OAuth2Application
	if err := decodeJSON(resp, &apps); err != nil {
		return nil, err
	}
	wantRedirect := strings.TrimRight(redirectURI, "/")
	var byName *OAuth2Application
	for i := range apps {
		app := &apps[i]
		for _, u := range app.RedirectURIs {
			if strings.TrimRight(u, "/") == wantRedirect {
				return app, nil
			}
		}
		if byName == nil && strings.EqualFold(strings.TrimSpace(app.Name), name) {
			byName = app
		}
	}
	return byName, nil
}
