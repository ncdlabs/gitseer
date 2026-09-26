package store_test

import (
	"context"
	"database/sql"
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

func TestGetPrimaryGiteaInstancePrefersOAuth(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "primary.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	first, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name: "First Gitea", ForgeType: models.ForgeTypeGitea, BaseURL: "https://git-a.example.com",
		SyncTokenCiphertext: "tok-a", WebhookSecretCiphertext: "hook-a",
		AllowUnsignedWebhooks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	oauth, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name: "OAuth Gitea", ForgeType: models.ForgeTypeGitea, BaseURL: "https://git-b.example.com",
		SyncTokenCiphertext: "tok-b", WebhookSecretCiphertext: "hook-b",
		OAuthClientID: "cid", OAuthClientSecretCipher: "csec",
		AllowUnsignedWebhooks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name: "GitHub", ForgeType: models.ForgeTypeGitHub, BaseURL: "https://api.github.com",
		SyncTokenCiphertext: "gh", WebhookSecretCiphertext: "gh-hook",
		AllowUnsignedWebhooks: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	prim, err := st.GetPrimaryGiteaInstance(ctx)
	if err != nil || prim == nil {
		t.Fatalf("primary: %+v err=%v", prim, err)
	}
	if prim.ID != oauth.ID {
		t.Fatalf("expected oauth instance %d, got %d (first=%d)", oauth.ID, prim.ID, first.ID)
	}

	if err := st.DeleteInstance(ctx, oauth.ID); err != nil {
		t.Fatal(err)
	}
	prim2, err := st.GetPrimaryGiteaInstance(ctx)
	if err != nil || prim2 == nil || prim2.ID != first.ID {
		t.Fatalf("fallback primary: %+v err=%v", prim2, err)
	}
}

func TestUpdateInstanceSecretsByIDAndDelete(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "upd.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name: "Lab", ForgeType: models.ForgeTypeGitea, BaseURL: "https://git.example.com",
		SyncTokenCiphertext: "tok", WebhookSecretCiphertext: "hook",
		AllowUnsignedWebhooks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := st.UpdateInstanceSecretsByID(ctx, inst.ID, store.InstanceSecrets{
		Name: "Lab Renamed", BaseURL: "https://git2.example.com",
		AllowUnsignedWebhooks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Lab Renamed" || updated.BaseURL != "https://git2.example.com" {
		t.Fatalf("updated=%+v", updated)
	}
	if updated.SyncTokenCiphertext != "tok" {
		t.Fatal("token should be preserved")
	}
	if err := st.DeleteInstance(ctx, inst.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetInstanceByID(ctx, inst.ID); err != sql.ErrNoRows {
		t.Fatalf("expected gone, got %v", err)
	}
}

func TestUpsertInstanceSecretsRejectsForgeTypeClobber(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "clobber.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	_, err = st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name: "Gitea", ForgeType: models.ForgeTypeGitea, BaseURL: "https://git.example.com",
		WebhookSecretCiphertext: "hook", AllowUnsignedWebhooks: true, SetFlags: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name: "GitHub", ForgeType: models.ForgeTypeGitHub, BaseURL: "https://git.example.com",
		WebhookSecretCiphertext: "gh-hook", AllowUnsignedWebhooks: true, SetFlags: true,
	})
	if err == nil {
		t.Fatal("expected forge type conflict")
	}
}

func TestGetRepositoryByOwnerNameAmbiguous(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "ambig.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	gitea, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name: "Gitea", ForgeType: models.ForgeTypeGitea, BaseURL: "https://git.example.com",
		WebhookSecretCiphertext: "h", AllowUnsignedWebhooks: true, SetFlags: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	gh, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name: "GitHub", ForgeType: models.ForgeTypeGitHub, BaseURL: "https://api.github.com",
		WebhookSecretCiphertext: "h", AllowUnsignedWebhooks: true, SetFlags: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertRepository(ctx, gitea.ID, models.Repository{
		ExternalID: 1, Owner: "acme", Name: "widgets", FullName: "acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertRepository(ctx, gh.ID, models.Repository{
		ExternalID: 2, Owner: "acme", Name: "widgets", FullName: "acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.GetRepositoryByOwnerName(ctx, "acme", "widgets")
	if err == nil {
		t.Fatal("expected ambiguous")
	}
	repo, err := st.GetRepositoryByOwnerNameInInstance(ctx, "acme", "widgets", gitea.ID)
	if err != nil || repo.InstanceID != gitea.ID {
		t.Fatalf("scoped = %+v err=%v", repo, err)
	}
}
