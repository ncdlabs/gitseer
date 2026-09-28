// Package sync orchestrates forge discovery, initial import, and reconciliation.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ncdlabs/gitseer/internal/attention"
	"github.com/ncdlabs/gitseer/internal/config"
	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/forge"
	_ "github.com/ncdlabs/gitseer/internal/forge/all"
	gitseermetrics "github.com/ncdlabs/gitseer/internal/metrics"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/realtime"
	"github.com/ncdlabs/gitseer/internal/store"
)

type Result struct {
	InstanceID           int64  `json:"instance_id"`
	ForgeType            string `json:"forge_type,omitempty"`
	Version              string `json:"version"`
	RepositoriesUpserted int    `json:"repositories_upserted"`
	PullRequestsUpserted int    `json:"pull_requests_upserted"`
	RunsUpserted         int    `json:"runs_upserted"`
	JobsUpserted         int    `json:"jobs_upserted"`
}

// Prefs are runtime sync overrides (instance label + history depth).
type Prefs struct {
	InstanceName    string
	SyncHistoryDays int
}

// PrefsSource supplies runtime sync preferences.
type PrefsSource interface {
	SyncPrefs() Prefs
}

// GiteaSource supplies effective Gitea URL/token/private-network settings (legacy fallback).
type GiteaSource interface {
	GiteaConnection() (url, token string, allowPrivate bool)
}

// SecretOpener decrypts instance ciphertext (sync token / webhook secret).
type SecretOpener interface {
	OpenSecret(stored string) (string, error)
}

// Service is the full sync orchestrator.
type Service struct {
	store   *store.Store
	att     *attention.Engine
	hub     *realtime.Hub
	cfg     config.Config
	prefs   PrefsSource
	gitea   GiteaSource
	secrets SecretOpener
	encKey  []byte
	log     *slog.Logger
}

func NewService(st *store.Store, att *attention.Engine, hub *realtime.Hub, cfg config.Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	var encKey []byte
	if strings.TrimSpace(cfg.Auth.EncryptionKey) != "" {
		if k, err := gitseercrypto.KeyFromString(cfg.Auth.EncryptionKey); err == nil {
			encKey = k
		}
	}
	return &Service{store: st, att: att, hub: hub, cfg: cfg, encKey: encKey, log: log}
}

// SetPrefsSource attaches runtime overrides for instance name / history days.
func (s *Service) SetPrefsSource(p PrefsSource) {
	s.prefs = p
}

// SetGiteaSource attaches runtime Gitea connection overrides (settings manager).
func (s *Service) SetGiteaSource(g GiteaSource) {
	s.gitea = g
}

// SetSecretOpener attaches decrypt for per-instance sync tokens.
func (s *Service) SetSecretOpener(o SecretOpener) {
	s.secrets = o
}

func (s *Service) giteaConn() (url, token string, allowPrivate bool) {
	if s.gitea != nil {
		return s.gitea.GiteaConnection()
	}
	return s.cfg.Gitea.URL, s.cfg.Gitea.Token, s.cfg.Gitea.AllowPrivateNetwork
}

func (s *Service) openSecret(stored string) (string, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "", nil
	}
	if s.secrets != nil {
		return s.secrets.OpenSecret(stored)
	}
	if len(s.encKey) != 32 {
		if gitseercrypto.LooksLikeCiphertext(stored) {
			return "", fmt.Errorf("GITSEER_ENCRYPTION_KEY is required to decrypt stored secrets")
		}
		if gitseercrypto.LooksLikeBase64Blob(stored) {
			return "", fmt.Errorf("GITSEER_ENCRYPTION_KEY is required to decrypt stored secrets")
		}
		return stored, nil
	}
	pt, err := gitseercrypto.Decrypt(s.encKey, stored)
	if err != nil {
		return "", err
	}
	return pt, nil
}

func (s *Service) historyDays() int {
	days := s.cfg.Sync.HistoryDays
	if s.prefs != nil {
		p := s.prefs.SyncPrefs()
		if p.SyncHistoryDays > 0 {
			days = p.SyncHistoryDays
		}
	}
	return days
}

func (s *Service) defaultInstanceName(forgeType string) string {
	name := s.cfg.UI.InstanceName
	if s.prefs != nil {
		p := s.prefs.SyncPrefs()
		if p.InstanceName != "" {
			name = p.InstanceName
		}
	}
	if name != "" {
		return name
	}
	switch forgeType {
	case models.ForgeTypeGitHub:
		return "GitHub"
	case models.ForgeTypeGitLab:
		return "GitLab"
	case models.ForgeTypeBitbucket:
		return "Bitbucket"
	case models.ForgeTypeForgejo:
		return "Forgejo"
	default:
		return "Gitea"
	}
}

// SyncFromConfig syncs every enabled forge instance that has a decryptable sync token.
// Falls back to legacy GiteaSource/config when no instance rows hold tokens yet.
func (s *Service) SyncFromConfig(ctx context.Context) (*Result, error) {
	results, err := s.SyncAllInstances(ctx)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no forge instances with sync tokens configured")
	}
	agg := &Result{
		InstanceID: results[0].InstanceID,
		ForgeType:  results[0].ForgeType,
		Version:    results[0].Version,
	}
	for _, r := range results {
		agg.RepositoriesUpserted += r.RepositoriesUpserted
		agg.PullRequestsUpserted += r.PullRequestsUpserted
		agg.RunsUpserted += r.RunsUpserted
		agg.JobsUpserted += r.JobsUpserted
	}
	if len(results) > 1 {
		agg.InstanceID = 0
		agg.ForgeType = ""
		agg.Version = ""
	}
	return agg, nil
}

// SyncAllInstances runs a full sync for each instance with a sync token.
func (s *Service) SyncAllInstances(ctx context.Context) ([]Result, error) {
	targets, err := s.syncTargets(ctx)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no forge instances with sync tokens configured")
	}
	out := make([]Result, 0, len(targets))
	var firstErr error
	leaseTTL := s.cfg.Sync.ReconcileInterval
	if leaseTTL <= 0 {
		leaseTTL = 5 * time.Minute
	}
	for _, t := range targets {
		instID := t.inst.ID
		if instID <= 0 {
			meta, err := s.store.UpsertInstanceMeta(ctx, t.inst.ForgeType, t.inst.Name, t.inst.BaseURL, t.inst.Version, t.inst.CapabilitiesJSON)
			if err != nil || meta == nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s (%s): ensure instance: %w", t.inst.Name, t.inst.ForgeType, err)
				}
				continue
			}
			instID = meta.ID
			t.inst.ID = instID
		}
		holder := fmt.Sprintf("gitseer-manual-%d", instID)
		ok, err := s.store.TryAcquireSyncLease(ctx, instID, holder, leaseTTL)
		if err != nil {
			s.log.Error("sync lease acquire failed", "instance_id", instID, "err", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s (%s): lease: %w", t.inst.Name, t.inst.ForgeType, err)
			}
			continue
		}
		if !ok {
			s.log.Info("sync skipped; lease held", "instance_id", instID, "forge", t.inst.ForgeType)
			continue
		}
		res, err := s.syncTarget(ctx, t)
		_ = s.store.ReleaseSyncLease(ctx, instID, holder)
		if err != nil {
			s.log.Error("instance sync failed", "instance_id", t.inst.ID, "forge", t.inst.ForgeType, "err", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s (%s): %w", t.inst.Name, t.inst.ForgeType, err)
			}
			continue
		}
		out = append(out, *res)
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no forge instances available to sync (leases held or no tokens)")
	}
	return out, nil
}

type syncTarget struct {
	inst  models.Instance
	token string
}

func (s *Service) syncTargets(ctx context.Context) ([]syncTarget, error) {
	instances, err := s.store.ListInstances(ctx)
	if err != nil {
		return nil, err
	}
	var out []syncTarget
	for _, inst := range instances {
		if strings.TrimSpace(inst.SyncTokenCiphertext) == "" {
			continue
		}
		token, err := s.openSecret(inst.SyncTokenCiphertext)
		if err != nil {
			s.log.Warn("skip instance: decrypt sync token failed", "instance_id", inst.ID, "err", err)
			continue
		}
		if token == "" || strings.TrimSpace(inst.BaseURL) == "" {
			continue
		}
		out = append(out, syncTarget{inst: inst, token: token})
	}
	if len(out) > 0 {
		return out, nil
	}
	// Legacy fallback: app_settings / config Gitea connection before instance secrets exist.
	url, token, allowPrivate := s.giteaConn()
	if url == "" || token == "" {
		return nil, nil
	}
	return []syncTarget{{
		inst: models.Instance{
			Name:                s.defaultInstanceName(models.ForgeTypeGitea),
			ForgeType:           models.ForgeTypeGitea,
			BaseURL:             url,
			AllowPrivateNetwork: allowPrivate,
		},
		token: token,
	}}, nil
}

func (s *Service) syncTarget(ctx context.Context, t syncTarget) (*Result, error) {
	f, err := forge.NewFromInstance(t.inst, t.token)
	if err != nil {
		return nil, err
	}
	ft := t.inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	name := t.inst.Name
	if name == "" {
		name = s.defaultInstanceName(ft)
	}
	return s.FullSync(ctx, f, ft, name, t.inst.BaseURL, s.historyDays())
}

const maxJobFetchesPerRepo = 50

func (s *Service) FullSync(ctx context.Context, f forge.Forge, forgeType, instanceName, baseURL string, historyDays int) (*Result, error) {
	started := time.Now()
	res, err := s.fullSync(ctx, f, forgeType, instanceName, baseURL, historyDays)
	gitseermetrics.ObserveSync(started, err)
	return res, err
}

func (s *Service) fullSync(ctx context.Context, f forge.Forge, forgeType, instanceName, baseURL string, historyDays int) (*Result, error) {
	ft := forgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	info, err := f.GetInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	caps, err := f.DetectCapabilities(ctx)
	if err != nil {
		return nil, fmt.Errorf("capabilities: %w", err)
	}
	capsJSON, _ := json.Marshal(caps)
	if instanceName == "" {
		instanceName = s.defaultInstanceName(ft)
	}
	inst, err := s.store.UpsertInstanceMeta(ctx, ft, instanceName, baseURL, info.Version, string(capsJSON))
	if err != nil {
		return nil, err
	}
	_ = s.store.SetSyncState(ctx, inst.ID, "instance", "organizations", "")

	if orgs, err := f.ListOrganizations(ctx); err == nil {
		for _, o := range orgs {
			_, _ = s.store.UpsertOrganization(ctx, inst.ID, o)
		}
	}

	_ = s.store.SetSyncState(ctx, inst.ID, "instance", "repositories", "")
	syncStart := time.Now().UTC()
	var seen []int64
	repoCount := 0
	for page := 1; ; page++ {
		p, err := f.ListRepositories(ctx, forge.ListReposOpts{Page: page, PageSize: 50})
		if err != nil {
			_ = s.store.SetSyncState(ctx, inst.ID, "instance", "error", err.Error())
			return nil, err
		}
		for _, repo := range p.Items {
			saved, err := s.store.UpsertRepository(ctx, inst.ID, repo)
			if err != nil {
				return nil, err
			}
			seen = append(seen, saved.ExternalID)
			repoCount++
		}
		if !p.HasMore {
			break
		}
	}
	if len(seen) > 0 {
		_ = s.store.SoftDeleteMissing(ctx, inst.ID, seen, syncStart)
	} else {
		s.log.Warn("repo sync returned zero repositories; skipping soft-delete", "forge", ft, "instance_id", inst.ID)
	}
	if _, err := s.store.LinkRepositoriesToOrganizations(ctx, inst.ID); err != nil {
		s.log.Warn("link repositories to organizations failed", "forge", ft, "instance_id", inst.ID, "err", err)
	}

	repos, err := s.store.ListAllAliveRepos(ctx, inst.ID)
	if err != nil {
		return nil, err
	}

	prCount, runCount, jobCount := 0, 0, 0
	_ = s.store.SetSyncState(ctx, inst.ID, "instance", "pull_requests", "")
	for _, repo := range repos {
		ref := models.RepoRef{Owner: repo.Owner, Name: repo.Name}
		openNumbers := make([]int64, 0)
		listOK := true
		for page := 1; ; page++ {
			p, err := f.ListPullRequests(ctx, ref, forge.PROpts{State: "open", Page: page, PageSize: 50})
			if err != nil {
				s.log.Warn("list PRs failed", "repo", repo.FullName, "err", err)
				listOK = false
				break
			}
			for _, pr := range p.Items {
				if pr.HeadSHA != "" {
					if st, err := f.GetCombinedCommitStatus(ctx, ref, pr.HeadSHA); err == nil && st != "" {
						pr.CIState = st
					} else if err != nil {
						s.log.Debug("commit status failed", "repo", repo.FullName, "sha", pr.HeadSHA, "err", err)
					}
				}
				if rs, err := f.GetPullRequestReviewState(ctx, ref, pr.Number); err == nil {
					pr.ReviewState = rs
				} else if err != nil {
					s.log.Debug("review state failed", "repo", repo.FullName, "pr", pr.Number, "err", err)
				}
				saved, err := s.store.UpsertPullRequest(ctx, repo.ID, pr)
				if err != nil {
					return nil, err
				}
				openNumbers = append(openNumbers, saved.Number)
				prCount++
				if s.att != nil {
					_ = s.att.EvaluatePullRequest(ctx, inst.ID, saved)
				}
			}
			if !p.HasMore {
				break
			}
		}
		if listOK {
			// Resolve attention for PRs about to be marked closed.
			if s.att != nil {
				wasOpen, _, _ := s.store.ListPullRequests(ctx, store.ListPRsOpts{
					RepoID: repo.ID, State: "open", Limit: 200, BootstrapAll: true,
				})
				seen := make(map[int64]struct{}, len(openNumbers))
				for _, n := range openNumbers {
					seen[n] = struct{}{}
				}
				for i := range wasOpen {
					if _, ok := seen[wasOpen[i].Number]; ok {
						continue
					}
					wasOpen[i].State = "closed"
					_ = s.att.EvaluatePullRequest(ctx, inst.ID, &wasOpen[i])
				}
			}
			if _, err := s.store.CloseOpenPRsNotInSet(ctx, repo.ID, openNumbers); err != nil {
				s.log.Warn("close missing open PRs failed", "repo", repo.FullName, "err", err)
			}
		}

		if !caps.ActionsAPI {
			continue
		}
		cutoff := time.Time{}
		if historyDays > 0 {
			cutoff = time.Now().UTC().AddDate(0, 0, -historyDays)
		}
		jobsFetched := 0
		for page := 1; page <= 5; page++ {
			p, err := f.ListWorkflowRuns(ctx, ref, forge.RunOpts{Page: page, PageSize: 50})
			if err != nil {
				s.log.Warn("list runs failed", "repo", repo.FullName, "err", err)
				break
			}
			for _, run := range p.Items {
				if historyDays > 0 && run.StartedAt != nil && run.StartedAt.Before(cutoff) {
					continue
				}
				saved, err := s.store.UpsertWorkflowRun(ctx, repo.ID, run)
				if err != nil {
					return nil, err
				}
				runCount++
				inFlight := saved.Status == models.StatusQueued || saved.Status == models.StatusWaiting || saved.Status == models.StatusRunning
				if inFlight || jobsFetched < maxJobFetchesPerRepo {
					jobs, err := f.ListJobs(ctx, ref, saved.ExternalID)
					if !inFlight {
						jobsFetched++
					}
					if err != nil {
						s.log.Debug("list jobs failed", "repo", repo.FullName, "run", saved.ExternalID, "err", err)
					} else {
						for _, job := range jobs {
							if _, err := s.store.UpsertJob(ctx, repo.ID, saved.ID, job); err != nil {
								return nil, err
							}
							jobCount++
						}
					}
				}
				if s.att != nil {
					_ = s.att.EvaluateRun(ctx, inst.ID, saved)
				}
				if s.hub != nil {
					s.hub.Publish(realtime.Event{Type: "workflow_run", ID: saved.ID, RepoID: repo.ID})
				}
			}
			if !p.HasMore {
				break
			}
		}

		s.reconcileInFlightRuns(ctx, f, ref, &repo, inst.ID)

		// Fill PR ci_state gaps from indexed runs when commit-status was empty.
		s.refreshOpenPRCIFromRuns(ctx, repo.ID)
	}

	_ = s.store.SetSyncState(ctx, inst.ID, "instance", "complete", "")
	s.log.Info("full sync complete", "forge", ft, "instance_id", inst.ID, "repos", repoCount, "prs", prCount, "runs", runCount, "jobs", jobCount)
	return &Result{
		InstanceID:           inst.ID,
		ForgeType:            ft,
		Version:              info.Version,
		RepositoriesUpserted: repoCount,
		PullRequestsUpserted: prCount,
		RunsUpserted:         runCount,
		JobsUpserted:         jobCount,
	}, nil
}

func (s *Service) refreshOpenPRCIFromRuns(ctx context.Context, repoID int64) {
	prs, _, err := s.store.ListPullRequests(ctx, store.ListPRsOpts{
		RepoID: repoID, State: "open", Limit: 200, BootstrapAll: true,
	})
	if err != nil {
		s.log.Debug("list open PRs for CI refresh failed", "repo_id", repoID, "err", err)
		return
	}
	seen := map[string]struct{}{}
	for _, pr := range prs {
		if pr.HeadSHA == "" || pr.CIState != "" {
			continue
		}
		if _, ok := seen[pr.HeadSHA]; ok {
			continue
		}
		seen[pr.HeadSHA] = struct{}{}
		runs, err := s.store.ListWorkflowRunsByCommitSHA(ctx, repoID, pr.HeadSHA)
		if err != nil || len(runs) == 0 {
			continue
		}
		state := forge.AggregateCIState(runs)
		if state == "" {
			continue
		}
		_ = s.store.SetPullRequestsCIStateByHeadSHA(ctx, repoID, pr.HeadSHA, state)
	}
}

// reconcileInFlightRuns closes DB runs that vanished from the forge (missed webhooks /
// purged history) so Active Actions does not keep ghost queued/waiting/running rows.
func (s *Service) reconcileInFlightRuns(ctx context.Context, f forge.Forge, ref models.RepoRef, repo *models.Repository, instanceID int64) {
	inFlight, err := s.store.ListInFlightWorkflowRunsByRepo(ctx, repo.ID)
	if err != nil {
		s.log.Debug("list in-flight runs failed", "repo", repo.FullName, "err", err)
		return
	}
	for i := range inFlight {
		run := inFlight[i]
		remote, err := f.GetWorkflowRun(ctx, ref, run.ExternalID)
		if forge.IsNotFound(err) {
			closed, cerr := s.store.CompleteOrphanedWorkflowRun(ctx, run.ID)
			if cerr != nil {
				s.log.Warn("close orphaned run failed", "repo", repo.FullName, "run", run.ExternalID, "err", cerr)
				continue
			}
			s.log.Info("closed orphaned in-flight run", "repo", repo.FullName, "run", run.ExternalID)
			if s.att != nil && closed != nil {
				_ = s.att.EvaluateRun(ctx, instanceID, closed)
			}
			if s.hub != nil {
				s.hub.Publish(realtime.Event{Type: "workflow_run", ID: run.ID, RepoID: repo.ID})
			}
			continue
		}
		if err != nil {
			s.log.Debug("get workflow run failed", "repo", repo.FullName, "run", run.ExternalID, "err", err)
			continue
		}
		if remote == nil {
			continue
		}
		saved, err := s.store.UpsertWorkflowRun(ctx, repo.ID, *remote)
		if err != nil {
			s.log.Debug("upsert reconciled run failed", "repo", repo.FullName, "run", run.ExternalID, "err", err)
			continue
		}
		if jobs, jerr := f.ListJobs(ctx, ref, saved.ExternalID); jerr == nil {
			for _, job := range jobs {
				_, _ = s.store.UpsertJob(ctx, repo.ID, saved.ID, job)
			}
		}
		if s.att != nil {
			_ = s.att.EvaluateRun(ctx, instanceID, saved)
		}
		if s.hub != nil {
			s.hub.Publish(realtime.Event{Type: "workflow_run", ID: saved.ID, RepoID: repo.ID})
		}
	}
}

func (s *Service) RunReconcile(ctx context.Context) {
	interval := s.cfg.Sync.ReconcileInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcileOnce(ctx, interval)
		}
	}
}

func (s *Service) reconcileOnce(ctx context.Context, leaseTTL time.Duration) {
	targets, err := s.syncTargets(ctx)
	if err != nil || len(targets) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, t := range targets {
		t := t
		// Ensure instance row exists so lease id is stable (legacy fallback has id 0).
		instID := t.inst.ID
		if instID <= 0 {
			meta, err := s.store.UpsertInstanceMeta(ctx, t.inst.ForgeType, t.inst.Name, t.inst.BaseURL, t.inst.Version, t.inst.CapabilitiesJSON)
			if err != nil || meta == nil {
				continue
			}
			instID = meta.ID
			t.inst.ID = instID
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			holder := fmt.Sprintf("gitseer-instance-%d", instID)
			ok, err := s.store.TryAcquireSyncLease(ctx, instID, holder, leaseTTL)
			if err != nil || !ok {
				return
			}
			_, syncErr := s.syncTarget(ctx, t)
			_ = s.store.ReleaseSyncLease(ctx, instID, holder)
			if syncErr != nil {
				s.log.Error("reconcile failed", "instance_id", instID, "forge", t.inst.ForgeType, "err", syncErr)
			}
		}()
	}
	wg.Wait()
}
