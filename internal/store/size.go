package store

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// DatabaseSizeInfo describes on-disk / estimated database size for ops guardrails.
type DatabaseSizeInfo struct {
	Bytes    int64  `json:"bytes"`
	Driver   string `json:"driver"`
	Method   string `json:"method"` // file | pages | pg_database_size
	Path     string `json:"path,omitempty"`
	Estimate bool   `json:"estimate"`
}

// DatabaseSize returns SQLite file size (path + WAL/SHM when present) or a Postgres size estimate.
// sqlitePath is used only for the sqlite driver; pass cfg.Database.Path.
func (s *Store) DatabaseSize(ctx context.Context, sqlitePath string) (DatabaseSizeInfo, error) {
	info := DatabaseSizeInfo{Driver: s.driver}
	var err error
	switch s.driver {
	case "postgres":
		var n int64
		if s.pg != nil {
			n, err = s.pg.GetPostgresDatabaseSize(ctx)
		} else {
			err = s.queryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&n)
		}
		if err != nil {
			return info, err
		}
		info.Bytes = n
		info.Method = "pg_database_size"
		info.Estimate = true
		return info, nil
	default:
		path := strings.TrimSpace(sqlitePath)
		if path != "" {
			total, err := sqliteFileBytes(path)
			if err == nil {
				info.Bytes = total
				info.Method = "file"
				info.Path = path
				return info, nil
			}
		}
		var pages, pageSize int64
		if err := s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages); err != nil {
			return info, fmt.Errorf("sqlite page_count: %w", err)
		}
		if err := s.db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&pageSize); err != nil {
			return info, fmt.Errorf("sqlite page_size: %w", err)
		}
		info.Bytes = pages * pageSize
		info.Method = "pages"
		info.Path = path
		info.Estimate = true
		return info, nil
	}
}

func sqliteFileBytes(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	total := st.Size()
	for _, suffix := range []string{"-wal", "-shm"} {
		ws, err := os.Stat(path + suffix)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		total += ws.Size()
	}
	return total, nil
}
