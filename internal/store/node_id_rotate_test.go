package store_test

import (
	"context"
	"path/filepath"
	"testing"

	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func TestUpsertPersistsGitHubNodeIDs(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "nodeid.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	inst, err := st.UpsertInstanceByURL(ctx, "gh", "https://github.com", "github.com", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	org, err := st.UpsertOrganization(ctx, inst.ID, models.Organization{
		ExternalID: 1, NodeID: "O_kgDOOrg", Name: "acme", FullName: "Acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if org.NodeID != "O_kgDOOrg" {
		t.Fatalf("org node_id=%q", org.NodeID)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 42, NodeID: "R_kgDORepo", Owner: "acme", Name: "app", FullName: "acme/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.NodeID != "R_kgDORepo" {
		t.Fatalf("repo node_id=%q", repo.NodeID)
	}
	// Empty node_id on upsert must not wipe the stored GraphQL id (numeric join stays stable).
	repo2, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 42, Owner: "acme", Name: "app", FullName: "acme/app", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo2.NodeID != "R_kgDORepo" {
		t.Fatalf("node_id wiped: %q", repo2.NodeID)
	}

	pr, err := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 9, NodeID: "PR_kwDOPR", Number: 1, Title: "hi", State: "open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.NodeID != "PR_kwDOPR" {
		t.Fatalf("pr node_id=%q", pr.NodeID)
	}
	run, err := st.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID: 55, NodeID: "WFR_kwDORun", Name: "CI", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess, RunAttempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.NodeID != "WFR_kwDORun" {
		t.Fatalf("run node_id=%q", run.NodeID)
	}
	job, err := st.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID: 77, NodeID: "J_kwDOJob", Name: "build", Status: models.StatusCompleted, Conclusion: models.ConclusionSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.NodeID != "J_kwDOJob" {
		t.Fatalf("job node_id=%q", job.NodeID)
	}
}

func TestRotateSealedSecretsOldToNewKey(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "rotate.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	oldKey, err := gitseercrypto.KeyFromString("old-encryption-key-24ch!!")
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := gitseercrypto.KeyFromString("new-encryption-key-24ch!!")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := gitseercrypto.Encrypt(oldKey, "sync-pat")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := st.InsertInstanceSecrets(ctx, store.InstanceSecrets{
		Name:                "lab",
		ForgeType:           models.ForgeTypeGitea,
		BaseURL:             "https://git.example",
		SyncTokenCiphertext: sealed,
		SetFlags:            true,
	})
	if err != nil {
		t.Fatal(err)
	}

	n, err := st.RotateSealedSecrets(ctx, oldKey, newKey)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("updated=%d", n)
	}
	got, err := st.GetInstanceByID(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gitseercrypto.Decrypt(oldKey, got.SyncTokenCiphertext); err == nil {
		t.Fatal("old key should not open rotated ciphertext")
	}
	pt, err := gitseercrypto.Decrypt(newKey, got.SyncTokenCiphertext)
	if err != nil || pt != "sync-pat" {
		t.Fatalf("decrypt new: %q %v", pt, err)
	}

	// Fail closed: wrong old key must not mutate DB.
	before := got.SyncTokenCiphertext
	if _, err := st.RotateSealedSecrets(ctx, oldKey, newKey); err == nil {
		t.Fatal("expected rotate with stale old key to fail")
	}
	after, err := st.GetInstanceByID(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.SyncTokenCiphertext != before {
		t.Fatal("ciphertext changed after failed rotate")
	}
}
