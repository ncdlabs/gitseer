package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/realtime"
	"github.com/ncdlabs/gitseer/internal/store"
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

func TestValidGitHubSignature(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte("gh-sekret"))
	mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !VerifyGitHubSignatureForTest("gh-sekret", body, header) {
		t.Fatal("expected valid github signature")
	}
	if VerifyGitHubSignatureForTest("gh-sekret", body, hex.EncodeToString(mac.Sum(nil))) {
		t.Fatal("bare hex must fail without sha256= prefix")
	}
	if VerifyGitHubSignatureForTest("gh-sekret", body, "sha256=deadbeef") {
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

func TestGitHubWebhookAcceptsSignedDelivery(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "gh-wh.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                    "GitHub",
		ForgeType:               models.ForgeTypeGitHub,
		BaseURL:                 "https://api.github.com",
		WebhookSecretCiphertext: "gh-hook-secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	p := NewProcessor(st, nil, nil, nil)
	body := []byte(`{"action":"opened","pull_request":{"id":1,"number":1,"title":"t","state":"open"},"repository":{"id":9,"name":"r","full_name":"o/r","owner":{"login":"o"}}}`)
	mac := hmac.New(sha256.New, []byte("gh-hook-secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/webhooks/github/%d", inst.ID), bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "deliv-1")
	rr := httptest.NewRecorder()
	p.HandleGitHubHTTPForInstance(rr, req, inst.ID)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
