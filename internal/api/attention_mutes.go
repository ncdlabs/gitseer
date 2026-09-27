package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ncdlabs/gitseer/internal/attention"
	"github.com/ncdlabs/gitseer/internal/store"
)

type muteAttentionBody struct {
	Until  string `json:"until"`  // "24h" | "7d" | "resolved"
	Reason string `json:"reason"`
}

type ruleOverrideBody struct {
	Overrides []store.AttentionRuleOverride `json:"overrides"`
}

func (h *Handler) muteAttention(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid attention id")
		return
	}
	item, err := h.store.GetAttentionByID(r.Context(), id)
	if err != nil || item == nil {
		writeError(w, http.StatusNotFound, "attention item not found")
		return
	}
	ok, err := h.authz.CanAccessRepo(r.Context(), user, item.RepoID)
	if err != nil || !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body muteAttentionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	until := strings.ToLower(strings.TrimSpace(body.Until))
	var untilAt *time.Time
	switch until {
	case "24h":
		t := time.Now().UTC().Add(24 * time.Hour)
		untilAt = &t
	case "7d":
		t := time.Now().UTC().Add(7 * 24 * time.Hour)
		untilAt = &t
	case "resolved", "until_resolved", "":
		untilAt = nil
	default:
		writeError(w, http.StatusBadRequest, `until must be "24h", "7d", or "resolved"`)
		return
	}

	mute := store.AttentionMute{
		RuleType:    item.Type,
		Fingerprint: item.Fingerprint,
		UntilAt:     untilAt,
		Reason:      strings.TrimSpace(body.Reason),
	}
	repoID := item.RepoID
	mute.RepoID = &repoID
	global := user != nil && user.IsBootstrapAdmin
	if user != nil {
		uid := user.ID
		mute.CreatedBy = &uid
		// Bootstrap admins create global mutes (evaluate skip for everyone).
		// Other users create personal mutes (list filter only).
		if !global {
			mute.UserID = &uid
		}
	}

	created, err := h.store.InsertAttentionMute(r.Context(), mute)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Global mutes hide for everyone immediately; personal mutes leave the row
	// so other viewers still see it (list filters for the muting user).
	if global {
		_ = h.store.ResolveAttentionByFingerprint(r.Context(), item.Fingerprint)
	}

	writeJSON(w, http.StatusOK, map[string]any{"mute": created})
}

func (h *Handler) unmuteAttention(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid attention id")
		return
	}
	item, err := h.store.GetAttentionByID(r.Context(), id)
	if err != nil || item == nil {
		// Allow unmute by fingerprint when item already resolved: look up via query? For now require id of known item.
		writeError(w, http.StatusNotFound, "attention item not found")
		return
	}
	ok, err := h.authz.CanAccessRepo(r.Context(), user, item.RepoID)
	if err != nil || !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	globalOnly := false
	var userID int64
	if user != nil {
		userID = user.ID
		if user.IsBootstrapAdmin {
			// Admin unmute clears all mutes on this fingerprint.
			userID = 0
			globalOnly = false
		}
	}
	if err := h.store.DeleteAttentionMutesByFingerprint(r.Context(), item.Fingerprint, userID, globalOnly); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) getAttentionRuleOverrides(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListAttentionRuleOverrides(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defaults := make([]map[string]string, 0, len(attention.KnownRuleTypes()))
	for _, rt := range attention.KnownRuleTypes() {
		defaults = append(defaults, map[string]string{
			"rule_type":        rt,
			"default_severity": attention.DefaultSeverityFor(rt),
		})
	}
	user := userFromCtx(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"overrides": list,
		"defaults":  defaults,
		"editable":  user != nil && user.IsBootstrapAdmin,
	})
}

func (h *Handler) putAttentionRuleOverrides(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "bootstrap admin required")
		return
	}
	var body ruleOverrideBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	known := map[string]struct{}{}
	for _, rt := range attention.KnownRuleTypes() {
		known[rt] = struct{}{}
	}
	cleaned := make([]store.AttentionRuleOverride, 0, len(body.Overrides))
	for _, o := range body.Overrides {
		rt := strings.TrimSpace(o.RuleType)
		sev := strings.ToLower(strings.TrimSpace(o.Severity))
		if rt == "" {
			continue
		}
		if _, ok := known[rt]; !ok {
			writeError(w, http.StatusBadRequest, "unknown rule_type: "+rt)
			return
		}
		if sev != attention.SeverityCritical && sev != attention.SeverityWarning && sev != attention.SeverityWaiting {
			writeError(w, http.StatusBadRequest, "severity must be critical, warning, or waiting")
			return
		}
		// Skip no-op overrides that match defaults.
		if sev == attention.DefaultSeverityFor(rt) {
			continue
		}
		cleaned = append(cleaned, store.AttentionRuleOverride{RuleType: rt, Severity: sev})
	}
	if err := h.store.ReplaceAttentionRuleOverrides(r.Context(), cleaned); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if h.att != nil {
		_ = h.att.ReloadSeverityOverrides(r.Context())
	}
	list, err := h.store.ListAttentionRuleOverrides(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"overrides": list})
}
