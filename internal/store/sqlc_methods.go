package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/models"
	postgressqlc "github.com/ncdlabs/gitseer/internal/store/sqlc/postgres"
	sqlitesqlc "github.com/ncdlabs/gitseer/internal/store/sqlc/sqlite"
)

func mapInstanceFields(
	id int64, name, forgeType, baseURL, version, capsJSON,
	syncCipher, webhookCipher, oauthID, oauthSecret, externalURL string,
	allowPrivate, allowUnsigned int64,
	verifiedAt, ensureAt, ensureErr, verifyToken *string,
	createdAt, updatedAt string,
) *models.Instance {
	inst := &models.Instance{
		ID: id, Name: name, ForgeType: forgeType, BaseURL: baseURL, Version: version,
		CapabilitiesJSON: capsJSON, SyncTokenCiphertext: syncCipher, WebhookSecretCiphertext: webhookCipher,
		OAuthClientID: oauthID, OAuthClientSecretCipher: oauthSecret, ExternalURL: externalURL,
		AllowPrivateNetwork: allowPrivate != 0, AllowUnsignedWebhooks: allowUnsigned != 0,
		WebhookEnsureError: derefStr(ensureErr), WebhookVerifyToken: derefStr(verifyToken),
		CreatedAt: parseRequiredTime(createdAt), UpdatedAt: parseRequiredTime(updatedAt),
	}
	inst.WebhookVerifiedAt = parseOptionalTimePtr(verifiedAt)
	inst.WebhookEnsureAt = parseOptionalTimePtr(ensureAt)
	if inst.ForgeType == "" {
		inst.ForgeType = models.ForgeTypeGitea
	}
	return inst
}

func mapJobFields(
	id, runID, repoID, externalID int64, nodeID, name, status, conclusion, upstreamStatus, upstreamConclusion string,
	runnerID *int64, runnerName, htmlURL string, startedAt, completedAt, stepsJSON, labelsJSON *string, message string,
) *models.Job {
	j := &models.Job{
		ID: id, RunID: runID, RepoID: repoID, ExternalID: externalID, NodeID: nodeID, Name: name,
		Status: status, Conclusion: conclusion, UpstreamStatus: upstreamStatus, UpstreamConclusion: upstreamConclusion,
		RunnerID: runnerID, RunnerName: runnerName, HTMLURL: htmlURL, Message: message,
		StartedAt: parseOptionalTimePtr(startedAt), CompletedAt: parseOptionalTimePtr(completedAt),
		StepsJSON: stepsJSON, LabelsJSON: labelsJSON,
	}
	return j
}

func mapRepoFields(
	id, instanceID int64, orgID *int64, externalID int64, nodeID, owner, name, fullName, defaultBranch string,
	private, archived, empty, fork int64, htmlURL string, lastSynced, deleted *string, created, updated string,
) *models.Repository {
	r := &models.Repository{
		ID: id, InstanceID: instanceID, ExternalID: externalID, NodeID: nodeID,
		Owner: owner, Name: name, FullName: fullName, DefaultBranch: defaultBranch,
		Private: private != 0, Archived: archived != 0, Empty: empty != 0, Fork: fork != 0,
		HTMLURL: htmlURL, OrgID: orgID,
		LastSyncedAt: parseOptionalTimePtr(lastSynced), DeletedAt: parseOptionalTimePtr(deleted),
		CreatedAt: parseRequiredTime(created), UpdatedAt: parseRequiredTime(updated),
	}
	return r
}

func (s *Store) getInstanceSQLCByID(ctx context.Context, id int64) (*models.Instance, error) {
	if s.driver == "postgres" {
		r, err := s.pg.GetInstanceByID(ctx, id)
		if err != nil {
			return nil, err
		}
		return mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
			r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
			r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
			r.CreatedAt, r.UpdatedAt), nil
	}
	r, err := s.sqlite.GetInstanceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
		r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
		r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
		r.CreatedAt, r.UpdatedAt), nil
}

func (s *Store) getInstanceSQLCByURL(ctx context.Context, baseURL string) (*models.Instance, error) {
	if s.driver == "postgres" {
		r, err := s.pg.GetInstanceByURL(ctx, baseURL)
		if err != nil {
			return nil, err
		}
		return mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
			r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
			r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
			r.CreatedAt, r.UpdatedAt), nil
	}
	r, err := s.sqlite.GetInstanceByURL(ctx, baseURL)
	if err != nil {
		return nil, err
	}
	return mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
		r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
		r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
		r.CreatedAt, r.UpdatedAt), nil
}

func (s *Store) listInstancesSQLC(ctx context.Context) ([]models.Instance, error) {
	if s.driver == "postgres" {
		rows, err := s.pg.ListInstances(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]models.Instance, 0, len(rows))
		for _, r := range rows {
			out = append(out, *mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
				r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
				r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
				r.CreatedAt, r.UpdatedAt))
		}
		return out, nil
	}
	rows, err := s.sqlite.ListInstances(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.Instance, 0, len(rows))
	for _, r := range rows {
		out = append(out, *mapInstanceFields(r.ID, r.Name, r.ForgeType, r.BaseUrl, r.Version, r.CapabilitiesJson,
			r.SyncTokenCiphertext, r.WebhookSecretCiphertext, r.OauthClientID, r.OauthClientSecretCiphertext, r.ExternalUrl,
			r.AllowPrivateNetwork, r.AllowUnsignedWebhooks, r.WebhookVerifiedAt, r.WebhookEnsureAt, r.WebhookEnsureError, r.WebhookVerifyToken,
			r.CreatedAt, r.UpdatedAt))
	}
	return out, nil
}

func (s *Store) getJobSQLCByID(ctx context.Context, id int64) (*models.Job, error) {
	if s.driver == "postgres" {
		r, err := s.pg.GetJobByID(ctx, id)
		if err != nil {
			return nil, err
		}
		return mapJobFields(r.ID, r.RunID, r.RepoID, r.ExternalID, r.NodeID, r.Name, r.Status, r.Conclusion, r.UpstreamStatus, r.UpstreamConclusion,
			r.RunnerID, r.RunnerName, r.HtmlUrl, r.StartedAt, r.CompletedAt, r.StepsJson, r.LabelsJson, r.Message), nil
	}
	r, err := s.sqlite.GetJobByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return mapJobFields(r.ID, r.RunID, r.RepoID, r.ExternalID, r.NodeID, r.Name, r.Status, r.Conclusion, r.UpstreamStatus, r.UpstreamConclusion,
		r.RunnerID, r.RunnerName, r.HtmlUrl, r.StartedAt, r.CompletedAt, r.StepsJson, r.LabelsJson, r.Message), nil
}

func (s *Store) getJobSQLCByExternalID(ctx context.Context, repoID, externalID int64) (*models.Job, error) {
	if s.driver == "postgres" {
		r, err := s.pg.GetJobByExternalID(ctx, postgressqlc.GetJobByExternalIDParams{RepoID: repoID, ExternalID: externalID})
		if err != nil {
			return nil, err
		}
		return mapJobFields(r.ID, r.RunID, r.RepoID, r.ExternalID, r.NodeID, r.Name, r.Status, r.Conclusion, r.UpstreamStatus, r.UpstreamConclusion,
			r.RunnerID, r.RunnerName, r.HtmlUrl, r.StartedAt, r.CompletedAt, r.StepsJson, r.LabelsJson, r.Message), nil
	}
	r, err := s.sqlite.GetJobByExternalID(ctx, sqlitesqlc.GetJobByExternalIDParams{RepoID: repoID, ExternalID: externalID})
	if err != nil {
		return nil, err
	}
	return mapJobFields(r.ID, r.RunID, r.RepoID, r.ExternalID, r.NodeID, r.Name, r.Status, r.Conclusion, r.UpstreamStatus, r.UpstreamConclusion,
		r.RunnerID, r.RunnerName, r.HtmlUrl, r.StartedAt, r.CompletedAt, r.StepsJson, r.LabelsJson, r.Message), nil
}

func (s *Store) listJobsSQLCByRunID(ctx context.Context, runID int64) ([]models.Job, error) {
	if s.driver == "postgres" {
		rows, err := s.pg.ListJobsByRunID(ctx, runID)
		if err != nil {
			return nil, err
		}
		out := make([]models.Job, 0, len(rows))
		for _, r := range rows {
			out = append(out, *mapJobFields(r.ID, r.RunID, r.RepoID, r.ExternalID, r.NodeID, r.Name, r.Status, r.Conclusion, r.UpstreamStatus, r.UpstreamConclusion,
				r.RunnerID, r.RunnerName, r.HtmlUrl, r.StartedAt, r.CompletedAt, r.StepsJson, r.LabelsJson, r.Message))
		}
		return out, nil
	}
	rows, err := s.sqlite.ListJobsByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]models.Job, 0, len(rows))
	for _, r := range rows {
		out = append(out, *mapJobFields(r.ID, r.RunID, r.RepoID, r.ExternalID, r.NodeID, r.Name, r.Status, r.Conclusion, r.UpstreamStatus, r.UpstreamConclusion,
			r.RunnerID, r.RunnerName, r.HtmlUrl, r.StartedAt, r.CompletedAt, r.StepsJson, r.LabelsJson, r.Message))
	}
	return out, nil
}

func (s *Store) getRepoSQLCByID(ctx context.Context, id int64) (*models.Repository, error) {
	if s.driver == "postgres" {
		r, err := s.pg.GetRepositoryByID(ctx, id)
		if err != nil {
			return nil, err
		}
		return mapRepoFields(r.ID, r.InstanceID, r.OrgID, r.ExternalID, r.NodeID, r.Owner, r.Name, r.FullName, r.DefaultBranch,
			r.Private, r.Archived, r.Empty, r.Fork, r.HtmlUrl, r.LastSyncedAt, r.DeletedAt, r.CreatedAt, r.UpdatedAt), nil
	}
	r, err := s.sqlite.GetRepositoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return mapRepoFields(r.ID, r.InstanceID, r.OrgID, r.ExternalID, r.NodeID, r.Owner, r.Name, r.FullName, r.DefaultBranch,
		r.Private, r.Archived, r.Empty, r.Fork, r.HtmlUrl, r.LastSyncedAt, r.DeletedAt, r.CreatedAt, r.UpdatedAt), nil
}

func (s *Store) getAppSettingsSQLC(ctx context.Context) (*AppSettings, error) {
	var (
		instanceName, longRunning, serverURL, giteaURL, giteaToken, giteaWebhook, oauthID, oauthSecret, updated string
		syncDays, retRuns, retWebhooks, retAttention, setup                                                    int64
		allowPrivate, allowUnsigned                                                                            *int64
		err                                                                                                    error
	)
	if s.driver == "postgres" {
		r, e := s.pg.GetAppSettings(ctx)
		err = e
		if err == nil {
			instanceName, longRunning, serverURL = r.InstanceName, r.AttentionLongRunningAfter, r.ServerExternalUrl
			giteaURL, giteaToken, giteaWebhook = r.GiteaUrl, r.GiteaTokenCipher, r.GiteaWebhookSecretCipher
			oauthID, oauthSecret, updated = r.OauthClientID, r.OauthClientSecretCipher, r.UpdatedAt
			syncDays, retRuns, retWebhooks, retAttention, setup = r.SyncHistoryDays, r.RetentionRunsDays, r.RetentionWebhooksDays, r.RetentionAttentionDays, r.SetupCompleted
			allowPrivate, allowUnsigned = r.GiteaAllowPrivateNetwork, r.GiteaAllowUnsignedWebhooks
		}
	} else {
		r, e := s.sqlite.GetAppSettings(ctx)
		err = e
		if err == nil {
			instanceName, longRunning, serverURL = r.InstanceName, r.AttentionLongRunningAfter, r.ServerExternalUrl
			giteaURL, giteaToken, giteaWebhook = r.GiteaUrl, r.GiteaTokenCipher, r.GiteaWebhookSecretCipher
			oauthID, oauthSecret, updated = r.OauthClientID, r.OauthClientSecretCipher, r.UpdatedAt
			syncDays, retRuns, retWebhooks, retAttention, setup = r.SyncHistoryDays, r.RetentionRunsDays, r.RetentionWebhooksDays, r.RetentionAttentionDays, r.SetupCompleted
			allowPrivate, allowUnsigned = r.GiteaAllowPrivateNetwork, r.GiteaAllowUnsignedWebhooks
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row := &AppSettings{
		InstanceName: instanceName, SyncHistoryDays: int(syncDays), AttentionLongRunningAfter: longRunning,
		RetentionRunsDays: int(retRuns), RetentionWebhooksDays: int(retWebhooks), RetentionAttentionDays: int(retAttention),
		ServerExternalURL: serverURL, GiteaURL: giteaURL, GiteaTokenCipher: giteaToken, GiteaWebhookSecretCipher: giteaWebhook,
		OAuthClientID: oauthID, OAuthClientSecretCipher: oauthSecret, SetupCompleted: setup != 0,
		UpdatedAt: parseRequiredTime(updated),
	}
	if allowPrivate != nil {
		row.GiteaAllowPrivateNetwork = sql.NullInt64{Int64: *allowPrivate, Valid: true}
	}
	if allowUnsigned != nil {
		row.GiteaAllowUnsignedWebhooks = sql.NullInt64{Int64: *allowUnsigned, Valid: true}
	}
	return row, nil
}

func (s *Store) countWorkflowRunsSQLC(ctx context.Context) (int64, error) {
	if s.driver == "postgres" {
		return s.pg.CountWorkflowRuns(ctx)
	}
	return s.sqlite.CountWorkflowRuns(ctx)
}

func (s *Store) resolveAttentionSQLC(ctx context.Context, fingerprint string) error {
	now := formatTime(time.Now().UTC())
	if s.driver == "postgres" {
		return s.pg.ResolveAttentionByFingerprint(ctx, postgressqlc.ResolveAttentionByFingerprintParams{
			ResolvedAt: &now, UpdatedAt: now, Fingerprint: fingerprint,
		})
	}
	return s.sqlite.ResolveAttentionByFingerprint(ctx, sqlitesqlc.ResolveAttentionByFingerprintParams{
		ResolvedAt: &now, UpdatedAt: now, Fingerprint: fingerprint,
	})
}

func (s *Store) clearUntilResolvedMutesSQLC(ctx context.Context, fingerprint string) error {
	if s.driver == "postgres" {
		return s.pg.ClearUntilResolvedMutes(ctx, fingerprint)
	}
	return s.sqlite.ClearUntilResolvedMutes(ctx, fingerprint)
}

func (s *Store) deleteSessionSQLC(ctx context.Context, id string) error {
	if s.driver == "postgres" {
		return s.pg.DeleteSession(ctx, id)
	}
	return s.sqlite.DeleteSession(ctx, id)
}

func (s *Store) createSessionSQLC(ctx context.Context, id, tokenHash string, userID int64, expires time.Time, ip, ua string) error {
	paramsExpires := formatTime(expires)
	paramsCreated := formatTime(time.Now().UTC())
	if s.driver == "postgres" {
		return s.pg.CreateSession(ctx, postgressqlc.CreateSessionParams{
			ID: id, UserID: userID, TokenHash: tokenHash, ExpiresAt: paramsExpires, CreatedAt: paramsCreated, Ip: ip, UserAgent: ua,
		})
	}
	return s.sqlite.CreateSession(ctx, sqlitesqlc.CreateSessionParams{
		ID: id, UserID: userID, TokenHash: tokenHash, ExpiresAt: paramsExpires, CreatedAt: paramsCreated, Ip: ip, UserAgent: ua,
	})
}

func (s *Store) getSessionSQLC(ctx context.Context, hash string) (*models.Session, error) {
	var (
		id, expires, created, ip, ua string
		userID                       int64
		elevated                     *string
		err                          error
	)
	if s.driver == "postgres" {
		r, e := s.pg.GetSessionByTokenHash(ctx, hash)
		err = e
		if err == nil {
			id, userID, expires, created, ip, ua = r.ID, r.UserID, r.ExpiresAt, r.CreatedAt, r.Ip, r.UserAgent
			elevated = r.BootstrapElevatedUntil
		}
	} else {
		r, e := s.sqlite.GetSessionByTokenHash(ctx, hash)
		err = e
		if err == nil {
			id, userID, expires, created, ip, ua = r.ID, r.UserID, r.ExpiresAt, r.CreatedAt, r.Ip, r.UserAgent
			elevated = r.BootstrapElevatedUntil
		}
	}
	if err != nil {
		return nil, err
	}
	sess := &models.Session{ID: id, UserID: userID, IP: ip, UserAgent: ua}
	sess.ExpiresAt, _ = parseTime(expires)
	sess.CreatedAt, _ = parseTime(created)
	if elevated != nil && strings.TrimSpace(*elevated) != "" {
		if t, err := parseTime(*elevated); err == nil {
			sess.BootstrapElevatedUntil = &t
		}
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		return nil, sql.ErrNoRows
	}
	return sess, nil
}

func (s *Store) getUserSQLCByID(ctx context.Context, id int64) (*models.User, error) {
	var (
		login, email, displayName, avatarURL, created, updated string
		instanceID, giteaID, githubID, githubInstID            *int64
		gitlabID, gitlabInstID, bbID, bbInstID                 *int64
		bootstrap                                              int64
		err                                                    error
		uid                                                    int64
	)
	if s.driver == "postgres" {
		r, e := s.pg.GetUserByID(ctx, id)
		err = e
		if err == nil {
			uid, instanceID, giteaID, githubID, githubInstID = r.ID, r.InstanceID, r.GiteaUserID, r.GithubUserID, r.GithubInstanceID
			gitlabID, gitlabInstID, bbID, bbInstID = r.GitlabUserID, r.GitlabInstanceID, r.BitbucketUserID, r.BitbucketInstanceID
			login, email, displayName, avatarURL = r.Login, r.Email, r.DisplayName, r.AvatarUrl
			bootstrap, created, updated = r.IsBootstrapAdmin, r.CreatedAt, r.UpdatedAt
		}
	} else {
		r, e := s.sqlite.GetUserByID(ctx, id)
		err = e
		if err == nil {
			uid, instanceID, giteaID, githubID, githubInstID = r.ID, r.InstanceID, r.GiteaUserID, r.GithubUserID, r.GithubInstanceID
			gitlabID, gitlabInstID, bbID, bbInstID = r.GitlabUserID, r.GitlabInstanceID, r.BitbucketUserID, r.BitbucketInstanceID
			login, email, displayName, avatarURL = r.Login, r.Email, r.DisplayName, r.AvatarUrl
			bootstrap, created, updated = r.IsBootstrapAdmin, r.CreatedAt, r.UpdatedAt
		}
	}
	if err != nil {
		return nil, err
	}
	u := &models.User{
		ID: uid, InstanceID: instanceID, GiteaUserID: giteaID, GitHubUserID: githubID, GitHubInstanceID: githubInstID,
		GitLabUserID: gitlabID, GitLabInstanceID: gitlabInstID, BitbucketUserID: bbID, BitbucketInstanceID: bbInstID,
		Login: login, Email: email, DisplayName: displayName, AvatarURL: avatarURL,
		IsBootstrapAdmin: bootstrap != 0,
		CreatedAt:        parseRequiredTime(created), UpdatedAt: parseRequiredTime(updated),
	}
	return u, nil
}

func (s *Store) getWorkflowGraphSQLC(ctx context.Context, repoID int64, path, commitSHA string) (string, error) {
	if s.driver == "postgres" {
		return s.pg.GetWorkflowGraph(ctx, postgressqlc.GetWorkflowGraphParams{RepoID: repoID, Path: path, CommitSha: commitSHA})
	}
	return s.sqlite.GetWorkflowGraph(ctx, sqlitesqlc.GetWorkflowGraphParams{RepoID: repoID, Path: path, CommitSha: commitSHA})
}

func (s *Store) upsertWorkflowGraphSQLC(ctx context.Context, repoID int64, path, commitSHA, nodesJSON string) error {
	if s.driver == "postgres" {
		return s.pg.UpsertWorkflowGraph(ctx, postgressqlc.UpsertWorkflowGraphParams{
			RepoID: repoID, Path: path, CommitSha: commitSHA, NodesJson: nodesJSON,
		})
	}
	return s.sqlite.UpsertWorkflowGraph(ctx, sqlitesqlc.UpsertWorkflowGraphParams{
		RepoID: repoID, Path: path, CommitSha: commitSHA, NodesJson: nodesJSON,
	})
}

func (s *Store) softDeleteRepoSQLC(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	if s.driver == "postgres" {
		if err := s.pg.SoftDeleteRepository(ctx, postgressqlc.SoftDeleteRepositoryParams{
			DeletedAt: &now, UpdatedAt: now, ID: id,
		}); err != nil {
			return err
		}
		return s.pg.DeleteAccessForRepo(ctx, id)
	}
	if err := s.sqlite.SoftDeleteRepository(ctx, sqlitesqlc.SoftDeleteRepositoryParams{
		DeletedAt: &now, UpdatedAt: now, ID: id,
	}); err != nil {
		return err
	}
	return s.sqlite.DeleteAccessForRepo(ctx, id)
}

func (s *Store) getWallboardTokenSQLC(ctx context.Context, id int64) (*WallboardToken, error) {
	var (
		name, prefix, created string
		createdBy             *int64
		lastUsed, revoked     *string
		tid                   int64
		err                   error
	)
	if s.driver == "postgres" {
		r, e := s.pg.GetWallboardTokenByID(ctx, id)
		err = e
		if err == nil {
			tid, name, prefix, createdBy, created, lastUsed, revoked = r.ID, r.Name, r.TokenPrefix, r.CreatedByUserID, r.CreatedAt, r.LastUsedAt, r.RevokedAt
		}
	} else {
		r, e := s.sqlite.GetWallboardTokenByID(ctx, id)
		err = e
		if err == nil {
			tid, name, prefix, createdBy, created, lastUsed, revoked = r.ID, r.Name, r.TokenPrefix, r.CreatedByUserID, r.CreatedAt, r.LastUsedAt, r.RevokedAt
		}
	}
	if err != nil {
		return nil, err
	}
	return &WallboardToken{
		ID: tid, Name: name, TokenPrefix: prefix, CreatedByUserID: createdBy,
		CreatedAt: parseRequiredTime(created), LastUsedAt: parseOptionalTimePtr(lastUsed), RevokedAt: parseOptionalTimePtr(revoked),
	}, nil
}
