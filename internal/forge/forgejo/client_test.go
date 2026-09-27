package forgejo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/forge/forgejo"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestForgejoRegistersAndDelegates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.21.0-forgejo"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := forge.New(forge.Options{
		ForgeType:           models.ForgeTypeForgejo,
		BaseURL:             srv.URL,
		Token:               "t",
		AllowPrivateNetwork: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.GetInstance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "1.21.0-forgejo" {
		t.Fatalf("version=%q", info.Version)
	}
	fj, ok := c.(*forgejo.Client)
	if !ok || fj.Inner() == nil {
		t.Fatal("expected *forgejo.Client wrapping gitea")
	}
}
