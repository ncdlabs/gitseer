package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ncdlabs/gitseer/internal/database"
	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/notify"
	"github.com/ncdlabs/gitseer/internal/store"
)

type memSecrets struct {
	key []byte
}

func (m memSecrets) EncryptionConfigured() bool { return len(m.key) == 32 }
func (m memSecrets) EncryptionKeyBytes() []byte  { return m.key }
func (m memSecrets) OpenSecret(ciphertext string) (string, error) {
	return gitseercrypto.Decrypt(m.key, ciphertext)
}
func (m memSecrets) ExternalURL() string { return "https://gitseer.example.com" }

func TestEnqueueAndDeliverGenericWebhook(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "notify-svc.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	key, err := gitseercrypto.KeyFromString("test-encryption-key-24chars!!")
	if err != nil {
		t.Fatal(err)
	}
	secrets := memSecrets{key: key}

	var hits atomic.Int32
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lastBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	sealed, err := notify.SealSecret(secrets, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := st.GetNotificationSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ns.Enabled = true
	ns.ImmediateEnabled = true
	ns.MinSeverity = "critical"
	ns.WebhookEnabled = true
	ns.WebhookURLCiphertext = sealed
	if err := st.UpsertNotificationSettings(ctx, *ns); err != nil {
		t.Fatal(err)
	}

	svc := notify.New(st, secrets, secrets, nil)
	svc.OnAttentionOpened(ctx, &models.AttentionItem{
		Title: "Boom", Severity: "critical", RepoFull: "o/r", Type: "failed_default_branch_workflow",
		Fingerprint: "fp1", OpenedAt: time.Now().UTC(), EntityType: "workflow_run", EntityID: 9,
	})

	// Drain once via claim/deliver loop internals: Run would block; call MaybeEnqueueDigest no-op and process via short Run cancel.
	done := make(chan struct{})
	runCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(done)
		svc.Run(runCtx)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hits.Load() >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done

	if hits.Load() < 1 {
		t.Fatal("expected webhook delivery")
	}
	var payload notify.Payload
	if err := json.Unmarshal(lastBody, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Title != "Boom" || payload.Severity != "critical" {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.DeepLink == "" {
		t.Fatal("expected deep link")
	}
}

func TestDigestEnqueue(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "digest.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)

	key, err := gitseercrypto.KeyFromString("test-encryption-key-24chars!!")
	if err != nil {
		t.Fatal(err)
	}
	secrets := memSecrets{key: key}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	sealed, _ := notify.SealSecret(secrets, srv.URL)

	ns, _ := st.GetNotificationSettings(ctx)
	ns.Enabled = true
	ns.DigestEnabled = true
	ns.DigestHourUTC = time.Now().UTC().Hour()
	ns.WebhookEnabled = true
	ns.WebhookURLCiphertext = sealed
	_ = st.UpsertNotificationSettings(ctx, *ns)

	inst, _ := st.UpsertInstanceByURL(ctx, "lab", "https://git.example.com", "1", `{}`)
	repo, _ := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	_, _ = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "failed_default_branch_workflow",
		Severity: "critical", EntityType: "workflow_run", EntityID: 1,
		Title: "crit", Fingerprint: "fp-d",
	})

	svc := notify.New(st, secrets, secrets, nil)
	svc.MaybeEnqueueDigest(ctx)
	svc.MaybeEnqueueDigest(ctx) // second call same day should no-op after mark

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Run(runCtx)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hits.Load() >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done
	if hits.Load() < 1 {
		t.Fatal("expected digest delivery")
	}
}
