package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncdlabs/gitea-lens/internal/database"
	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/realtime"
	"github.com/ncdlabs/gitea-lens/internal/store"
)

func TestValidHMAC(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte("sekret"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if !VerifySignatureForTest("sekret", body, sig) {
		t.Fatal("expected valid")
	}
	if VerifySignatureForTest("sekret", body, "deadbeef") {
		t.Fatal("expected invalid")
	}
}

func TestApplyJobStepsJSONAndSSE(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "webhooks.db"), "")
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
		ExternalID: 42, Owner: "acme", Name: "widgets", FullName: "acme/widgets",
	})
	if err != nil {
		t.Fatal(err)
	}

	hub := realtime.NewHub()
	ch := hub.Subscribe()
	t.Cleanup(func() { hub.Unsubscribe(ch) })
	p := NewProcessor(st, nil, hub, nil)

	payload := `{
		"workflow_job": {
			"id": 9001,
			"run_id": 500,
			"name": "build",
			"status": "in_progress",
			"conclusion": "",
			"html_url": "https://git.example.com/acme/widgets/actions/runs/500/jobs/9001",
			"started_at": "2026-09-21T10:00:00Z",
			"completed_at": "",
			"steps": [{"name":"Checkout","status":"completed","conclusion":"success","number":1}]
		},
		"repository": {
			"id": 42,
			"name": "widgets",
			"full_name": "acme/widgets",
			"owner": {"login": "acme"}
		}
	}`
	ev := store.WebhookEvent{
		InstanceID: inst.ID,
		EventType:  "workflow_job",
		Payload:    payload,
	}
	if err := p.apply(ctx, ev); err != nil {
		t.Fatal(err)
	}

	job, err := st.GetJobByExternalID(ctx, repo.ID, 9001)
	if err != nil {
		t.Fatal(err)
	}
	if job.StepsJSON == nil || *job.StepsJSON == "" {
		t.Fatal("expected steps_json to be set")
	}
	if !strings.Contains(*job.StepsJSON, `"Checkout"`) {
		t.Fatalf("steps_json=%s", *job.StepsJSON)
	}
	if job.Status != models.StatusRunning {
		t.Fatalf("status=%q want running", job.Status)
	}

	select {
	case got := <-ch:
		if got.Type != "workflow_job" {
			t.Fatalf("event type=%q", got.Type)
		}
		if got.ID != job.ID {
			t.Fatalf("event id=%d want %d", got.ID, job.ID)
		}
		if got.RepoID != repo.ID {
			t.Fatalf("event repo_id=%d want %d", got.RepoID, repo.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for workflow_job SSE event")
	}
}
