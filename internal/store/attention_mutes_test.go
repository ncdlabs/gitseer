package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
)

func setupAttentionMuteStore(t *testing.T) (context.Context, *Store, int64, *models.Repository) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "m.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := New(db)
	inst, err := st.UpsertInstanceByURL(ctx, "t", "https://git.example.com", "1.26", "{}")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, st, inst.ID, repo
}

func TestAttentionMuteFingerprintGlobal(t *testing.T) {
	ctx, st, _, repo := setupAttentionMuteStore(t)
	fp := "abc123fingerprint"
	_, err := st.InsertAttentionMute(ctx, AttentionMute{
		Fingerprint: fp,
		RuleType:    "pr_ci_failure",
		RepoID:      &repo.ID,
		Reason:      "flaky",
	})
	if err != nil {
		t.Fatal(err)
	}
	muted, err := st.IsAttentionMuted(ctx, MuteMatch{Fingerprint: fp, RuleType: "pr_ci_failure", RepoID: repo.ID, GlobalOnly: true})
	if err != nil || !muted {
		t.Fatalf("expected global mute, muted=%v err=%v", muted, err)
	}
	// expired mute should not match
	past := time.Now().UTC().Add(-time.Hour)
	_, err = st.InsertAttentionMute(ctx, AttentionMute{
		Fingerprint: "expiredfp",
		UntilAt:     &past,
	})
	if err != nil {
		t.Fatal(err)
	}
	muted, err = st.IsAttentionMuted(ctx, MuteMatch{Fingerprint: "expiredfp", GlobalOnly: true})
	if err != nil || muted {
		t.Fatalf("expected expired mute inactive, muted=%v err=%v", muted, err)
	}
}

func TestAttentionMuteUntilResolvedClear(t *testing.T) {
	ctx, st, _, _ := setupAttentionMuteStore(t)
	fp := "until-resolved-fp"
	_, err := st.InsertAttentionMute(ctx, AttentionMute{Fingerprint: fp})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ClearUntilResolvedMutes(ctx, fp); err != nil {
		t.Fatal(err)
	}
	muted, err := st.IsAttentionMuted(ctx, MuteMatch{Fingerprint: fp, GlobalOnly: true})
	if err != nil || muted {
		t.Fatalf("expected cleared, muted=%v err=%v", muted, err)
	}
}

func TestAttentionRuleOverrides(t *testing.T) {
	ctx, st, _, _ := setupAttentionMuteStore(t)
	if err := st.ReplaceAttentionRuleOverrides(ctx, []AttentionRuleOverride{
		{RuleType: "long_running_workflow", Severity: "critical"},
		{RuleType: "awaiting_review", Severity: "warning"},
	}); err != nil {
		t.Fatal(err)
	}
	m, err := st.MapAttentionRuleOverrides(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m["long_running_workflow"] != "critical" || m["awaiting_review"] != "warning" {
		t.Fatalf("map=%v", m)
	}
	if err := st.ReplaceAttentionRuleOverrides(ctx, nil); err != nil {
		t.Fatal(err)
	}
	m, err = st.MapAttentionRuleOverrides(ctx)
	if err != nil || len(m) != 0 {
		t.Fatalf("expected empty after replace, map=%v err=%v", m, err)
	}
}

func TestGetAttentionByID(t *testing.T) {
	ctx, st, instID, repo := setupAttentionMuteStore(t)
	item, err := st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: instID, RepoID: repo.ID, Type: "pr_ci_failure", Severity: "critical",
		EntityType: "pull_request", EntityID: 1, Title: "fail", Fingerprint: "get-by-id-fp",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAttentionByID(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fingerprint != "get-by-id-fp" || got.RepoFull != "o/r" {
		t.Fatalf("got=%+v", got)
	}
}
