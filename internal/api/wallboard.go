package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func wallboardTokenFromRequest(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) >= 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
		return t
	}
	return ""
}

func (h *Handler) requireWallboardToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plain := wallboardTokenFromRequest(r)
		if plain == "" {
			writeError(w, http.StatusUnauthorized, "wallboard token required")
			return
		}
		tok, err := h.store.LookupWallboardTokenByPlain(r.Context(), plain)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "invalid wallboard token")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		_ = tok
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) wallboardSnapshot(w http.ResponseWriter, r *http.Request) {
	// Bootstrap scope for wallboard — token is the ACL; treat as admin inventory view.
	sum, err := h.store.Summary(r.Context(), 0, true, nil, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	sum.Days = 0
	items, _, err := h.store.ListAttention(r.Context(), store.ListAttentionOpts{
		UserID: 0, BootstrapAll: true, OpenOnly: true, Limit: 40, Offset: 0,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []models.AttentionItem{}
	}
	stats, err := h.store.StatsBySection(r.Context(), 0, true, time.Now().UTC(), true, store.StatsSectionCore)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, models.WallboardSnapshot{
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Summary:             *sum,
		Attention:           items,
		AttentionBySeverity: stats.AttentionBySeverity,
	})
}

func (h *Handler) listWallboardTokens(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	items, err := h.store.ListWallboardTokens(r.Context(), r.URL.Query().Get("all") == "1")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createWallboardToken(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	tok, plain, err := h.store.CreateWallboardToken(r.Context(), body.Name, user.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":   tok,
		"secret":  plain, // shown once
		"warning": "Store this secret now; it cannot be retrieved again. Anyone with the token can read summary and attention (read-only).",
	})
}

func (h *Handler) revokeWallboardToken(w http.ResponseWriter, r *http.Request) {
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
	if err := h.store.RevokeWallboardToken(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
