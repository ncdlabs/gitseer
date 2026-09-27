package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestUpsertRepositoryIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "store.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.26.4", `{"version":"1.26.4"}`)
	if err != nil {
		t.Fatal(err)
	}
	if inst.ID == 0 {
		t.Fatal("expected gitseer instance id")
	}

	r1, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 100, Owner: "org", Name: "repo", FullName: "org/repo", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r1.ID == 0 || r1.ExternalID != 100 {
		t.Fatalf("ids gitseer=%d external=%d", r1.ID, r1.ExternalID)
	}

	r2, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 100, Owner: "org", Name: "repo-renamed", FullName: "org/repo-renamed", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r2.ID != r1.ID {
		t.Fatalf("gitseer id changed on upsert: %d vs %d", r1.ID, r2.ID)
	}
	if r2.Name != "repo-renamed" {
		t.Fatalf("name=%s", r2.Name)
	}

	if err := st.SoftDeleteMissing(ctx, inst.ID, []int64{999999}, time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	list, total, err := st.ListRepositories(ctx, store.ListRepositoriesOpts{InstanceID: inst.ID, BootstrapAll: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(list) != 0 {
		t.Fatalf("expected soft-deleted hidden, got total=%d", total)
	}
	// Empty seen must not wipe remaining catalog.
	if err := st.SoftDeleteMissing(ctx, inst.ID, nil, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestPullRequestCIState(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "pr-ci.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25.5", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "org", Name: "repo", FullName: "org/repo",
	})
	if err != nil {
		t.Fatal(err)
	}

	pr, err := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 10, Number: 7, Title: "feat", State: "open", HeadSHA: "abc123",
		CIState: "success", AuthorLogin: "lou",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.CIState != "success" {
		t.Fatalf("ci_state=%q", pr.CIState)
	}

	// Empty ci_state on upsert must preserve existing value when head_sha unchanged.
	pr, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 10, Number: 7, Title: "feat", State: "open", HeadSHA: "abc123",
		AuthorLogin: "lou",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.CIState != "success" {
		t.Fatalf("preserved ci_state=%q", pr.CIState)
	}

	// New head SHA with empty ci_state clears prior status.
	pr, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 10, Number: 7, Title: "feat", State: "open", HeadSHA: "def456",
		AuthorLogin: "lou",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.CIState != "" {
		t.Fatalf("cleared ci_state=%q", pr.CIState)
	}

	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 99, Name: "ci", CommitSHA: "def456", Status: "completed", Conclusion: "failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetPullRequestsCIStateByHeadSHA(ctx, repo.ID, "def456", "failure"); err != nil {
		t.Fatal(err)
	}
	pr, err = st.GetPullRequestByNumber(ctx, repo.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if pr.CIState != "failure" {
		t.Fatalf("updated ci_state=%q", pr.CIState)
	}

	runs, err := st.ListWorkflowRunsByCommitSHA(ctx, repo.ID, "def456")
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%d err=%v", len(runs), err)
	}
}

func TestUpsertPreservesNilTimestampsAndRejectsStale(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "ts.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	inst, _ := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	repo, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r"})

	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	pr, err := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 1, Title: "a", State: "open", CreatedAt: &created, UpdatedAt: &updated,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.CreatedAt == nil || !pr.CreatedAt.Equal(created) {
		t.Fatalf("created=%v", pr.CreatedAt)
	}

	// Nil created_at must not wipe existing; title update carries matching updated_at.
	pr, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 1, Title: "b", State: "open", UpdatedAt: &updated,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Title != "b" || pr.CreatedAt == nil || !pr.CreatedAt.Equal(created) {
		t.Fatalf("title=%s created=%v", pr.Title, pr.CreatedAt)
	}

	// Incoming without updated_at must not overwrite a fresher row.
	pr, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 1, Title: "no-ts", State: "open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Title != "b" {
		t.Fatalf("nil updated_at overwrite title=%s", pr.Title)
	}

	// Stale updated_at must not overwrite.
	stale := updated.Add(-time.Hour)
	pr, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 1, Title: "stale", State: "open", UpdatedAt: &stale,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Title != "b" {
		t.Fatalf("stale overwrite title=%s", pr.Title)
	}

	started := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 5, Name: "ci", Status: models.StatusRunning, RunAttempt: 1, StartedAt: &started,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 5, Name: "ci", Status: models.StatusQueued, RunAttempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = st.GetWorkflowRunByExternalID(ctx, repo.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != models.StatusRunning {
		t.Fatalf("regressed status=%s", run.Status)
	}
	if run.StartedAt == nil || !run.StartedAt.Equal(started) {
		t.Fatalf("started wiped: %v", run.StartedAt)
	}

	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 5, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess, RunAttempt: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = st.GetWorkflowRunByExternalID(ctx, repo.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if run.RunAttempt != 2 || run.Status != models.StatusCompleted {
		t.Fatalf("attempt=%d status=%s", run.RunAttempt, run.Status)
	}

	completed := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 5, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
		RunAttempt: 2, CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Higher attempt re-run must clear completed_at when back to running.
	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 5, Name: "ci", Status: models.StatusRunning, RunAttempt: 3, StartedAt: &started,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = st.GetWorkflowRunByExternalID(ctx, repo.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if run.RunAttempt != 3 || run.Status != models.StatusRunning {
		t.Fatalf("rerun attempt=%d status=%s", run.RunAttempt, run.Status)
	}
	if run.CompletedAt != nil {
		t.Fatalf("rerun kept completed_at=%v", run.CompletedAt)
	}
}

func TestSoftDeleteMissingRespectsSyncStart(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "race.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	inst, _ := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	old, err := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 1, Owner: "o", Name: "old", FullName: "o/old"})
	if err != nil {
		t.Fatal(err)
	}
	syncStart := time.Now().UTC().Add(time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	// Simulate webhook mid-sync after syncStart.
	webhookRepo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 2, Owner: "o", Name: "new", FullName: "o/new"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SoftDeleteMissing(ctx, inst.ID, []int64{old.ExternalID}, syncStart); err != nil {
		t.Fatal(err)
	}
	alive, err := st.ListAllAliveRepos(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(alive) != 2 {
		t.Fatalf("expected old+webhook to survive when old is seen; got %d", len(alive))
	}
	// Old gone from forge; webhook-created (newer stamp) survives even when unseen.
	if err := st.SoftDeleteMissing(ctx, inst.ID, []int64{999}, syncStart); err != nil {
		t.Fatal(err)
	}
	alive, err = st.ListAllAliveRepos(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(alive) != 1 || alive[0].ID != webhookRepo.ID {
		t.Fatalf("expected only webhook repo, got %+v", alive)
	}
}

func TestCloseOpenPRsNotInSet(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "prs.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	inst, _ := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	repo, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r"})
	_, _ = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{ExternalID: 1, Number: 1, Title: "keep", State: "open"})
	_, _ = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{ExternalID: 2, Number: 2, Title: "gone", State: "open"})
	n, err := st.CloseOpenPRsNotInSet(ctx, repo.ID, []int64{1})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	pr, _ := st.GetPullRequestByNumber(ctx, repo.ID, 2)
	if pr.State != "closed" {
		t.Fatalf("state=%s", pr.State)
	}
}

func TestWebhookReaper(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "wh.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	inst, _ := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	id, ok, err := st.InsertWebhookEvent(ctx, inst.ID, "d1", "ping", `{}`)
	if err != nil || !ok {
		t.Fatalf("insert id=%d ok=%v err=%v", id, ok, err)
	}
	claimed, err := st.ClaimPendingWebhooks(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claimed=%d err=%v", len(claimed), err)
	}
	// Fresh claim must not reap even if received_at is old (backlog case).
	_, err = db.ExecContext(ctx, `UPDATE webhook_events SET received_at=? WHERE id=?`,
		time.Now().UTC().Add(-10*time.Minute).Format(time.RFC3339Nano), claimed[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.ReapStaleProcessingWebhooks(ctx, 5*time.Minute)
	if err != nil || n != 0 {
		t.Fatalf("expected no reap on fresh claim, reaped=%d err=%v", n, err)
	}
	// Stale processing_started_at should reap.
	_, err = db.ExecContext(ctx, `UPDATE webhook_events SET processing_started_at=? WHERE id=?`,
		time.Now().UTC().Add(-10*time.Minute).Format(time.RFC3339Nano), claimed[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	n, err = st.ReapStaleProcessingWebhooks(ctx, 5*time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("reaped=%d err=%v", n, err)
	}
	again, err := st.ClaimPendingWebhooks(ctx, 10)
	if err != nil || len(again) != 1 {
		t.Fatalf("reclaim=%d err=%v", len(again), err)
	}
}

func TestClaimPendingWebhooksRetriesError(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "wh-retry.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	inst, _ := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	id, ok, err := st.InsertWebhookEvent(ctx, inst.ID, "d-retry", "push", `{}`)
	if err != nil || !ok {
		t.Fatalf("insert id=%d ok=%v err=%v", id, ok, err)
	}
	claimed, err := st.ClaimPendingWebhooks(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%d err=%v", len(claimed), err)
	}
	if err := st.MarkWebhookProcessed(ctx, claimed[0].ID, "apply failed"); err != nil {
		t.Fatal(err)
	}
	// Fresh error is within backoff window — should not reclaim yet.
	none, err := st.ClaimPendingWebhooks(ctx, 10)
	if err != nil || len(none) != 0 {
		t.Fatalf("immediate reclaim=%d err=%v", len(none), err)
	}
	_, err = db.ExecContext(ctx, `UPDATE webhook_events SET processed_at=? WHERE id=?`,
		time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339Nano), claimed[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := st.ClaimPendingWebhooks(ctx, 10)
	if err != nil || len(retried) != 1 {
		t.Fatalf("retry claim=%d err=%v", len(retried), err)
	}
}

func TestSummaryTimeRange(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "summary.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "org", Name: "repo", FullName: "org/repo", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	old := now.AddDate(0, 0, -14)
	recent := now.AddDate(0, 0, -2)

	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 1, Title: "old", State: "open", CreatedAt: &old, UpdatedAt: &old,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 2, Number: 2, Title: "recent", State: "open", CreatedAt: &recent, UpdatedAt: &recent,
	})
	if err != nil {
		t.Fatal(err)
	}

	oldFail, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 10, Name: "old-fail", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
		StartedAt: &old, CompletedAt: &old,
	})
	if err != nil {
		t.Fatal(err)
	}
	recentFail, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 11, Name: "recent-fail", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
		StartedAt: &recent, CompletedAt: &recent,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 12, Name: "running", Status: models.StatusRunning, Conclusion: models.ConclusionUnknown,
		StartedAt: &recent,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "workflow_failure", Severity: "critical",
		EntityType: "workflow_run", EntityID: recentFail.ID, Title: "fail", Fingerprint: "fp-recent",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO attention_items (
  instance_id, repo_id, type, severity, entity_type, entity_id, title, metadata_json, fingerprint, opened_at, resolved_at, updated_at
) VALUES (?, ?, 'workflow_failure', 'critical', 'workflow_run', ?, 'old fail', '{}', 'fp-old', ?, ?, ?)`,
		inst.ID, repo.ID, oldFail.ID, old.Format(time.RFC3339Nano), old.Format(time.RFC3339Nano), old.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}

	all, err := st.Summary(ctx, 0, true, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if all.Repositories != 1 || all.OpenPRs != 2 || all.Attention != 1 || all.FailedRuns != 2 || all.RunningRuns != 1 {
		t.Fatalf("all-time summary = %+v", all)
	}

	since7 := now.AddDate(0, 0, -7)
	week, err := st.Summary(ctx, 0, true, &since7, false)
	if err != nil {
		t.Fatal(err)
	}
	if week.Repositories != 1 {
		t.Fatalf("repos should stay inventory count, got %d", week.Repositories)
	}
	if week.OpenPRs != 1 || week.Attention != 1 || week.FailedRuns != 1 || week.RunningRuns != 1 {
		t.Fatalf("7d summary = %+v", week)
	}
	if week.Since == "" {
		t.Fatal("expected since timestamp")
	}

	snap, err := st.Summary(ctx, 0, true, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repositories != 1 || snap.OpenPRs != 2 || snap.Attention != 1 || snap.RunningRuns != 1 {
		t.Fatalf("snapshot summary = %+v", snap)
	}
	// Snapshot failed runs only count failures with open attention, not historical volume.
	if snap.FailedRuns != 1 {
		t.Fatalf("snapshot failed_runs = %d, want 1 (attention-linked only)", snap.FailedRuns)
	}
	if snap.Since != "" {
		t.Fatalf("snapshot should omit since, got %q", snap.Since)
	}
}

func TestStatsReportBucketingAndPercentiles(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "stats.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "org", Name: "repo", FullName: "org/repo", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	day0 := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	day2 := day0.AddDate(0, 0, -2)
	day4 := day0.AddDate(0, 0, -4)
	old := day0.AddDate(0, 0, -20)

	seedRun := func(ext int64, name, status, conclusion string, started, completed time.Time) {
		t.Helper()
		stAt, doneAt := started, completed
		_, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
			ExternalID: ext, Name: name, Status: status, Conclusion: conclusion,
			StartedAt: &stAt, CompletedAt: &doneAt,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// day2: success 10s, failure 20s, cancelled
	seedRun(1, "ok", models.StatusCompleted, models.ConclusionSuccess, day2, day2.Add(10*time.Second))
	seedRun(2, "fail", models.StatusCompleted, models.ConclusionFailure, day2, day2.Add(20*time.Second))
	seedRun(3, "cancel", models.StatusCompleted, models.ConclusionCancelled, day2, day2.Add(5*time.Second))
	// day4: skipped (other) + success 40s
	seedRun(4, "skip", models.StatusCompleted, models.ConclusionSkipped, day4, day4.Add(1*time.Second))
	seedRun(5, "ok2", models.StatusCompleted, models.ConclusionSuccess, day4, day4.Add(40*time.Second))
	// outside window
	seedRun(6, "old", models.StatusCompleted, models.ConclusionFailure, old, old.Add(100*time.Second))

	opened := day2
	merged := day4
	closed := day2
	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 1, Title: "open-ok", State: "open", CIState: models.CIStateSuccess,
		CreatedAt: &opened, UpdatedAt: &opened,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 2, Number: 2, Title: "merged", State: "closed", CIState: models.CIStateFailure,
		CreatedAt: &opened, UpdatedAt: &merged, MergedAt: &merged, ClosedAt: &merged,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 3, Number: 3, Title: "closed-no-merge", State: "closed", CIState: models.CIStateCancelled,
		CreatedAt: &opened, UpdatedAt: &closed, ClosedAt: &closed,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 4, Number: 4, Title: "open-pending", State: "open", CIState: models.CIStatePending,
		CreatedAt: &day4, UpdatedAt: &day4,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "workflow_failure", Severity: "critical",
		EntityType: "workflow_run", EntityID: 2, Title: "fail", Fingerprint: "fp-crit",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "stale_pr", Severity: "warning",
		EntityType: "pull_request", EntityID: 1, Title: "stale", Fingerprint: "fp-warn",
	})
	if err != nil {
		t.Fatal(err)
	}
	// resolved should be excluded from open attention buckets
	_, err = db.ExecContext(ctx, `
INSERT INTO attention_items (
  instance_id, repo_id, type, severity, entity_type, entity_id, title, metadata_json, fingerprint, opened_at, resolved_at, updated_at
) VALUES (?, ?, 'workflow_failure', 'critical', 'workflow_run', 99, 'resolved', '{}', 'fp-resolved', ?, ?, ?)`,
		inst.ID, repo.ID, day2.Format(time.RFC3339Nano), day2.Format(time.RFC3339Nano), day2.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}

	since := day0.AddDate(0, 0, -7)
	rep, err := st.StatsReport(ctx, 0, true, since, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Since == "" {
		t.Fatal("expected since")
	}
	if len(rep.RunsByDay) < 8 {
		t.Fatalf("expected zero-filled days (>=8 for 7d window), got %d", len(rep.RunsByDay))
	}

	byDay := map[string]models.DayRunBucket{}
	for _, b := range rep.RunsByDay {
		byDay[b.Day] = b
	}
	d2 := day2.Format("2006-01-02")
	d4 := day4.Format("2006-01-02")
	if got := byDay[d2]; got.Success != 1 || got.Failure != 1 || got.Cancelled != 1 || got.Other != 0 {
		t.Fatalf("runs %s = %+v", d2, got)
	}
	if got := byDay[d4]; got.Success != 1 || got.Failure != 0 || got.Cancelled != 0 || got.Other != 1 {
		t.Fatalf("runs %s = %+v", d4, got)
	}
	// zero-fill: a quiet day in the middle should exist with zeros
	quiet := day0.AddDate(0, 0, -1).Format("2006-01-02")
	if got := byDay[quiet]; got.Success != 0 || got.Failure != 0 || got.Cancelled != 0 || got.Other != 0 {
		t.Fatalf("quiet day %s = %+v", quiet, got)
	}

	concl := map[string]int{}
	for _, b := range rep.RunConclusions {
		concl[b.Key] = b.Count
	}
	if concl[models.ConclusionSuccess] != 2 || concl[models.ConclusionFailure] != 1 ||
		concl[models.ConclusionCancelled] != 1 || concl[models.ConclusionSkipped] != 1 {
		t.Fatalf("conclusions=%v", concl)
	}

	if rep.RunDuration == nil || rep.RunDuration.SampleCount != 5 {
		t.Fatalf("duration=%+v", rep.RunDuration)
	}
	// durations: 10,20,5,1,40 → sorted 1,5,10,20,40; p50=10, p95≈36
	if rep.RunDuration.P50Seconds != 10 {
		t.Fatalf("p50=%v want 10", rep.RunDuration.P50Seconds)
	}
	if rep.RunDuration.P95Seconds < 36 || rep.RunDuration.P95Seconds > 40 {
		t.Fatalf("p95=%v want ~36-40", rep.RunDuration.P95Seconds)
	}

	prByDay := map[string]models.DayPRBucket{}
	for _, b := range rep.PRsByDay {
		prByDay[b.Day] = b
	}
	if got := prByDay[d2]; got.Opened != 3 || got.Merged != 0 || got.Closed != 1 {
		t.Fatalf("prs %s = %+v", d2, got)
	}
	if got := prByDay[d4]; got.Opened != 1 || got.Merged != 1 || got.Closed != 0 {
		t.Fatalf("prs %s = %+v", d4, got)
	}

	ci := map[string]int{}
	for _, b := range rep.PRCIStates {
		ci[b.Key] = b.Count
	}
	if ci[models.CIStateSuccess] != 1 || ci[models.CIStatePending] != 1 {
		t.Fatalf("open PR ci states=%v (closed PRs must be excluded)", ci)
	}

	sev := map[string]int{}
	for _, b := range rep.AttentionBySeverity {
		sev[b.Key] = b.Count
	}
	if sev["critical"] != 1 || sev["warning"] != 1 {
		t.Fatalf("attention severity=%v", sev)
	}
	types := map[string]int{}
	for _, b := range rep.AttentionByType {
		types[b.Key] = b.Count
	}
	if types["workflow_failure"] != 1 || types["stale_pr"] != 1 {
		t.Fatalf("attention type=%v", types)
	}
}

func TestStatsReportAuthzScope(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "stats-acl.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "org", Name: "allowed", FullName: "org/allowed", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	denied, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 2, Owner: "org", Name: "denied", FullName: "org/denied", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	instID := inst.ID
	giteaUID := int64(42)
	user, err := st.UpsertGiteaUser(ctx, &instID, models.User{
		GiteaUserID: &giteaUID, Login: "alice", Email: "a@example.com", DisplayName: "Alice",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceUserRepoAccess(ctx, user.ID, []int64{allowed.ID}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	recent := now.AddDate(0, 0, -1)
	for _, repoID := range []int64{allowed.ID, denied.ID} {
		_, err := st.UpsertWorkflowRun(ctx, repoID, models.WorkflowRun{
			ExternalID: 1, Name: "run", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
			StartedAt: &recent, CompletedAt: &recent,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = st.UpsertPullRequest(ctx, repoID, models.PullRequest{
			ExternalID: 1, Number: 1, Title: "pr", State: "open", CIState: models.CIStateFailure,
			CreatedAt: &recent, UpdatedAt: &recent,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = st.UpsertAttention(ctx, models.AttentionItem{
			InstanceID: inst.ID, RepoID: repoID, Type: "workflow_failure", Severity: "critical",
			EntityType: "workflow_run", EntityID: 1, Title: "fail",
			Fingerprint: "fp-acl-" + strconv.FormatInt(repoID, 10),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	since := now.AddDate(0, 0, -7)
	rep, err := st.StatsReport(ctx, user.ID, false, since, false)
	if err != nil {
		t.Fatal(err)
	}

	failCount := 0
	for _, b := range rep.RunsByDay {
		failCount += b.Failure
	}
	if failCount != 1 {
		t.Fatalf("expected 1 failure from allowed repo only, got %d", failCount)
	}
	conclFail := 0
	for _, b := range rep.RunConclusions {
		if b.Key == models.ConclusionFailure {
			conclFail = b.Count
		}
	}
	if conclFail != 1 {
		t.Fatalf("conclusions failure=%d", conclFail)
	}
	ciFail := 0
	for _, b := range rep.PRCIStates {
		if b.Key == models.CIStateFailure {
			ciFail = b.Count
		}
	}
	if ciFail != 1 {
		t.Fatalf("pr ci failure=%d", ciFail)
	}
	att := 0
	for _, b := range rep.AttentionBySeverity {
		att += b.Count
	}
	if att != 1 {
		t.Fatalf("attention open=%d", att)
	}

	// bootstrap sees both repos
	all, err := st.StatsReport(ctx, 0, true, since, false)
	if err != nil {
		t.Fatal(err)
	}
	allFail := 0
	for _, b := range all.RunsByDay {
		allFail += b.Failure
	}
	if allFail != 2 {
		t.Fatalf("bootstrap failures=%d want 2", allFail)
	}
}

func TestStatsBySectionIsolation(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "stats-section.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "org", Name: "repo", FullName: "org/repo", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	started := now.Add(-2 * time.Minute)
	completed := now.Add(-time.Minute)
	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
		StartedAt: &started, CompletedAt: &completed,
	})
	if err != nil {
		t.Fatal(err)
	}
	opened := now.Add(-time.Hour)
	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 1, Title: "open", State: "open", CIState: models.CIStatePending,
		CreatedAt: &opened, UpdatedAt: &opened,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "stale_pr", Severity: "warning",
		EntityType: "pull_request", EntityID: 1, Title: "stale", Fingerprint: "fp-section",
	})
	if err != nil {
		t.Fatal(err)
	}

	since := now.AddDate(0, 0, -7)

	core, err := st.StatsBySection(ctx, 0, true, since, false, store.StatsSectionCore)
	if err != nil {
		t.Fatal(err)
	}
	if core.Since == "" || len(core.PRCIStates) == 0 || len(core.AttentionBySeverity) == 0 {
		t.Fatalf("core = %+v", core)
	}
	if len(core.RunsByDay) != 0 || core.RunDuration != nil {
		t.Fatalf("core leaked trends/duration: %+v", core)
	}

	trends, err := st.StatsBySection(ctx, 0, true, since, false, store.StatsSectionTrends)
	if err != nil {
		t.Fatal(err)
	}
	if trends.Since == "" || len(trends.RunsByDay) == 0 || len(trends.RunConclusions) == 0 {
		t.Fatalf("trends = %+v", trends)
	}
	if len(trends.PRCIStates) != 0 || trends.RunDuration != nil {
		t.Fatalf("trends leaked core/duration: %+v", trends)
	}

	duration, err := st.StatsBySection(ctx, 0, true, since, false, store.StatsSectionDuration)
	if err != nil {
		t.Fatal(err)
	}
	if duration.Since == "" || duration.RunDuration == nil || duration.RunDuration.SampleCount != 1 {
		t.Fatalf("duration = %+v", duration)
	}
	if len(duration.RunsByDay) != 0 || len(duration.PRCIStates) != 0 {
		t.Fatalf("duration leaked trends/core: %+v", duration)
	}

	snap, err := st.StatsBySection(ctx, 0, true, since, true, store.StatsSectionCore)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Since != "" || len(snap.PRCIStates) == 0 {
		t.Fatalf("snapshot core = %+v", snap)
	}

	_, err = st.StatsBySection(ctx, 0, true, since, false, "nope")
	if err != store.ErrInvalidStatsSection {
		t.Fatalf("invalid section err=%v", err)
	}
}

func TestCompleteOrphanedWorkflowRun(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "orphan-run.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25.5", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "coThink", Name: "api", FullName: "coThink/api",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 12550, Name: "ci.yaml", Status: models.StatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 1, Name: "check", Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 2, Name: "Cursor autofix failed CI job", Status: models.StatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}

	closed, err := st.CompleteOrphanedWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != models.StatusCompleted || closed.Conclusion != models.ConclusionCancelled {
		t.Fatalf("run status=%s conclusion=%s", closed.Status, closed.Conclusion)
	}
	jobs, err := st.ListJobsByRunID(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs=%d", len(jobs))
	}
	for _, j := range jobs {
		if j.Status != models.StatusCompleted {
			t.Fatalf("job %s still %s", j.Name, j.Status)
		}
		if j.ExternalID == 2 && j.Conclusion != models.ConclusionCancelled {
			t.Fatalf("autofix conclusion=%s", j.Conclusion)
		}
	}
	inFlight, err := st.ListInFlightWorkflowRunsByRepo(ctx, repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inFlight) != 0 {
		t.Fatalf("expected no in-flight runs, got %d", len(inFlight))
	}
}

func TestListWorkflowRunsStatusesAndJobsByRunIDs(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "runs-active.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25.5", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "org", Name: "repo", FullName: "org/repo",
	})
	if err != nil {
		t.Fatal(err)
	}

	queued, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "ci-queued", Status: models.StatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	running, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 2, Name: "ci-running", Status: models.StatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 3, Name: "ci-done", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}

	active, total, err := st.ListWorkflowRuns(ctx, store.ListRunsOpts{
		BootstrapAll: true,
		Statuses:     []string{models.StatusQueued, models.StatusWaiting, models.StatusRunning},
		Limit:        50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(active) != 2 {
		t.Fatalf("active total=%d len=%d", total, len(active))
	}
	for _, run := range active {
		if run.Status == models.StatusCompleted {
			t.Fatalf("completed run leaked: %+v", run)
		}
	}

	single, total, err := st.ListWorkflowRuns(ctx, store.ListRunsOpts{
		BootstrapAll: true, Status: models.StatusCompleted, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(single) != 1 || single[0].Name != "ci-done" {
		t.Fatalf("single status filter: total=%d items=%+v", total, single)
	}

	empty, err := st.ListJobsByRunIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty runIDs: map=%v err=%v", empty, err)
	}

	jobA, err := st.UpsertJob(ctx, repo.ID, queued.ID, models.Job{
		ExternalID: 10, Name: "job-a", Status: models.StatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	jobB, err := st.UpsertJob(ctx, repo.ID, running.ID, models.Job{
		ExternalID: 11, Name: "job-b", Status: models.StatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, repo.ID, running.ID, models.Job{
		ExternalID: 12, Name: "job-c", Status: models.StatusWaiting,
	})
	if err != nil {
		t.Fatal(err)
	}

	byRun, err := st.ListJobsByRunIDs(ctx, []int64{queued.ID, running.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(byRun[queued.ID]) != 1 || byRun[queued.ID][0].ID != jobA.ID {
		t.Fatalf("queued jobs=%+v", byRun[queued.ID])
	}
	if len(byRun[running.ID]) != 2 {
		t.Fatalf("running jobs=%+v", byRun[running.ID])
	}
	_ = jobB
}

func TestUpsertGiteaUserRejectsBootstrapLogin(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "gitseer.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	if _, err := st.EnsureBootstrapUser(ctx); err != nil {
		t.Fatal(err)
	}
	uid := int64(99)
	_, err = st.UpsertGiteaUser(ctx, nil, models.User{GiteaUserID: &uid, Login: "bootstrap"}, nil)
	if !errors.Is(err, store.ErrReservedLogin) {
		t.Fatalf("err=%v", err)
	}
}

func TestUpsertGiteaUserLinkAttachesWithoutOrphan(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "gitseer-link.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	ghInst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitHub, "gh", "https://api.github.com", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	fjInst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeForgejo, "fj", "https://forgejo.example.com", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	ghUID := int64(501)
	target, err := st.UpsertGitHubUser(ctx, ghInst.ID, models.User{
		GitHubUserID: &ghUID, Login: "linker", Email: "l@example.com",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeCount := len(before)

	fjUID := int64(777)
	linkID := target.ID
	linked, err := st.UpsertGiteaUser(ctx, &fjInst.ID, models.User{
		GiteaUserID: &fjUID, Login: "forgejo-login", Email: "fj@example.com", DisplayName: "FJ",
	}, &linkID)
	if err != nil {
		t.Fatal(err)
	}
	if linked.ID != target.ID {
		t.Fatalf("linked id=%d want %d", linked.ID, target.ID)
	}
	if linked.GiteaUserID == nil || *linked.GiteaUserID != fjUID {
		t.Fatalf("gitea_user_id=%v", linked.GiteaUserID)
	}
	if linked.InstanceID == nil || *linked.InstanceID != fjInst.ID {
		t.Fatalf("instance_id=%v", linked.InstanceID)
	}
	after, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != beforeCount {
		t.Fatalf("user count=%d want %d (orphan insert)", len(after), beforeCount)
	}
}

func TestUpsertJobRejectsStatusRegression(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "gitseer.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	inst, err := st.UpsertInstanceByURL(ctx, "g", "https://git.example", "1.0", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "ci", Status: models.StatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := time.Now().UTC()
	job, err := st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 1, Name: "build", Status: models.StatusCompleted, CompletedAt: &done,
	})
	if err != nil {
		t.Fatal(err)
	}
	regressed, err := st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 1, Name: "build", Status: models.StatusQueued,
	})
	if err != nil {
		t.Fatal(err)
	}
	if regressed.Status != models.StatusCompleted || regressed.ID != job.ID {
		t.Fatalf("expected completed retained, got %+v", regressed)
	}
}

func TestListForgeTypeFilters(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "forge-filter.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	gitea, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitea, "Lab", "https://git.example.com", "1.0", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	github, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitHub, "GitHub", "https://api.github.com", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	gRepo, err := st.UpsertRepository(ctx, gitea.ID, models.Repository{
		ExternalID: 1, Owner: "org", Name: "gitea-repo", FullName: "org/gitea-repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	ghRepo, err := st.UpsertRepository(ctx, github.ID, models.Repository{
		ExternalID: 2, Owner: "org", Name: "gh-repo", FullName: "org/gh-repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertPullRequest(ctx, gRepo.ID, models.PullRequest{
		ExternalID: 1, Number: 1, Title: "gitea pr", State: "open",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertPullRequest(ctx, ghRepo.ID, models.PullRequest{
		ExternalID: 2, Number: 2, Title: "github pr", State: "open",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertWorkflowRun(ctx, gRepo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "gitea-ci", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertWorkflowRun(ctx, ghRepo.ID, models.WorkflowRun{
		ExternalID: 2, Name: "gh-ci", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	att, err := st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: gitea.ID, RepoID: gRepo.ID, Type: "workflow_failure", Severity: "critical",
		EntityType: "workflow_run", EntityID: 1, Title: "gitea fail", Fingerprint: "fp-gitea",
	})
	if err != nil {
		t.Fatal(err)
	}
	if att.ForgeType != models.ForgeTypeGitea || att.InstanceName != "Lab" {
		t.Fatalf("attention forge fields = type=%q name=%q", att.ForgeType, att.InstanceName)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: github.ID, RepoID: ghRepo.ID, Type: "workflow_failure", Severity: "critical",
		EntityType: "workflow_run", EntityID: 2, Title: "gh fail", Fingerprint: "fp-gh",
	})
	if err != nil {
		t.Fatal(err)
	}

	repos, total, err := st.ListRepositories(ctx, store.ListRepositoriesOpts{
		BootstrapAll: true, ForgeType: models.ForgeTypeGitHub,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(repos) != 1 || repos[0].FullName != "org/gh-repo" {
		t.Fatalf("repos forge filter: total=%d items=%v", total, repos)
	}

	prs, total, err := st.ListPullRequests(ctx, store.ListPRsOpts{
		BootstrapAll: true, State: "open", ForgeType: models.ForgeTypeGitea,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(prs) != 1 || prs[0].Title != "gitea pr" {
		t.Fatalf("prs forge filter: total=%d items=%v", total, prs)
	}

	runs, total, err := st.ListWorkflowRuns(ctx, store.ListRunsOpts{
		BootstrapAll: true, ForgeType: models.ForgeTypeGitHub,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(runs) != 1 || runs[0].Name != "gh-ci" {
		t.Fatalf("runs forge filter: total=%d items=%v", total, runs)
	}

	items, total, err := st.ListAttention(ctx, store.ListAttentionOpts{
		BootstrapAll: true, OpenOnly: true, ForgeType: models.ForgeTypeGitea,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].Title != "gitea fail" {
		t.Fatalf("attention forge filter: total=%d items=%v", total, items)
	}
	if items[0].ForgeType != models.ForgeTypeGitea || items[0].InstanceName != "Lab" {
		t.Fatalf("list attention forge fields = %+v", items[0])
	}

	byInst, total, err := st.ListRepositories(ctx, store.ListRepositoriesOpts{
		BootstrapAll: true, InstanceID: github.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(byInst) != 1 || byInst[0].FullName != "org/gh-repo" {
		t.Fatalf("repos instance_id filter: total=%d items=%v", total, byInst)
	}
}

func TestSearchMatchesFieldsAndACL(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "search.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1.25.5", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	org, err := st.UpsertOrganization(ctx, inst.ID, models.Organization{
		ExternalID: 7, Name: "acme-org", FullName: "Acme Org",
	})
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "acme-org", Name: "widget", FullName: "acme-org/widget",
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 2, Owner: "other", Name: "secret", FullName: "other/secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE repositories SET org_id = ? WHERE id = ?`, org.ID, allowed.ID); err != nil {
		t.Fatal(err)
	}

	_, err = st.UpsertPullRequest(ctx, allowed.ID, models.PullRequest{
		ExternalID: 11, Number: 42, Title: "Fix widget crash", AuthorLogin: "alice",
		SourceBranch: "fix/crash", TargetBranch: "main", State: "open",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertPullRequest(ctx, hidden.ID, models.PullRequest{
		ExternalID: 12, Number: 42, Title: "Hidden PR", AuthorLogin: "bob", State: "open",
	})
	if err != nil {
		t.Fatal(err)
	}

	run, err := st.UpsertWorkflowRun(ctx, allowed.ID, models.WorkflowRun{
		ExternalID: 21, Name: "ci", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
		Branch: "main", CommitSHA: "abcdef0123456789", ActorLogin: "alice", WorkflowPath: ".gitea/workflows/ci.yaml",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertJob(ctx, allowed.ID, run.ID, models.Job{
		ExternalID: 31, Name: "build-widget", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: allowed.ID, Type: "workflow_failure", Severity: "critical",
		EntityType: "workflow_run", EntityID: run.ID, Title: "Widget pipeline failed", Fingerprint: "fp-search-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: hidden.ID, Type: "workflow_failure", Severity: "critical",
		EntityType: "workflow_run", EntityID: 99, Title: "Secret pipeline failed", Fingerprint: "fp-search-2",
	})
	if err != nil {
		t.Fatal(err)
	}

	gid := int64(55)
	user, err := st.UpsertGiteaUser(ctx, &inst.ID, models.User{GiteaUserID: &gid, Login: "searcher"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceUserRepoAccess(ctx, user.ID, []int64{allowed.ID}); err != nil {
		t.Fatal(err)
	}

	res, err := st.Search(ctx, user.ID, false, "#42", 20)
	if err != nil {
		t.Fatal(err)
	}
	prs := res["pull_requests"].([]models.PullRequest)
	if len(prs) != 1 || prs[0].Title != "Fix widget crash" {
		t.Fatalf("PR number search: %+v", prs)
	}

	res, err = st.Search(ctx, user.ID, false, "alice", 20)
	if err != nil {
		t.Fatal(err)
	}
	prs = res["pull_requests"].([]models.PullRequest)
	if len(prs) != 1 {
		t.Fatalf("author search: %+v", prs)
	}

	res, err = st.Search(ctx, user.ID, false, "abcdef0", 20)
	if err != nil {
		t.Fatal(err)
	}
	runs := res["workflow_runs"].([]models.WorkflowRun)
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("SHA search: %+v", runs)
	}

	res, err = st.Search(ctx, user.ID, false, "build-widget", 20)
	if err != nil {
		t.Fatal(err)
	}
	runs = res["workflow_runs"].([]models.WorkflowRun)
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("job name search: %+v", runs)
	}

	res, err = st.Search(ctx, user.ID, false, "Widget pipeline", 20)
	if err != nil {
		t.Fatal(err)
	}
	atts := res["attention"].([]models.AttentionItem)
	if len(atts) != 1 || atts[0].Title != "Widget pipeline failed" {
		t.Fatalf("attention search: %+v", atts)
	}

	res, err = st.Search(ctx, user.ID, false, "acme", 20)
	if err != nil {
		t.Fatal(err)
	}
	orgs := res["organizations"].([]models.Organization)
	if len(orgs) != 1 || orgs[0].Name != "acme-org" {
		t.Fatalf("org search: %+v", orgs)
	}
	repos := res["repositories"].([]models.Repository)
	if len(repos) != 1 || repos[0].FullName != "acme-org/widget" {
		t.Fatalf("repo ACL search: %+v", repos)
	}
}

func TestUpsertBitbucketUserRemapsStaleIdentityHash(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "gitseer-bb-remap.db")
	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeBitbucket, "bb", "https://api.bitbucket.org/2.0", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	staleUID := int64(111)
	created, err := st.UpsertBitbucketUser(ctx, inst.ID, models.User{
		BitbucketUserID: &staleUID, Login: "bbuser", Email: "bb@example.com", DisplayName: "BB",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	canonicalUID := int64(222)
	updated, err := st.UpsertBitbucketUser(ctx, inst.ID, models.User{
		BitbucketUserID: &canonicalUID, Login: "bbuser", Email: "bb@example.com", DisplayName: "BB Updated",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID {
		t.Fatalf("expected same user row remapped, got id=%d want=%d", updated.ID, created.ID)
	}
	if updated.BitbucketUserID == nil || *updated.BitbucketUserID != canonicalUID {
		t.Fatalf("bitbucket_user_id=%v want %d", updated.BitbucketUserID, canonicalUID)
	}
	if updated.DisplayName != "BB Updated" {
		t.Fatalf("display_name=%q", updated.DisplayName)
	}

	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bbCount int
	for _, u := range users {
		if u.BitbucketUserID != nil {
			bbCount++
		}
	}
	if bbCount != 1 {
		t.Fatalf("expected one bitbucket user row, got %d", bbCount)
	}
}

