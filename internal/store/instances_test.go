package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestUpsertInstanceSecretsAndForgeType(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "inst.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                    "Lab Gitea",
		ForgeType:               models.ForgeTypeGitea,
		BaseURL:                 "https://git.example.com",
		SyncTokenCiphertext:     "tok-cipher",
		WebhookSecretCiphertext: "hook-cipher",
		AllowPrivateNetwork:     true,
		AllowUnsignedWebhooks:   false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.ID == 0 || inst.ForgeType != models.ForgeTypeGitea {
		t.Fatalf("got %+v", inst)
	}
	if inst.SyncTokenCiphertext != "tok-cipher" || !inst.AllowPrivateNetwork {
		t.Fatalf("secrets/flags = %+v", inst)
	}

	// Empty ciphertext leaves prior secrets; flags update.
	inst2, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                  "Lab Gitea",
		ForgeType:             models.ForgeTypeGitea,
		BaseURL:               "https://git.example.com",
		AllowPrivateNetwork:   false,
		AllowUnsignedWebhooks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst2.ID != inst.ID {
		t.Fatalf("id changed %d vs %d", inst2.ID, inst.ID)
	}
	if inst2.SyncTokenCiphertext != "tok-cipher" || inst2.WebhookSecretCiphertext != "hook-cipher" {
		t.Fatalf("secrets cleared: %+v", inst2)
	}
	if inst2.AllowPrivateNetwork || !inst2.AllowUnsignedWebhooks {
		t.Fatalf("flags = private=%v unsigned=%v", inst2.AllowPrivateNetwork, inst2.AllowUnsignedWebhooks)
	}

	gh, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitHub, "GitHub", "https://api.github.com", "enterprise", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if gh.ForgeType != models.ForgeTypeGitHub {
		t.Fatalf("forge_type=%q", gh.ForgeType)
	}

	byForge, err := st.GetInstanceByForgeAndURL(ctx, models.ForgeTypeGitea, "https://git.example.com")
	if err != nil || byForge == nil || byForge.ID != inst.ID {
		t.Fatalf("by forge: %+v err=%v", byForge, err)
	}

	list, err := st.ListInstances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list len=%d", len(list))
	}
	if !store.InstanceHasSecrets(&list[0]) {
		t.Fatal("expected gitea instance to report secrets")
	}
}

func TestUpsertInstanceByURLDefaultsForgeTypeGitea(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "meta.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25.5", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if inst.ForgeType != models.ForgeTypeGitea {
		t.Fatalf("forge_type=%q", inst.ForgeType)
	}
	if store.InstanceHasSecrets(inst) {
		t.Fatal("meta upsert should not invent secrets")
	}
}
