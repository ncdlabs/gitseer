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
	"github.com/ncdlabs/gitseer/internal/forge/bitbucket"
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

// HandleGitLabHTTPForInstance accepts a GitLab webhook for a specific GitSeer instance ID.
func (p *Processor) HandleGitLabHTTPForInstance(w http.ResponseWriter, r *http.Request, instanceID int64) {
	if instanceID <= 0 {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}
	p.handleForgeWebhook(w, r, instanceID, models.ForgeTypeGitLab)
}

// HandleBitbucketHTTPForInstance accepts a Bitbucket webhook for a specific GitSeer instance ID.
func (p *Processor) HandleBitbucketHTTPForInstance(w http.ResponseWriter, r *http.Request, instanceID int64) {
	if instanceID <= 0 {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}
	p.handleForgeWebhook(w, r, instanceID, models.ForgeTypeBitbucket)
}

// HandleForgejoHTTPForInstance accepts a Forgejo webhook (Gitea-compatible HMAC) for an instance.
func (p *Processor) HandleForgejoHTTPForInstance(w http.ResponseWriter, r *http.Request, instanceID int64) {
	if instanceID <= 0 {
		http.Error(w, "instance required", http.StatusBadRequest)
		return
	}
	p.handleForgeWebhook(w, r, instanceID, models.ForgeTypeForgejo)
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
	inheritLegacy := instanceID == 0 && (forgeType == "" || forgeType == models.ForgeTypeGitea)
	secret, allowUnsigned, err := p.webhookAuthForInstance(inst, inheritLegacy)
	if err != nil {
		p.log.Error("webhook secret decrypt", "instance_id", inst.ID, "err", err)
		http.Error(w, "webhook secret unavailable", http.StatusServiceUnavailable)
		return
	}
	if secret != "" {
		ok := false
		switch forgeType {
		case models.ForgeTypeGitHub, models.ForgeTypeBitbucket:
			sig := r.Header.Get("X-Hub-Signature-256")
			if sig == "" {
				sig = r.Header.Get("X-Hub-Signature")
			}
			ok = validGitHubSignature(secret, body, sig)
		case models.ForgeTypeGitLab:
			ok = validGitLabToken(secret, r.Header.Get("X-Gitlab-Token"))
			if !ok {
				// Optional HMAC when GitLab sends X-Gitlab-Signature / X-Hub-Signature-256.
				sig := r.Header.Get("X-Gitlab-Signature")
				if sig == "" {
					sig = r.Header.Get("X-Hub-Signature-256")
				}
				if sig != "" {
					ok = validGitHubSignature(secret, body, sig) || validHMAC(secret, body, sig)
				}
			}
		case models.ForgeTypeForgejo:
			sig := r.Header.Get("X-Forgejo-Signature")
			if sig == "" {
				sig = r.Header.Get("X-Gitea-Signature")
			}
			if sig != "" {
				ok = validHMAC(secret, body, sig)
			} else {
				ok = validGitHubSignature(secret, body, r.Header.Get("X-Hub-Signature-256"))
			}
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
	eventType = normalizeWebhookEventType(forgeType, eventType)
	payload := string(body)
	if norm, ok := normalizeWebhookPayload(forgeType, eventType, body); ok {
		payload = string(norm)
	}
	_, inserted, err := p.store.InsertWebhookEvent(r.Context(), inst.ID, delivery, eventType, payload)
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
	case models.ForgeTypeGitLab:
		eventType = r.Header.Get("X-Gitlab-Event")
		delivery = r.Header.Get("X-Gitlab-Event-UUID")
		if delivery == "" {
			delivery = r.Header.Get("X-Request-Id")
		}
		return eventType, delivery
	case models.ForgeTypeBitbucket:
		eventType = r.Header.Get("X-Event-Key")
		delivery = r.Header.Get("X-Request-UUID")
		if delivery == "" {
			delivery = r.Header.Get("X-Hook-UUID")
		}
		return eventType, delivery
	case models.ForgeTypeForgejo:
		eventType = r.Header.Get("X-Forgejo-Event")
		if eventType == "" {
			eventType = r.Header.Get("X-Gitea-Event")
		}
		if eventType == "" {
			eventType = r.Header.Get("X-GitHub-Event")
		}
		delivery = r.Header.Get("X-Forgejo-Delivery")
		if delivery == "" {
			delivery = r.Header.Get("X-Gitea-Delivery")
		}
		if delivery == "" {
			delivery = r.Header.Get("X-GitHub-Delivery")
		}
		return eventType, delivery
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

// validGitLabToken checks the shared secret token header (constant-time).
func validGitLabToken(secret, header string) bool {
	secret = strings.TrimSpace(secret)
	header = strings.TrimSpace(header)
	if secret == "" || header == "" {
		return false
	}
	return hmac.Equal([]byte(secret), []byte(header))
}

// normalizeWebhookEventType maps forge-specific event names onto the apply() vocabulary.
func normalizeWebhookEventType(forgeType, eventType string) string {
	et := strings.TrimSpace(eventType)
	switch forgeType {
	case models.ForgeTypeGitLab:
		switch strings.ToLower(et) {
		case "merge request hook", "merge_request":
			return "pull_request"
		case "pipeline hook", "pipeline":
			return "workflow_run"
		case "job hook", "build hook", "job", "build":
			return "workflow_job"
		case "note hook":
			return "pull_request_review"
		default:
			return et
		}
	case models.ForgeTypeBitbucket:
		switch strings.ToLower(et) {
		case "pullrequest:created", "pullrequest:updated", "pullrequest:fulfilled", "pullrequest:rejected", "pullrequest:approved", "pullrequest:unapproved":
			return "pull_request"
		case "repo:commit_status_created", "repo:commit_status_updated":
			return "status"
		default:
			return et
		}
	default:
		return et
	}
}

// normalizeWebhookPayload rewrites forge-specific bodies into the Gitea/GitHub-shaped
// JSON that applyPR / applyRun already understand. Returns ok=false to keep original.
func normalizeWebhookPayload(forgeType, eventType string, body []byte) ([]byte, bool) {
	switch forgeType {
	case models.ForgeTypeGitLab:
		switch eventType {
		case "pull_request":
			return normalizeGitLabMergeRequest(body)
		case "workflow_run":
			return normalizeGitLabPipeline(body)
		case "workflow_job":
			return normalizeGitLabJob(body)
		}
	case models.ForgeTypeBitbucket:
		switch eventType {
		case "pull_request":
			return normalizeBitbucketPullRequest(body)
		case "status":
			return normalizeBitbucketCommitStatus(body)
		}
	}
	return nil, false
}

func normalizeGitLabMergeRequest(body []byte) ([]byte, bool) {
	var raw struct {
		ObjectKind string `json:"object_kind"`
		Project    struct {
			ID                int64  `json:"id"`
			Name              string `json:"name"`
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
		ObjectAttributes struct {
			ID           int64  `json:"id"`
			IID          int64  `json:"iid"`
			Title        string `json:"title"`
			Description  string `json:"description"`
			State        string `json:"state"`
			Draft        bool   `json:"draft"`
			URL          string `json:"url"`
			CreatedAt    string `json:"created_at"`
			UpdatedAt    string `json:"updated_at"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
			LastCommit   *struct {
				ID string `json:"id"`
			} `json:"last_commit"`
		} `json:"object_attributes"`
		User *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, false
	}
	state := raw.ObjectAttributes.State
	if state == "opened" {
		state = "open"
	} else if state == "merged" || state == "closed" {
		state = "closed"
	}
	owner := ""
	if parts := strings.SplitN(raw.Project.PathWithNamespace, "/", 2); len(parts) > 0 {
		owner = parts[0]
	}
	headSHA := ""
	if raw.ObjectAttributes.LastCommit != nil {
		headSHA = raw.ObjectAttributes.LastCommit.ID
	}
	out := map[string]any{
		"action": "updated",
		"pull_request": map[string]any{
			"id":         raw.ObjectAttributes.ID,
			"number":     raw.ObjectAttributes.IID,
			"title":      raw.ObjectAttributes.Title,
			"body":       raw.ObjectAttributes.Description,
			"state":      state,
			"draft":      raw.ObjectAttributes.Draft,
			"html_url":   raw.ObjectAttributes.URL,
			"created_at": raw.ObjectAttributes.CreatedAt,
			"updated_at": raw.ObjectAttributes.UpdatedAt,
			"user": func() any {
				if raw.User == nil {
					return nil
				}
				return map[string]any{"id": raw.User.ID, "login": raw.User.Username}
			}(),
			"head": map[string]any{"ref": raw.ObjectAttributes.SourceBranch, "sha": headSHA},
			"base": map[string]any{"ref": raw.ObjectAttributes.TargetBranch, "sha": ""},
		},
		"repository": map[string]any{
			"id":        raw.Project.ID,
			"name":      raw.Project.Name,
			"full_name": raw.Project.PathWithNamespace,
			"owner":     map[string]any{"login": owner},
		},
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

func normalizeGitLabPipeline(body []byte) ([]byte, bool) {
	var raw struct {
		ObjectKind string `json:"object_kind"`
		Project    struct {
			ID                int64  `json:"id"`
			Name              string `json:"name"`
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
		ObjectAttributes struct {
			ID         int64  `json:"id"`
			IID        int64  `json:"iid"`
			Name       string `json:"name"`
			Ref        string `json:"ref"`
			SHA        string `json:"sha"`
			Status     string `json:"status"`
			Source     string `json:"source"`
			CreatedAt  string `json:"created_at"`
			FinishedAt string `json:"finished_at"`
			URL        string `json:"url"`
		} `json:"object_attributes"`
		Commit *struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, false
	}
	runID := raw.ObjectAttributes.ID
	if runID == 0 {
		runID = raw.ObjectAttributes.IID
	}
	if runID == 0 || raw.Project.ID == 0 {
		return nil, false
	}
	owner, name := splitOwnerName(raw.Project.PathWithNamespace, raw.Project.Name)
	if owner == "" || name == "" {
		return nil, false
	}
	sha := raw.ObjectAttributes.SHA
	if sha == "" && raw.Commit != nil {
		sha = raw.Commit.ID
	}
	runName := strings.TrimSpace(raw.ObjectAttributes.Name)
	if runName == "" {
		runName = "pipeline"
	}
	status, conclusion := gitlabWebhookStatus(raw.ObjectAttributes.Status)
	html := raw.ObjectAttributes.URL
	if html == "" && raw.Project.PathWithNamespace != "" {
		html = fmt.Sprintf("%s/-/pipelines/%d", strings.TrimRight(raw.Project.PathWithNamespace, "/"), runID)
	}
	out := map[string]any{
		"workflow_run": map[string]any{
			"id":             runID,
			"name":           runName,
			"event":          raw.ObjectAttributes.Source,
			"status":         status,
			"conclusion":     conclusion,
			"html_url":       html,
			"head_branch":    raw.ObjectAttributes.Ref,
			"head_sha":       sha,
			"path":           ".gitlab-ci.yml",
			"run_attempt":    1,
			"run_started_at": raw.ObjectAttributes.CreatedAt,
			"updated_at":     firstNonEmpty(raw.ObjectAttributes.FinishedAt, raw.ObjectAttributes.CreatedAt),
			"created_at":     raw.ObjectAttributes.CreatedAt,
		},
		"repository": map[string]any{
			"id":        raw.Project.ID,
			"name":      name,
			"full_name": raw.Project.PathWithNamespace,
			"owner":     map[string]any{"login": owner},
		},
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

func normalizeGitLabJob(body []byte) ([]byte, bool) {
	var raw struct {
		ObjectKind       string `json:"object_kind"`
		Ref              string `json:"ref"`
		BuildID          int64  `json:"build_id"`
		BuildName        string `json:"build_name"`
		BuildStatus      string `json:"build_status"`
		BuildStartedAt   string `json:"build_started_at"`
		BuildFinishedAt  string `json:"build_finished_at"`
		BuildFailureReason string `json:"build_failure_reason"`
		PipelineID       int64  `json:"pipeline_id"`
		ProjectID        int64  `json:"project_id"`
		ProjectName      string `json:"project_name"`
		Project          *struct {
			ID                int64  `json:"id"`
			Name              string `json:"name"`
			PathWithNamespace string `json:"path_with_namespace"`
			WebURL            string `json:"web_url"`
		} `json:"project"`
		Repository *struct {
			Name        string `json:"name"`
			URL         string `json:"url"`
			Homepage    string `json:"homepage"`
			Description string `json:"description"`
		} `json:"repository"`
		Commit *struct {
			SHA string `json:"sha"`
			ID  string `json:"id"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, false
	}
	jobID := raw.BuildID
	runID := raw.PipelineID
	if jobID == 0 || runID == 0 {
		return nil, false
	}
	projectID := raw.ProjectID
	fullName, name, owner := "", raw.ProjectName, ""
	if raw.Project != nil {
		if projectID == 0 {
			projectID = raw.Project.ID
		}
		if raw.Project.Name != "" {
			name = raw.Project.Name
		}
		fullName = raw.Project.PathWithNamespace
	}
	if fullName == "" && raw.Repository != nil && raw.Repository.Homepage != "" {
		// Best-effort: leave fullName empty and rely on name/owner split below.
	}
	owner, name = splitOwnerName(fullName, name)
	if projectID == 0 || owner == "" || name == "" {
		return nil, false
	}
	if fullName == "" {
		fullName = owner + "/" + name
	}
	status, conclusion := gitlabWebhookStatus(raw.BuildStatus)
	html := ""
	if raw.Project != nil && raw.Project.WebURL != "" {
		html = fmt.Sprintf("%s/-/jobs/%d", strings.TrimRight(raw.Project.WebURL, "/"), jobID)
	}
	out := map[string]any{
		"workflow_job": map[string]any{
			"id":           jobID,
			"run_id":       runID,
			"name":         raw.BuildName,
			"status":       status,
			"conclusion":   conclusion,
			"html_url":     html,
			"started_at":   raw.BuildStartedAt,
			"completed_at": raw.BuildFinishedAt,
			"message":      strings.TrimSpace(raw.BuildFailureReason),
		},
		"repository": map[string]any{
			"id":        projectID,
			"name":      name,
			"full_name": fullName,
			"owner":     map[string]any{"login": owner},
		},
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

func gitlabWebhookStatus(s string) (status, conclusion string) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "created", "waiting_for_resource", "preparing", "pending", "manual", "scheduled":
		return "queued", ""
	case "running":
		return "in_progress", ""
	case "success":
		return "completed", "success"
	case "failed":
		return "completed", "failure"
	case "canceled", "cancelled":
		return "completed", "cancelled"
	case "skipped":
		return "completed", "skipped"
	default:
		return s, ""
	}
}

func splitOwnerName(pathWithNamespace, fallbackName string) (owner, name string) {
	pathWithNamespace = strings.TrimSpace(pathWithNamespace)
	fallbackName = strings.TrimSpace(fallbackName)
	if pathWithNamespace != "" {
		parts := strings.Split(pathWithNamespace, "/")
		if len(parts) >= 2 {
			return parts[0], parts[len(parts)-1]
		}
		if len(parts) == 1 && parts[0] != "" {
			return parts[0], fallbackName
		}
	}
	return "", fallbackName
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func normalizeBitbucketPullRequest(body []byte) ([]byte, bool) {
	var raw struct {
		PullRequest struct {
			ID          int64  `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			State       string `json:"state"`
			CreatedOn   string `json:"created_on"`
			UpdatedOn   string `json:"updated_on"`
			Author      *struct {
				UUID     string `json:"uuid"`
				Nickname string `json:"nickname"`
				Username string `json:"username"`
			} `json:"author"`
			Source *struct {
				Branch *struct {
					Name string `json:"name"`
				} `json:"branch"`
				Commit *struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"source"`
			Destination *struct {
				Branch *struct {
					Name string `json:"name"`
				} `json:"branch"`
			} `json:"destination"`
			Links *struct {
				HTML *struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"pullrequest"`
		Repository struct {
			UUID     string `json:"uuid"`
			Name     string `json:"name"`
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, false
	}
	state := strings.ToLower(raw.PullRequest.State)
	if state == "open" {
		state = "open"
	} else {
		state = "closed"
	}
	owner, name := "", raw.Repository.Name
	if parts := strings.SplitN(raw.Repository.FullName, "/", 2); len(parts) == 2 {
		owner, name = parts[0], parts[1]
	}
	html := ""
	if raw.PullRequest.Links != nil && raw.PullRequest.Links.HTML != nil {
		html = raw.PullRequest.Links.HTML.Href
	}
	login := ""
	if raw.PullRequest.Author != nil {
		login = raw.PullRequest.Author.Username
		if login == "" {
			login = raw.PullRequest.Author.Nickname
		}
	}
	srcRef, headSHA := "", ""
	if raw.PullRequest.Source != nil {
		if raw.PullRequest.Source.Branch != nil {
			srcRef = raw.PullRequest.Source.Branch.Name
		}
		if raw.PullRequest.Source.Commit != nil {
			headSHA = raw.PullRequest.Source.Commit.Hash
		}
	}
	dstRef := ""
	if raw.PullRequest.Destination != nil && raw.PullRequest.Destination.Branch != nil {
		dstRef = raw.PullRequest.Destination.Branch.Name
	}
	// StableID keeps webhook repo external_id aligned with Bitbucket API sync.
	repoID := bitbucket.StableID(raw.Repository.UUID)
	out := map[string]any{
		"action": "updated",
		"pull_request": map[string]any{
			"id":         raw.PullRequest.ID,
			"number":     raw.PullRequest.ID,
			"title":      raw.PullRequest.Title,
			"body":       raw.PullRequest.Description,
			"state":      state,
			"html_url":   html,
			"created_at": raw.PullRequest.CreatedOn,
			"updated_at": raw.PullRequest.UpdatedOn,
			"user":       map[string]any{"id": 0, "login": login},
			"head":       map[string]any{"ref": srcRef, "sha": headSHA},
			"base":       map[string]any{"ref": dstRef, "sha": ""},
		},
		"repository": map[string]any{
			"id":        repoID,
			"name":      name,
			"full_name": raw.Repository.FullName,
			"owner":     map[string]any{"login": owner},
		},
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

func normalizeBitbucketCommitStatus(body []byte) ([]byte, bool) {
	var raw struct {
		Repository struct {
			UUID     string `json:"uuid"`
			Name     string `json:"name"`
			FullName string `json:"full_name"`
		} `json:"repository"`
		CommitStatus struct {
			State  string `json:"state"`
			Key    string `json:"key"`
			Name   string `json:"name"`
			URL    string `json:"url"`
			Type   string `json:"type"`
			Commit *struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"commit_status"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, false
	}
	repoID := bitbucket.StableID(raw.Repository.UUID)
	sha := ""
	if raw.CommitStatus.Commit != nil {
		sha = raw.CommitStatus.Commit.Hash
	}
	if repoID == 0 || sha == "" {
		return nil, false
	}
	state := mapBitbucketCIState(raw.CommitStatus.State)
	if state == "" {
		return nil, false
	}
	out := map[string]any{
		"sha":   sha,
		"state": state,
		"repository": map[string]any{
			"id":        repoID,
			"name":      raw.Repository.Name,
			"full_name": raw.Repository.FullName,
		},
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

func mapBitbucketCIState(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SUCCESSFUL", "SUCCESS":
		return "success"
	case "FAILED", "FAILURE", "ERROR":
		return "failure"
	case "INPROGRESS", "IN_PROGRESS", "PENDING":
		return "pending"
	case "STOPPED", "CANCELLED", "CANCELED":
		return "cancelled"
	default:
		return forge.NormalizeCIState(s)
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
			StartedAt    string `json:"started_at"`
			CompletedAt  string `json:"completed_at"`
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
	if payload.WorkflowRun.ID == 0 {
		return fmt.Errorf("workflow_run external_id is required")
	}
	owner := strings.TrimSpace(payload.Repository.Owner.Login)
	name := strings.TrimSpace(payload.Repository.Name)
	if payload.Repository.ID == 0 || owner == "" || name == "" {
		return fmt.Errorf("workflow_run repository id/owner/name required")
	}
	repo, err := p.ensureRepo(ctx, ev.InstanceID, payload.Repository.ID, owner, name, payload.Repository.FullName)
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
	started := parseWebhookTime(payload.WorkflowRun.StartedAt)
	if started == nil {
		started = parseWebhookTime(payload.WorkflowRun.RunStartedAt)
	}
	if started == nil {
		started = parseWebhookTime(payload.WorkflowRun.CreatedAt)
	}
	var completed *time.Time
	if st == models.StatusCompleted {
		completed = parseWebhookTime(payload.WorkflowRun.CompletedAt)
		if completed == nil {
			completed = parseWebhookTime(payload.WorkflowRun.UpdatedAt)
		}
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
			Labels      json.RawMessage `json:"labels"`
			Message     string          `json:"message"`
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
	if payload.WorkflowJob.ID == 0 || payload.WorkflowJob.RunID == 0 {
		return fmt.Errorf("workflow_job external_id and run_id are required")
	}
	owner := strings.TrimSpace(payload.Repository.Owner.Login)
	name := strings.TrimSpace(payload.Repository.Name)
	if payload.Repository.ID == 0 || owner == "" || name == "" {
		return fmt.Errorf("workflow_job repository id/owner/name required")
	}
	repo, err := p.ensureRepo(ctx, ev.InstanceID, payload.Repository.ID, owner, name, payload.Repository.FullName)
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
	var labels *string
	if len(payload.WorkflowJob.Labels) > 0 && string(payload.WorkflowJob.Labels) != "null" {
		s := string(payload.WorkflowJob.Labels)
		labels = &s
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
		LabelsJSON:         labels,
		Message:            strings.TrimSpace(payload.WorkflowJob.Message),
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
