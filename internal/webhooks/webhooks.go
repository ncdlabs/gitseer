// Package webhooks accepts, persists, and applies forge webhook deliveries.
package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ncdlabs/gitseer/internal/attention"
	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/forge"
	gitseermetrics "github.com/ncdlabs/gitseer/internal/metrics"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/realtime"
	"github.com/ncdlabs/gitseer/internal/store"
	"github.com/ncdlabs/gitseer/internal/workflows"
)

const maxBody = 2 << 20

// SecretOpener decrypts per-instance webhook secret ciphertext.
type SecretOpener interface {
	OpenSecret(stored string) (string, error)
}

type Processor struct {
	store         *store.Store
	att           *attention.Engine
	hub           *realtime.Hub
	log           *slog.Logger
	secrets       SecretOpener
	encKey        []byte
	mu            sync.RWMutex
	secret        string // optional legacy HMAC secret from config/settings (Gitea primary path)
	allowUnsigned bool
}

func NewProcessor(st *store.Store, att *attention.Engine, hub *realtime.Hub, log *slog.Logger) *Processor {
	if log == nil {
		log = slog.Default()
	}
	return &Processor{store: st, att: att, hub: hub, log: log}
}

func (p *Processor) SetSecret(secret string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.secret = secret
}

func (p *Processor) SetAllowUnsigned(allow bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.allowUnsigned = allow
}

// SetSecretOpener attaches decrypt for per-instance webhook secrets.
func (p *Processor) SetSecretOpener(o SecretOpener) {
	p.secrets = o
}

// SetEncryptionKey enables direct decrypt when SecretOpener is unset.
func (p *Processor) SetEncryptionKey(key []byte) {
	p.encKey = key
}

func (p *Processor) openSecret(stored string) (string, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "", nil
	}
	if p.secrets != nil {
		return p.secrets.OpenSecret(stored)
	}
	if len(p.encKey) != 32 {
		if gitseercrypto.LooksLikeCiphertext(stored) {
			return "", fmt.Errorf("GITSEER_ENCRYPTION_KEY is required to decrypt stored secrets")
		}
		if gitseercrypto.LooksLikeBase64Blob(stored) {
			return "", fmt.Errorf("GITSEER_ENCRYPTION_KEY is required to decrypt stored secrets")
		}
		return stored, nil
	}
	return gitseercrypto.Decrypt(p.encKey, stored)
}

func (p *Processor) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	p.HandleHTTPForInstance(w, r, 0)
}

// HandleHTTPForInstance accepts a Gitea webhook for a specific GitSeer instance ID.
// When instanceID is 0, the primary (oldest) instance is used.
func (p *Processor) HandleHTTPForInstance(w http.ResponseWriter, r *http.Request, instanceID int64) {
	p.handleForgeWebhook(w, r, instanceID, models.ForgeTypeGitea)
}

// HandleGitHubHTTPForInstance accepts a GitHub webhook for a specific GitSeer instance ID.
func (p *Processor) HandleGitHubHTTPForInstance(w http.ResponseWriter, r *http.Request, instanceID int64) {
	if instanceID <= 0 {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}
	p.handleForgeWebhook(w, r, instanceID, models.ForgeTypeGitHub)
}

func (p *Processor) handleForgeWebhook(w http.ResponseWriter, r *http.Request, instanceID int64, forgeType string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if len(body) > maxBody {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	var inst *models.Instance
	if instanceID > 0 {
		inst, err = p.store.GetInstanceByID(r.Context(), instanceID)
	} else if forgeType == models.ForgeTypeGitHub {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	} else {
		inst, err = p.store.GetPrimaryGiteaInstance(r.Context())
	}
	if err != nil || inst == nil {
		http.Error(w, "no instance", http.StatusServiceUnavailable)
		return
	}
	if forgeType != "" && inst.ForgeType != "" && inst.ForgeType != forgeType {
		http.Error(w, "forge type mismatch", http.StatusBadRequest)
		return
	}

	// Legacy global HMAC/allow-unsigned applies only to the unscoped Gitea route
	// (instanceID==0). Instance-scoped routes never inherit another forge's secret/flag.
	inheritLegacy := instanceID == 0 && forgeType != models.ForgeTypeGitHub
	secret, allowUnsigned, err := p.webhookAuthForInstance(inst, inheritLegacy)
	if err != nil {
		p.log.Error("webhook secret decrypt", "instance_id", inst.ID, "err", err)
		http.Error(w, "webhook secret unavailable", http.StatusServiceUnavailable)
		return
	}
	if secret != "" {
		ok := false
		switch forgeType {
		case models.ForgeTypeGitHub:
			ok = validGitHubSignature(secret, body, r.Header.Get("X-Hub-Signature-256"))
		default:
			ok = validHMAC(secret, body, r.Header.Get("X-Gitea-Signature"))
		}
		if !ok {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	} else if !allowUnsigned {
		http.Error(w, "webhook secret required", http.StatusUnauthorized)
		return
	}

	eventType, delivery := webhookHeaders(r, forgeType)
	_, inserted, err := p.store.InsertWebhookEvent(r.Context(), inst.ID, delivery, eventType, string(body))
	if err != nil {
		p.log.Error("persist webhook", "err", err)
		http.Error(w, "persist failed", http.StatusInternalServerError)
		return
	}
	if inserted {
		gitseermetrics.WebhooksReceivedTotal.WithLabelValues(eventType).Inc()
		if _, verr := p.store.MarkWebhookVerifiedIfPending(r.Context(), inst.ID); verr != nil {
			p.log.Warn("webhook verify mark", "instance_id", inst.ID, "err", verr)
		}
	}
	if !inserted {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"duplicate"}`))
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

// webhookAuthForInstance resolves HMAC secret and allow-unsigned for a delivery.
// When inheritLegacyGlobal is false (per-instance webhook routes), empty ciphertext
// uses only the instance's allow_unsigned_webhooks flag — never the legacy global
// Gitea secret or global allow-unsigned (which would fail-open secondary forges).
func (p *Processor) webhookAuthForInstance(inst *models.Instance, inheritLegacyGlobal bool) (secret string, allowUnsigned bool, err error) {
	if inst != nil && strings.TrimSpace(inst.WebhookSecretCiphertext) != "" {
		secret, err = p.openSecret(inst.WebhookSecretCiphertext)
		if err != nil {
			return "", false, err
		}
		return secret, inst.AllowUnsignedWebhooks, nil
	}
	allow := false
	if inst != nil {
		allow = inst.AllowUnsignedWebhooks
	}
	if !inheritLegacyGlobal {
		return "", allow, nil
	}
	// Legacy unscoped Gitea path (pre-per-instance secrets).
	p.mu.RLock()
	defer p.mu.RUnlock()
	if inst == nil {
		allow = p.allowUnsigned
	}
	return p.secret, allow, nil
}

func webhookHeaders(r *http.Request, forgeType string) (eventType, delivery string) {
	switch forgeType {
	case models.ForgeTypeGitHub:
		return r.Header.Get("X-GitHub-Event"), r.Header.Get("X-GitHub-Delivery")
	default:
		eventType = r.Header.Get("X-Gitea-Event")
		if eventType == "" {
			eventType = r.Header.Get("X-GitHub-Event")
		}
		delivery = r.Header.Get("X-Gitea-Delivery")
		if delivery == "" {
			delivery = r.Header.Get("X-GitHub-Delivery")
		}
		return eventType, delivery
	}
}

func validHMAC(secret string, body []byte, sig string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.ToLower(sig)), []byte(strings.ToLower(expected)))
}

// validGitHubSignature checks X-Hub-Signature-256: sha256=<hex>.
func validGitHubSignature(secret string, body []byte, header string) bool {
	header = strings.TrimSpace(header)
	const prefix = "sha256="
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return false
	}
	return validHMAC(secret, body, header[len(prefix):])
}

func (p *Processor) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	reaperEvery := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reaperEvery++
			if reaperEvery%15 == 0 { // ~30s
				if n, err := p.store.ReapStaleProcessingWebhooks(ctx, 5*time.Minute); err != nil {
					p.log.Error("webhook reaper", "err", err)
				} else if n > 0 {
					p.log.Info("reaped stale processing webhooks", "count", n)
				}
			}
			events, err := p.store.ClaimPendingWebhooks(ctx, 20)
			if err != nil {
				p.log.Error("claim webhooks", "err", err)
				continue
			}
			for _, ev := range events {
				errMsg := ""
				if err := p.apply(ctx, ev); err != nil {
					errMsg = err.Error()
					p.log.Warn("webhook apply failed", "id", ev.ID, "type", ev.EventType, "err", err)
					gitseermetrics.WebhookProcessingErrorsTotal.WithLabelValues(ev.EventType).Inc()
				}
				if err := p.store.MarkWebhookProcessed(ctx, ev.ID, errMsg); err != nil {
					p.log.Error("mark webhook processed failed", "id", ev.ID, "err", err)
				}
			}
		}
	}
}

func (p *Processor) apply(ctx context.Context, ev store.WebhookEvent) error {
	switch ev.EventType {
	case "pull_request":
		return p.applyPR(ctx, ev)
	case "pull_request_review":
		return p.applyPRReview(ctx, ev)
	case "workflow_run", "actions_run":
		return p.applyRun(ctx, ev)
	case "workflow_job", "actions_job":
		return p.applyJob(ctx, ev)
	case "repository":
		return p.applyRepository(ctx, ev)
	case "status", "check_run", "check_suite":
		return p.applyCommitStatus(ctx, ev)
	default:
		return nil // ignore unknown
	}
}

func (p *Processor) applyPR(ctx context.Context, ev store.WebhookEvent) error {
	var payload struct {
		Action string `json:"action"`
		PR     struct {
			ID        int64  `json:"id"`
			Number    int64  `json:"number"`
			Title     string `json:"title"`
			Body      string `json:"body"`
			State     string `json:"state"`
			Draft     bool   `json:"draft"`
			Mergeable *bool  `json:"mergeable"`
			HTMLURL   string `json:"html_url"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			User      *struct {
				ID    int64  `json:"id"`
				Login string `json:"login"`
			} `json:"user"`
			Head *struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"head"`
			Base *struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"base"`
		} `json:"pull_request"`
		Repository struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			FullName string `json:"full_name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
		return err
	}
	repo, err := p.store.GetRepositoryByExternalID(ctx, ev.InstanceID, payload.Repository.ID)
	if err != nil {
		// create minimal repo row
		repo, err = p.store.UpsertRepository(ctx, ev.InstanceID, models.Repository{
			ExternalID: payload.Repository.ID,
			Owner:      payload.Repository.Owner.Login,
			Name:       payload.Repository.Name,
			FullName:   payload.Repository.FullName,
		})
		if err != nil {
			return err
		}
	}
	pr := models.PullRequest{
		ExternalID:  payload.PR.ID,
		Number:      payload.PR.Number,
		Title:       payload.PR.Title,
		BodyExcerpt: truncate(payload.PR.Body, 500),
		State:       payload.PR.State,
		Draft:       payload.PR.Draft,
		Mergeable:   payload.PR.Mergeable,
		HTMLURL:     payload.PR.HTMLURL,
		CreatedAt:   parseWebhookTime(payload.PR.CreatedAt),
		UpdatedAt:   parseWebhookTime(payload.PR.UpdatedAt),
	}
	if payload.PR.User != nil {
		pr.AuthorLogin = payload.PR.User.Login
		id := payload.PR.User.ID
		pr.AuthorExternalID = &id
	}
	if payload.PR.Head != nil {
		pr.SourceBranch = payload.PR.Head.Ref
		pr.HeadSHA = payload.PR.Head.SHA
	}
	if payload.PR.Base != nil {
		pr.TargetBranch = payload.PR.Base.Ref
		pr.BaseSHA = payload.PR.Base.SHA
	}
	saved, err := p.store.UpsertPullRequest(ctx, repo.ID, pr)
	if err != nil {
		return err
	}
	if p.att != nil {
		_ = p.att.EvaluatePullRequest(ctx, ev.InstanceID, saved)
	}
	if p.hub != nil {
		p.hub.Publish(realtime.Event{Type: "pull_request", ID: saved.ID, RepoID: repo.ID})
	}
	return nil
}

func (p *Processor) applyPRReview(ctx context.Context, ev store.WebhookEvent) error {
	var payload struct {
		Action string `json:"action"`
		Review struct {
			State string `json:"state"`
		} `json:"review"`
		PR struct {
			ID     int64 `json:"id"`
			Number int64 `json:"number"`
		} `json:"pull_request"`
		Repository struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			FullName string `json:"full_name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
		return err
	}
	if payload.PR.Number <= 0 || payload.Repository.ID == 0 {
		return nil
	}
	repo, err := p.ensureRepo(ctx, ev.InstanceID, payload.Repository.ID, payload.Repository.Owner.Login, payload.Repository.Name, payload.Repository.FullName)
	if err != nil {
		return err
	}
	existing, err := p.store.GetPullRequestByNumber(ctx, repo.ID, payload.PR.Number)
	if err != nil || existing == nil {
		// Minimal row; full sync will enrich.
		existing, err = p.store.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
			ExternalID: payload.PR.ID,
			Number:     payload.PR.Number,
			State:      "open",
			Title:      fmt.Sprintf("PR #%d", payload.PR.Number),
		})
		if err != nil {
			return err
		}
	}
	state := forge.AggregateReviewState([]string{payload.Review.State, existing.ReviewState})
	if state == "" && payload.Review.State != "" {
		state = forge.AggregateReviewState([]string{payload.Review.State})
	}
	existing.ReviewState = state
	saved, err := p.store.UpsertPullRequest(ctx, repo.ID, *existing)
	if err != nil {
		return err
	}
	if p.att != nil {
		_ = p.att.EvaluatePullRequest(ctx, ev.InstanceID, saved)
	}
	if p.hub != nil {
		p.hub.Publish(realtime.Event{Type: "pull_request", ID: saved.ID, RepoID: repo.ID})
	}
	return nil
}

func (p *Processor) applyCommitStatus(ctx context.Context, ev store.WebhookEvent) error {
	var payload struct {
		SHA    string `json:"sha"`
		State  string `json:"state"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
		CheckRun struct {
			HeadSHA    string `json:"head_sha"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_run"`
		CheckSuite struct {
			HeadSHA    string `json:"head_sha"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_suite"`
		Repository struct {
			ID int64 `json:"id"`
		} `json:"repository"`
	}
	if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
		return err
	}
	sha := payload.SHA
	if sha == "" {
		sha = payload.Commit.SHA
	}
	if sha == "" {
		sha = payload.CheckRun.HeadSHA
	}
	if sha == "" {
		sha = payload.CheckSuite.HeadSHA
	}
	if sha == "" || payload.Repository.ID == 0 {
		return nil
	}
	repo, err := p.store.GetRepositoryByExternalID(ctx, ev.InstanceID, payload.Repository.ID)
	if err != nil || repo == nil {
		return nil
	}
	ci := forge.NormalizeCIState(payload.State)
	if ci == "" && payload.CheckRun.HeadSHA != "" {
		if strings.ToLower(payload.CheckRun.Status) != "completed" {
			ci = models.CIStatePending
		} else {
			ci = forge.NormalizeCIState(payload.CheckRun.Conclusion)
			if ci == "" {
				_, conc := forge.NormalizeStatus(payload.CheckRun.Conclusion)
				switch conc {
				case models.ConclusionFailure, models.ConclusionTimedOut, models.ConclusionActionRequired:
					ci = models.CIStateFailure
				case models.ConclusionSuccess:
					ci = models.CIStateSuccess
				case models.ConclusionCancelled, models.ConclusionSkipped, models.ConclusionNeutral:
					ci = models.CIStateCancelled
				}
			}
		}
	}
	if ci == "" && payload.CheckSuite.HeadSHA != "" {
		if strings.ToLower(payload.CheckSuite.Status) != "completed" {
			ci = models.CIStatePending
		} else {
			ci = forge.NormalizeCIState(payload.CheckSuite.Conclusion)
		}
	}
	if ci == "" {
		return nil
	}
	return p.store.SetPullRequestsCIStateByHeadSHA(ctx, repo.ID, sha, ci)
}

func (p *Processor) applyRun(ctx context.Context, ev store.WebhookEvent) error {
	var payload struct {
		WorkflowRun struct {
			ID           int64  `json:"id"`
			Name         string `json:"name"`
			Event        string `json:"event"`
			Status       string `json:"status"`
			Conclusion   string `json:"conclusion"`
			HTMLURL      string `json:"html_url"`
			HeadBranch   string `json:"head_branch"`
			HeadSHA      string `json:"head_sha"`
			Path         string `json:"path"`
			RunAttempt   int    `json:"run_attempt"`
			RunStartedAt string `json:"run_started_at"`
			UpdatedAt    string `json:"updated_at"`
			CreatedAt    string `json:"created_at"`
		} `json:"workflow_run"`
		Repository struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
		return err
	}
	repo, err := p.ensureRepo(ctx, ev.InstanceID, payload.Repository.ID, payload.Repository.Owner.Login, payload.Repository.Name, payload.Repository.FullName)
	if err != nil {
		return err
	}
	st, conc := forge.NormalizeStatus(payload.WorkflowRun.Status)
	if payload.WorkflowRun.Conclusion != "" {
		conc = forge.NormalizeConclusion(payload.WorkflowRun.Conclusion)
		st = models.StatusCompleted
	}
	attempt := payload.WorkflowRun.RunAttempt
	if attempt == 0 {
		attempt = 1
	}
	started := parseWebhookTime(payload.WorkflowRun.RunStartedAt)
	if started == nil {
		started = parseWebhookTime(payload.WorkflowRun.CreatedAt)
	}
	var completed *time.Time
	if st == models.StatusCompleted {
		completed = parseWebhookTime(payload.WorkflowRun.UpdatedAt)
	}
	saved, err := p.store.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{
		ExternalID:         payload.WorkflowRun.ID,
		Name:               workflows.DisplayWorkflowName(payload.WorkflowRun.Name, payload.WorkflowRun.Path),
		Event:              payload.WorkflowRun.Event,
		Branch:             payload.WorkflowRun.HeadBranch,
		CommitSHA:          payload.WorkflowRun.HeadSHA,
		Status:             st,
		Conclusion:         conc,
		UpstreamStatus:     payload.WorkflowRun.Status,
		UpstreamConclusion: payload.WorkflowRun.Conclusion,
		HTMLURL:            payload.WorkflowRun.HTMLURL,
		WorkflowPath:       workflows.NormalizeWorkflowPath(payload.WorkflowRun.Path),
		RunAttempt:         attempt,
		StartedAt:          started,
		CompletedAt:        completed,
	})
	if err != nil {
		return err
	}
	if saved.CommitSHA != "" {
		if runs, err := p.store.ListWorkflowRunsByCommitSHA(ctx, repo.ID, saved.CommitSHA); err == nil {
			if state := forge.AggregateCIState(runs); state != "" {
				_ = p.store.SetPullRequestsCIStateByHeadSHA(ctx, repo.ID, saved.CommitSHA, state)
			}
		}
	}
	if p.att != nil {
		_ = p.att.EvaluateRun(ctx, ev.InstanceID, saved)
	}
	if p.hub != nil {
		p.hub.Publish(realtime.Event{Type: "workflow_run", ID: saved.ID, RepoID: repo.ID})
	}
	return nil
}

func (p *Processor) applyJob(ctx context.Context, ev store.WebhookEvent) error {
	var payload struct {
		WorkflowJob struct {
			ID          int64           `json:"id"`
			RunID       int64           `json:"run_id"`
			Name        string          `json:"name"`
			Status      string          `json:"status"`
			Conclusion  string          `json:"conclusion"`
			HTMLURL     string          `json:"html_url"`
			StartedAt   string          `json:"started_at"`
			CompletedAt string          `json:"completed_at"`
			Steps       json.RawMessage `json:"steps"`
		} `json:"workflow_job"`
		Repository struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
		return err
	}
	repo, err := p.ensureRepo(ctx, ev.InstanceID, payload.Repository.ID, payload.Repository.Owner.Login, payload.Repository.Name, payload.Repository.FullName)
	if err != nil {
		return err
	}
	run, err := p.store.GetWorkflowRunByExternalID(ctx, repo.ID, payload.WorkflowJob.RunID)
	if err != nil {
		run, err = p.store.UpsertWorkflowRun(ctx, repo.ID, models.WorkflowRun{ExternalID: payload.WorkflowJob.RunID, Name: "unknown"})
		if err != nil {
			return err
		}
	}
	st, conc := forge.NormalizeStatus(payload.WorkflowJob.Status)
	if payload.WorkflowJob.Conclusion != "" {
		conc = forge.NormalizeConclusion(payload.WorkflowJob.Conclusion)
		st = models.StatusCompleted
	}
	var steps *string
	if len(payload.WorkflowJob.Steps) > 0 && string(payload.WorkflowJob.Steps) != "null" {
		s := string(payload.WorkflowJob.Steps)
		steps = &s
	}
	job, err := p.store.UpsertJob(ctx, repo.ID, run.ID, models.Job{
		ExternalID:         payload.WorkflowJob.ID,
		Name:               payload.WorkflowJob.Name,
		Status:             st,
		Conclusion:         conc,
		UpstreamStatus:     payload.WorkflowJob.Status,
		UpstreamConclusion: payload.WorkflowJob.Conclusion,
		HTMLURL:            payload.WorkflowJob.HTMLURL,
		StartedAt:          parseWebhookTime(payload.WorkflowJob.StartedAt),
		CompletedAt:        parseWebhookTime(payload.WorkflowJob.CompletedAt),
		StepsJSON:          steps,
	})
	if err != nil {
		return err
	}
	if p.att != nil {
		_ = p.att.EvaluateJob(ctx, ev.InstanceID, job, run)
	}
	if p.hub != nil {
		p.hub.Publish(realtime.Event{Type: "workflow_job", ID: job.ID, RepoID: repo.ID})
	}
	return nil
}

func (p *Processor) applyRepository(ctx context.Context, ev store.WebhookEvent) error {
	var payload struct {
		Action     string `json:"action"`
		Repository struct {
			ID            int64  `json:"id"`
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			Private       bool   `json:"private"`
			Fork          bool   `json:"fork"`
			Empty         bool   `json:"empty"`
			Archived      bool   `json:"archived"`
			HTMLURL       string `json:"html_url"`
			DefaultBranch string `json:"default_branch"`
			Owner         struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
		return err
	}
	if payload.Action == "deleted" {
		repo, err := p.store.GetRepositoryByExternalID(ctx, ev.InstanceID, payload.Repository.ID)
		if err != nil {
			return nil
		}
		_ = p.store.DeleteAccessForRepo(ctx, repo.ID)
		return p.store.SoftDeleteRepository(ctx, repo.ID)
	}
	repo, err := p.store.UpsertRepository(ctx, ev.InstanceID, models.Repository{
		ExternalID:    payload.Repository.ID,
		Owner:         payload.Repository.Owner.Login,
		Name:          payload.Repository.Name,
		FullName:      payload.Repository.FullName,
		Private:       payload.Repository.Private,
		Fork:          payload.Repository.Fork,
		Empty:         payload.Repository.Empty,
		Archived:      payload.Repository.Archived,
		HTMLURL:       payload.Repository.HTMLURL,
		DefaultBranch: payload.Repository.DefaultBranch,
	})
	if err != nil {
		return err
	}
	// Collaborator/visibility changes are not always distinguishable; invalidate ACL for the repo
	// so the next periodic refresh re-grants correctly.
	switch payload.Action {
	case "privatized", "publicized", "transferred":
		_ = p.store.DeleteAccessForRepo(ctx, repo.ID)
	}
	return nil
}

func (p *Processor) ensureRepo(ctx context.Context, instanceID, externalID int64, owner, name, full string) (*models.Repository, error) {
	repo, err := p.store.GetRepositoryByExternalID(ctx, instanceID, externalID)
	if err == nil {
		return repo, nil
	}
	return p.store.UpsertRepository(ctx, instanceID, models.Repository{
		ExternalID: externalID, Owner: owner, Name: name, FullName: full,
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func parseWebhookTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

// VerifySignatureForTest exports HMAC check for tests.
func VerifySignatureForTest(secret string, body []byte, sig string) bool {
	return validHMAC(secret, body, sig)
}

// VerifyGitHubSignatureForTest exports GitHub X-Hub-Signature-256 check for tests.
func VerifyGitHubSignatureForTest(secret string, body []byte, header string) bool {
	return validGitHubSignature(secret, body, header)
}
