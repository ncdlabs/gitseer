package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/database"
)

func TestDatabaseSizeSQLiteFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "size.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := NewWithDriver(db, "sqlite")

	if _, err := db.ExecContext(ctx, `CREATE TABLE t (id INTEGER PRIMARY KEY, blob BLOB)`); err != nil {
		t.Fatal(err)
	}
	blob := make([]byte, 64*1024)
	for i := range blob {
		blob[i] = byte(i)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO t (blob) VALUES (?)`, blob); err != nil {
		t.Fatal(err)
	}

	info, err := st.DatabaseSize(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Driver != "sqlite" {
		t.Fatalf("driver=%s", info.Driver)
	}
	if info.Method != "file" {
		t.Fatalf("method=%s", info.Method)
	}
	if info.Bytes < 1024 {
		t.Fatalf("bytes too small: %d", info.Bytes)
	}
	stMain, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Bytes < stMain.Size() {
		t.Fatalf("reported %d < main file %d", info.Bytes, stMain.Size())
	}
}

func TestDatabaseSizeSQLitePagesFallback(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "pages.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := NewWithDriver(db, "sqlite")

	info, err := st.DatabaseSize(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if info.Method != "pages" {
		t.Fatalf("method=%s want pages", info.Method)
	}
	if info.Bytes <= 0 {
		t.Fatalf("bytes=%d", info.Bytes)
	}
	if !info.Estimate {
		t.Fatal("pages fallback should be marked estimate")
	}
}
