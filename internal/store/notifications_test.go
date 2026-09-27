package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestNotificationSettingsAndOutbox(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "notify.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	ns, err := st.GetNotificationSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ns.MinSeverity != "critical" || !ns.ImmediateEnabled {
		t.Fatalf("defaults: %+v", ns)
	}

	ns.Enabled = true
	ns.SMTPEnabled = true
	ns.SMTPHost = "smtp.example.com"
	ns.SMTPTo = "ops@example.com"
	ns.SMTPFrom = "gitseer@example.com"
	ns.SlackEnabled = true
	ns.SlackWebhookCiphertext = "sealed-slack"
	if err := st.UpsertNotificationSettings(ctx, *ns); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetNotificationSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.SMTPHost != "smtp.example.com" || got.SlackWebhookCiphertext != "sealed-slack" {
		t.Fatalf("saved: %+v", got)
	}

	row, err := st.EnqueueNotification(ctx, store.NotifyChannelSlack, store.NotifyKindImmediate, `{"title":"t"}`, "dedupe-1")
	if err != nil || row == nil {
		t.Fatalf("enqueue: row=%v err=%v", row, err)
	}
	dup, err := st.EnqueueNotification(ctx, store.NotifyChannelSlack, store.NotifyKindImmediate, `{"title":"t2"}`, "dedupe-1")
	if err != nil {
		t.Fatal(err)
	}
	if dup != nil {
		t.Fatal("expected dedupe skip")
	}

	claimed, err := st.ClaimNotificationOutbox(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != row.ID {
		t.Fatalf("claimed=%+v", claimed)
	}
	if err := st.MarkNotificationSent(ctx, claimed[0].ID); err != nil {
		t.Fatal(err)
	}
	again, err := st.ClaimNotificationOutbox(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("expected empty after sent, got %d", len(again))
	}
}

func TestSeverityMeetsMin(t *testing.T) {
	if !store.SeverityMeetsMin("critical", "critical") {
		t.Fatal("critical >= critical")
	}
	if !store.SeverityMeetsMin("critical", "warning") {
		t.Fatal("critical >= warning")
	}
	if store.SeverityMeetsMin("waiting", "critical") {
		t.Fatal("waiting < critical")
	}
}

func TestListOpenAttentionForNotify(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "attn-notify.db"), "")
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
		ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	crit, err := st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "failed_default_branch_workflow",
		Severity: "critical", EntityType: "workflow_run", EntityID: 1,
		Title: "crit", Fingerprint: "fp-crit",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "awaiting_review",
		Severity: "waiting", EntityType: "pull_request", EntityID: 2,
		Title: "wait", Fingerprint: "fp-wait",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = crit

	items, err := st.ListOpenAttentionForNotify(ctx, "critical", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Fingerprint != "fp-crit" {
		t.Fatalf("items=%+v", items)
	}

	before, err := st.GetAttentionByFingerprint(ctx, "fp-crit")
	if err != nil || before == nil {
		t.Fatal(err)
	}
	_ = st.ResolveAttentionByFingerprint(ctx, "fp-crit")
	reopened, err := st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "failed_default_branch_workflow",
		Severity: "critical", EntityType: "workflow_run", EntityID: 1,
		Title: "crit again", Fingerprint: "fp-crit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.OpenedAt.Equal(before.OpenedAt) {
		t.Fatal("expected opened_at refresh on reopen")
	}
}
