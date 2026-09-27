package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/forge/gitea"
	"github.com/ncdlabs/gitseer/internal/forge/github"
	"github.com/ncdlabs/gitseer/internal/models"
)

type ensureWebhookBody struct {
	Org  string `json:"org"`  // GitHub: preferred org for org hook
	Repo string `json:"repo"` // GitHub: owner/name or name under org
}

type verifyWebhookBody struct {
	// Confirm marks verified after the operator sent a forge "Ping" / test delivery.
	// When false (default), only arms a pending token that the next accepted delivery consumes.
	Confirm bool `json:"confirm"`
}

func (h *Handler) ensureInstanceWebhook(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.store.GetInstanceByID(r.Context(), id)
	if err != nil || inst == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	var body ensureWebhookBody
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	ft := inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	token, err := h.instanceSyncToken(r.Context(), *inst)
	if err != nil || token == "" {
		writeError(w, http.StatusBadRequest, "instance sync token required to ensure webhook")
		return
	}
	secret, err := h.instanceWebhookSecret(r.Context(), *inst)
	if err != nil || secret == "" {
		writeError(w, http.StatusBadRequest, "instance webhook secret required; set it in Integration first")
		return
	}
	if h.webhookDeliveryURLBase() == "" {
		writeError(w, http.StatusBadRequest, "GitSeer public URL (server.external_url) is required")
		return
	}

	now := time.Now().UTC()
	var (
		created  bool
		manual   bool
		hookID   int64
		hint     string
		delivery string
	)

	switch ft {
	case models.ForgeTypeGitHub:
		delivery = h.githubWebhookDeliveryURL(inst.ID)
		client, cerr := github.New(inst.BaseURL, token, inst.AllowPrivateNetwork)
		if cerr != nil {
			_ = h.store.SetWebhookEnsureMeta(r.Context(), inst.ID, now, cerr.Error())
			writeError(w, http.StatusBadGateway, "github client: "+cerr.Error())
			return
		}
		org := strings.TrimSpace(body.Org)
		repo := strings.TrimSpace(body.Repo)
		var hook *github.Hook
		var ensureErr error
		switch {
		case org != "" && repo != "":
			owner := org
			name := repo
			if strings.Contains(repo, "/") {
				parts := strings.SplitN(repo, "/", 2)
				owner, name = parts[0], parts[1]
			}
			hook, created, ensureErr = client.EnsureRepoWebhook(r.Context(), owner, name, delivery, secret)
		case org != "":
			hook, created, ensureErr = client.EnsureOrgWebhook(r.Context(), org, delivery, secret)
		default:
			orgs, lerr := client.ListMembershipOrgs(r.Context())
			if lerr != nil {
				ensureErr = lerr
			} else {
				var lastErr error
				for _, o := range orgs {
					if !client.CanManageOrgHooks(r.Context(), o) {
						continue
					}
					hook, created, ensureErr = client.EnsureOrgWebhook(r.Context(), o, delivery, secret)
					if ensureErr == nil {
						org = o
						break
					}
					lastErr = ensureErr
				}
				if hook == nil {
					manual = true
					if lastErr != nil {
						ensureErr = lastErr
					} else {
						ensureErr = fmt.Errorf("no org with admin:org_hook; pass {\"org\":\"…\"} or add the webhook manually")
					}
				}
			}
		}
		if ensureErr != nil {
			_ = h.store.SetWebhookEnsureMeta(r.Context(), inst.ID, now, ensureErr.Error())
			if manual {
				preview := github.NewWebhookPreview(delivery, secret)
				writeJSON(w, http.StatusOK, map[string]any{
					"ok":           false,
					"created":      false,
					"updated":      false,
					"manual":       true,
					"delivery_url": delivery,
					"webhook":      preview,
					"hint":         ensureErr.Error(),
					"instance_id":  inst.ID,
				})
				return
			}
			writeError(w, http.StatusBadGateway, "failed to ensure github webhook: "+ensureErr.Error())
			return
		}
		if hook != nil {
			hookID = hook.ID
		}
		hint = "GitHub org/repo hook ensured"
		if org != "" {
			hint = fmt.Sprintf("GitHub webhook ensured on org %s", org)
		}
	default:
		delivery = h.giteaWebhookDeliveryURL(inst.ID)
		client, cerr := gitea.New(inst.BaseURL, token, inst.AllowPrivateNetwork)
		if cerr != nil {
			_ = h.store.SetWebhookEnsureMeta(r.Context(), inst.ID, now, cerr.Error())
			writeError(w, http.StatusBadGateway, "gitea client: "+cerr.Error())
			return
		}
		hook, wasCreated, ensureErr := client.EnsureSystemWebhook(r.Context(), delivery, secret)
		if ensureErr != nil {
			_ = h.store.SetWebhookEnsureMeta(r.Context(), inst.ID, now, ensureErr.Error())
			writeError(w, http.StatusBadGateway, "failed to ensure system webhook: "+ensureErr.Error())
			return
		}
		created = wasCreated
		if hook != nil {
			hookID = hook.ID
		}
		hint = "Gitea system webhook ensured"
	}

	_ = h.store.SetWebhookEnsureMeta(r.Context(), inst.ID, now, "")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"created":      created,
		"updated":      !created && !manual,
		"manual":       manual,
		"hook_id":      hookID,
		"delivery_url": delivery,
		"hint":         hint,
		"instance_id":  inst.ID,
		"ensure_at":    now.Format(time.RFC3339Nano),
	})
}

func (h *Handler) verifyInstanceWebhook(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inst, err := h.store.GetInstanceByID(r.Context(), id)
	if err != nil || inst == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	var body verifyWebhookBody
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	if body.Confirm {
		stats, serr := h.store.WebhookStatsSince(r.Context(), id, time.Now().UTC().Add(-15*time.Minute))
		if serr == nil && stats != nil && stats.Total > 0 {
			if err := h.store.MarkWebhookVerifiedForce(r.Context(), id); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to mark verified")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":          true,
				"verified":    true,
				"mode":        "confirm",
				"instance_id": id,
			})
			return
		}
		writeError(w, http.StatusBadRequest, "no webhook deliveries in the last 15 minutes; send a Ping/test from the forge, then Confirm again")
		return
	}

	tok, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate verify token")
		return
	}
	if err := h.store.SetWebhookVerifyPending(r.Context(), id, tok); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to arm verification")
		return
	}
	ft := inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	delivery := h.giteaWebhookDeliveryURL(id)
	if ft == models.ForgeTypeGitHub {
		delivery = h.githubWebhookDeliveryURL(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"verified":     false,
		"pending":      true,
		"mode":         "arm",
		"delivery_url": delivery,
		"hint":         "Send a Ping / Redeploy / test delivery from the forge to this URL. The next accepted delivery will mark the webhook verified.",
		"instance_id":  id,
	})
}

func (h *Handler) instanceWebhookSecret(_ context.Context, inst models.Instance) (string, error) {
	if strings.TrimSpace(inst.WebhookSecretCiphertext) == "" {
		return "", fmt.Errorf("no webhook secret")
	}
	if h.settings == nil {
		return "", fmt.Errorf("settings unavailable")
	}
	return h.settings.OpenSecret(inst.WebhookSecretCiphertext)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// enrichForgeStatusOps adds sync/lease/webhook/capability ops fields to a forge status entry.
func (h *Handler) enrichForgeStatusOps(ctx context.Context, inst *models.Instance, entry map[string]any) {
	if inst == nil || entry == nil {
		return
	}
	since := time.Now().UTC().Add(-24 * time.Hour)

	if st, err := h.store.GetSyncState(ctx, inst.ID, "instance"); err == nil && st != nil {
		entry["sync_phase"] = st.Phase
		if st.LastSuccessAt != nil {
			entry["sync_last_success_at"] = st.LastSuccessAt.UTC().Format(time.RFC3339Nano)
		}
		if st.LastError != "" {
			entry["sync_last_error"] = st.LastError
		}
	}
	if lease, err := h.store.GetSyncLease(ctx, inst.ID); err == nil && lease != nil {
		entry["lease_holder"] = lease.Holder
		entry["lease_expires_at"] = lease.ExpiresAt.UTC().Format(time.RFC3339Nano)
		entry["lease_active"] = lease.ExpiresAt.After(time.Now().UTC())
	}
	if stats, err := h.store.WebhookStatsSince(ctx, inst.ID, since); err == nil && stats != nil {
		entry["webhook_stats_24h"] = map[string]any{
			"ok":         stats.OK,
			"failed":     stats.Failed,
			"pending":    stats.Pending,
			"processing": stats.Processing,
			"total":      stats.Total,
		}
		if stats.LastAt != nil {
			entry["webhook_last_at"] = stats.LastAt.UTC().Format(time.RFC3339Nano)
		}
		if stats.LastError != "" {
			entry["webhook_last_error"] = stats.LastError
		}
		if stats.LastStatus != "" {
			entry["webhook_last_status"] = stats.LastStatus
		}
		if stats.LastEvent != "" {
			entry["webhook_last_event"] = stats.LastEvent
		}
	}
	if inst.WebhookVerifiedAt != nil {
		entry["webhook_verified_at"] = inst.WebhookVerifiedAt.UTC().Format(time.RFC3339Nano)
		entry["webhook_verified"] = true
	} else {
		entry["webhook_verified"] = false
	}
	if inst.WebhookEnsureAt != nil {
		entry["webhook_ensure_at"] = inst.WebhookEnsureAt.UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(inst.WebhookEnsureError) != "" {
		entry["webhook_ensure_error"] = inst.WebhookEnsureError
	}
	entry["webhook_verify_pending"] = strings.TrimSpace(inst.WebhookVerifyToken) != ""

	entry["ops_checklist"] = h.instanceOpsChecklist(inst, entry)

	if caps := capabilityMatrix(inst.CapabilitiesJSON, inst.ForgeType); caps != nil {
		entry["capability_matrix"] = caps
	}

	inFlight, _ := h.store.CountInFlightWorkflowRuns(ctx, inst.ID)
	wfHooks, _ := h.store.CountWebhookEventsByTypeSince(ctx, inst.ID, since, []string{"workflow_run", "workflow_job", "check_run", "check_suite"})
	entry["active_actions_hint"] = activeActionsHint(inFlight, wfHooks, entry)
}

func (h *Handler) instanceOpsChecklist(inst *models.Instance, entry map[string]any) []map[string]any {
	items := make([]map[string]any, 0, 8)
	add := func(id, label, status, detail string) {
		items = append(items, map[string]any{
			"id": id, "label": label, "status": status, "detail": detail,
		})
	}

	tokenOK, _ := entry["token_configured"].(bool)
	if tokenOK {
		add("token", "Sync Token", "pass", "Service token configured")
	} else {
		add("token", "Sync Token", "fail", "Missing sync token")
	}

	whOK, _ := entry["webhook_secret_configured"].(bool)
	if whOK {
		add("webhook_secret", "Webhook Secret", "pass", "HMAC secret configured")
	} else {
		add("webhook_secret", "Webhook Secret", "fail", "Missing webhook secret")
	}

	if verified, _ := entry["webhook_verified"].(bool); verified {
		add("webhook_delivery", "Webhook Delivery", "pass", "Verified")
	} else if pending, _ := entry["webhook_verify_pending"].(bool); pending {
		add("webhook_delivery", "Webhook Delivery", "warn", "Waiting for forge Ping / test delivery")
	} else if whOK {
		add("webhook_delivery", "Webhook Delivery", "warn", "Not verified — use Verify Delivery")
	} else {
		add("webhook_delivery", "Webhook Delivery", "fail", "Configure secret then Ensure Webhook")
	}

	ft := inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	if ft == models.ForgeTypeGitea {
		if oauth, _ := entry["oauth_configured"].(bool); oauth {
			add("oauth", "OAuth App", "pass", "Client id and secret configured")
		} else {
			add("oauth", "OAuth App", "warn", "OAuth not configured (bootstrap-only login)")
		}
	}

	phase, _ := entry["sync_phase"].(string)
	if phase == "complete" {
		add("sync", "Last Sync", "pass", "Phase complete")
	} else if phase == "error" {
		detail := "Sync error"
		if e, ok := entry["sync_last_error"].(string); ok && e != "" {
			detail = e
		}
		add("sync", "Last Sync", "fail", detail)
	} else if phase != "" {
		add("sync", "Last Sync", "warn", "Phase: "+phase)
	} else {
		add("sync", "Last Sync", "warn", "No sync recorded yet — use Sync Now")
	}

	return items
}

func capabilityMatrix(capsJSON, forgeType string) []map[string]any {
	capsJSON = strings.TrimSpace(capsJSON)
	if capsJSON == "" || capsJSON == "{}" {
		return []map[string]any{
			{"id": "actions_api", "label": "Actions API", "status": "unknown", "detail": "Not probed yet"},
		}
	}
	var caps models.Capabilities
	if err := json.Unmarshal([]byte(capsJSON), &caps); err != nil {
		return nil
	}
	ft := forgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	row := func(id, label string, ok bool, okDetail, failDetail string) map[string]any {
		status := "pass"
		detail := okDetail
		if !ok {
			status = "warn"
			detail = failDetail
		}
		return map[string]any{"id": id, "label": label, "status": status, "detail": detail}
	}
	out := []map[string]any{
		row("actions_api", "Actions API", caps.ActionsAPI,
			"Workflow runs/jobs available", "Actions endpoints missing or empty — pipelines degrade"),
		row("job_logs", "Job Logs", caps.JobLogsAPI,
			"On-demand log fetch available", "Job logs API unavailable"),
		row("rerun_workflow", "Rerun Workflow", caps.RerunWorkflowAPI,
			"POST rerun available", "Rerun Workflow unavailable on this forge"),
		row("cancel_workflow", "Cancel Workflow", caps.CancelWorkflowAPI,
			"POST cancel available", "Cancel Workflow unavailable on this forge"),
		row("runners", "Runners API", caps.RunnersAPI,
			"Runner listing available — runner_unavailable_queued attention stays stubbed (no offline/queued signal)",
			"Runner-unavailable attention rule stays inactive (no runners API)"),
		row("workflow_webhooks", "Workflow Webhooks", caps.WorkflowRunWebhook || caps.WorkflowJobWebhook,
			"workflow_run / workflow_job events supported", "Rely on reconcile for run updates"),
	}
	if ft == models.ForgeTypeGitea {
		out = append(out, row("system_hooks", "System Hooks", caps.SystemHooksAPI,
			"Admin system hooks available", "Ensure Webhook may fail — create hook manually"))
		out = append(out, row("oauth_provider", "OAuth Provider", caps.OAuthProvider,
			"Forge can host OAuth apps", "OAuth login unavailable on this forge"))
	} else {
		out = append(out, map[string]any{
			"id":     "checks_vs_status",
			"label":  "Checks vs Commit Status",
			"status": "pass",
			"detail": "GitHub combines Checks API and commit status on sync",
		})
	}
	return out
}

func activeActionsHint(inFlight, workflowWebhooks24h int64, entry map[string]any) string {
	if inFlight > 0 {
		return ""
	}
	if workflowWebhooks24h == 0 {
		phase, _ := entry["sync_phase"].(string)
		if phase == "" {
			return "No in-flight runs and no workflow webhooks yet. Complete setup, Ensure Webhook, then Sync Now."
		}
		return "No in-flight runs and no workflow_* webhooks in the last 24h. Confirm the forge sends workflow_run/workflow_job (or check_run) events, or wait for reconcile."
	}
	return "No in-flight Actions right now. Recent workflow webhooks were received — idle is expected."
}

func (h *Handler) encryptionHealth(ctx context.Context) (healthy bool, source, errMsg string) {
	if h.settings == nil {
		return false, "", "settings unavailable"
	}
	configured := h.settings.EncryptionConfigured()
	source = h.settings.EncryptionSource()
	if !configured {
		return false, source, "encryption key not configured"
	}
	cipher, err := h.store.ProbeDecryptableSecret(ctx)
	if err != nil {
		return false, source, "probe query failed"
	}
	if strings.TrimSpace(cipher) != "" {
		if _, err := h.settings.OpenSecret(cipher); err != nil {
			return false, source, "decrypt failed — wrong key or corrupt ciphertext"
		}
		return true, source, ""
	}
	key := h.settings.EncryptionKeyBytes()
	if len(key) != 32 {
		return false, source, "encryption key length invalid"
	}
	sealed, err := gitseercrypto.Encrypt(key, "gitseer-encryption-canary")
	if err != nil {
		return false, source, "seal canary failed"
	}
	plain, err := gitseercrypto.Decrypt(key, sealed)
	if err != nil || plain != "gitseer-encryption-canary" {
		return false, source, "decrypt canary failed"
	}
	return true, source, ""
}
