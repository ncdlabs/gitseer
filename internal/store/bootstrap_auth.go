package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	postgressqlc "github.com/ncdlabs/gitseer/internal/store/sqlc/postgres"
	sqlitesqlc "github.com/ncdlabs/gitseer/internal/store/sqlc/sqlite"
)

// BootstrapAuthState is the claimed bootstrap password / policy stored in app_settings.
type BootstrapAuthState struct {
	Username       string
	PasswordHash   string
	KeepAfterSetup bool
}

// GetBootstrapAuthState reads bootstrap claim columns from app_settings.
func (s *Store) GetBootstrapAuthState(ctx context.Context) (*BootstrapAuthState, error) {
	var username, hash string
	var keep int
	err := s.queryRow(ctx, `
SELECT COALESCE(bootstrap_username, ''), COALESCE(bootstrap_password_hash, ''),
       COALESCE(bootstrap_keep_after_setup, 1)
FROM app_settings WHERE id = 1`).Scan(&username, &hash, &keep)
	if err == sql.ErrNoRows {
		return &BootstrapAuthState{KeepAfterSetup: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &BootstrapAuthState{
		Username:       username,
		PasswordHash:   hash,
		KeepAfterSetup: keep != 0,
	}, nil
}

// SetBootstrapAuthClaim writes username + password hash (claim or password reset).
// keepAfterSetup is only updated when updateKeep is true.
func (s *Store) SetBootstrapAuthClaim(ctx context.Context, username, passwordHash string, keepAfterSetup bool, updateKeep bool) error {
	username = strings.TrimSpace(username)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	keep := 1
	if !keepAfterSetup {
		keep = 0
	}
	if updateKeep {
		_, err := s.exec(ctx, `
INSERT INTO app_settings (id, bootstrap_username, bootstrap_password_hash, bootstrap_keep_after_setup, updated_at)
VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  bootstrap_username=excluded.bootstrap_username,
  bootstrap_password_hash=excluded.bootstrap_password_hash,
  bootstrap_keep_after_setup=excluded.bootstrap_keep_after_setup,
  updated_at=excluded.updated_at
`, username, passwordHash, keep, now)
		return err
	}
	_, err := s.exec(ctx, `
INSERT INTO app_settings (id, bootstrap_username, bootstrap_password_hash, updated_at)
VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  bootstrap_username=excluded.bootstrap_username,
  bootstrap_password_hash=excluded.bootstrap_password_hash,
  updated_at=excluded.updated_at
`, username, passwordHash, now)
	return err
}

// SetBootstrapKeepAfterSetup updates only the keep-after-setup policy flag.
func (s *Store) SetBootstrapKeepAfterSetup(ctx context.Context, keep bool) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	v := 1
	if !keep {
		v = 0
	}
	_, err := s.exec(ctx, `
INSERT INTO app_settings (id, bootstrap_keep_after_setup, updated_at)
VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  bootstrap_keep_after_setup=excluded.bootstrap_keep_after_setup,
  updated_at=excluded.updated_at
`, v, now)
	return err
}

// SetBootstrapUsernameSeed sets the suggested username before claim (install / config seed).
func (s *Store) SetBootstrapUsernameSeed(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.exec(ctx, `
INSERT INTO app_settings (id, bootstrap_username, updated_at)
VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  bootstrap_username=CASE
    WHEN COALESCE(app_settings.bootstrap_password_hash, '') = '' THEN excluded.bootstrap_username
    ELSE app_settings.bootstrap_username
  END,
  updated_at=excluded.updated_at
`, username, now)
	return err
}

// SetSessionBootstrapElevatedUntil sets or clears the session elevation expiry.
func (s *Store) SetSessionBootstrapElevatedUntil(ctx context.Context, sessionID string, until *time.Time) error {
	var untilStr *string
	if until != nil {
		formatted := formatTime(*until)
		untilStr = &formatted
	}
	if s.driver == "postgres" {
		return s.pg.SetSessionBootstrapElevatedUntil(ctx, postgressqlc.SetSessionBootstrapElevatedUntilParams{
			BootstrapElevatedUntil: untilStr,
			ID:                     sessionID,
		})
	}
	return s.sqlite.SetSessionBootstrapElevatedUntil(ctx, sqlitesqlc.SetSessionBootstrapElevatedUntilParams{
		BootstrapElevatedUntil: untilStr,
		ID:                     sessionID,
	})
}
