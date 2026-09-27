package backup_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/backup"
	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/database"
	_ "modernc.org/sqlite"
)

func TestBackupRestoreSQLiteRoundTrip(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "data", "gitseer.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE backup_marker (v TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO backup_marker (v) VALUES ('round-trip')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	keyPath := filepath.Join(tmp, "data", "gitseer.encryption_key")
	if err := os.WriteFile(keyPath, []byte("twenty-four-char-key-ok!!"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{}
	cfg.Database.Driver = "sqlite"
	cfg.Database.Path = dbPath

	outDir := filepath.Join(tmp, "backup-out")
	man, err := backup.Backup(ctx, backup.Options{OutDir: outDir, Version: "test", Cfg: cfg})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if man.Driver != "sqlite" {
		t.Fatalf("driver=%s", man.Driver)
	}
	if man.Paths["database"] == "" {
		t.Fatal("expected database path in manifest")
	}
	if man.Paths["encryption_key"] == "" {
		t.Fatal("expected encryption_key in manifest")
	}

	restoreDB := filepath.Join(tmp, "restored", "gitseer.db")
	restoreCfg := config.Config{}
	restoreCfg.Database.Driver = "sqlite"
	restoreCfg.Database.Path = restoreDB
	if _, err := backup.Restore(ctx, backup.Options{FromDir: outDir, Cfg: restoreCfg}); err != nil {
		t.Fatalf("restore without force into empty path: %v", err)
	}

	rdb, err := sql.Open("sqlite", restoreDB)
	if err != nil {
		t.Fatal(err)
	}
	defer rdb.Close()
	var v string
	if err := rdb.QueryRowContext(ctx, `SELECT v FROM backup_marker`).Scan(&v); err != nil {
		t.Fatalf("marker: %v", err)
	}
	if v != "round-trip" {
		t.Fatalf("marker=%q", v)
	}

	if _, err := backup.Restore(ctx, backup.Options{FromDir: outDir, Cfg: restoreCfg, Force: false}); err == nil {
		t.Fatal("expected refuse overwrite without force")
	}
	if _, err := backup.Restore(ctx, backup.Options{FromDir: outDir, Cfg: restoreCfg, Force: true}); err != nil {
		t.Fatalf("force restore: %v", err)
	}
}
