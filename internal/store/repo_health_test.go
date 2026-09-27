package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestRepoHealthSummariesAndFailureClusters(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "health.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.26", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "acme", Name: "widgets", FullName: "acme/widgets", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 2, Owner: "acme", Name: "ok", FullName: "acme/ok", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	completed := now.Add(-time.Hour)
	failRun, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 10, Name: "CI", Branch: "main", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, WorkflowPath: ".gitea/workflows/ci.yaml",
		CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}
	okRun, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 11, Name: "CI", Branch: "feature", Status: models.StatusCompleted,
		Conclusion: models.ConclusionSuccess, WorkflowPath: ".gitea/workflows/ci.yaml",
		CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = okRun

	_, err = st.UpsertJob(ctx, repo.ID, failRun.ID, models.Job{
		ExternalID: 100, Name: "test", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, failRun.ID, models.Job{
		ExternalID: 101, Name: "test", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, failRun.ID, models.Job{
		ExternalID: 102, Name: "lint", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}

	staleUpdated := now.Add(-20 * 24 * time.Hour)
	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 7, Title: "old", State: "open", UpdatedAt: &staleUpdated, CreatedAt: &staleUpdated,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "failed_default_branch_workflow",
		Severity: "critical", EntityType: "workflow_run", EntityID: failRun.ID,
		Title: "Failed default-branch", Fingerprint: "fp-health-crit",
	})
	if err != nil {
		t.Fatal(err)
	}

	summaries, err := st.RepoHealthSummaries(ctx, []int64{repo.ID, healthy.ID}, store.RepoHealthOpts{
		WindowDays: 7, StalePRDays: 14,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := summaries[repo.ID]
	if !h.FailingDefaultBranch {
		t.Fatalf("expected failing default branch: %+v", h)
	}
	if h.FailingDefaultBranchRun == nil || *h.FailingDefaultBranchRun != failRun.ID {
		t.Fatalf("failing run id=%v want %d", h.FailingDefaultBranchRun, failRun.ID)
	}
	if h.OpenCriticalAttention != 1 {
		t.Fatalf("critical=%d", h.OpenCriticalAttention)
	}
	if h.StaleOpenPRs != 1 {
		t.Fatalf("stale=%d", h.StaleOpenPRs)
	}
	if h.CIRunsInWindow < 1 || h.CIFailuresInWindow < 1 {
		t.Fatalf("ci window runs=%d failures=%d", h.CIRunsInWindow, h.CIFailuresInWindow)
	}
	if h.Grade == models.HealthGradeHealthy || h.Score >= 80 {
		t.Fatalf("expected unhealthy score/grade: %+v", h)
	}

	ok := summaries[healthy.ID]
	if ok.FailingDefaultBranch || ok.OpenCriticalAttention != 0 || ok.Grade != models.HealthGradeHealthy {
		t.Fatalf("healthy repo unexpected: %+v", ok)
	}

	clusters, err := st.FailureClusters(ctx, repo.ID, now.Add(-48*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) < 2 {
		t.Fatalf("clusters=%d want >=2", len(clusters))
	}
	byJob := map[string]int{}
	for _, c := range clusters {
		byJob[c.JobName] = c.FailureCount
		if c.WorkflowPath == "" {
			t.Fatalf("missing workflow path: %+v", c)
		}
	}
	if byJob["test"] < 2 {
		t.Fatalf("test failures=%d", byJob["test"])
	}
	if byJob["lint"] < 1 {
		t.Fatalf("lint failures=%d", byJob["lint"])
	}
}
