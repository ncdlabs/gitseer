package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/auth"
)

func (h *Handler) githubOAuthLogin(w http.ResponseWriter, r *http.Request) {
	redirectTo := auth.SafeRedirectPath(r.URL.Query().Get("redirect"))
	var linkUserID *int64
	if r.URL.Query().Get("link") == "1" || strings.EqualFold(r.URL.Query().Get("link"), "true") {
		user, _, err := h.auth.UserFromRequest(r.Context(), r)
		if err != nil || user == nil || user.IsBootstrapAdmin {
			writeError(w, http.StatusUnauthorized, "sign in before linking GitHub")
			return
		}
		id := user.ID
		linkUserID = &id
		if redirectTo == "/" {
			redirectTo = "/settings#access"
		}
	}
	url, err := h.auth.BeginGitHubOAuth(r.Context(), w, redirectTo, linkUserID)
	if err != nil {
		if errors.Is(err, auth.ErrOAuthNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "github oauth is not configured; set OAuth client id on the GitHub instance and GITSEER_SERVER_EXTERNAL_URL")
			return
		}
		h.log.Error("github oauth begin", "err", err)
		writeError(w, http.StatusInternalServerError, "oauth start failed")
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) githubOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		writeError(w, http.StatusBadRequest, "oauth denied")
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	result, err := h.auth.CompleteGitHubOAuth(r.Context(), r, w, code, state, r.RemoteAddr, r.UserAgent())
	if err != nil {
		h.log.Error("github oauth callback", "err", err)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.refreshUserACLForInstance(r.Context(), result.User.ID, result.InstanceID, result.AccessToken); err != nil {
		h.log.Warn("acl refresh after github oauth", "err", err, "user", result.User.Login)
	}
	h.auth.SetSessionCookie(w, result.SessionToken)
	_, _ = h.auth.IssueCSRFToken(w)
	http.Redirect(w, r, auth.ApplyPathPrefix(result.RedirectTo, h.cfg.PathPrefix()), http.StatusFound)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "bootstrap admin required")
		return
	}
	users, err := h.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		row := map[string]any{
			"id":                 u.ID,
			"login":              u.Login,
			"display_name":       u.DisplayName,
			"email":              u.Email,
			"is_bootstrap_admin": u.IsBootstrapAdmin,
			"has_gitea":          u.GiteaUserID != nil,
			"has_github":         u.GitHubUserID != nil,
		}
		if u.InstanceID != nil {
			row["gitea_instance_id"] = *u.InstanceID
		}
		if u.GitHubInstanceID != nil {
			row["github_instance_id"] = *u.GitHubInstanceID
		}
		tokenInst, _ := h.store.ListUserTokenInstanceIDs(r.Context(), u.ID)
		row["token_instance_ids"] = tokenInst
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (h *Handler) getUserAccess(w http.ResponseWriter, r *http.Request) {
	admin := userFromCtx(r.Context())
	if admin == nil || !admin.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "bootstrap admin required")
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	instanceID, _ := strconv.ParseInt(r.URL.Query().Get("instance_id"), 10, 64)
	if instanceID <= 0 {
		writeError(w, http.StatusBadRequest, "instance_id is required")
		return
	}
	ids, err := h.store.ListUserRepoIDsForInstance(r.Context(), userID, instanceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":     userID,
		"instance_id": instanceID,
		"repo_ids":    ids,
	})
}

func (h *Handler) putUserAccess(w http.ResponseWriter, r *http.Request) {
	admin := userFromCtx(r.Context())
	if admin == nil || !admin.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "bootstrap admin required")
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	target, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil || target == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if target.IsBootstrapAdmin {
		writeError(w, http.StatusBadRequest, "bootstrap admin already has allow-all access")
		return
	}
	var body struct {
		InstanceID int64   `json:"instance_id"`
		RepoIDs    []int64 `json:"repo_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.InstanceID <= 0 {
		writeError(w, http.StatusBadRequest, "instance_id is required")
		return
	}
	inst, err := h.store.GetInstanceByID(r.Context(), body.InstanceID)
	if err != nil || inst == nil {
		writeError(w, http.StatusBadRequest, "instance not found")
		return
	}
	if body.RepoIDs == nil {
		body.RepoIDs = []int64{}
	}
	// Validate repos belong to the instance.
	valid := make([]int64, 0, len(body.RepoIDs))
	for _, rid := range body.RepoIDs {
		repo, rerr := h.store.GetRepositoryByID(r.Context(), rid)
		if rerr != nil || repo == nil || repo.InstanceID != body.InstanceID || repo.DeletedAt != nil {
			continue
		}
		valid = append(valid, rid)
	}
	if err := h.store.ReplaceUserRepoAccessForInstances(r.Context(), userID, []int64{body.InstanceID}, valid); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":     userID,
		"instance_id": body.InstanceID,
		"repo_ids":    valid,
		"forge_type":  inst.ForgeType,
	})
}