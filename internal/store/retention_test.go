package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestPurgeRetentionDeletesAgedRows(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "ret.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := NewWithDriver(db, "sqlite")

	now := time.Now().UTC()
	old := now.AddDate(0, 0, -40)
	fresh := now.AddDate(0, 0, -5)

	inst, err := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com/ret", "1.25", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 10, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 1, Name: "old", Status: "completed", Conclusion: "success",
		Branch: "main", Event: "push", ActorLogin: "u",
		StartedAt: &old, CompletedAt: &old,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 2, Name: "fresh", Status: "completed", Conclusion: "success",
		Branch: "main", Event: "push", ActorLogin: "u",
		StartedAt: &fresh, CompletedAt: &fresh,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.exec(ctx, `
INSERT INTO webhook_events (instance_id, delivery_id, event_type, payload_hash, payload_json, received_at, status)
VALUES
 (?, 'd1', 'push', 'h1', '{}', ?, 'ok'),
 (?, 'd2', 'push', 'h2', '{}', ?, 'ok'),
 (?, 'd3', 'push', 'h3', '{}', ?, 'pending')`,
		inst.ID, formatTime(old),
		inst.ID, formatTime(fresh),
		inst.ID, formatTime(old)); err != nil {
		t.Fatal(err)
	}

	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "workflow_failure", Severity: "critical",
		EntityType: "workflow_run", EntityID: 1, Title: "open", Fingerprint: "fp-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.exec(ctx, `
INSERT INTO attention_items (
  instance_id, repo_id, type, severity, entity_type, entity_id, title, metadata_json, fingerprint, opened_at, resolved_at, updated_at
) VALUES (?, ?, 'workflow_failure', 'critical', 'workflow_run', 99, 'old', '{}', 'fp-old', ?, ?, ?)`,
		inst.ID, repo.ID, formatTime(old), formatTime(old), formatTime(old)); err != nil {
		t.Fatal(err)
	}

	stats, err := st.PurgeRetention(ctx, 30, 30, 30)
	if err != nil {
		t.Fatal(err)
	}
	if stats["workflow_runs"] != 1 {
		t.Fatalf("workflow_runs purged=%d want 1", stats["workflow_runs"])
	}
	if stats["webhook_events"] != 1 {
		t.Fatalf("webhook_events purged=%d want 1 (pending must survive)", stats["webhook_events"])
	}
	if stats["attention_items"] != 1 {
		t.Fatalf("attention_items purged=%d want 1", stats["attention_items"])
	}

	var runs, hooks, attn int
	_ = st.queryRow(ctx, `SELECT COUNT(*) FROM workflow_runs`).Scan(&runs)
	_ = st.queryRow(ctx, `SELECT COUNT(*) FROM webhook_events`).Scan(&hooks)
	_ = st.queryRow(ctx, `SELECT COUNT(*) FROM attention_items`).Scan(&attn)
	if runs != 1 || hooks != 2 || attn != 1 {
		t.Fatalf("remaining runs=%d hooks=%d attn=%d", runs, hooks, attn)
	}
}
