package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/models"
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
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO users (instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
ON CONFLICT(login) DO UPDATE SET
  instance_id=excluded.instance_id, gitea_user_id=excluded.gitea_user_id, email=excluded.email,
  display_name=excluded.display_name, avatar_url=excluded.avatar_url, updated_at=excluded.updated_at
`, nullInt64(instanceID), nullInt64(u.GiteaUserID), u.Login, u.Email, u.DisplayName, u.AvatarURL, now, now)
	if err != nil {
		return nil, err
	}
	row := s.queryRow(ctx, `
SELECT id, instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at
FROM users WHERE login = ?`, u.Login)
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

func (s *Store) GetUserAccessTokenCipher(ctx context.Context, userID int64) (string, error) {
	var cipher string
	err := s.queryRow(ctx, `SELECT access_token_ciphertext FROM user_tokens WHERE user_id=?`, userID).Scan(&cipher)
	return cipher, err
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

	rows, err := tx.QueryContext(ctx, s.sql(`
SELECT id, instance_id, delivery_id, event_type, payload_json, attempts
FROM webhook_events WHERE status='pending' ORDER BY id ASC LIMIT ?`), limit)
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

	var out []WebhookEvent
	for _, e := range candidates {
		res, err := tx.ExecContext(ctx, s.sql(`
UPDATE webhook_events SET status='processing' WHERE id=? AND status='pending'`), e.ID)
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
func (s *Store) ReapStaleProcessingWebhooks(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		olderThan = DefaultWebhookReaperAge
	}
	cutoff := formatTime(time.Now().UTC().Add(-olderThan))
	res, err := s.exec(ctx, `
UPDATE webhook_events SET status='pending'
WHERE status='processing' AND received_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
