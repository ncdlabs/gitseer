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

	"github.com/ncdlabs/gitseer/internal/models"
)

// Reserved / collision errors for OAuth user upserts.
var (
	ErrReservedLogin    = errors.New("oauth login is reserved")
	ErrLoginConflict    = errors.New("login already linked to another account")
	ErrBootstrapClash   = errors.New("cannot link oauth user to bootstrap admin")
	ErrMissingGiteaUID     = errors.New("gitea_user_id is required")
	ErrMissingGitHubUID    = errors.New("github_user_id is required")
	ErrMissingGitLabUID    = errors.New("gitlab_user_id is required")
	ErrMissingBitbucketUID = errors.New("bitbucket_user_id is required")
	ErrIdentityLinked      = errors.New("forge identity already linked to another account")
)

const userSelectCols = ` id, instance_id, gitea_user_id, github_user_id, github_instance_id, gitlab_user_id, gitlab_instance_id, bitbucket_user_id, bitbucket_instance_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at`

func (s *Store) EnsureBootstrapUser(ctx context.Context) (*models.User, error) {
	const login = "bootstrap"
	row := s.queryRow(ctx, `SELECT`+userSelectCols+` FROM users WHERE login = ?`, login)
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
	var instanceID, giteaID, githubID, githubInstID, gitlabID, gitlabInstID, bbID, bbInstID sql.NullInt64
	var bootstrap int
	var created, updated string
	if err := row.Scan(
		&u.ID, &instanceID, &giteaID, &githubID, &githubInstID,
		&gitlabID, &gitlabInstID, &bbID, &bbInstID,
		&u.Login, &u.Email, &u.DisplayName, &u.AvatarURL, &bootstrap, &created, &updated,
	); err != nil {
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
	if githubID.Valid {
		v := githubID.Int64
		u.GitHubUserID = &v
	}
	if githubInstID.Valid {
		v := githubInstID.Int64
		u.GitHubInstanceID = &v
	}
	if gitlabID.Valid {
		v := gitlabID.Int64
		u.GitLabUserID = &v
	}
	if gitlabInstID.Valid {
		v := gitlabInstID.Int64
		u.GitLabInstanceID = &v
	}
	if bbID.Valid {
		v := bbID.Int64
		u.BitbucketUserID = &v
	}
	if bbInstID.Valid {
		v := bbInstID.Int64
		u.BitbucketInstanceID = &v
	}
	u.IsBootstrapAdmin = bootstrap != 0
	u.CreatedAt, _ = parseTime(created)
	u.UpdatedAt, _ = parseTime(updated)
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	return s.getUserSQLCByID(ctx, id)
}

// UpsertGiteaUser creates or updates a user keyed by (instance_id, gitea_user_id).
// Forgejo OAuth reuses these columns (no separate forgejo_* identity). When linkUserID
// is non-nil, attaches the identity to that existing account (no orphan inserts).
func (s *Store) UpsertGiteaUser(ctx context.Context, instanceID *int64, u models.User, linkUserID *int64) (*models.User, error) {
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
	giteaUID := *u.GiteaUserID
	if byUID, err := s.getUserByGiteaUID(ctx, instanceID, giteaUID); err == nil {
		if linkUserID != nil && *linkUserID != byUID.ID {
			return nil, ErrIdentityLinked
		}
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

	if linkUserID != nil {
		target, err := s.GetUserByID(ctx, *linkUserID)
		if err != nil {
			return nil, err
		}
		if target.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
		if target.GiteaUserID != nil && (*target.GiteaUserID != giteaUID ||
			(instanceID == nil && target.InstanceID != nil) ||
			(instanceID != nil && (target.InstanceID == nil || *target.InstanceID != *instanceID))) {
			return nil, ErrIdentityLinked
		}
		_, err = s.exec(ctx, `
UPDATE users SET instance_id=?, gitea_user_id=?, email=COALESCE(NULLIF(?, ''), email),
  display_name=COALESCE(NULLIF(?, ''), display_name), avatar_url=COALESCE(NULLIF(?, ''), avatar_url), updated_at=?
WHERE id=?`, nullInt64(instanceID), giteaUID, u.Email, u.DisplayName, u.AvatarURL, now, target.ID)
		if err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, target.ID)
	}

	// Re-bind orphaned users whose instance was deleted (instance_id SET NULL).
	if instanceID != nil {
		if orphan, err := s.getUserByGiteaUID(ctx, nil, giteaUID); err == nil {
			if orphan.Login != login {
				if other, oerr := s.getUserByLogin(ctx, login); oerr == nil && other.ID != orphan.ID {
					return nil, ErrLoginConflict
				} else if oerr != nil && !errors.Is(oerr, sql.ErrNoRows) {
					return nil, oerr
				}
			}
			_, err := s.exec(ctx, `
UPDATE users SET instance_id=?, login=?, email=?, display_name=?, avatar_url=?, updated_at=?
WHERE id=?`, nullInt64(instanceID), login, u.Email, u.DisplayName, u.AvatarURL, now, orphan.ID)
			if err != nil {
				return nil, err
			}
			return s.GetUserByID(ctx, orphan.ID)
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if existing, err := s.getUserByLogin(ctx, login); err == nil {
			if existing.InstanceID == nil && existing.GiteaUserID != nil && *existing.GiteaUserID == giteaUID && !existing.IsBootstrapAdmin {
				_, err := s.exec(ctx, `
UPDATE users SET instance_id=?, email=?, display_name=?, avatar_url=?, updated_at=?
WHERE id=?`, nullInt64(instanceID), u.Email, u.DisplayName, u.AvatarURL, now, existing.ID)
				if err != nil {
					return nil, err
				}
				return s.GetUserByID(ctx, existing.ID)
			}
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if existing, err := s.getUserByLogin(ctx, login); err == nil {
		// Login taken by a non-bootstrap row with a different Gitea id.
		if existing.GiteaUserID != nil && *existing.GiteaUserID != giteaUID {
			return nil, ErrLoginConflict
		}
		if existing.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
		// Same login already used by another forge identity — require explicit link while signed in.
		if existing.GitHubUserID != nil || existing.GitLabUserID != nil || existing.BitbucketUserID != nil {
			if existing.GiteaUserID == nil {
				return nil, ErrLoginConflict
			}
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	_, err := s.exec(ctx, `
INSERT INTO users (instance_id, gitea_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
`, nullInt64(instanceID), giteaUID, login, u.Email, u.DisplayName, u.AvatarURL, now, now)
	if err != nil {
		return nil, err
	}
	return s.getUserByGiteaUID(ctx, instanceID, giteaUID)
}

func (s *Store) getUserByLogin(ctx context.Context, login string) (*models.User, error) {
	row := s.queryRow(ctx, `SELECT`+userSelectCols+` FROM users WHERE login = ?`, login)
	return scanUser(row)
}

func (s *Store) getUserByGiteaUID(ctx context.Context, instanceID *int64, giteaUID int64) (*models.User, error) {
	var row *sql.Row
	if instanceID == nil {
		row = s.queryRow(ctx, `SELECT`+userSelectCols+`
FROM users WHERE instance_id IS NULL AND gitea_user_id = ?`, giteaUID)
	} else {
		row = s.queryRow(ctx, `SELECT`+userSelectCols+`
FROM users WHERE instance_id = ? AND gitea_user_id = ?`, *instanceID, giteaUID)
	}
	return scanUser(row)
}

func (s *Store) getUserByGitHubUID(ctx context.Context, githubInstanceID int64, githubUID int64) (*models.User, error) {
	row := s.queryRow(ctx, `SELECT`+userSelectCols+`
FROM users WHERE github_instance_id = ? AND github_user_id = ?`, githubInstanceID, githubUID)
	return scanUser(row)
}

// UpsertGitHubUser creates or updates a user keyed by (github_instance_id, github_user_id).
// When linkUserID is non-nil, attaches the GitHub identity to that existing account.
func (s *Store) UpsertGitHubUser(ctx context.Context, githubInstanceID int64, u models.User, linkUserID *int64) (*models.User, error) {
	if u.GitHubUserID == nil || *u.GitHubUserID == 0 {
		return nil, ErrMissingGitHubUID
	}
	if githubInstanceID <= 0 {
		return nil, fmt.Errorf("github instance_id is required")
	}
	login := strings.TrimSpace(u.Login)
	if login == "" {
		return nil, fmt.Errorf("login is required")
	}
	if strings.EqualFold(login, "bootstrap") {
		return nil, ErrReservedLogin
	}
	now := formatTime(time.Now().UTC())
	ghUID := *u.GitHubUserID

	if byUID, err := s.getUserByGitHubUID(ctx, githubInstanceID, ghUID); err == nil {
		if linkUserID != nil && *linkUserID != byUID.ID {
			return nil, ErrIdentityLinked
		}
		if byUID.Login != login {
			if other, oerr := s.getUserByLogin(ctx, login); oerr == nil && other.ID != byUID.ID {
				// Keep existing login when GitHub login is taken by another row.
				login = byUID.Login
			} else if oerr != nil && !errors.Is(oerr, sql.ErrNoRows) {
				return nil, oerr
			}
		}
		_, err := s.exec(ctx, `
UPDATE users SET github_instance_id=?, github_user_id=?, login=?, email=?, display_name=?, avatar_url=?, updated_at=?
WHERE id=?`, githubInstanceID, ghUID, login, u.Email, u.DisplayName, u.AvatarURL, now, byUID.ID)
		if err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, byUID.ID)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if linkUserID != nil {
		target, err := s.GetUserByID(ctx, *linkUserID)
		if err != nil {
			return nil, err
		}
		if target.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
		if target.GitHubUserID != nil && (*target.GitHubUserID != ghUID || target.GitHubInstanceID == nil || *target.GitHubInstanceID != githubInstanceID) {
			return nil, ErrIdentityLinked
		}
		_, err = s.exec(ctx, `
UPDATE users SET github_instance_id=?, github_user_id=?, email=COALESCE(NULLIF(?, ''), email),
  display_name=COALESCE(NULLIF(?, ''), display_name), avatar_url=COALESCE(NULLIF(?, ''), avatar_url), updated_at=?
WHERE id=?`, githubInstanceID, ghUID, u.Email, u.DisplayName, u.AvatarURL, now, target.ID)
		if err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, target.ID)
	}

	if existing, err := s.getUserByLogin(ctx, login); err == nil {
		if existing.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
		if existing.GitHubUserID != nil && *existing.GitHubUserID != ghUID {
			return nil, ErrLoginConflict
		}
		if existing.GiteaUserID != nil && existing.GitHubUserID == nil {
			// Same login already used by a Gitea-only account — require explicit link while signed in.
			return nil, ErrLoginConflict
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	_, err := s.exec(ctx, `
INSERT INTO users (github_instance_id, github_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
`, githubInstanceID, ghUID, login, u.Email, u.DisplayName, u.AvatarURL, now, now)
	if err != nil {
		return nil, err
	}
	return s.getUserByGitHubUID(ctx, githubInstanceID, ghUID)
}

func (s *Store) getUserByGitLabUID(ctx context.Context, gitlabInstanceID, gitlabUID int64) (*models.User, error) {
	row := s.queryRow(ctx, `SELECT`+userSelectCols+`
FROM users WHERE gitlab_instance_id = ? AND gitlab_user_id = ?`, gitlabInstanceID, gitlabUID)
	return scanUser(row)
}

func (s *Store) getUserByBitbucketUID(ctx context.Context, bitbucketInstanceID, bitbucketUID int64) (*models.User, error) {
	row := s.queryRow(ctx, `SELECT`+userSelectCols+`
FROM users WHERE bitbucket_instance_id = ? AND bitbucket_user_id = ?`, bitbucketInstanceID, bitbucketUID)
	return scanUser(row)
}

// UpsertGitLabUser creates or updates a user keyed by (gitlab_instance_id, gitlab_user_id).
func (s *Store) UpsertGitLabUser(ctx context.Context, gitlabInstanceID int64, u models.User, linkUserID *int64) (*models.User, error) {
	if u.GitLabUserID == nil || *u.GitLabUserID == 0 {
		return nil, ErrMissingGitLabUID
	}
	if gitlabInstanceID <= 0 {
		return nil, fmt.Errorf("gitlab instance_id is required")
	}
	login := strings.TrimSpace(u.Login)
	if login == "" {
		return nil, fmt.Errorf("login is required")
	}
	if strings.EqualFold(login, "bootstrap") {
		return nil, ErrReservedLogin
	}
	now := formatTime(time.Now().UTC())
	uid := *u.GitLabUserID

	if byUID, err := s.getUserByGitLabUID(ctx, gitlabInstanceID, uid); err == nil {
		if linkUserID != nil && *linkUserID != byUID.ID {
			return nil, ErrIdentityLinked
		}
		if byUID.Login != login {
			if other, oerr := s.getUserByLogin(ctx, login); oerr == nil && other.ID != byUID.ID {
				login = byUID.Login
			} else if oerr != nil && !errors.Is(oerr, sql.ErrNoRows) {
				return nil, oerr
			}
		}
		_, err := s.exec(ctx, `
UPDATE users SET gitlab_instance_id=?, gitlab_user_id=?, login=?, email=?, display_name=?, avatar_url=?, updated_at=?
WHERE id=?`, gitlabInstanceID, uid, login, u.Email, u.DisplayName, u.AvatarURL, now, byUID.ID)
		if err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, byUID.ID)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if linkUserID != nil {
		target, err := s.GetUserByID(ctx, *linkUserID)
		if err != nil {
			return nil, err
		}
		if target.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
		if target.GitLabUserID != nil && (*target.GitLabUserID != uid || target.GitLabInstanceID == nil || *target.GitLabInstanceID != gitlabInstanceID) {
			return nil, ErrIdentityLinked
		}
		_, err = s.exec(ctx, `
UPDATE users SET gitlab_instance_id=?, gitlab_user_id=?, email=COALESCE(NULLIF(?, ''), email),
  display_name=COALESCE(NULLIF(?, ''), display_name), avatar_url=COALESCE(NULLIF(?, ''), avatar_url), updated_at=?
WHERE id=?`, gitlabInstanceID, uid, u.Email, u.DisplayName, u.AvatarURL, now, target.ID)
		if err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, target.ID)
	}

	if existing, err := s.getUserByLogin(ctx, login); err == nil {
		if existing.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
		if existing.GitLabUserID != nil && *existing.GitLabUserID != uid {
			return nil, ErrLoginConflict
		}
		if (existing.GiteaUserID != nil || existing.GitHubUserID != nil) && existing.GitLabUserID == nil {
			return nil, ErrLoginConflict
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	_, err := s.exec(ctx, `
INSERT INTO users (gitlab_instance_id, gitlab_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
`, gitlabInstanceID, uid, login, u.Email, u.DisplayName, u.AvatarURL, now, now)
	if err != nil {
		return nil, err
	}
	return s.getUserByGitLabUID(ctx, gitlabInstanceID, uid)
}

// UpsertBitbucketUser creates or updates a user keyed by (bitbucket_instance_id, bitbucket_user_id).
func (s *Store) UpsertBitbucketUser(ctx context.Context, bitbucketInstanceID int64, u models.User, linkUserID *int64) (*models.User, error) {
	if u.BitbucketUserID == nil || *u.BitbucketUserID == 0 {
		return nil, ErrMissingBitbucketUID
	}
	if bitbucketInstanceID <= 0 {
		return nil, fmt.Errorf("bitbucket instance_id is required")
	}
	login := strings.TrimSpace(u.Login)
	if login == "" {
		return nil, fmt.Errorf("login is required")
	}
	if strings.EqualFold(login, "bootstrap") {
		return nil, ErrReservedLogin
	}
	now := formatTime(time.Now().UTC())
	uid := *u.BitbucketUserID

	if byUID, err := s.getUserByBitbucketUID(ctx, bitbucketInstanceID, uid); err == nil {
		if linkUserID != nil && *linkUserID != byUID.ID {
			return nil, ErrIdentityLinked
		}
		if byUID.Login != login {
			if other, oerr := s.getUserByLogin(ctx, login); oerr == nil && other.ID != byUID.ID {
				login = byUID.Login
			} else if oerr != nil && !errors.Is(oerr, sql.ErrNoRows) {
				return nil, oerr
			}
		}
		_, err := s.exec(ctx, `
UPDATE users SET bitbucket_instance_id=?, bitbucket_user_id=?, login=?, email=?, display_name=?, avatar_url=?, updated_at=?
WHERE id=?`, bitbucketInstanceID, uid, login, u.Email, u.DisplayName, u.AvatarURL, now, byUID.ID)
		if err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, byUID.ID)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if linkUserID != nil {
		target, err := s.GetUserByID(ctx, *linkUserID)
		if err != nil {
			return nil, err
		}
		if target.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
		if target.BitbucketUserID != nil && (*target.BitbucketUserID != uid || target.BitbucketInstanceID == nil || *target.BitbucketInstanceID != bitbucketInstanceID) {
			return nil, ErrIdentityLinked
		}
		_, err = s.exec(ctx, `
UPDATE users SET bitbucket_instance_id=?, bitbucket_user_id=?, email=COALESCE(NULLIF(?, ''), email),
  display_name=COALESCE(NULLIF(?, ''), display_name), avatar_url=COALESCE(NULLIF(?, ''), avatar_url), updated_at=?
WHERE id=?`, bitbucketInstanceID, uid, u.Email, u.DisplayName, u.AvatarURL, now, target.ID)
		if err != nil {
			return nil, err
		}
		return s.GetUserByID(ctx, target.ID)
	}

	if existing, err := s.getUserByLogin(ctx, login); err == nil {
		if existing.IsBootstrapAdmin {
			return nil, ErrBootstrapClash
		}
		if existing.BitbucketUserID != nil && *existing.BitbucketUserID != uid {
			return nil, ErrLoginConflict
		}
		if (existing.GiteaUserID != nil || existing.GitHubUserID != nil || existing.GitLabUserID != nil) && existing.BitbucketUserID == nil {
			return nil, ErrLoginConflict
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	_, err := s.exec(ctx, `
INSERT INTO users (bitbucket_instance_id, bitbucket_user_id, login, email, display_name, avatar_url, is_bootstrap_admin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
`, bitbucketInstanceID, uid, login, u.Email, u.DisplayName, u.AvatarURL, now, now)
	if err != nil {
		return nil, err
	}
	return s.getUserByBitbucketUID(ctx, bitbucketInstanceID, uid)
}

// ListUsers returns all users ordered by id (bootstrap-admin ACL grant UI).
func (s *Store) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.query(ctx, `SELECT`+userSelectCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// ListUserRepoIDsForInstance returns ACL repo ids for a user scoped to one forge instance.
func (s *Store) ListUserRepoIDsForInstance(ctx context.Context, userID, instanceID int64) ([]int64, error) {
	rows, err := s.query(ctx, `
SELECT ura.repo_id FROM user_repository_access ura
INNER JOIN repositories r ON r.id = ura.repo_id
WHERE ura.user_id=? AND r.instance_id=? AND r.deleted_at IS NULL
ORDER BY ura.repo_id`, userID, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) CreateSession(ctx context.Context, id, tokenHash string, userID int64, expires time.Time, ip, ua string) error {
	return s.createSessionSQLC(ctx, id, tokenHash, userID, expires, ip, ua)
}

func (s *Store) GetSessionByTokenHash(ctx context.Context, hash string) (*models.Session, error) {
	return s.getSessionSQLC(ctx, hash)
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.deleteSessionSQLC(ctx, id)
}

// OAuthStateMeta carries provider/instance/link metadata for an OAuth PKCE state.
type OAuthStateMeta struct {
	Provider   string
	InstanceID *int64
	LinkUserID *int64
}

func (s *Store) SaveOAuthState(ctx context.Context, state, verifier, redirectTo string, expires time.Time, meta OAuthStateMeta) error {
	provider := strings.TrimSpace(meta.Provider)
	if provider == "" {
		provider = "gitea"
	}
	_, err := s.exec(ctx, `
INSERT INTO oauth_states (state, code_verifier, redirect_to, expires_at, provider, instance_id, link_user_id)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(state) DO UPDATE SET
  code_verifier=excluded.code_verifier,
  redirect_to=excluded.redirect_to,
  expires_at=excluded.expires_at,
  provider=excluded.provider,
  instance_id=excluded.instance_id,
  link_user_id=excluded.link_user_id
`, state, verifier, redirectTo, formatTime(expires), provider, nullInt64(meta.InstanceID), nullInt64(meta.LinkUserID))
	return err
}

func (s *Store) TakeOAuthState(ctx context.Context, state string) (verifier, redirectTo string, meta OAuthStateMeta, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", meta, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, s.sql(`
SELECT code_verifier, redirect_to, expires_at, COALESCE(provider, 'gitea'), instance_id, link_user_id
FROM oauth_states WHERE state=?`), state)
	var expires string
	var instanceID, linkUserID sql.NullInt64
	if err = row.Scan(&verifier, &redirectTo, &expires, &meta.Provider, &instanceID, &linkUserID); err != nil {
		return "", "", meta, err
	}
	if instanceID.Valid {
		v := instanceID.Int64
		meta.InstanceID = &v
	}
	if linkUserID.Valid {
		v := linkUserID.Int64
		meta.LinkUserID = &v
	}
	res, err := tx.ExecContext(ctx, s.sql(`DELETE FROM oauth_states WHERE state=?`), state)
	if err != nil {
		return "", "", meta, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", "", meta, sql.ErrNoRows
	}
	exp, _ := parseTime(expires)
	if time.Now().UTC().After(exp) {
		_ = tx.Commit()
		return "", "", meta, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return "", "", meta, err
	}
	return verifier, redirectTo, meta, nil
}

func (s *Store) SaveUserToken(ctx context.Context, userID, instanceID int64, accessCipher, refreshCipher string, expires *time.Time) error {
	if instanceID <= 0 {
		return fmt.Errorf("instance_id is required for user tokens")
	}
	_, err := s.exec(ctx, `
INSERT INTO user_tokens (user_id, instance_id, access_token_ciphertext, refresh_token_ciphertext, expires_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(user_id, instance_id) DO UPDATE SET
  access_token_ciphertext=excluded.access_token_ciphertext,
  refresh_token_ciphertext=excluded.refresh_token_ciphertext,
  expires_at=excluded.expires_at
`, userID, instanceID, accessCipher, refreshCipher, formatTimePtr(expires))
	return err
}

// UserTokenRow is the encrypted OAuth token material for a GitSeer user on one forge instance.
type UserTokenRow struct {
	UserID        int64
	InstanceID    int64
	AccessCipher  string
	RefreshCipher string
	ExpiresAt     *time.Time
}

func (s *Store) GetUserToken(ctx context.Context, userID, instanceID int64) (*UserTokenRow, error) {
	var access, refresh string
	var expires sql.NullString
	err := s.queryRow(ctx, `
SELECT access_token_ciphertext, refresh_token_ciphertext, expires_at
FROM user_tokens WHERE user_id=? AND instance_id=?`, userID, instanceID).Scan(&access, &refresh, &expires)
	if err != nil {
		return nil, err
	}
	row := &UserTokenRow{UserID: userID, InstanceID: instanceID, AccessCipher: access, RefreshCipher: refresh}
	if expires.Valid && expires.String != "" {
		t, perr := parseTime(expires.String)
		if perr == nil {
			row.ExpiresAt = &t
		}
	}
	return row, nil
}

// GetAnyUserToken returns any stored token for the user (preferring the given instance when set).
func (s *Store) GetAnyUserToken(ctx context.Context, userID int64, preferInstanceID int64) (*UserTokenRow, error) {
	if preferInstanceID > 0 {
		row, err := s.GetUserToken(ctx, userID, preferInstanceID)
		if err == nil {
			return row, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	var instanceID int64
	var access, refresh string
	var expires sql.NullString
	err := s.queryRow(ctx, `
SELECT instance_id, access_token_ciphertext, refresh_token_ciphertext, expires_at
FROM user_tokens WHERE user_id=? AND access_token_ciphertext != ''
ORDER BY instance_id ASC LIMIT 1`, userID).Scan(&instanceID, &access, &refresh, &expires)
	if err != nil {
		return nil, err
	}
	row := &UserTokenRow{UserID: userID, InstanceID: instanceID, AccessCipher: access, RefreshCipher: refresh}
	if expires.Valid && expires.String != "" {
		t, perr := parseTime(expires.String)
		if perr == nil {
			row.ExpiresAt = &t
		}
	}
	return row, nil
}

// ListUserTokenInstanceIDs returns forge instance ids that have a stored token for the user.
func (s *Store) ListUserTokenInstanceIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.query(ctx, `
SELECT instance_id FROM user_tokens
WHERE user_id=? AND access_token_ciphertext != ''
ORDER BY instance_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
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

	// Retry failed applies with exponential-ish backoff until MaxWebhookAttempts.
	retryBefore := formatTime(time.Now().UTC().Add(-30 * time.Second))
	selectSQL := `
SELECT id, instance_id, delivery_id, event_type, payload_json, attempts
FROM webhook_events
WHERE status='pending'
   OR (status='error' AND attempts < ? AND processed_at IS NOT NULL AND processed_at < ?)
ORDER BY id ASC LIMIT ?`
	if s.driver == "postgres" {
		selectSQL = `
SELECT id, instance_id, delivery_id, event_type, payload_json, attempts
FROM webhook_events
WHERE status='pending'
   OR (status='error' AND attempts < ? AND processed_at IS NOT NULL AND processed_at < ?)
ORDER BY id ASC LIMIT ?
FOR UPDATE SKIP LOCKED`
	}
	rows, err := tx.QueryContext(ctx, s.sql(selectSQL), MaxWebhookAttempts, retryBefore, limit)
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
UPDATE webhook_events SET status='processing', processing_started_at=?, error=''
WHERE id=? AND (status='pending' OR status='error')`), now, e.ID)
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

// MaxWebhookAttempts caps apply retries for status=error rows.
const MaxWebhookAttempts = 5

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
