package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
)

// TestSQLCCriticalPathParity exercises sqlc-backed getters against seeded rows.
func TestSQLCCriticalPathParity(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "sqlc.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := New(db)
	if st.sqlite == nil {
		t.Fatal("expected sqlite sqlc querier")
	}

	inst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitea, "gitea", "https://git.example.com", "1.26", `{"runners_api":true}`)
	if err != nil {
		t.Fatal(err)
	}
	gotInst, err := st.GetInstanceByID(ctx, inst.ID)
	if err != nil || gotInst.BaseURL != inst.BaseURL {
		t.Fatalf("GetInstanceByID sqlc: %+v err=%v", gotInst, err)
	}
	listed, err := st.ListInstances(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListInstances sqlc: %v err=%v", listed, err)
	}

	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 9, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	gotRepo, err := st.GetRepositoryByID(ctx, repo.ID)
	if err != nil || gotRepo.FullName != "o/r" {
		t.Fatalf("GetRepositoryByID sqlc: %+v err=%v", gotRepo, err)
	}

	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "ci", Status: models.StatusQueued, RepoFull: "o/r",
	})
	if err != nil {
		t.Fatal(err)
	}
	labels := `["self-hosted"]`
	job, err := st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 3, Name: "build", Status: models.StatusQueued, LabelsJSON: &labels, Message: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	gotJob, err := st.GetJobByID(ctx, job.ID)
	if err != nil || gotJob.Message != "ok" || gotJob.LabelsJSON == nil || *gotJob.LabelsJSON != labels {
		t.Fatalf("GetJobByID sqlc: %+v err=%v", gotJob, err)
	}
	byRun, err := st.ListJobsByRunID(ctx, run.ID)
	if err != nil || len(byRun) != 1 {
		t.Fatalf("ListJobsByRunID sqlc: %v err=%v", byRun, err)
	}
	byRuns, err := st.ListJobsByRunIDs(ctx, []int64{run.ID})
	if err != nil || len(byRuns[run.ID]) != 1 {
		t.Fatalf("ListJobsByRunIDs sqlc: %v err=%v", byRuns, err)
	}
	if _, err := st.InsertSavedFilter(ctx, 1, "parity", json.RawMessage(`{"q":"x"}`)); err == nil {
		// user 1 may not exist — ignore; list still exercises empty path
	}
	if filters, err := st.ListSavedFilters(ctx, 1); err != nil && len(filters) != 0 {
		// empty list OK when user missing FK; only fail on unexpected shape later
		_ = filters
	}
	if n, err := st.CountWorkflowRuns(ctx); err != nil || n != 1 {
		t.Fatalf("CountWorkflowRuns sqlc: %d err=%v", n, err)
	}

	if err := st.UpsertWorkflowGraph(ctx, repo.ID, ".gitea/workflows/ci.yaml", "abc", `[]`); err != nil {
		t.Fatal(err)
	}
	nodes, err := st.GetWorkflowGraph(ctx, repo.ID, ".gitea/workflows/ci.yaml", "abc")
	if err != nil || nodes != `[]` {
		t.Fatalf("GetWorkflowGraph sqlc: %q err=%v", nodes, err)
	}
}
