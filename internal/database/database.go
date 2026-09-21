// Package database opens SQL databases and runs embedded goose migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncdlabs/gitea-lens/migrations"
	"github.com/pressly/goose/v3"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Open opens a database from driver/path/dsn and applies migrations.
func Open(ctx context.Context, driver, path, dsn string) (*sql.DB, error) {
	d := strings.ToLower(driver)
	var (
		db      *sql.DB
		err     error
		dialect string
		migFS   fs.FS
	)
	switch d {
	case "sqlite", "":
		dir := filepath.Dir(path)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create database dir: %w", err)
			}
		}
		db, err = sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
		dialect = "sqlite3"
		migFS = migrations.FS
		if err == nil {
			db.SetMaxOpenConns(1)
		}
	case "postgres", "postgresql":
		db, err = sql.Open("pgx", dsn)
		dialect = "postgres"
		migFS = migrations.PostgresFS
		if err == nil {
			db.SetMaxOpenConns(10)
			db.SetMaxIdleConns(5)
		}
	default:
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if dialect == "sqlite3" {
		if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL;`); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite pragmas: %w", err)
		}
	}
	if err := migrate(ctx, db, dialect, migFS); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB, dialect string, migFS fs.FS) error {
	root := migFS
	dir := "."
	if dialect == "postgres" {
		sub, err := fs.Sub(migFS, "postgres")
		if err != nil {
			return fmt.Errorf("postgres migrations: %w", err)
		}
		root = sub
	}
	goose.SetBaseFS(root)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect(dialect); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
