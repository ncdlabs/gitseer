package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SavedFilter is a per-user named query preset (Attention / Inbox / list pages).
type SavedFilter struct {
	ID        int64           `json:"id"`
	UserID    int64           `json:"user_id"`
	Name      string          `json:"name"`
	Query     json.RawMessage `json:"query"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func (s *Store) ListSavedFilters(ctx context.Context, userID int64) ([]SavedFilter, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("user_id required")
	}
	rows, err := s.query(ctx, `
SELECT id, user_id, name, query_json, created_at, updated_at
FROM saved_filters
WHERE user_id=?
ORDER BY LOWER(name) ASC, id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedFilter
	for rows.Next() {
		f, err := scanSavedFilter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	if out == nil {
		out = []SavedFilter{}
	}
	return out, rows.Err()
}

func (s *Store) GetSavedFilter(ctx context.Context, userID, id int64) (*SavedFilter, error) {
	row := s.queryRow(ctx, `
SELECT id, user_id, name, query_json, created_at, updated_at
FROM saved_filters WHERE id=? AND user_id=?`, id, userID)
	return scanSavedFilter(row)
}

func (s *Store) InsertSavedFilter(ctx context.Context, userID int64, name string, query json.RawMessage) (*SavedFilter, error) {
	name = strings.TrimSpace(name)
	if userID <= 0 {
		return nil, fmt.Errorf("user_id required")
	}
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	if len(name) > 80 {
		return nil, fmt.Errorf("name too long")
	}
	q, err := normalizeSavedFilterQuery(query)
	if err != nil {
		return nil, err
	}
	now := formatTime(time.Now().UTC())
	var id int64
	err = s.queryRow(ctx, `
INSERT INTO saved_filters (user_id, name, query_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING id`, userID, name, string(q), now, now).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("filter name already exists")
		}
		return nil, err
	}
	return s.GetSavedFilter(ctx, userID, id)
}

func (s *Store) UpdateSavedFilter(ctx context.Context, userID, id int64, name string, query json.RawMessage) (*SavedFilter, error) {
	existing, err := s.GetSavedFilter(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = existing.Name
	}
	if len(name) > 80 {
		return nil, fmt.Errorf("name too long")
	}
	q := existing.Query
	if query != nil {
		nq, err := normalizeSavedFilterQuery(query)
		if err != nil {
			return nil, err
		}
		q = nq
	}
	now := formatTime(time.Now().UTC())
	_, err = s.exec(ctx, `
UPDATE saved_filters SET name=?, query_json=?, updated_at=?
WHERE id=? AND user_id=?`, name, string(q), now, id, userID)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("filter name already exists")
		}
		return nil, err
	}
	return s.GetSavedFilter(ctx, userID, id)
}

func (s *Store) DeleteSavedFilter(ctx context.Context, userID, id int64) error {
	res, err := s.exec(ctx, `DELETE FROM saved_filters WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func normalizeSavedFilterQuery(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("query must be JSON object")
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("query must be JSON object")
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if len(out) > 8<<10 {
		return nil, fmt.Errorf("query too large")
	}
	return out, nil
}

func scanSavedFilter(row scanner) (*SavedFilter, error) {
	var f SavedFilter
	var q string
	var created, updated sql.NullString
	if err := row.Scan(&f.ID, &f.UserID, &f.Name, &q, &created, &updated); err != nil {
		return nil, err
	}
	if q == "" {
		q = "{}"
	}
	f.Query = json.RawMessage(q)
	if t := nullTime(created); t != nil {
		f.CreatedAt = *t
	}
	if t := nullTime(updated); t != nil {
		f.UpdatedAt = *t
	}
	return &f, nil
}
