package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Server.Listen != "0.0.0.0:8090" {
		t.Fatalf("listen = %q", cfg.Server.Listen)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("driver = %q", cfg.Database.Driver)
	}
}

func TestLoadYAMLAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
server:
  listen: 127.0.0.1:9000
database:
  driver: sqlite
  path: /tmp/from-file.db
gitea:
  url: https://git.example.com
  allow_unsigned_webhooks: true
ui:
  instance_name: FromFile
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LENS_SERVER_LISTEN", "0.0.0.0:8091")
	t.Setenv("LENS_UI_INSTANCE_NAME", "FromEnv")
	t.Setenv("LENS_SYNC_HISTORY_DAYS", "7")
	t.Setenv("LENS_SYNC_RECONCILE_INTERVAL", "2m")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen != "0.0.0.0:8091" {
		t.Fatalf("listen override = %q", cfg.Server.Listen)
	}
	if cfg.UI.InstanceName != "FromEnv" {
		t.Fatalf("ui name = %q", cfg.UI.InstanceName)
	}
	if cfg.Gitea.URL != "https://git.example.com" {
		t.Fatalf("gitea url = %q", cfg.Gitea.URL)
	}
	if cfg.Sync.HistoryDays != 7 {
		t.Fatalf("history_days = %d", cfg.Sync.HistoryDays)
	}
	if cfg.Sync.ReconcileInterval != 2*time.Minute {
		t.Fatalf("reconcile = %s", cfg.Sync.ReconcileInterval)
	}
}

func TestTokenFileEnv(t *testing.T) {
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokPath, []byte("sekret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("database:\n  path: "+filepath.Join(dir, "db")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LENS_GITEA_TOKEN_FILE", tokPath)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gitea.Token != "sekret" {
		t.Fatalf("token = %q", cfg.Gitea.Token)
	}
}

func TestValidateRejectsBadDriver(t *testing.T) {
	cfg := Default()
	cfg.Database.Driver = "mysql"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateWebhookFailClosed(t *testing.T) {
	cfg := Default()
	cfg.Gitea.URL = "https://git.example.com"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected webhook secret required")
	}
	cfg.Gitea.AllowUnsignedWebhooks = true
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Gitea.AllowUnsignedWebhooks = false
	cfg.Gitea.WebhookSecret = "sekret"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLookupEnvFileUnreadable(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("database:\n  path: "+filepath.Join(dir, "db")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LENS_GITEA_TOKEN_FILE", filepath.Join(dir, "missing-token"))
	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error for unreadable token file")
	}
}
