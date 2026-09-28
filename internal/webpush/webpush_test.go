package webpush_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/webpush"
)

func TestEnsureKeysGeneratesAndPersists(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Database.Path = filepath.Join(dir, "gitseer.db")
	cfg.Notifications.VAPIDSubject = "mailto:test@example.com"

	s := webpush.NewSender(nil, nil, "")
	if err := s.EnsureKeys(cfg); err != nil {
		t.Fatal(err)
	}
	if !s.Configured() || s.PublicKey() == "" {
		t.Fatal("expected configured keys")
	}
	path := webpush.DefaultVAPIDPath(cfg)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected key file: %v", err)
	}

	s2 := webpush.NewSender(nil, nil, "")
	if err := s2.EnsureKeys(cfg); err != nil {
		t.Fatal(err)
	}
	if s2.PublicKey() != s.PublicKey() {
		t.Fatal("expected reload same public key")
	}
}

func TestEnsureKeysFromConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Notifications.VAPIDPublicKey = "pub"
	cfg.Notifications.VAPIDPrivateKey = "priv"
	s := webpush.NewSender(nil, nil, "")
	if err := s.EnsureKeys(cfg); err != nil {
		t.Fatal(err)
	}
	if s.PublicKey() != "pub" {
		t.Fatalf("got %q", s.PublicKey())
	}
}
