package settings_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/config"
	"github.com/ncdlabs/gitea-lens/internal/database"
	"github.com/ncdlabs/gitea-lens/internal/settings"
	"github.com/ncdlabs/gitea-lens/internal/store"
)

func TestManagerLoadUpdate(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "lens.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	cfg := config.Default()
	cfg.UI.InstanceName = "Default Name"
	cfg.Sync.HistoryDays = 14
	cfg.Attention.LongRunningAfter = time.Hour
	cfg.Retention.RunsDays = 60

	mgr := settings.New(cfg, st)
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	got := mgr.Get()
	if got.InstanceName != "Default Name" || got.SyncHistoryDays != 14 {
		t.Fatalf("defaults = %+v", got)
	}

	updated, err := mgr.Update(ctx, settings.Values{
		InstanceName:              "Ops Console",
		SyncHistoryDays:           45,
		AttentionLongRunningAfter: "3h",
		RetentionRunsDays:         120,
		RetentionWebhooksDays:     14,
		RetentionAttentionDays:    200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.InstanceName != "Ops Console" {
		t.Fatalf("updated = %+v", updated)
	}
	if mgr.LongRunningAfter() != 3*time.Hour {
		t.Fatalf("long running = %s", mgr.LongRunningAfter())
	}
	if mgr.Retention().RunsDays != 120 {
		t.Fatalf("retention = %+v", mgr.Retention())
	}

	mgr2 := settings.New(cfg, st)
	if err := mgr2.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if mgr2.Get().InstanceName != "Ops Console" {
		t.Fatalf("reloaded = %+v", mgr2.Get())
	}
}

func TestValidateRejectsBadDuration(t *testing.T) {
	_, err := settings.Validate(settings.Values{
		InstanceName:              "x",
		SyncHistoryDays:           7,
		AttentionLongRunningAfter: "nope",
		RetentionRunsDays:         1,
		RetentionWebhooksDays:     1,
		RetentionAttentionDays:    1,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIntegrationMergeLeaveBlankAndSetup(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "lens.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	cfg := config.Default()
	cfg.Gitea.URL = "https://git.env.example"
	cfg.Gitea.Token = "env-token"
	cfg.Gitea.WebhookSecret = "env-hook"
	cfg.Gitea.AllowPrivateNetwork = true
	cfg.Auth.OAuthClientID = "env-oauth"
	cfg.Auth.OAuthClientSecret = "env-oauth-secret"
	cfg.Auth.EncryptionKey = "sixteen-chars-key!!"

	mgr := settings.New(cfg, st)
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	integ := mgr.Integration()
	if integ.URL != "https://git.env.example" || integ.Token != "env-token" {
		t.Fatalf("config fallback = %+v", integ)
	}
	if mgr.SetupCompleted() {
		t.Fatal("setup should be incomplete by default")
	}

	pub, err := mgr.UpdateIntegration(ctx, settings.IntegrationPatch{
		GiteaURL:                   "https://git.db.example",
		GiteaToken:                 "db-token",
		GiteaWebhookSecret:         "db-hook",
		GiteaAllowPrivateNetwork:   false,
		GiteaAllowUnsignedWebhooks: false,
		OAuthClientID:              "db-oauth",
		OAuthClientSecret:          "db-oauth-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pub.GiteaURL != "https://git.db.example" || !pub.GiteaTokenConfigured || pub.OAuthClientID != "db-oauth" {
		t.Fatalf("public = %+v", pub)
	}
	if pub.GiteaAllowPrivateNetwork {
		t.Fatal("DB bool should win over config")
	}

	// Leave blank secrets: token/webhook/oauth secret unchanged.
	pub2, err := mgr.UpdateIntegration(ctx, settings.IntegrationPatch{
		GiteaURL:                   "https://git.db.example",
		GiteaAllowPrivateNetwork:   false,
		GiteaAllowUnsignedWebhooks: false,
		OAuthClientID:              "db-oauth-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pub2.GiteaTokenConfigured || !pub2.GiteaWebhookSecretConfigured || !pub2.OAuthClientSecretConfigured {
		t.Fatalf("leave-blank cleared secrets: %+v", pub2)
	}
	if mgr.Integration().Token != "db-token" || mgr.Integration().OAuthClientID != "db-oauth-2" {
		t.Fatalf("leave-blank integ = %+v", mgr.Integration())
	}

	if err := mgr.SetSetupCompleted(ctx, true); err != nil {
		t.Fatal(err)
	}
	if !mgr.SetupCompleted() {
		t.Fatal("expected setup completed")
	}

	mgr2 := settings.New(cfg, st)
	if err := mgr2.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if mgr2.Integration().URL != "https://git.db.example" || mgr2.Integration().Token != "db-token" {
		t.Fatalf("reloaded integ = %+v", mgr2.Integration())
	}
	if !mgr2.SetupCompleted() {
		t.Fatal("setup_completed not persisted")
	}

	// URL without webhook secret requires allow-unsigned.
	_, err = mgr2.UpdateIntegration(ctx, settings.IntegrationPatch{
		GiteaURL:                 "https://git.db.example",
		ClearGiteaWebhookSecret:  true,
		GiteaAllowUnsignedWebhooks: false,
	})
	if err == nil {
		t.Fatal("expected webhook secret validation error")
	}
}

func TestIntegrationPublicNeverExposesSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.Gitea.Token = "sekret-token"
	cfg.Gitea.WebhookSecret = "sekret-hook"
	cfg.Auth.OAuthClientSecret = "sekret-oauth"
	mgr := settings.New(cfg, store.New(nil))
	pub := mgr.IntegrationPublic()
	raw, _ := json.Marshal(pub)
	s := string(raw)
	for _, bad := range []string{"sekret-token", "sekret-hook", "sekret-oauth"} {
		if strings.Contains(s, bad) {
			t.Fatalf("secret leaked in public json: %s", s)
		}
	}
}
