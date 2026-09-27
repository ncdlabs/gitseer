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

func TestRunnerUtilizationFlakyReleasesAndOrgScope(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "product.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.26", `{"runners_api":true}`)
	if err != nil {
		t.Fatal(err)
	}
	org, err := st.UpsertOrganization(ctx, inst.ID, models.Organization{
		ExternalID: 1, Name: "acme", FullName: "Acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	repoA, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 10, Owner: "acme", Name: "widgets",
		FullName: "acme/widgets", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE repositories SET org_id = ? WHERE id = ?`, org.ID, repoA.ID); err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 11, Owner: "other", Name: "tools",
		FullName: "other/tools", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	runDeploy, err := st.UpsertWorkflowRun(ctx, repoA.ID, models.WorkflowRun{
		ExternalID: 100, Name: "Deploy prod", WorkflowPath: ".gitea/workflows/deploy.yaml",
		Event: "push", Branch: "main", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
		StartedAt: &now, CompletedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	runCI, err := st.UpsertWorkflowRun(ctx, repoA.ID, models.WorkflowRun{
		ExternalID: 101, Name: "CI", WorkflowPath: ".gitea/workflows/ci.yaml",
		Event: "push", Branch: "main", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
		StartedAt: &now, CompletedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repoA.ID, runCI.ID, models.Job{
		ExternalID: 1, Name: "test",
		Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
		RunnerName: "runner-1", StartedAt: &now, CompletedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repoA.ID, runCI.ID, models.Job{
		ExternalID: 2, Name: "test",
		Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
		RunnerName: "runner-1", StartedAt: &now, CompletedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repoA.ID, runDeploy.ID, models.Job{
		ExternalID: 3, Name: "deploy",
		Status: models.StatusRunning, RunnerName: "runner-1", StartedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}

	util, err := st.RunnerUtilization(ctx, 0, true, now.Add(-48*time.Hour), 20)
	if err != nil {
		t.Fatal(err)
	}
	if !util.RunnersAPICapable {
		t.Fatal("expected runners_api_capable from instance capabilities")
	}
	if len(util.Items) == 0 || util.Items[0].RunnerName != "runner-1" {
		t.Fatalf("utilization=%+v", util)
	}
	if util.Items[0].BusyJobs < 1 {
		t.Fatalf("expected busy job, got %+v", util.Items[0])
	}

	flaky, err := st.FlakyJobs(ctx, 0, true, repoA.ID, now.Add(-48*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(flaky) != 1 || flaky[0].JobName != "test" || flaky[0].FlipCount < 2 {
		t.Fatalf("flaky=%+v", flaky)
	}

	releases, err := st.ListReleaseRuns(ctx, 0, true, now.Add(-48*time.Hour), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].ID != runDeploy.ID {
		t.Fatalf("releases=%+v", releases)
	}

	sumAll, err := st.Summary(ctx, 0, true, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	sumOrg, err := st.SummaryScoped(ctx, 0, true, nil, true, store.DashboardScope{OrgID: org.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sumAll.Repositories < 2 {
		t.Fatalf("expected >=2 repos, got %d", sumAll.Repositories)
	}
	if sumOrg.Repositories != 1 {
		t.Fatalf("org-scoped repos=%d", sumOrg.Repositories)
	}
	sumOwner, err := st.SummaryScoped(ctx, 0, true, nil, true, store.DashboardScope{Owner: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if sumOwner.Repositories != 1 {
		t.Fatalf("owner-scoped repos=%d", sumOwner.Repositories)
	}
}

func TestWallboardTokenRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "wallboard.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	tok, secret, err := st.CreateWallboardToken(ctx, "Lobby", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tok.ID == 0 || secret == "" || tok.TokenPrefix == "" {
		t.Fatalf("tok=%+v secret empty=%v", tok, secret == "")
	}
	got, err := st.LookupWallboardTokenByPlain(ctx, secret)
	if err != nil || got.ID != tok.ID {
		t.Fatalf("lookup err=%v got=%+v", err, got)
	}
	if err := st.RevokeWallboardToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.LookupWallboardTokenByPlain(ctx, secret); err == nil {
		t.Fatal("expected revoked token to fail")
	}
}
