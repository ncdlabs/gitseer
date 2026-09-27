package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/settings"
)

func (h *Handler) listInstances(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	items, err := h.settings.ListInstancesPublic(r.Context())
	if err != nil {
		h.log.Error("list instances", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []settings.InstancePublic{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createInstance(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	var body settings.InstancePatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ft := strings.ToLower(strings.TrimSpace(body.ForgeType))
	if ft != "" && !models.IsSupportedForgeType(ft) {
		writeError(w, http.StatusBadRequest, "forge_type must be a supported forge (gitea, github, gitlab, bitbucket, forgejo)")
		return
	}
	pub, err := h.settings.CreateInstance(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pub)
}

func (h *Handler) updateInstance(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body settings.InstancePatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ft := strings.ToLower(strings.TrimSpace(body.ForgeType))
	if ft != "" && !models.IsSupportedForgeType(ft) {
		writeError(w, http.StatusBadRequest, "forge_type must be a supported forge (gitea, github, gitlab, bitbucket, forgejo)")
		return
	}
	pub, err := h.settings.UpdateInstance(r.Context(), id, body)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pub)
}

func (h *Handler) deleteInstance(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.settings.DeleteInstance(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		h.log.Error("delete instance", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// forgeStatusEntry builds one system-status forges[] row from an instance.
// When redactSensitive is true, SSRF/unsigned flags are omitted (non-admin viewers).
func forgeStatusEntry(inst *models.Instance, redactSensitive bool) map[string]any {
	ft := inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	tokenConfigured := strings.TrimSpace(inst.SyncTokenCiphertext) != ""
	oauthConfigured := strings.TrimSpace(inst.OAuthClientID) != "" &&
		strings.TrimSpace(inst.OAuthClientSecretCipher) != ""
	entry := map[string]any{
		"instance_id":               inst.ID,
		"name":                      inst.Name,
		"forge_type":                ft,
		"url":                       inst.BaseURL,
		"configured":                tokenConfigured,
		"connected":                 strings.TrimSpace(inst.Version) != "",
		"webhook_hmac":              strings.TrimSpace(inst.WebhookSecretCiphertext) != "",
		"oauth_configured":          oauthConfigured,
		"token_configured":          tokenConfigured,
		"webhook_secret_configured": strings.TrimSpace(inst.WebhookSecretCiphertext) != "",
	}
	if !redactSensitive {
		entry["allow_private_network"] = inst.AllowPrivateNetwork
		entry["allow_unsigned"] = inst.AllowUnsignedWebhooks
	}
	if strings.TrimSpace(inst.Version) != "" {
		entry["version"] = inst.Version
	}
	if strings.TrimSpace(inst.CapabilitiesJSON) != "" && inst.CapabilitiesJSON != "{}" {
		entry["capabilities"] = json.RawMessage(inst.CapabilitiesJSON)
	}
	return entry
}
