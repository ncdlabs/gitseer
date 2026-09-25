package forge_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ncdlabs/gitseer/internal/forge"
	_ "github.com/ncdlabs/gitseer/internal/forge/all"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestNewUnsupportedType(t *testing.T) {
	_, err := forge.New(forge.Options{ForgeType: "gitlab", BaseURL: "https://gitlab.com", Token: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewFromInstanceDefaultsGitea(t *testing.T) {
	// Empty forge type → gitea; construction fails on SSRF without allow-private for loopback-ish hosts,
	// so use a clearly invalid host that fails parse/scheme checks instead.
	_, err := forge.NewFromInstance(models.Instance{
		ForgeType: "",
		BaseURL:   "not-a-url",
	}, "t")
	if err == nil {
		t.Fatal("expected error from gitea constructor")
	}
}

func TestNewGitHubRegisters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	_, err := forge.New(forge.Options{
		ForgeType:           models.ForgeTypeGitHub,
		BaseURL:             srv.URL,
		Token:               "t",
		AllowPrivateNetwork: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}
