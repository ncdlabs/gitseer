package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ncdlabs/gitseer/internal/auth"
)

func (h *Handler) forgeOAuthLogin(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redirectTo := auth.SafeRedirectPath(r.URL.Query().Get("redirect"))
		var linkUserID *int64
		if r.URL.Query().Get("link") == "1" || strings.EqualFold(r.URL.Query().Get("link"), "true") {
			user, _, err := h.auth.UserFromRequest(r.Context(), r)
			if err != nil || user == nil || user.IsBootstrapAdmin {
				writeError(w, http.StatusUnauthorized, "sign in before linking "+provider)
				return
			}
			id := user.ID
			linkUserID = &id
			if redirectTo == "/" {
				redirectTo = "/settings#access"
			}
		}
		var (
			url string
			err error
		)
		switch provider {
		case "gitlab":
			url, err = h.auth.BeginGitLabOAuth(r.Context(), w, redirectTo, linkUserID)
		case "bitbucket":
			url, err = h.auth.BeginBitbucketOAuth(r.Context(), w, redirectTo, linkUserID)
		case "forgejo":
			url, err = h.auth.BeginForgejoOAuth(r.Context(), w, redirectTo, linkUserID)
		default:
			writeError(w, http.StatusBadRequest, "unknown oauth provider")
			return
		}
		if err != nil {
			if errors.Is(err, auth.ErrOAuthNotConfigured) {
				writeError(w, http.StatusServiceUnavailable, provider+" oauth is not configured; set OAuth client id on the instance and GITSEER_SERVER_EXTERNAL_URL")
				return
			}
			h.log.Error(provider+" oauth begin", "err", err)
			writeError(w, http.StatusInternalServerError, "oauth start failed")
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
	}
}

func (h *Handler) forgeOAuthCallback(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			writeError(w, http.StatusBadRequest, "oauth denied")
			return
		}
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		var (
			result *auth.CompleteForgeOAuthResult
			err    error
		)
		switch provider {
		case "gitlab":
			result, err = h.auth.CompleteGitLabOAuth(r.Context(), r, w, code, state, r.RemoteAddr, r.UserAgent())
		case "bitbucket":
			result, err = h.auth.CompleteBitbucketOAuth(r.Context(), r, w, code, state, r.RemoteAddr, r.UserAgent())
		case "forgejo":
			result, err = h.auth.CompleteForgejoOAuth(r.Context(), r, w, code, state, r.RemoteAddr, r.UserAgent())
		default:
			writeError(w, http.StatusBadRequest, "unknown oauth provider")
			return
		}
		if err != nil {
			h.log.Error(provider+" oauth callback", "err", err)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err := h.refreshUserACLForInstance(r.Context(), result.User.ID, result.InstanceID, result.AccessToken); err != nil {
			h.log.Warn("acl refresh after "+provider+" oauth", "err", err, "user", result.User.Login)
		}
		h.auth.SetSessionCookie(w, result.SessionToken)
		_, _ = h.auth.IssueCSRFToken(w)
		http.Redirect(w, r, auth.ApplyPathPrefix(result.RedirectTo, h.cfg.PathPrefix()), http.StatusFound)
	}
}
