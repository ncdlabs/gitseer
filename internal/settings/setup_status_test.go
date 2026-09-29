package settings_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/settings"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestEvaluateSetupUsesConfigNotOnlyFlag(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "setup-status.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	cfg := config.Default()
	mgr := settings.New(cfg, st)
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}

	st0 := mgr.EvaluateSetup(ctx, false)
	if !st0.NeedsSetup || !st0.FirstRun {
		t.Fatalf("empty install: %+v", st0)
	}
	if st0.EncryptionConfigured || st0.ForgeConfigured {
		t.Fatalf("expected missing enc/forge: %+v", st0)
	}

	// Mark completed without encryption/forge — still needs setup (misconfigured flag).
	if err := mgr.SetSetupCompleted(ctx, true); err != nil {
		t.Fatal(err)
	}
	st1 := mgr.EvaluateSetup(ctx, false)
	if !st1.NeedsSetup || st1.FirstRun {
		t.Fatalf("completed without assets should still need setup: %+v", st1)
	}
	if len(st1.Reasons) == 0 {
		t.Fatal("expected reasons")
	}

	// Local skip after complete: do not force wizard.
	stSkip := mgr.EvaluateSetup(ctx, true)
	if stSkip.NeedsSetup {
		t.Fatalf("allowSkipSetup + completed should not need setup: %+v", stSkip)
	}

	// Config with encryption + gitea URL/token should satisfy readiness once completed.
	cfg2 := config.Default()
	cfg2.Auth.EncryptionKey = "twenty-four-char-key-ok!!"
	cfg2.Gitea.URL = "https://git.example.com"
	cfg2.Gitea.Token = "tok"
	mgr2 := settings.New(cfg2, st)
	if err := mgr2.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if err := mgr2.SetSetupCompleted(ctx, true); err != nil {
		t.Fatal(err)
	}
	st2 := mgr2.EvaluateSetup(ctx, false)
	if st2.NeedsSetup {
		t.Fatalf("fully configured from config file should not need setup: %+v", st2)
	}
	if !st2.EncryptionConfigured || !st2.ForgeConfigured {
		t.Fatalf("expected enc+forge from config: %+v", st2)
	}
}

func TestEvaluateSetupPartialForgeFromConfig(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "partial-forge.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	cfg := config.Default()
	cfg.Auth.EncryptionKey = "twenty-four-char-key-ok!!"
	cfg.Gitea.URL = "https://git.example.com"
	// token intentionally empty — misconfigured
	mgr := settings.New(cfg, st)
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetSetupCompleted(ctx, true); err != nil {
		t.Fatal(err)
	}
	got := mgr.EvaluateSetup(ctx, false)
	if !got.NeedsSetup || got.ForgeConfigured {
		t.Fatalf("URL without token should need setup: %+v", got)
	}
	found := false
	for _, r := range got.Reasons {
		if r == "gitea/forgejo URL set in config without token" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected partial forge hint, got %v", got.Reasons)
	}
}

func TestHasUsableForgeFromInstance(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "forge-inst.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	cfg := config.Default()
	cfg.Auth.EncryptionKey = "twenty-four-char-key-ok!!"
	mgr := settings.New(cfg, st)
	if err := mgr.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if mgr.HasUsableForge(ctx) {
		t.Fatal("expected no forge")
	}
	allow := true
	if _, err := mgr.CreateInstance(ctx, settings.InstancePatch{
		ForgeType:             "gitlab",
		Name:                  "GL",
		BaseURL:               "https://gitlab.example.com",
		Token:                 "gl-tok",
		AllowUnsignedWebhooks: &allow,
	}); err != nil {
		t.Fatal(err)
	}
	if !mgr.HasUsableForge(ctx) {
		t.Fatal("expected gitlab instance to count")
	}
	if err := mgr.SetSetupCompleted(ctx, true); err != nil {
		t.Fatal(err)
	}
	if mgr.NeedsSetup(ctx, false) {
		t.Fatal("enc + instance + completed should be ready")
	}
}
