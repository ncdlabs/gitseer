package database_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitea-lens/internal/database"
)

func TestOpenMigrates(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lens.db")
	db, err := database.Open(ctx, "sqlite", path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='repositories'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("repositories table missing")
	}
	// Second open is idempotent.
	db2, err := database.Open(ctx, "sqlite", path, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = db2.Close()
}
