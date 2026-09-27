//go:generate sqlc generate -f ../../sqlc.yaml

// Package store provides SQL persistence for GitSeer models.
// Critical-path queries are sqlc-generated (see sqlc.yaml + internal/store/sqlc/);
// dynamic/list/upsert SQL remains hand-written in this package.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
	postgressqlc "github.com/ncdlabs/gitseer/internal/store/sqlc/postgres"
	sqlitesqlc "github.com/ncdlabs/gitseer/internal/store/sqlc/sqlite"
)

type Store struct {
	db     *sql.DB
	driver string
	sqlite *sqlitesqlc.Queries
	pg     *postgressqlc.Queries
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
	s := &Store{db: db, driver: d}
	s.initSQLC()
	return s
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

// isUniqueViolation reports whether err is a UNIQUE constraint failure (SQLite or Postgres).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique violation") ||
		strings.Contains(msg, "duplicate key")
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

const instanceSelectCols = `
id, name, forge_type, base_url, version, capabilities_json,
sync_token_ciphertext, webhook_secret_ciphertext,
oauth_client_id, oauth_client_secret_ciphertext, external_url,
allow_private_network, allow_unsigned_webhooks,
webhook_verified_at, webhook_ensure_at, webhook_ensure_error, webhook_verify_token,
created_at, updated_at`

// UpsertInstanceByURL inserts or updates instance metadata keyed by base_url.
// Existing forge_type and credential columns are preserved on conflict; new rows default to gitea.
func (s *Store) UpsertInstanceByURL(ctx context.Context, name, baseURL, version, capsJSON string) (*models.Instance, error) {
	return s.UpsertInstanceMeta(ctx, models.ForgeTypeGitea, name, baseURL, version, capsJSON)
}

// UpsertInstanceMeta inserts or updates instance metadata for a forge type + base URL.
// Credential ciphertext and allow_* flags are preserved on conflict.
func (s *Store) UpsertInstanceMeta(ctx context.Context, forgeType, name, baseURL, version, capsJSON string) (*models.Instance, error) {
	ft := normalizeForgeType(forgeType)
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO instances (name, forge_type, base_url, version, capabilities_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(base_url) DO UPDATE SET
  name=excluded.name,
  forge_type=excluded.forge_type,
  version=excluded.version,
  capabilities_json=excluded.capabilities_json,
  updated_at=excluded.updated_at
`, name, ft, baseURL, version, capsJSON, now, now)
	if err != nil {
		return nil, err
	}
	return s.GetInstanceByURL(ctx, baseURL)
}

func (s *Store) GetInstanceByURL(ctx context.Context, baseURL string) (*models.Instance, error) {
	return s.getInstanceSQLCByURL(ctx, baseURL)
}

func (s *Store) GetInstanceByID(ctx context.Context, id int64) (*models.Instance, error) {
	return s.getInstanceSQLCByID(ctx, id)
}

// GetInstanceByForgeAndURL returns the instance for forge_type + base_url, or nil if missing.
func (s *Store) GetInstanceByForgeAndURL(ctx context.Context, forgeType, baseURL string) (*models.Instance, error) {
	ft := normalizeForgeType(forgeType)
	if s.driver == "postgres" {
		r, err := s.pg.GetInstanceByForgeAndURL(ctx, postgressqlc.GetInstanceByForgeAndURLParams{ForgeType: ft, BaseUrl: baseURL})
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
			r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
			r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
			r.CreatedAt, r.UpdatedAt), nil
	}
	r, err := s.sqlite.GetInstanceByForgeAndURL(ctx, sqlitesqlc.GetInstanceByForgeAndURLParams{ForgeType: ft, BaseUrl: baseURL})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
		r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
		r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
		r.CreatedAt, r.UpdatedAt), nil
}

func (s *Store) GetPrimaryInstance(ctx context.Context) (*models.Instance, error) {
	if s.driver == "postgres" {
		r, err := s.pg.GetPrimaryInstance(ctx)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
			r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
			r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
			r.CreatedAt, r.UpdatedAt), nil
	}
	r, err := s.sqlite.GetPrimaryInstance(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
		r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
		r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
		r.CreatedAt, r.UpdatedAt), nil
}

// GetPrimaryGiteaInstance returns the Gitea instance preferred for OAuth and legacy
// webhook fallback: OAuth client id+secret configured first, else lowest id.
func (s *Store) GetPrimaryGiteaInstance(ctx context.Context) (*models.Instance, error) {
	row := s.queryRow(ctx, `SELECT`+instanceSelectCols+` FROM instances
WHERE forge_type = ?
  AND TRIM(COALESCE(oauth_client_id, '')) != ''
  AND TRIM(COALESCE(oauth_client_secret_ciphertext, '')) != ''
ORDER BY id ASC LIMIT 1`, models.ForgeTypeGitea)
	inst, err := scanInstance(row)
	if err == nil {
		return inst, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	row = s.queryRow(ctx, `SELECT`+instanceSelectCols+` FROM instances
WHERE forge_type = ? OR forge_type = '' OR forge_type IS NULL
ORDER BY id ASC LIMIT 1`, models.ForgeTypeGitea)
	inst, err = scanInstance(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return inst, err
}

// GetPrimaryGitHubInstance returns the GitHub instance preferred for OAuth:
// OAuth client id+secret configured first, else lowest id.
func (s *Store) GetPrimaryGitHubInstance(ctx context.Context) (*models.Instance, error) {
	row := s.queryRow(ctx, `SELECT`+instanceSelectCols+` FROM instances
WHERE forge_type = ?
  AND TRIM(COALESCE(oauth_client_id, '')) != ''
  AND TRIM(COALESCE(oauth_client_secret_ciphertext, '')) != ''
ORDER BY id ASC LIMIT 1`, models.ForgeTypeGitHub)
	inst, err := scanInstance(row)
	if err == nil {
		return inst, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	row = s.queryRow(ctx, `SELECT`+instanceSelectCols+` FROM instances
WHERE forge_type = ?
ORDER BY id ASC LIMIT 1`, models.ForgeTypeGitHub)
	inst, err = scanInstance(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return inst, err
}
// GetPrimaryInstanceForForge returns the instance preferred for OAuth for forgeType:
// OAuth client id+secret configured first, else lowest id.
func (s *Store) GetPrimaryInstanceForForge(ctx context.Context, forgeType string) (*models.Instance, error) {
	forgeType = strings.TrimSpace(strings.ToLower(forgeType))
	if forgeType == "" {
		forgeType = models.ForgeTypeGitea
	}
	row := s.queryRow(ctx, `SELECT`+instanceSelectCols+` FROM instances
WHERE forge_type = ?
  AND TRIM(COALESCE(oauth_client_id, '')) != ''
  AND TRIM(COALESCE(oauth_client_secret_ciphertext, '')) != ''
ORDER BY id ASC LIMIT 1`, forgeType)
	inst, err := scanInstance(row)
	if err == nil {
		return inst, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	row = s.queryRow(ctx, `SELECT`+instanceSelectCols+` FROM instances
WHERE forge_type = ?
ORDER BY id ASC LIMIT 1`, forgeType)
	inst, err = scanInstance(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return inst, err
}

func (s *Store) GetPrimaryGitLabInstance(ctx context.Context) (*models.Instance, error) {
	return s.GetPrimaryInstanceForForge(ctx, models.ForgeTypeGitLab)
}

func (s *Store) GetPrimaryBitbucketInstance(ctx context.Context) (*models.Instance, error) {
	return s.GetPrimaryInstanceForForge(ctx, models.ForgeTypeBitbucket)
}

func (s *Store) GetPrimaryForgejoInstance(ctx context.Context) (*models.Instance, error) {
	return s.GetPrimaryInstanceForForge(ctx, models.ForgeTypeForgejo)
}


func scanInstance(row scanner) (*models.Instance, error) {
	var inst models.Instance
	var created, updated string
	var allowPrivate, allowUnsigned int
	var verifiedAt, ensureAt, ensureErr, verifyToken sql.NullString
	if err := row.Scan(
		&inst.ID, &inst.Name, &inst.ForgeType, &inst.BaseURL, &inst.Version, &inst.CapabilitiesJSON,
		&inst.SyncTokenCiphertext, &inst.WebhookSecretCiphertext, &inst.OAuthClientID, &inst.OAuthClientSecretCipher,
		&inst.ExternalURL, &allowPrivate, &allowUnsigned,
		&verifiedAt, &ensureAt, &ensureErr, &verifyToken,
		&created, &updated,
	); err != nil {
		return nil, err
	}
	inst.AllowPrivateNetwork = allowPrivate != 0
	inst.AllowUnsignedWebhooks = allowUnsigned != 0
	if verifiedAt.Valid && verifiedAt.String != "" {
		if t, err := parseTime(verifiedAt.String); err == nil {
			inst.WebhookVerifiedAt = &t
		}
	}
	if ensureAt.Valid && ensureAt.String != "" {
		if t, err := parseTime(ensureAt.String); err == nil {
			inst.WebhookEnsureAt = &t
		}
	}
	if ensureErr.Valid {
		inst.WebhookEnsureError = ensureErr.String
	}
	if verifyToken.Valid {
		inst.WebhookVerifyToken = verifyToken.String
	}
	inst.CreatedAt, _ = parseTime(created)
	inst.UpdatedAt, _ = parseTime(updated)
	if inst.ForgeType == "" {
		inst.ForgeType = models.ForgeTypeGitea
	}
	return &inst, nil
}

func normalizeForgeType(ft string) string {
	ft = strings.ToLower(strings.TrimSpace(ft))
	if models.IsSupportedForgeType(ft) {
		return ft
	}
	return models.ForgeTypeGitea
}

// parseListForgeType returns a forge_type filter value, or empty when unset/invalid (no filter).
func parseListForgeType(ft string) string {
	ft = strings.ToLower(strings.TrimSpace(ft))
	if models.IsSupportedForgeType(ft) {
		return ft
	}
	return ""
}

func (s *Store) UpsertOrganization(ctx context.Context, instanceID int64, org models.Organization) (*models.Organization, error) {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO organizations (instance_id, external_id, node_id, name, full_name, avatar_url, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(instance_id, external_id) DO UPDATE SET
  node_id=CASE WHEN excluded.node_id != '' THEN excluded.node_id ELSE organizations.node_id END,
  name=excluded.name, full_name=excluded.full_name, avatar_url=excluded.avatar_url, synced_at=excluded.synced_at
`, instanceID, org.ExternalID, org.NodeID, org.Name, org.FullName, org.AvatarURL, now)
	if err != nil {
		return nil, err
	}
	row := s.queryRow(ctx, `SELECT id, instance_id, external_id, node_id, name, full_name, avatar_url, synced_at FROM organizations WHERE instance_id=? AND external_id=?`, instanceID, org.ExternalID)
	var o models.Organization
	var synced sql.NullString
	if err := row.Scan(&o.ID, &o.InstanceID, &o.ExternalID, &o.NodeID, &o.Name, &o.FullName, &o.AvatarURL, &synced); err != nil {
		return nil, err
	}
	o.SyncedAt = nullTime(synced)
	return &o, nil
}

func (s *Store) UpsertRepository(ctx context.Context, instanceID int64, repo models.Repository) (*models.Repository, error) {
	now := formatTime(time.Now().UTC())
	_, err := s.exec(ctx, `
INSERT INTO repositories (
  instance_id, external_id, node_id, owner, name, full_name, default_branch,
  private, archived, empty, fork, html_url, last_synced_at, deleted_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
ON CONFLICT(instance_id, external_id) DO UPDATE SET
  node_id=CASE WHEN excluded.node_id != '' THEN excluded.node_id ELSE repositories.node_id END,
  owner=excluded.owner, name=excluded.name, full_name=excluded.full_name,
  default_branch=excluded.default_branch, private=excluded.private, archived=excluded.archived,
  empty=excluded.empty, fork=excluded.fork, html_url=excluded.html_url,
  last_synced_at=excluded.last_synced_at, deleted_at=NULL, updated_at=excluded.updated_at
`, instanceID, repo.ExternalID, repo.NodeID, repo.Owner, repo.Name, repo.FullName, repo.DefaultBranch,
		boolToInt(repo.Private), boolToInt(repo.Archived), boolToInt(repo.Empty), boolToInt(repo.Fork),
		repo.HTMLURL, now, now, now)
	if err != nil {
		return nil, err
	}
	return s.GetRepositoryByExternalID(ctx, instanceID, repo.ExternalID)
}

func (s *Store) GetRepositoryByExternalID(ctx context.Context, instanceID, externalID int64) (*models.Repository, error) {
	if s.driver == "postgres" {
		r, err := s.pg.GetRepositoryByExternalID(ctx, postgressqlc.GetRepositoryByExternalIDParams{InstanceID: instanceID, ExternalID: externalID})
		if err != nil {
			return nil, err
		}
		return mapRepoFields(r.ID, r.InstanceID, r.OrgID, r.ExternalID, r.NodeID, r.Owner, r.Name, r.FullName, r.DefaultBranch,
			r.Private, r.Archived, r.Empty, r.Fork, r.HtmlUrl, r.LastSyncedAt, r.DeletedAt, r.CreatedAt, r.UpdatedAt), nil
	}
	r, err := s.sqlite.GetRepositoryByExternalID(ctx, sqlitesqlc.GetRepositoryByExternalIDParams{InstanceID: instanceID, ExternalID: externalID})
	if err != nil {
		return nil, err
	}
	return mapRepoFields(r.ID, r.InstanceID, r.OrgID, r.ExternalID, r.NodeID, r.Owner, r.Name, r.FullName, r.DefaultBranch,
		r.Private, r.Archived, r.Empty, r.Fork, r.HtmlUrl, r.LastSyncedAt, r.DeletedAt, r.CreatedAt, r.UpdatedAt), nil
}

func (s *Store) GetRepositoryByID(ctx context.Context, id int64) (*models.Repository, error) {
	return s.getRepoSQLCByID(ctx, id)
}

func (s *Store) GetRepositoryByOwnerName(ctx context.Context, owner, name string) (*models.Repository, error) {
	return s.GetRepositoryByOwnerNameInInstance(ctx, owner, name, 0)
}

// ListRepositoriesByOwnerName returns all alive repos matching owner/name (any instance).
func (s *Store) ListRepositoriesByOwnerName(ctx context.Context, owner, name string) ([]models.Repository, error) {
	if s.driver == "postgres" {
		rows, err := s.pg.ListRepositoriesByOwnerName(ctx, postgressqlc.ListRepositoriesByOwnerNameParams{Owner: owner, Name: name})
		if err != nil {
			return nil, err
		}
		out := make([]models.Repository, 0, len(rows))
		for _, r := range rows {
			out = append(out, *mapRepoFields(r.ID, r.InstanceID, r.OrgID, r.ExternalID, r.NodeID, r.Owner, r.Name, r.FullName, r.DefaultBranch,
				r.Private, r.Archived, r.Empty, r.Fork, r.HtmlUrl, r.LastSyncedAt, r.DeletedAt, r.CreatedAt, r.UpdatedAt))
		}
		return out, nil
	}
	rows, err := s.sqlite.ListRepositoriesByOwnerName(ctx, sqlitesqlc.ListRepositoriesByOwnerNameParams{Owner: owner, Name: name})
	if err != nil {
		return nil, err
	}
	out := make([]models.Repository, 0, len(rows))
	for _, r := range rows {
		out = append(out, *mapRepoFields(r.ID, r.InstanceID, r.OrgID, r.ExternalID, r.NodeID, r.Owner, r.Name, r.FullName, r.DefaultBranch,
			r.Private, r.Archived, r.Empty, r.Fork, r.HtmlUrl, r.LastSyncedAt, r.DeletedAt, r.CreatedAt, r.UpdatedAt))
	}
	return out, nil
}

// ErrAmbiguousRepository is returned when owner/name matches multiple alive repos
// across instances and no instance_id was provided.
var ErrAmbiguousRepository = fmt.Errorf("repository owner/name is ambiguous across forge instances")

// GetRepositoryByOwnerNameInInstance looks up an alive repo by owner/name.
// When instanceID > 0, the lookup is scoped to that instance.
// When instanceID is 0 and multiple instances share the same owner/name, returns ErrAmbiguousRepository.
func (s *Store) GetRepositoryByOwnerNameInInstance(ctx context.Context, owner, name string, instanceID int64) (*models.Repository, error) {
	if instanceID > 0 {
		if s.driver == "postgres" {
			r, err := s.pg.GetRepositoryByOwnerNameInInstance(ctx, postgressqlc.GetRepositoryByOwnerNameInInstanceParams{
				InstanceID: instanceID, Owner: owner, Name: name,
			})
			if err != nil {
				return nil, err
			}
			return mapRepoFields(r.ID, r.InstanceID, r.OrgID, r.ExternalID, r.NodeID, r.Owner, r.Name, r.FullName, r.DefaultBranch,
				r.Private, r.Archived, r.Empty, r.Fork, r.HtmlUrl, r.LastSyncedAt, r.DeletedAt, r.CreatedAt, r.UpdatedAt), nil
		}
		r, err := s.sqlite.GetRepositoryByOwnerNameInInstance(ctx, sqlitesqlc.GetRepositoryByOwnerNameInInstanceParams{
			InstanceID: instanceID, Owner: owner, Name: name,
		})
		if err != nil {
			return nil, err
		}
		return mapRepoFields(r.ID, r.InstanceID, r.OrgID, r.ExternalID, r.NodeID, r.Owner, r.Name, r.FullName, r.DefaultBranch,
			r.Private, r.Archived, r.Empty, r.Fork, r.HtmlUrl, r.LastSyncedAt, r.DeletedAt, r.CreatedAt, r.UpdatedAt), nil
	}
	repos, err := s.ListRepositoriesByOwnerName(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, sql.ErrNoRows
	}
	if len(repos) > 1 {
		return nil, ErrAmbiguousRepository
	}
	return &repos[0], nil
}

func scanRepo(row scanner) (*models.Repository, error) {
	var r models.Repository
	var orgID sql.NullInt64
	var private, archived, empty, fork int
	var lastSynced, deleted, created, updated sql.NullString
	if err := row.Scan(
		&r.ID, &r.InstanceID, &orgID, &r.ExternalID, &r.NodeID, &r.Owner, &r.Name, &r.FullName, &r.DefaultBranch,
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

func scanRepoWithForge(row scanner) (*models.Repository, error) {
	var r models.Repository
	var orgID sql.NullInt64
	var private, archived, empty, fork int
	var lastSynced, deleted, created, updated sql.NullString
	var forgeType string
	var instanceName string
	if err := row.Scan(
		&r.ID, &r.InstanceID, &orgID, &r.ExternalID, &r.NodeID, &r.Owner, &r.Name, &r.FullName, &r.DefaultBranch,
		&private, &archived, &empty, &fork, &r.HTMLURL, &lastSynced, &deleted, &created, &updated,
		&forgeType, &instanceName,
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
	r.ForgeType = forgeType
	if r.ForgeType == "" {
		r.ForgeType = models.ForgeTypeGitea
	}
	r.InstanceName = strings.TrimSpace(instanceName)
	r.LastSyncedAt = nullTime(lastSynced)
	r.DeletedAt = nullTime(deleted)
	r.CreatedAt, _ = parseTime(created.String)
	r.UpdatedAt, _ = parseTime(updated.String)
	return &r, nil
}

type ListRepositoriesOpts struct {
	InstanceID     int64
	ForgeType      string // gitea | github; empty = all
	UserID         int64  // when >0, join user_repository_access (unless BootstrapAllowAll)
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
	forgeType := parseListForgeType(opts.ForgeType)
	if forgeType != "" {
		where = append(where, "COALESCE(NULLIF(TRIM(i.forge_type), ''), 'gitea') = ?")
		args = append(args, forgeType)
	}
	if q := strings.TrimSpace(opts.Query); q != "" {
		if like := likePattern(q); like != "" {
			where = append(where, "(r.full_name LIKE ? OR r.owner LIKE ? OR r.name LIKE ?)")
			args = append(args, like, like, like)
		}
	}
	clause := strings.Join(where, " AND ")
	instJoin := " LEFT JOIN instances i ON i.id = r.instance_id"

	var total int
	countSQL := `SELECT COUNT(*) FROM repositories r ` + join + instJoin + ` WHERE ` + clause
	if err := s.queryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	listJoin := join + instJoin
	args = append(args, opts.Limit, opts.Offset)
	rows, err := s.query(ctx, `
SELECT r.id, r.instance_id, r.org_id, r.external_id, r.node_id, r.owner, r.name, r.full_name, r.default_branch,
       r.private, r.archived, r.empty, r.fork, r.html_url, r.last_synced_at, r.deleted_at, r.created_at, r.updated_at,
       COALESCE(NULLIF(TRIM(i.forge_type), ''), 'gitea'), COALESCE(i.name, '')
FROM repositories r `+listJoin+`
WHERE `+clause+`
ORDER BY r.full_name ASC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.Repository
	for rows.Next() {
		r, err := scanRepoWithForge(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *r)
	}
	return out, total, rows.Err()
}

func (s *Store) SoftDeleteRepository(ctx context.Context, id int64) error {
	return s.softDeleteRepoSQLC(ctx, id)
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
SELECT id, instance_id, org_id, external_id, node_id, owner, name, full_name, default_branch,
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

// TryAcquireSyncLease acquires a lease keyed by leaseID (typically instances.id)
// so concurrent forges do not block each other. holder identifies the process claiming it.
func (s *Store) TryAcquireSyncLease(ctx context.Context, leaseID int64, holder string, ttl time.Duration) (bool, error) {
	if leaseID <= 0 {
		return false, fmt.Errorf("sync lease id must be positive")
	}
	now := time.Now().UTC()
	expires := formatTime(now.Add(ttl))
	nowStr := formatTime(now)
	res, err := s.exec(ctx, `
INSERT INTO sync_leases (id, holder, expires_at) VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET holder=excluded.holder, expires_at=excluded.expires_at
WHERE sync_leases.expires_at < ? OR sync_leases.holder = ?`, leaseID, holder, expires, nowStr, holder)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ReleaseSyncLease clears a lease held by holder so another process can acquire it.
func (s *Store) ReleaseSyncLease(ctx context.Context, leaseID int64, holder string) error {
	if leaseID <= 0 || strings.TrimSpace(holder) == "" {
		return nil
	}
	_, err := s.exec(ctx, `DELETE FROM sync_leases WHERE id = ? AND holder = ?`, leaseID, holder)
	return err
}
