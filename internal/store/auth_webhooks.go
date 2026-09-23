package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/models"
)

// Reserved / collision errors for OAuth user upserts.
var (
	ErrReservedLogin   = errors.New("oauth login is reserved")
	ErrLoginConflict   = errors.New("login already linked to another account")
	ErrBootstrapClash  = errors.New("cannot link oauth user to bootstrap admin")
	ErrMissingGiteaUID = errors.New("gitea_user_id is required")
)

func (s *Store) EnsureBootstrapUser(ctx context.Context) (*models.User, error) {
	const login = "bootstrap"
	row := s.queryRow(ctx, `
SELECT id, instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at
FROM users WHERE login = ?`, login)
	u, err := scanUser(row)
	if err == nil {
		return u, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	now := formatTime(time.Now().UTC())
	var id int64
	err = s.queryRow(ctx, `
INSERT INTO users (login, display_name, is_bootstrap_admin, created_at, updated_at)
VALUES (?, 'Bootstrap Admin', 1, ?, ?)
RETURNING id`, login, now, now).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetUserByID(ctx, id)
}

func scanUser(row scanner) (*models.User, error) {
	var u models.User
	var instanceID, giteaID sql.NullInt64
	var bootstrap int
	var created, updated string
	if err := row.Scan(&u.ID, &instanceID, &giteaID, &u.Login, &u.Email, &u.DisplayName, &u.AvatarURL, &bootstrap, &created, &updated); err != nil {
		return nil, err
	}
	if instanceID.Valid {
		v := instanceID.Int64
		u.InstanceID = &v
	}
	if giteaID.Valid {
		v := giteaID.Int64
		u.GiteaUserID = &v
	}
	u.IsBootstrapAdmin = bootstrap != 0
	u.CreatedAt, _ = parseTime(created)
	u.UpdatedAt, _ = parseTime(updated)
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	row := s.queryRow(ctx, `
SELECT id, instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at
FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) UpsertGiteaUser(ctx context.Context, instanceID *int64, u models.User) (*models.User, error) {
	if u.GiteaUserID == nil || *u.GiteaUserID == 0 {
		return nil, ErrMissingGiteaUID
	}
	login := strings.TrimSpace(u.Login)
	if login == "" {
		return nil, fmt.Errorf("login is required")
	}
	if strings.EqualFold(login, "bootstrap") {
		return nil, ErrReservedLogin
	}

	// Never allow OAuth to attach to the local bootstrap admin row (login collision).
	if existing, err := s.getUserByLogin(ctx, login); err == nil && existing.IsBootstrapAdmin {
		return nil, ErrBootstrapClash
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := formatTime(time.Now().UTC())
	if byUID, err := s.getUserByGiteaUID(ctx, instanceID, *u.GiteaUserID); err == nil {
		if byUID.Login != login {
			if other, oerr := s.getUserByLogin(ctx, login); oerr == nil && other.ID != byUID.ID {
				return nil, ErrLoginConflict
			} else if oerr != nil && !errors.Is(oerr, sql.ErrNoRows) {
				return nil, oerr
			}
		}
		_, err := s.exec(ctx, `
UPDATE users SET instance_id=?, login=?, email=?, display_name=?, avatar_url=?, updated_at=?
WHERE id=?`, nullInt64(instanceID), login, u.Email, u.DisplayName, u.AvatarURL, now, byUID.ID)
		if err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, byUID.ID)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if existing, err := s.getUserByLogin(ctx, login); err == nil {
		// Login taken by a non-bootstrap row with a different Gitea id.
		if existing.GiteaUserID != nil && *existing.GiteaUserID != *u.GiteaUserID {
			return nil, ErrLoginConflict
		}
		if existing.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	_, err := s.exec(ctx, `
INSERT INTO users (instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
`, nullInt64(instanceID), *u.GiteaUserID, login, u.Email, u.DisplayName, u.AvatarURL, now, now)
	if err != nil {
		return nil, err
	}
	return s.getUserByGiteaUID(ctx, instanceID, *u.GiteaUserID)
}

func (s *Store) getUserByLogin(ctx context.Context, login string) (*models.User, error) {
	row := s.queryRow(ctx, `
SELECT id, instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at
FROM users WHERE login = ?`, login)
	return scanUser(row)
}

func (s *Store) getUserByGiteaUID(ctx context.Context, instanceID *int64, giteaUID int64) (*models.User, error) {
	var row *sql.Row
	if instanceID == nil {
		row = s.queryRow(ctx, `
SELECT id, instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at
FROM users WHERE instance_id IS NULL AND gitea_user_id = ?`, giteaUID)
	} else {
		row = s.queryRow(ctx, `
SELECT id, instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at
FROM users WHERE instance_id = ? AND gitea_user_id = ?`, *instanceID, giteaUID)
	}
	return scanUser(row)
}

func (s *Store) CreateSession(ctx context.Context, id, tokenHash string, userID int64, expires time.Time, ip, ua string) error {
	_, err := s.exec(ctx, `
INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at, ip, user_agent)
VALUES (?, ?, ?, ?, ?, ?, ?)`, id, userID, tokenHash, formatTime(expires), formatTime(time.Now().UTC()), ip, ua)
	return err
}

func (s *Store) GetSessionByTokenHash(ctx context.Context, hash string) (*models.Session, error) {
	row := s.queryRow(ctx, `
SELECT id, user_id, expires_at, created_at, ip, user_agent
FROM sessions WHERE token_hash = ?`, hash)
	var sess models.Session
	var expires, created string
	if err := row.Scan(&sess.ID, &sess.UserID, &expires, &created, &sess.IP, &sess.UserAgent); err != nil {
		return nil, err
	}
	sess.ExpiresAt, _ = parseTime(expires)
	sess.CreatedAt, _ = parseTime(created)
	if time.Now().UTC().After(sess.ExpiresAt) {
		return nil, sql.ErrNoRows
	}
	return &sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) SaveOAuthState(ctx context.Context, state, verifier, redirectTo string, expires time.Time) error {
	_, err := s.exec(ctx, `
INSERT INTO oauth_states (state, code_verifier, redirect_to, expires_at) VALUES (?, ?, ?, ?)
ON CONFLICT(state) DO UPDATE SET code_verifier=excluded.code_verifier, redirect_to=excluded.redirect_to, expires_at=excluded.expires_at
`, state, verifier, redirectTo, formatTime(expires))
	return err
}

func (s *Store) TakeOAuthState(ctx context.Context, state string) (verifier, redirectTo string, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, s.sql(`SELECT code_verifier, redirect_to, expires_at FROM oauth_states WHERE state=?`), state)
	var expires string
	if err = row.Scan(&verifier, &redirectTo, &expires); err != nil {
		return "", "", err
	}
	res, err := tx.ExecContext(ctx, s.sql(`DELETE FROM oauth_states WHERE state=?`), state)
	if err != nil {
		return "", "", err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", "", sql.ErrNoRows
	}
	exp, _ := parseTime(expires)
	if time.Now().UTC().After(exp) {
		_ = tx.Commit()
		return "", "", sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return verifier, redirectTo, nil
}

func (s *Store) SaveUserToken(ctx context.Context, userID int64, accessCipher, refreshCipher string, expires *time.Time) error {
	_, err := s.exec(ctx, `
INSERT INTO user_tokens (user_id, access_token_ciphertext, refresh_token_ciphertext, expires_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
  access_token_ciphertext=excluded.access_token_ciphertext,
  refresh_token_ciphertext=excluded.refresh_token_ciphertext,
  expires_at=excluded.expires_at
`, userID, accessCipher, refreshCipher, formatTimePtr(expires))
	return err
}

// UserTokenRow is the encrypted OAuth token material for a Lens user.
type UserTokenRow struct {
	AccessCipher  string
	RefreshCipher string
	ExpiresAt     *time.Time
}

func (s *Store) GetUserToken(ctx context.Context, userID int64) (*UserTokenRow, error) {
	var access, refresh string
	var expires sql.NullString
	err := s.queryRow(ctx, `
SELECT access_token_ciphertext, refresh_token_ciphertext, expires_at
FROM user_tokens WHERE user_id=?`, userID).Scan(&access, &refresh, &expires)
	if err != nil {
		return nil, err
	}
	row := &UserTokenRow{AccessCipher: access, RefreshCipher: refresh}
	if expires.Valid && expires.String != "" {
		t, perr := parseTime(expires.String)
		if perr == nil {
			row.ExpiresAt = &t
		}
	}
	return row, nil
}

func (s *Store) InsertWebhookEvent(ctx context.Context, instanceID int64, deliveryID, eventType, payload string) (int64, bool, error) {
	sum := sha256.Sum256([]byte(payload))
	hash := hex.EncodeToString(sum[:])
	if deliveryID == "" {
		deliveryID = hash
	}
	var id int64
	err := s.queryRow(ctx, `
INSERT INTO webhook_events (instance_id, delivery_id, event_type, payload_hash, payload_json, status)
VALUES (?, ?, ?, ?, ?, 'pending')
ON CONFLICT(instance_id, delivery_id) DO NOTHING
RETURNING id
`, instanceID, deliveryID, eventType, hash, payload).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (s *Store) ClaimPendingWebhooks(ctx context.Context, limit int) ([]WebhookEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	selectSQL := `
SELECT id, instance_id, delivery_id, event_type, payload_json, attempts
FROM webhook_events WHERE status='pending' ORDER BY id ASC LIMIT ?`
	if s.driver == "postgres" {
		selectSQL = `
SELECT id, instance_id, delivery_id, event_type, payload_json, attempts
FROM webhook_events WHERE status='pending' ORDER BY id ASC LIMIT ?
FOR UPDATE SKIP LOCKED`
	}
	rows, err := tx.QueryContext(ctx, s.sql(selectSQL), limit)
	if err != nil {
		return nil, err
	}
	var candidates []WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		if err := rows.Scan(&e.ID, &e.InstanceID, &e.DeliveryID, &e.EventType, &e.Payload, &e.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	now := formatTime(time.Now().UTC())
	var out []WebhookEvent
	for _, e := range candidates {
		res, err := tx.ExecContext(ctx, s.sql(`
UPDATE webhook_events SET status='processing', processing_started_at=?
WHERE id=? AND status='pending'`), now, e.ID)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			out = append(out, e)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

type WebhookEvent struct {
	ID         int64
	InstanceID int64
	DeliveryID string
	EventType  string
	Payload    string
	Attempts   int
}

func (s *Store) MarkWebhookProcessed(ctx context.Context, id int64, errMsg string) error {
	now := formatTime(time.Now().UTC())
	status := "processed"
	if errMsg != "" {
		status = "error"
	}
	_, err := s.exec(ctx, `
UPDATE webhook_events SET status=?, error=?, processed_at=?, attempts=attempts+1 WHERE id=?
`, status, errMsg, now, id)
	return err
}

// DefaultWebhookReaperAge is how long a claim may stay in processing before reset.
const DefaultWebhookReaperAge = 5 * time.Minute

// ReapStaleProcessingWebhooks resets stuck processing rows back to pending.
// Age is measured from processing_started_at (claim time), not received_at, so
// backlogged events are not reaped while still being applied.
func (s *Store) ReapStaleProcessingWebhooks(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		olderThan = DefaultWebhookReaperAge
	}
	cutoff := formatTime(time.Now().UTC().Add(-olderThan))
	res, err := s.exec(ctx, `
UPDATE webhook_events SET status='pending', processing_started_at=NULL
WHERE status='processing'
  AND processing_started_at IS NOT NULL
  AND processing_started_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
