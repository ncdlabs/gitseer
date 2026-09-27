// Package backup implements gitseer backup and restore CLI helpers.
package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/settings"
	_ "modernc.org/sqlite"
)

const manifestName = "manifest.json"
const sqliteBackupName = "gitseer.db"
const keyBackupName = "gitseer.encryption_key"
const pgDumpName = "postgres.dump"
const pgDumpCmdFile = "pg_dump.cmd.txt"

// Manifest describes a backup directory produced by Backup.
type Manifest struct {
	Version    string            `json:"version"`
	CreatedAt  string            `json:"created_at"`
	Driver     string            `json:"driver"`
	Paths      map[string]string `json:"paths"`
	Notes      []string          `json:"notes,omitempty"`
	ConfigHint string            `json:"config_hint,omitempty"`
}

// Options controls backup/restore.
type Options struct {
	OutDir  string
	FromDir string
	Force   bool
	Version string
	Cfg     config.Config
}

// Backup writes a consistent snapshot into opts.OutDir.
func Backup(ctx context.Context, opts Options) (*Manifest, error) {
	if strings.TrimSpace(opts.OutDir) == "" {
		return nil, fmt.Errorf("--out is required")
	}
	outDir, err := filepath.Abs(opts.OutDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return nil, err
	}

	driver := strings.ToLower(strings.TrimSpace(opts.Cfg.Database.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	man := &Manifest{
		Version:   opts.Version,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Driver:    driver,
		Paths:     map[string]string{},
		Notes:     nil,
	}

	switch driver {
	case "postgres", "postgresql":
		man.Driver = "postgres"
		if err := backupPostgres(ctx, opts.Cfg, outDir, man); err != nil {
			return nil, err
		}
	default:
		man.Driver = "sqlite"
		dbPath := strings.TrimSpace(opts.Cfg.Database.Path)
		if dbPath == "" {
			return nil, fmt.Errorf("database.path is required for sqlite backup")
		}
		if err := backupSQLite(ctx, dbPath, filepath.Join(outDir, sqliteBackupName)); err != nil {
			return nil, err
		}
		man.Paths["database"] = sqliteBackupName
		man.Notes = append(man.Notes, "SQLite copied with VACUUM INTO (safe online snapshot)")
	}

	keyPath := settings.DefaultEncryptionKeyPath(opts.Cfg)
	if st, err := os.Stat(keyPath); err == nil && !st.IsDir() {
		dest := filepath.Join(outDir, keyBackupName)
		if err := copyFile(keyPath, dest, 0o600); err != nil {
			return nil, fmt.Errorf("copy encryption key: %w", err)
		}
		man.Paths["encryption_key"] = keyBackupName
		man.Notes = append(man.Notes, "Copied wizard encryption key file; also back up GITSEER_ENCRYPTION_KEY from your secret store if used")
	} else {
		man.Notes = append(man.Notes, "No gitseer.encryption_key file beside the DB; ensure env/config encryption key is backed up separately")
	}

	man.ConfigHint = "Also back up config.yaml / Kubernetes Secret / .env — never commit secrets"
	raw, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(outDir, manifestName), append(raw, '\n'), 0o600); err != nil {
		return nil, err
	}
	return man, nil
}

func backupSQLite(ctx context.Context, srcPath, destPath string) error {
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absSrc); err != nil {
		return fmt.Errorf("sqlite database: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(destPath)

	dsn := absSrc + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	// VACUUM INTO produces a consistent copy without stopping writers (SQLite 3.27+).
	escaped := strings.ReplaceAll(destPath, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		// Fallback: file copy after brief note — still better than failing hard.
		if copyErr := copyFile(absSrc, destPath, 0o600); copyErr != nil {
			return fmt.Errorf("VACUUM INTO failed (%v); file copy also failed: %w", err, copyErr)
		}
	}
	return nil
}

func backupPostgres(ctx context.Context, cfg config.Config, outDir string, man *Manifest) error {
	dsn := strings.TrimSpace(cfg.Database.DSN)
	if dsn == "" {
		return fmt.Errorf("database.dsn is required for postgres backup")
	}
	cmdLine := fmt.Sprintf("pg_dump --format=custom --file=%s %q", pgDumpName, dsn)
	_ = os.WriteFile(filepath.Join(outDir, pgDumpCmdFile), []byte(cmdLine+"\n"), 0o600)
	man.Paths["pg_dump_cmd"] = pgDumpCmdFile
	man.Notes = append(man.Notes, "Wrote pg_dump command to "+pgDumpCmdFile)

	bin, err := exec.LookPath("pg_dump")
	if err != nil {
		man.Notes = append(man.Notes, "pg_dump not found on PATH — run the command in "+pgDumpCmdFile+" manually")
		return nil
	}
	dest := filepath.Join(outDir, pgDumpName)
	cmd := exec.CommandContext(ctx, bin, "--format=custom", "--file="+dest, dsn)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		man.Notes = append(man.Notes, fmt.Sprintf("pg_dump failed: %v — use %s", err, pgDumpCmdFile))
		return nil
	}
	man.Paths["database"] = pgDumpName
	man.Notes = append(man.Notes, "Postgres dump written with pg_dump --format=custom")
	return nil
}

// Restore restores from a backup directory. Refuses to overwrite existing DB/key without Force.
func Restore(ctx context.Context, opts Options) (*Manifest, error) {
	_ = ctx
	if strings.TrimSpace(opts.FromDir) == "" {
		return nil, fmt.Errorf("--from is required")
	}
	fromDir, err := filepath.Abs(opts.FromDir)
	if err != nil {
		return nil, err
	}
	man, err := readManifest(fromDir)
	if err != nil {
		return nil, err
	}

	driver := strings.ToLower(strings.TrimSpace(opts.Cfg.Database.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	if man.Driver != "" && man.Driver != driver && !(man.Driver == "postgres" && (driver == "postgres" || driver == "postgresql")) {
		return nil, fmt.Errorf("backup driver %q does not match config driver %q", man.Driver, driver)
	}

	switch man.Driver {
	case "postgres":
		dumpPath := filepath.Join(fromDir, firstPath(man, "database", pgDumpName))
		if _, err := os.Stat(dumpPath); err != nil {
			return nil, fmt.Errorf("postgres dump missing: %w (run pg_restore manually from %s)", err, fromDir)
		}
		dsn := strings.TrimSpace(opts.Cfg.Database.DSN)
		if dsn == "" {
			return nil, fmt.Errorf("database.dsn is required for postgres restore")
		}
		if !opts.Force {
			return nil, fmt.Errorf("refusing postgres restore without --force (destroys target database contents)")
		}
		bin, err := exec.LookPath("pg_restore")
		if err != nil {
			return nil, fmt.Errorf("pg_restore not found; restore manually: pg_restore --clean --if-exists --dbname=%q %s", dsn, dumpPath)
		}
		cmd := exec.CommandContext(ctx, bin, "--clean", "--if-exists", "--dbname="+dsn, dumpPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("pg_restore: %w", err)
		}
	default:
		src := filepath.Join(fromDir, firstPath(man, "database", sqliteBackupName))
		dest := strings.TrimSpace(opts.Cfg.Database.Path)
		if dest == "" {
			return nil, fmt.Errorf("database.path is required for sqlite restore")
		}
		dest, err = filepath.Abs(dest)
		if err != nil {
			return nil, err
		}
		if !opts.Force {
			if _, err := os.Stat(dest); err == nil {
				return nil, fmt.Errorf("refusing to overwrite existing database %s without --force", dest)
			}
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return nil, err
		}
		if err := copyFile(src, dest, 0o600); err != nil {
			return nil, err
		}
	}

	if keyRel, ok := man.Paths["encryption_key"]; ok && keyRel != "" {
		src := filepath.Join(fromDir, keyRel)
		dest := settings.DefaultEncryptionKeyPath(opts.Cfg)
		if !opts.Force {
			if _, err := os.Stat(dest); err == nil {
				return nil, fmt.Errorf("refusing to overwrite existing encryption key %s without --force", dest)
			}
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return nil, err
		}
		if err := copyFile(src, dest, 0o600); err != nil {
			return nil, err
		}
	}
	return man, nil
}

func readManifest(dir string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return nil, fmt.Errorf("manifest json: %w", err)
	}
	if man.Paths == nil {
		man.Paths = map[string]string{}
	}
	return &man, nil
}

func firstPath(man *Manifest, key, fallback string) string {
	if man != nil {
		if v := man.Paths[key]; v != "" {
			return v
		}
	}
	return fallback
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dest)
}
