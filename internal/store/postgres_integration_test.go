//go:build postgres

package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/store"
)

// Run with: GITSEER_TEST_POSTGRES_DSN='postgres://…' go test -tags postgres ./internal/store/ -run Postgres
func TestPostgresLeaseAndWebhookClaim(t *testing.T) {
	dsn := os.Getenv("GITSEER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GITSEER_TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, "postgres", "", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()
	st := store.NewWithDriver(db, "postgres")

	inst, err := st.UpsertInstanceMeta(ctx, "gitea", "pg-lab", "https://pg-gitea.example/"+t.Name(), "1.25", "{}")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := st.TryAcquireSyncLease(ctx, inst.ID, "pg-holder", time.Minute)
	if err != nil || !ok {
		t.Fatalf("lease ok=%v err=%v", ok, err)
	}
	lease, err := st.GetSyncLease(ctx, inst.ID)
	if err != nil || lease == nil || lease.Holder != "pg-holder" {
		t.Fatalf("lease=%+v err=%v", lease, err)
	}
	_, inserted, err := st.InsertWebhookEvent(ctx, inst.ID, "pg-d1", "ping", `{}`)
	if err != nil || !inserted {
		t.Fatalf("insert inserted=%v err=%v", inserted, err)
	}
	claimed, err := st.ClaimPendingWebhooks(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) == 0 {
		t.Fatal("expected claimed webhook with FOR UPDATE SKIP LOCKED path")
	}
}
