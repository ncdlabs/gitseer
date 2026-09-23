// Package store provides SQL persistence for Lens models.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/models"
)

type Store struct {
	db     *sql.DB
	driver string
}

func New(db *sql.DB) *Store { return NewWithDriver(db, "sqlite") }

func NewWithDriver(db *sql.DB, driver string) *Store {
	d := strings.ToLower(driver)
	if d == "postgresql" {
		d = "postgres"
	}
	if d == "" {
		d = "sqlite"
	}
	return &Store{db: db, driver: d}
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Driver() string { return s.driver }

func (s *Store) sql(query string) string {
	if s.driver != "postgres" {
		return query
	}
	return rebindPostgres(query)
}

func rebindPostgres(query string) string {
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.sql(query), args...)
}

func (s *Store) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.sql(query), args...)
}

func (s *Store) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.sql(query), args...)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, f := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q", s)
}

func nullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil
	}
	return &t
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type scanner interface{ Scan(dest ...any) error }

func (s *Store) UpsertInstanceByURL(ctx context.Context, name, baseURL, version, capsJSON string) (*models.Instance, error) {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO instances (name, base_url, version, capabilities_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(base_url) DO UPDATE SET
  name=excluded.name, version=excluded.version, capabilities_json=excluded.capabilities_json, updated_at=excluded.updated_at
`, name, baseURL, version, capsJSON, now, now)
	if err != nil {
		return nil, err
	}
	return s.GetInstanceByURL(ctx, baseURL)
}

func (s *Store) GetInstanceByURL(ctx context.Context, baseURL string) (*models.Instance, error) {
	row := s.queryRow(ctx, `
SELECT id, name, base_url, version, capabilities_json, sync_token_ciphertext, webhook_secret_ciphertext,
       oauth_client_id, oauth_client_secret_ciphertext, external_url, created_at, updated_at
FROM instances WHERE base_url = ?`, baseURL)
	return scanInstance(row)
}

func (s *Store) GetInstanceByID(ctx context.Context, id int64) (*models.Instance, error) {
	row := s.queryRow(ctx, `
SELECT id, name, base_url, version, capabilities_json, sync_token_ciphertext, webhook_secret_ciphertext,
       oauth_client_id, oauth_client_secret_ciphertext, external_url, created_at, updated_at
FROM instances WHERE id = ?`, id)
	return scanInstance(row)
}

func (s *Store) GetPrimaryInstance(ctx context.Context) (*models.Instance, error) {
	row := s.queryRow(ctx, `
SELECT id, name, base_url, version, capabilities_json, sync_token_ciphertext, webhook_secret_ciphertext,
       oauth_client_id, oauth_client_secret_ciphertext, external_url, created_at, updated_at
FROM instances ORDER BY id ASC LIMIT 1`)
	inst, err := scanInstance(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return inst, err
}

func scanInstance(row scanner) (*models.Instance, error) {
	var inst models.Instance
	var created, updated string
	if err := row.Scan(&inst.ID, &inst.Name, &inst.BaseURL, &inst.Version, &inst.CapabilitiesJSON,
		&inst.SyncTokenCiphertext, &inst.WebhookSecretCiphertext, &inst.OAuthClientID, &inst.OAuthClientSecretCipher,
		&inst.ExternalURL, &created, &updated); err != nil {
		return nil, err
	}
	inst.CreatedAt, _ = parseTime(created)
	inst.UpdatedAt, _ = parseTime(updated)
	return &inst, nil
}

func (s *Store) UpsertOrganization(ctx context.Context, instanceID int64, org models.Organization) (*models.Organization, error) {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO organizations (instance_id, external_id, name, full_name, avatar_url, synced_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(instance_id, external_id) DO UPDATE SET
  name=excluded.name, full_name=excluded.full_name, avatar_url=excluded.avatar_url, synced_at=excluded.synced_at
`, instanceID, org.ExternalID, org.Name, org.FullName, org.AvatarURL, now)
	if err != nil {
		return nil, err
	}
	row := s.queryRow(ctx, `SELECT id, instance_id, external_id, name, full_name, avatar_url, synced_at FROM organizations WHERE instance_id=? AND external_id=?`, instanceID, org.ExternalID)
	var o models.Organization
	var synced sql.NullString
	if err := row.Scan(&o.ID, &o.InstanceID, &o.ExternalID, &o.Name, &o.FullName, &o.AvatarURL, &synced); err != nil {
		return nil, err
	}
	o.SyncedAt = nullTime(synced)
	return &o, nil
}

func (s *Store) UpsertRepository(ctx context.Context, instanceID int64, repo models.Repository) (*models.Repository, error) {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO repositories (
  instance_id, external_id, owner, name, full_name, default_branch,
  private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
ON CONFLICT(instance_id, external_id) DO UPDATE SET
  owner=excluded.owner, name=excluded.name, full_name=excluded.full_name,
  default_branch=excluded.default_branch, private=excluded.private, archived=excluded.archived,
  empty=excluded.empty, fork=excluded.fork, html_url=excluded.html_url,
  last_synced_at=excluded.last_synced_at, deleted_at=NULL, updated_at=excluded.updated_at
`, instanceID, repo.ExternalID, repo.Owner, repo.Name, repo.FullName, repo.DefaultBranch,
		boolToInt(repo.Private), boolToInt(repo.Archived), boolToInt(repo.Empty), boolToInt(repo.Fork),
		repo.HTMLURL, now, now, now)
	if err != nil {
		return nil, err
	}
	return s.GetRepositoryByExternalID(ctx, instanceID, repo.ExternalID)
}

func (s *Store) GetRepositoryByExternalID(ctx context.Context, instanceID, externalID int64) (*models.Repository, error) {
	row := s.queryRow(ctx, `
SELECT id, instance_id, org_id, external_id, owner, name, full_name, default_branch,
       private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
FROM repositories WHERE instance_id = ? AND external_id = ?`, instanceID, externalID)
	return scanRepo(row)
}

func (s *Store) GetRepositoryByID(ctx context.Context, id int64) (*models.Repository, error) {
	row := s.queryRow(ctx, `
SELECT id, instance_id, org_id, external_id, owner, name, full_name, default_branch,
       private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
FROM repositories WHERE id = ?`, id)
	return scanRepo(row)
}

func (s *Store) GetRepositoryByOwnerName(ctx context.Context, owner, name string) (*models.Repository, error) {
	row := s.queryRow(ctx, `
SELECT id, instance_id, org_id, external_id, owner, name, full_name, default_branch,
       private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
FROM repositories WHERE owner = ? AND name = ? AND deleted_at IS NULL`, owner, name)
	return scanRepo(row)
}

func scanRepo(row scanner) (*models.Repository, error) {
	var r models.Repository
	var orgID sql.NullInt64
	var private, archived, empty, fork int
	var lastSynced, deleted, created, updated sql.NullString
	if err := row.Scan(
		&r.ID, &r.InstanceID, &orgID, &r.ExternalID, &r.Owner, &r.Name, &r.FullName, &r.DefaultBranch,
		&private, &archived, &empty, &fork, &r.HTMLURL, &lastSynced, &deleted, &created, &updated,
	); err != nil {
		return nil, err
	}
	if orgID.Valid {
		v := orgID.Int64
		r.OrgID = &v
	}
	r.Private = private != 0
	r.Archived = archived != 0
	r.Empty = empty != 0
	r.Fork = fork != 0
	r.LastSyncedAt = nullTime(lastSynced)
	r.DeletedAt = nullTime(deleted)
	r.CreatedAt, _ = parseTime(created.String)
	r.UpdatedAt, _ = parseTime(updated.String)
	return &r, nil
}

type ListRepositoriesOpts struct {
	InstanceID     int64
	UserID         int64 // when >0, join user_repository_access (unless BootstrapAllowAll)
	BootstrapAll   bool
	Query          string
	Limit          int
	Offset         int
	IncludeDeleted bool
}

func (s *Store) ListRepositories(ctx context.Context, opts ListRepositoriesOpts) ([]models.Repository, int, error) {
	if err := requireListScope(opts.UserID, opts.BootstrapAll); err != nil {
		return nil, 0, err
	}
	opts.Limit = clampLimit(opts.Limit, 50, 200)
	where := []string{"r.deleted_at IS NULL OR ? = 1"}
	args := []any{boolToInt(opts.IncludeDeleted)}
	if !opts.IncludeDeleted {
		where = []string{"r.deleted_at IS NULL"}
		args = nil
	}
	if opts.InstanceID > 0 {
		where = append(where, "r.instance_id = ?")
		args = append(args, opts.InstanceID)
	}
	join := ""
	if opts.UserID > 0 && !opts.BootstrapAll {
		join = "INNER JOIN user_repository_access ura ON ura.repo_id = r.id AND ura.user_id = ?"
		args = append([]any{opts.UserID}, args...)
	}
	if q := strings.TrimSpace(opts.Query); q != "" {
		if like := likePattern(q); like != "" {
			where = append(where, "(r.full_name LIKE ? OR r.owner LIKE ? OR r.name LIKE ?)")
			args = append(args, like, like, like)
		}
	}
	clause := strings.Join(where, " AND ")

	var total int
	countSQL := `SELECT COUNT(*) FROM repositories r ` + join + ` WHERE ` + clause
	if err := s.queryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, opts.Limit, opts.Offset)
	rows, err := s.query(ctx, `
SELECT r.id, r.instance_id, r.org_id, r.external_id, r.owner, r.name, r.full_name, r.default_branch,
       r.private, r.archived, r.empty, r.fork, r.html_url, r.last_synced_at, r.deleted_at, r.created_at, r.updated_at
FROM repositories r `+join+`
WHERE `+clause+`
ORDER BY r.full_name ASC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.Repository
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *r)
	}
	return out, total, rows.Err()
}

func (s *Store) SoftDeleteRepository(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `UPDATE repositories SET deleted_at=?, updated_at=? WHERE id=? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		return err
	}
	return s.DeleteAccessForRepo(ctx, id)
}

// SoftDeleteMissing soft-deletes repos not in seenExternalIDs whose last_synced_at
// is strictly before syncStart. Rows stamped at/after syncStart (e.g. webhook-created
// mid-reconcile) survive even when absent from the forge list page.
func (s *Store) SoftDeleteMissing(ctx context.Context, instanceID int64, seenExternalIDs []int64, syncStart time.Time) error {
	// Never wipe the catalog on an empty reconcile (API glitch / empty page).
	if len(seenExternalIDs) == 0 {
		return nil
	}
	now := formatTime(time.Now().UTC())
	start := formatTime(syncStart.UTC())
	ph := make([]string, len(seenExternalIDs))
	args := []any{now, now, instanceID, start}
	for i, id := range seenExternalIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	_, err := s.exec(ctx, fmt.Sprintf(`
UPDATE repositories SET deleted_at=?, updated_at=?
WHERE instance_id=? AND deleted_at IS NULL
  AND (last_synced_at IS NULL OR last_synced_at < ?)
  AND external_id NOT IN (%s)`, strings.Join(ph, ",")), args...)
	if err != nil {
		return err
	}
	// Clear ACL for repos soft-deleted in this pass (parity with webhook delete path).
	_, err = s.exec(ctx, fmt.Sprintf(`
DELETE FROM user_repository_access
WHERE repo_id IN (
  SELECT id FROM repositories
  WHERE instance_id=? AND deleted_at IS NOT NULL
    AND (last_synced_at IS NULL OR last_synced_at < ?)
    AND external_id NOT IN (%s)
)`, strings.Join(ph, ",")), append([]any{instanceID, start}, args[4:]...)...)
	return err
}

func (s *Store) ListAllAliveRepos(ctx context.Context, instanceID int64) ([]models.Repository, error) {
	rows, err := s.query(ctx, `
SELECT id, instance_id, org_id, external_id, owner, name, full_name, default_branch,
       private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
FROM repositories WHERE instance_id=? AND deleted_at IS NULL ORDER BY id`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Repository
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *Store) SetSyncState(ctx context.Context, instanceID int64, scope, phase, lastErr string) error {
	now := formatTime(time.Now().UTC())
	var success any
	if lastErr == "" {
		success = now
	}
	_, err := s.exec(ctx, `
INSERT INTO sync_state (instance_id, scope, scope_id, cursor_json, last_success_at, last_error, phase)
VALUES (?, ?, 0, '{}', ?, ?, ?)
ON CONFLICT(instance_id, scope, scope_id) DO UPDATE SET
  last_success_at=COALESCE(excluded.last_success_at, sync_state.last_success_at),
  last_error=excluded.last_error, phase=excluded.phase
`, instanceID, scope, success, lastErr, phase)
	return err
}

func (s *Store) TryAcquireSyncLease(ctx context.Context, holder string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	expires := formatTime(now.Add(ttl))
	nowStr := formatTime(now)
	res, err := s.exec(ctx, `
INSERT INTO sync_leases (id, holder, expires_at) VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET holder=excluded.holder, expires_at=excluded.expires_at
WHERE sync_leases.expires_at < ? OR sync_leases.holder = ?`, holder, expires, nowStr, holder)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
