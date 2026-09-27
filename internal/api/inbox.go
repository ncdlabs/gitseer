package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/store"
)

func (h *Handler) getInbox(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit = limitOr(limit, 50)
	reason := strings.TrimSpace(strings.ToLower(q.Get("reason")))
	switch reason {
	case "", store.InboxReasonAuthor, store.InboxReasonRequestedReviewer, store.InboxReasonFailingCI, store.InboxReasonBlockedOnMe:
	default:
		writeError(w, http.StatusBadRequest, "invalid reason")
		return
	}
	items, total, err := h.store.ListInbox(r.Context(), store.ListInboxOpts{
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll,
		Identity: store.InboxIdentity{
			Login:        user.Login,
			GiteaUserID:  user.GiteaUserID,
			GitHubUserID: user.GitHubUserID,
		},
		Reason: reason, Query: q.Get("q"),
		ForgeType: q.Get("forge_type"), InstanceID: parseQueryInt64(q.Get("instance_id")),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []store.InboxItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) listSavedFilters(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	list, err := h.store.ListSavedFilters(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *Handler) createSavedFilter(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	body, ok := decodeSavedFilterBody(w, r)
	if !ok {
		return
	}
	f, err := h.store.InsertSavedFilter(r.Context(), user.ID, body.Name, body.Query)
	if err != nil {
		writeSavedFilterStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handler) updateSavedFilter(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid filter id")
		return
	}
	body, ok := decodeSavedFilterBody(w, r)
	if !ok {
		return
	}
	f, err := h.store.UpdateSavedFilter(r.Context(), user.ID, id, body.Name, body.Query)
	if err != nil {
		writeSavedFilterStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handler) deleteSavedFilter(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid filter id")
		return
	}
	if err := h.store.DeleteSavedFilter(r.Context(), user.ID, id); err != nil {
		writeSavedFilterStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type savedFilterBody struct {
	Name  string          `json:"name"`
	Query json.RawMessage `json:"query"`
}

func decodeSavedFilterBody(w http.ResponseWriter, r *http.Request) (savedFilterBody, bool) {
	var body savedFilterBody
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 32<<10))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return body, false
	}
	if len(raw) == 0 {
		writeError(w, http.StatusBadRequest, "invalid body")
		return body, false
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return body, false
	}
	return body, true
}

func writeSavedFilterStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "name required"),
		strings.Contains(msg, "name too long"),
		strings.Contains(msg, "query must"),
		strings.Contains(msg, "query too large"),
		strings.Contains(msg, "filter name already exists"):
		writeError(w, http.StatusBadRequest, msg)
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
