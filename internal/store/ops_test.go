package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestOpsSyncLeaseWebhookStatsAndVerify(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "ops.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewWithDriver(db, "sqlite")

	inst, err := st.UpsertInstanceMeta(ctx, "gitea", "lab", "https://gitea.example", "1.25", `{"actions_api":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSyncState(ctx, inst.ID, "instance", "complete", ""); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSyncState(ctx, inst.ID, "instance")
	if err != nil || got == nil || got.Phase != "complete" {
		t.Fatalf("sync state=%+v err=%v", got, err)
	}

	ok, err := st.TryAcquireSyncLease(ctx, inst.ID, "holder-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("lease acquire ok=%v err=%v", ok, err)
	}
	lease, err := st.GetSyncLease(ctx, inst.ID)
	if err != nil || lease == nil || lease.Holder != "holder-a" {
		t.Fatalf("lease=%+v err=%v", lease, err)
	}

	id, inserted, err := st.InsertWebhookEvent(ctx, inst.ID, "d1", "workflow_run", `{"ok":true}`)
	if err != nil || !inserted {
		t.Fatalf("insert webhook inserted=%v err=%v", inserted, err)
	}
	_ = st.MarkWebhookProcessed(ctx, id, "")
	stats, err := st.WebhookStatsSince(ctx, inst.ID, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total < 1 || stats.OK < 1 {
		t.Fatalf("stats=%+v", stats)
	}

	if err := st.SetWebhookVerifyPending(ctx, inst.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	marked, err := st.MarkWebhookVerifiedIfPending(ctx, inst.ID)
	if err != nil || !marked {
		t.Fatalf("marked=%v err=%v", marked, err)
	}
	reload, err := st.GetInstanceByID(ctx, inst.ID)
	if err != nil || reload.WebhookVerifiedAt == nil {
		t.Fatalf("verified_at missing: %+v err=%v", reload, err)
	}

	n, err := st.CountWebhookEventsByTypeSince(ctx, inst.ID, time.Now().UTC().Add(-time.Hour), []string{"workflow_run"})
	if err != nil || n < 1 {
		t.Fatalf("type count=%d err=%v", n, err)
	}
}
