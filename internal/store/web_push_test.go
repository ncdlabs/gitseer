package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestWebPushSubscriptionAndPrefs(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "webpush.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	u, err := st.EnsureBootstrapUser(ctx)
	if err != nil {
		t.Fatal(err)
	}

	prefs, err := st.GetUserAlertPrefs(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.BrowserEnabled || prefs.PushEnabled || prefs.MinSeverity != "critical" {
		t.Fatalf("unexpected defaults: %+v", prefs)
	}

	prefs.BrowserEnabled = true
	prefs.PushEnabled = true
	prefs.MinSeverity = "warning"
	saved, err := st.UpsertUserAlertPrefs(ctx, *prefs)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.BrowserEnabled || !saved.PushEnabled || saved.MinSeverity != "warning" {
		t.Fatalf("upsert mismatch: %+v", saved)
	}

	sub, err := st.UpsertWebPushSubscription(ctx, u.ID, "https://push.example/x", "p256", "auth", "test-ua")
	if err != nil {
		t.Fatal(err)
	}
	if sub.ID == 0 || sub.Endpoint != "https://push.example/x" {
		t.Fatalf("bad sub: %+v", sub)
	}
	list, err := st.ListWebPushSubscriptions(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 sub, got %d", len(list))
	}

	if err := st.DeleteWebPushSubscriptionForUser(ctx, u.ID, sub.Endpoint); err != nil {
		t.Fatal(err)
	}
	list, err = st.ListWebPushSubscriptions(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(list))
	}
}

func TestListUserIDsForRepoAlert(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "webpush-acl.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	admin, err := st.EnsureBootstrapUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.26", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r",
	})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.ListUserIDsForRepoAlert(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range ids {
		if id == admin.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected bootstrap admin in recipients, got %v", ids)
	}
}
