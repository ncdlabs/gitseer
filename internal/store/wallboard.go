package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// WallboardToken is a read-only public access token metadata (never includes plaintext).
type WallboardToken struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	TokenPrefix     string     `json:"token_prefix"`
	CreatedByUserID *int64     `json:"created_by_user_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
}

// CreateWallboardToken inserts a new token; returns the row and plaintext token (shown once).
func (s *Store) CreateWallboardToken(ctx context.Context, name string, createdBy int64) (*WallboardToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("name required")
	}
	if len(name) > 80 {
		return nil, "", fmt.Errorf("name too long")
	}
	raw, err := randomToken(32)
	if err != nil {
		return nil, "", err
	}
	prefix := raw
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	hash := hashWallboardToken(raw)
	now := formatTime(time.Now().UTC())
	var id int64
	var createdByArg any
	if createdBy > 0 {
		createdByArg = createdBy
	}
	err = s.queryRow(ctx, `
INSERT INTO wallboard_tokens (name, token_hash, token_prefix, created_by_user_id, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING id`, name, hash, prefix, createdByArg, now).Scan(&id)
	if err != nil {
		return nil, "", err
	}
	tok, err := s.GetWallboardToken(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return tok, raw, nil
}

// ListWallboardTokens returns non-revoked tokens (includeRevoked lists all).
func (s *Store) ListWallboardTokens(ctx context.Context, includeRevoked bool) ([]WallboardToken, error) {
	q := `
SELECT id, name, token_prefix, created_by_user_id, created_at, last_used_at, revoked_at
FROM wallboard_tokens`
	if !includeRevoked {
		q += ` WHERE revoked_at IS NULL`
	}
	q += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WallboardToken
	for rows.Next() {
		t, err := scanWallboardToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	if out == nil {
		out = []WallboardToken{}
	}
	return out, rows.Err()
}

func (s *Store) GetWallboardToken(ctx context.Context, id int64) (*WallboardToken, error) {
	return s.getWallboardTokenSQLC(ctx, id)
}

// LookupWallboardTokenByPlain validates a plaintext token and touches last_used_at.
func (s *Store) LookupWallboardTokenByPlain(ctx context.Context, plain string) (*WallboardToken, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return nil, sql.ErrNoRows
	}
	hash := hashWallboardToken(plain)
	row := s.queryRow(ctx, `
SELECT id, name, token_prefix, created_by_user_id, created_at, last_used_at, revoked_at
FROM wallboard_tokens WHERE token_hash=? AND revoked_at IS NULL`, hash)
	tok, err := scanWallboardToken(row)
	if err != nil {
		return nil, err
	}
	now := formatTime(time.Now().UTC())
	_, _ = s.exec(ctx, `UPDATE wallboard_tokens SET last_used_at=? WHERE id=?`, now, tok.ID)
	return tok, nil
}

func (s *Store) RevokeWallboardToken(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	res, err := s.exec(ctx, `
UPDATE wallboard_tokens SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanWallboardToken(row scanner) (*WallboardToken, error) {
	var t WallboardToken
	var createdBy sql.NullInt64
	var created, lastUsed, revoked sql.NullString
	if err := row.Scan(&t.ID, &t.Name, &t.TokenPrefix, &createdBy, &created, &lastUsed, &revoked); err != nil {
		return nil, err
	}
	if createdBy.Valid {
		v := createdBy.Int64
		t.CreatedByUserID = &v
	}
	if tm := nullTime(created); tm != nil {
		t.CreatedAt = *tm
	}
	t.LastUsedAt = nullTime(lastUsed)
	t.RevokedAt = nullTime(revoked)
	return &t, nil
}

func hashWallboardToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
