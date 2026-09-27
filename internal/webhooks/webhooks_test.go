package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func TestGitHubWebhookRejectsUnsignedWhenGlobalAllowUnsigned(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "gh-wh-unsigned.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	// GitHub instance with no ciphertext and allow_unsigned=false.
	inst, err := st.UpsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                  "GitHub",
		ForgeType:             models.ForgeTypeGitHub,
		BaseURL:               "https://api.github.com",
		AllowUnsignedWebhooks: false,
		SetFlags:              true,
	})
	if err != nil {
		t.Fatal(err)
	}

	p := NewProcessor(st, nil, nil, nil)
	p.SetSecret("legacy-gitea-secret")
	p.SetAllowUnsigned(true) // must not leak onto instance-scoped GitHub route

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/webhooks/github/%d", inst.ID), bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "deliv-unsigned")
	rr := httptest.NewRecorder()
	p.HandleGitHubHTTPForInstance(rr, req, inst.ID)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s want 401", rr.Code, rr.Body.String())
	}
}

func TestNormalizeGitLabPipelineAndJobFixtures(t *testing.T) {
	pipelineBody := []byte(`{
		"object_kind": "pipeline",
		"object_attributes": {
			"id": 55,
			"ref": "main",
			"sha": "abc123",
			"status": "success",
			"source": "push",
			"created_at": "2026-09-21T10:00:00Z",
			"finished_at": "2026-09-21T10:05:00Z",
			"url": "https://gitlab.example/acme/widgets/-/pipelines/55"
		},
		"project": {
			"id": 9,
			"name": "widgets",
			"path_with_namespace": "acme/widgets"
		}
	}`)
	norm, ok := normalizeWebhookPayload(models.ForgeTypeGitLab, "workflow_run", pipelineBody)
	if !ok {
		t.Fatal("expected pipeline normalize")
	}
	var runPayload struct {
		WorkflowRun struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"workflow_run"`
		Repository struct {
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
			Name string `json:"name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(norm, &runPayload); err != nil {
		t.Fatal(err)
	}
	if runPayload.WorkflowRun.ID != 55 {
		t.Fatalf("run id=%d", runPayload.WorkflowRun.ID)
	}
	if runPayload.Repository.Owner.Login != "acme" || runPayload.Repository.Name != "widgets" {
		t.Fatalf("repo=%+v", runPayload.Repository)
	}

	jobBody := []byte(`{
		"object_kind": "build",
		"build_id": 9001,
		"build_name": "test",
		"build_status": "running",
		"pipeline_id": 55,
		"project_id": 9,
		"project": {
			"id": 9,
			"name": "widgets",
			"path_with_namespace": "acme/widgets",
			"web_url": "https://gitlab.example/acme/widgets"
		}
	}`)
	jnorm, ok := normalizeWebhookPayload(models.ForgeTypeGitLab, "workflow_job", jobBody)
	if !ok {
		t.Fatal("expected job normalize")
	}
	var jobPayload struct {
		WorkflowJob struct {
			ID    int64 `json:"id"`
			RunID int64 `json:"run_id"`
		} `json:"workflow_job"`
	}
	if err := json.Unmarshal(jnorm, &jobPayload); err != nil {
		t.Fatal(err)
	}
	if jobPayload.WorkflowJob.ID != 9001 || jobPayload.WorkflowJob.RunID != 55 {
		t.Fatalf("job=%+v", jobPayload.WorkflowJob)
	}

	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "gl-wh.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	inst, err := st.UpsertInstanceMeta(ctx, models.ForgeTypeGitLab, "gl", "https://gitlab.example", "", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	p := NewProcessor(st, nil, nil, nil)
	if err := p.apply(ctx, store.WebhookEvent{InstanceID: inst.ID, EventType: "workflow_run", Payload: string(norm)}); err != nil {
		t.Fatal(err)
	}
	if err := p.apply(ctx, store.WebhookEvent{InstanceID: inst.ID, EventType: "workflow_job", Payload: string(jnorm)}); err != nil {
		t.Fatal(err)
	}

	zeroRun := `{"workflow_run":{"id":0,"name":"x"},"repository":{"id":1,"name":"n","owner":{"login":"o"}}}`
	if err := p.apply(ctx, store.WebhookEvent{InstanceID: inst.ID, EventType: "workflow_run", Payload: zeroRun}); err == nil {
		t.Fatal("expected reject external_id=0")
	}
	emptyOwner := `{"workflow_run":{"id":1,"name":"x"},"repository":{"id":1,"name":"n","owner":{"login":""}}}`
	if err := p.apply(ctx, store.WebhookEvent{InstanceID: inst.ID, EventType: "workflow_run", Payload: emptyOwner}); err == nil {
		t.Fatal("expected reject empty owner")
	}
}

func TestNormalizeBitbucketCommitStatus(t *testing.T) {
	uuid := "{11111111-2222-3333-4444-555555555555}"
	body := []byte(fmt.Sprintf(`{
		"repository": {"uuid": %q, "name": "widgets", "full_name": "acme/widgets"},
		"commit_status": {
			"state": "SUCCESSFUL",
			"key": "ci",
			"commit": {"hash": "deadbeef"}
		}
	}`, uuid))
	norm, ok := normalizeWebhookPayload(models.ForgeTypeBitbucket, "status", body)
	if !ok {
		t.Fatal("expected bitbucket status normalize")
	}
	var payload struct {
		SHA        string `json:"sha"`
		State      string `json:"state"`
		Repository struct {
			ID int64 `json:"id"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(norm, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SHA != "deadbeef" || payload.State != "success" {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.Repository.ID == 0 {
		t.Fatal("expected stable repo id")
	}
}
