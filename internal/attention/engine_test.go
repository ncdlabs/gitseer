package attention

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func setup(t *testing.T) (context.Context, *store.Store, *Engine, int64, *models.Repository) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "a.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	eng := NewWithConfig(st, nil, time.Hour)
	inst, _ := st.UpsertInstanceByURL(ctx, "t", "https://git.example.com", "1.26", "{}")
	repo, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	return ctx, st, eng, inst.ID, repo
}

func openCount(t *testing.T, st *store.Store, typ string) int {
	t.Helper()
	items, total, err := st.ListAttention(t.Context(), store.ListAttentionOpts{
		BootstrapAll: true, OpenOnly: true, Type: typ,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = items
	return total
}

func TestFailedDefaultBranch(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	run, _ := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 9, Name: "CI", Branch: "main", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, RepoFull: "o/r",
		HTMLURL: "https://git.example.com/o/r/actions/runs/9",
	})
	if err := eng.EvaluateRun(ctx, instID, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeFailedDefaultBranch) != 1 {
		t.Fatal("expected open")
	}
	run.Conclusion = models.ConclusionSuccess
	if err := eng.EvaluateRun(ctx, instID, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeFailedDefaultBranch) != 0 {
		t.Fatal("expected resolved")
	}
}

func TestPRCIFailureAndAwaitingReview(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	pr, _ := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 1, Number: 3, Title: "feat", State: "open", CIState: models.CIStateFailure,
	})
	if err := eng.EvaluatePullRequest(ctx, instID, pr); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypePRCIFailure) != 1 {
		t.Fatal("expected CI failure attention")
	}
	pr.CIState = models.CIStateSuccess
	pr.ReviewState = ""
	if err := eng.EvaluatePullRequest(ctx, instID, pr); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypePRCIFailure) != 0 {
		t.Fatal("expected CI failure resolved")
	}
	if openCount(t, st, TypeAwaitingReview) != 1 {
		t.Fatal("expected awaiting review")
	}
}

func TestApprovedBlockedByCI(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	pr, _ := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 2, Number: 4, Title: "fix", State: "open",
		CIState: models.CIStateFailure, ReviewState: "approved",
	})
	if err := eng.EvaluatePullRequest(ctx, instID, pr); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeApprovedBlockedCI) != 1 {
		t.Fatal("expected approved blocked")
	}
}

func TestMergeConflict(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	m := false
	pr, _ := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 3, Number: 5, Title: "conflict", State: "open", Mergeable: &m,
	})
	if err := eng.EvaluatePullRequest(ctx, instID, pr); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeMergeConflict) != 1 {
		t.Fatal("expected merge conflict")
	}
}

func TestLongRunning(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	started := time.Now().UTC().Add(-3 * time.Hour)
	run, _ := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 11, Name: "slow", Status: models.StatusRunning, StartedAt: &started,
	})
	if err := eng.EvaluateRun(ctx, instID, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeLongRunning) != 1 {
		t.Fatal("expected long running")
	}
	run.Status = models.StatusCompleted
	run.Conclusion = models.ConclusionSuccess
	now := time.Now().UTC()
	run.CompletedAt = &now
	if err := eng.EvaluateRun(ctx, instID, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeLongRunning) != 0 {
		t.Fatal("expected resolved")
	}
}

func TestDeployFailure(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	run, _ := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 12, Name: "Deploy prod", WorkflowPath: ".gitea/workflows/deploy.yaml",
		Status: models.StatusCompleted, Conclusion: models.ConclusionFailure,
	})
	if err := eng.EvaluateRun(ctx, instID, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeDeployFailure) != 1 {
		t.Fatal("expected deploy failure")
	}
}

func TestNoOpenPRNoise(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	pr, _ := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 9, Number: 9, Title: "quiet", State: "open", CIState: models.CIStatePending,
	})
	if err := eng.EvaluatePullRequest(ctx, instID, pr); err != nil {
		t.Fatal(err)
	}
	_, total, err := st.ListAttention(ctx, store.ListAttentionOpts{BootstrapAll: true, OpenOnly: true})
	if err != nil || total != 0 {
		t.Fatalf("expected no attention for quiet open PR, total=%d err=%v", total, err)
	}
}

func TestMuteSkipsOpen(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	run, _ := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 99, Name: "CI", Branch: "main", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, RepoFull: "o/r",
	})
	fp := fingerprint(TypeFailedDefaultBranch, fmt.Sprintf("%d", run.RepoID), fmt.Sprintf("%d", run.ExternalID))
	_, err := st.InsertAttentionMute(ctx, store.AttentionMute{Fingerprint: fp})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.EvaluateRun(ctx, instID, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeFailedDefaultBranch) != 0 {
		t.Fatal("expected mute to skip open")
	}
}

func TestSeverityOverride(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	eng.SetSeverityOverrides(map[string]string{TypeLongRunning: SeverityCritical})
	started := time.Now().UTC().Add(-3 * time.Hour)
	run, _ := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 21, Name: "slow", Status: models.StatusRunning, StartedAt: &started,
	})
	if err := eng.EvaluateRun(ctx, instID, run); err != nil {
		t.Fatal(err)
	}
	items, _, err := st.ListAttention(ctx, store.ListAttentionOpts{
		BootstrapAll: true, OpenOnly: true, Type: TypeLongRunning,
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if items[0].Severity != SeverityCritical {
		t.Fatalf("severity=%s want critical", items[0].Severity)
	}
}

func TestUntilResolvedMuteClearedOnResolve(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	run, _ := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 33, Name: "CI", Branch: "main", Status: models.StatusCompleted,
		Conclusion: models.ConclusionFailure, RepoFull: "o/r",
	})
	fp := fingerprint(TypeFailedDefaultBranch, fmt.Sprintf("%d", run.RepoID), fmt.Sprintf("%d", run.ExternalID))
	_, err := st.InsertAttentionMute(ctx, store.AttentionMute{Fingerprint: fp})
	if err != nil {
		t.Fatal(err)
	}
	run.Conclusion = models.ConclusionSuccess
	if err := eng.EvaluateRun(ctx, instID, run); err != nil {
		t.Fatal(err)
	}
	muted, err := st.IsAttentionMuted(ctx, store.MuteMatch{Fingerprint: fp, GlobalOnly: true})
	if err != nil || muted {
		t.Fatalf("expected until-resolved mute cleared, muted=%v err=%v", muted, err)
	}
}

func TestRunnerUnavailableQueuedBestEffort(t *testing.T) {
	ctx, st, eng, instID, repo := setup(t)
	run, _ := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 77, Name: "CI", Branch: "main", Status: models.StatusQueued, RepoFull: "o/r",
	})

	// Queued without a positive signal must not open (no fake always-on alerts).
	plain, _ := st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 1, Name: "build", Status: models.StatusQueued,
	})
	if err := eng.EvaluateJob(ctx, instID, plain, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeRunnerUnavailable) != 0 {
		t.Fatal("expected no attention without positive signal")
	}

	msg := "No runners available matching labels"
	labels := `["self-hosted","runner offline"]`
	signaled, _ := st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 2, Name: "build-sh", Status: models.StatusQueued,
		Message: msg, LabelsJSON: &labels,
	})
	if err := eng.EvaluateJob(ctx, instID, signaled, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeRunnerUnavailable) != 1 {
		t.Fatal("expected open on positive message/label signal")
	}

	// Completing the job clears the attention.
	signaled.Status = models.StatusCompleted
	signaled.Conclusion = models.ConclusionCancelled
	if err := eng.EvaluateJob(ctx, instID, signaled, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeRunnerUnavailable) != 0 {
		t.Fatal("expected resolved after leave queued")
	}

	// Conclusion/steps text alone while queued also opens.
	steps := `[{"name":"Set up job","conclusion":"failure","output":"The runner matching the labels was not found"}]`
	viaSteps, _ := st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 3, Name: "test", Status: models.StatusWaiting, StepsJSON: &steps,
	})
	if err := eng.EvaluateJob(ctx, instID, viaSteps, run); err != nil {
		t.Fatal(err)
	}
	if openCount(t, st, TypeRunnerUnavailable) != 1 {
		t.Fatal("expected open from steps_json signal")
	}

	ok, reason := jobSignalsRunnerUnavailable(&models.Job{Status: models.StatusQueued, Message: "Waiting for a runner to pick up this job"})
	if ok {
		t.Fatalf("normal queue text must not match, got reason=%q", reason)
	}
}

func TestJobSignalsRunnerUnavailablePhrases(t *testing.T) {
	cases := []struct {
		name string
		job  models.Job
		want bool
	}{
		{name: "empty", job: models.Job{Status: models.StatusQueued}, want: false},
		{name: "message", job: models.Job{Message: "Runner is offline"}, want: true},
		{name: "upstream", job: models.Job{UpstreamConclusion: "runner_system_failure"}, want: true},
		{name: "labels", job: models.Job{LabelsJSON: strPtr(`["no online runners"]`)}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := jobSignalsRunnerUnavailable(&tc.job)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
