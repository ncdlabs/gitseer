// Package sync orchestrates forge discovery, initial import, and reconciliation.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/attention"
	"github.com/ncdlabs/gitea-lens/internal/config"
	"github.com/ncdlabs/gitea-lens/internal/forge"
	"github.com/ncdlabs/gitea-lens/internal/forge/gitea"
	lensmetrics "github.com/ncdlabs/gitea-lens/internal/metrics"
	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/realtime"
	"github.com/ncdlabs/gitea-lens/internal/store"
)

type Result struct {
	InstanceID           int64  `json:"instance_id"`
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

// GiteaSource supplies effective Gitea URL/token/private-network settings.
type GiteaSource interface {
	GiteaConnection() (url, token string, allowPrivate bool)
}

// Service is the full sync orchestrator.
type Service struct {
	store *store.Store
	att   *attention.Engine
	hub   *realtime.Hub
	cfg   config.Config
	prefs PrefsSource
	gitea GiteaSource
	log   *slog.Logger
}

func NewService(st *store.Store, att *attention.Engine, hub *realtime.Hub, cfg config.Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: st, att: att, hub: hub, cfg: cfg, log: log}
}

// SetPrefsSource attaches runtime overrides for instance name / history days.
func (s *Service) SetPrefsSource(p PrefsSource) {
	s.prefs = p
}

// SetGiteaSource attaches runtime Gitea connection overrides (settings manager).
func (s *Service) SetGiteaSource(g GiteaSource) {
	s.gitea = g
}

func (s *Service) giteaConn() (url, token string, allowPrivate bool) {
	if s.gitea != nil {
		return s.gitea.GiteaConnection()
	}
	return s.cfg.Gitea.URL, s.cfg.Gitea.Token, s.cfg.Gitea.AllowPrivateNetwork
}

func (s *Service) forgeFromConfig() (forge.Forge, error) {
	url, token, allowPrivate := s.giteaConn()
	if url == "" || token == "" {
		return nil, fmt.Errorf("gitea.url and token required")
	}
	return gitea.New(url, token, allowPrivate)
}

func (s *Service) SyncFromConfig(ctx context.Context) (*Result, error) {
	f, err := s.forgeFromConfig()
	if err != nil {
		return nil, err
	}
	url, _, _ := s.giteaConn()
	name := s.cfg.UI.InstanceName
	days := s.cfg.Sync.HistoryDays
	if s.prefs != nil {
		p := s.prefs.SyncPrefs()
		if p.InstanceName != "" {
			name = p.InstanceName
		}
		if p.SyncHistoryDays > 0 {
			days = p.SyncHistoryDays
		}
	}
	return s.FullSync(ctx, f, name, url, days)
}

const maxJobFetchesPerRepo = 20

func (s *Service) FullSync(ctx context.Context, f forge.Forge, instanceName, baseURL string, historyDays int) (*Result, error) {
	started := time.Now()
	res, err := s.fullSync(ctx, f, instanceName, baseURL, historyDays)
	lensmetrics.ObserveSync(started, err)
	return res, err
}

func (s *Service) fullSync(ctx context.Context, f forge.Forge, instanceName, baseURL string, historyDays int) (*Result, error) {
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
		instanceName = "Gitea"
	}
	inst, err := s.store.UpsertInstanceByURL(ctx, instanceName, baseURL, info.Version, string(capsJSON))
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
		s.log.Warn("repo sync returned zero repositories; skipping soft-delete")
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
				if jobsFetched < maxJobFetchesPerRepo {
					jobs, err := f.ListJobs(ctx, ref, saved.ExternalID)
					jobsFetched++
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

		// Fill PR ci_state gaps from indexed runs when commit-status was empty.
		s.refreshOpenPRCIFromRuns(ctx, repo.ID)
	}

	_ = s.store.SetSyncState(ctx, inst.ID, "instance", "complete", "")
	s.log.Info("full sync complete", "repos", repoCount, "prs", prCount, "runs", runCount, "jobs", jobCount)
	return &Result{
		InstanceID:           inst.ID,
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
			ok, err := s.store.TryAcquireSyncLease(ctx, "lens", interval)
			if err != nil || !ok {
				continue
			}
			url, token, _ := s.giteaConn()
			if url == "" || token == "" {
				continue
			}
			if _, err := s.SyncFromConfig(ctx); err != nil {
				s.log.Error("reconcile failed", "err", err)
			}
		}
	}
}
