package sync_test

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/config"
	"github.com/ncdlabs/gitea-lens/internal/database"
	"github.com/ncdlabs/gitea-lens/internal/forge"
	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/store"
	"github.com/ncdlabs/gitea-lens/internal/sync"
)

type mockForge struct {
	repos []models.Repository
}

func (m *mockForge) GetInstance(context.Context) (*models.InstanceInfo, error) {
	return &models.InstanceInfo{Version: "1.25.5"}, nil
}

func (m *mockForge) DetectCapabilities(context.Context) (*models.Capabilities, error) {
	return &models.Capabilities{Version: "1.25.5", ActionsAPI: false}, nil
}

func (m *mockForge) ListOrganizations(context.Context) ([]models.Organization, error) {
	return nil, nil
}

func (m *mockForge) ListRepositories(_ context.Context, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	if opts.Page > 1 {
		return forge.Page[models.Repository]{Page: opts.Page, HasMore: false}, nil
	}
	return forge.Page[models.Repository]{Items: m.repos, Page: 1, HasMore: false}, nil
}

func (m *mockForge) GetRepository(context.Context, string, string) (*models.Repository, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockForge) ListPullRequests(context.Context, models.RepoRef, forge.PROpts) (forge.Page[models.PullRequest], error) {
	return forge.Page[models.PullRequest]{}, nil
}

func (m *mockForge) GetCombinedCommitStatus(context.Context, models.RepoRef, string) (string, error) {
	return "", nil
}

func (m *mockForge) ListWorkflowRuns(context.Context, models.RepoRef, forge.RunOpts) (forge.Page[models.WorkflowRun], error) {
	return forge.Page[models.WorkflowRun]{}, nil
}

func (m *mockForge) ListJobs(context.Context, models.RepoRef, int64) ([]models.Job, error) {
	return nil, nil
}

func (m *mockForge) GetJobLogs(context.Context, models.RepoRef, int64) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockForge) GetWorkflowYAML(context.Context, models.RepoRef, string, string) ([]byte, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockForge) ListAccessibleReposForUser(context.Context, string, forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return forge.Page[models.Repository]{}, nil
}

func (m *mockForge) GetAuthenticatedUser(context.Context, string) (*models.User, error) {
	return nil, fmt.Errorf("not found")
}

func setupSync(t *testing.T) (*sync.Service, *store.Store) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "sync.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.NewWithDriver(db, "sqlite")
	svc := sync.NewService(st, nil, nil, config.Default(), nil)
	return svc, st
}

func TestFullSyncSkipsSoftDeleteOnEmptyCatalog(t *testing.T) {
	ctx := context.Background()
	svc, st := setupSync(t)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25.5", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 42, Owner: "org", Name: "keep", FullName: "org/keep", DefaultBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}

	f := &mockForge{repos: nil}
	if _, err := svc.FullSync(ctx, f, "lab", "https://git.example.com", 30); err != nil {
		t.Fatal(err)
	}

	list, total, err := st.ListRepositories(ctx, store.ListRepositoriesOpts{InstanceID: inst.ID, BootstrapAll: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].FullName != "org/keep" {
		t.Fatalf("empty sync must not soft-delete catalog: total=%d list=%v", total, list)
	}
}

func TestFullSyncSoftDeletesMissingRepos(t *testing.T) {
	ctx := context.Background()
	svc, st := setupSync(t)

	fSeed := &mockForge{repos: []models.Repository{
		{ExternalID: 1, Owner: "org", Name: "a", FullName: "org/a", DefaultBranch: "main"},
		{ExternalID: 2, Owner: "org", Name: "b", FullName: "org/b", DefaultBranch: "main"},
	}}
	if _, err := svc.FullSync(ctx, fSeed, "lab", "https://git.example.com", 30); err != nil {
		t.Fatal(err)
	}

	// Second sync only returns repo 1 — repo 2 must soft-delete.
	time.Sleep(5 * time.Millisecond)
	fNext := &mockForge{repos: []models.Repository{
		{ExternalID: 1, Owner: "org", Name: "a", FullName: "org/a", DefaultBranch: "main"},
	}}
	if _, err := svc.FullSync(ctx, fNext, "lab", "https://git.example.com", 30); err != nil {
		t.Fatal(err)
	}

	inst, err := st.GetPrimaryInstance(ctx)
	if err != nil || inst == nil {
		t.Fatalf("instance: %v", err)
	}
	list, total, err := st.ListRepositories(ctx, store.ListRepositoriesOpts{InstanceID: inst.ID, BootstrapAll: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].ExternalID != 1 {
		t.Fatalf("expected only org/a alive, got total=%d list=%v", total, list)
	}
}

func TestSyncFromConfigRequiresGitea(t *testing.T) {
	svc, _ := setupSync(t)
	if _, err := svc.SyncFromConfig(context.Background()); err == nil {
		t.Fatal("expected error without gitea url/token")
	}
}

func TestTryAcquireSyncLeaseGatesReconcile(t *testing.T) {
	ctx := context.Background()
	_, st := setupSync(t)
	ok, err := st.TryAcquireSyncLease(ctx, "lens-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	ok2, err := st.TryAcquireSyncLease(ctx, "lens-b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok2 {
		t.Fatal("second holder must not acquire while lease is live")
	}
}
