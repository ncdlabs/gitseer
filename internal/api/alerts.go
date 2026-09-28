package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ncdlabs/gitseer/internal/store"
)

type alertPrefsPublic struct {
	BrowserEnabled bool   `json:"browser_enabled"`
	PushEnabled    bool   `json:"push_enabled"`
	MinSeverity    string `json:"min_severity"`
	PushConfigured bool   `json:"push_configured"`
	VAPIDPublicKey string `json:"vapid_public_key,omitempty"`
	SubscriptionCount int `json:"subscription_count"`
}

type alertPrefsPatch struct {
	BrowserEnabled *bool   `json:"browser_enabled"`
	PushEnabled    *bool   `json:"push_enabled"`
	MinSeverity    *string `json:"min_severity"`
}

type pushSubscribeBody struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type pushUnsubscribeBody struct {
	Endpoint string `json:"endpoint"`
}

func (h *Handler) getAlertPrefs(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	prefs, err := h.store.GetUserAlertPrefs(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	subs, err := h.store.ListWebPushSubscriptions(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := alertPrefsPublic{
		BrowserEnabled:    prefs.BrowserEnabled,
		PushEnabled:       prefs.PushEnabled,
		MinSeverity:       prefs.MinSeverity,
		PushConfigured:    h.push != nil && h.push.Configured(),
		SubscriptionCount: len(subs),
	}
	if out.PushConfigured {
		out.VAPIDPublicKey = h.push.PublicKey()
	}
	writeJSON(w, http.StatusOK, map[string]any{"prefs": out})
}

func (h *Handler) putAlertPrefs(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body alertPrefsPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	cur, err := h.store.GetUserAlertPrefs(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	next := *cur
	if body.BrowserEnabled != nil {
		next.BrowserEnabled = *body.BrowserEnabled
	}
	if body.PushEnabled != nil {
		next.PushEnabled = *body.PushEnabled
	}
	if body.MinSeverity != nil {
		sev := strings.ToLower(strings.TrimSpace(*body.MinSeverity))
		switch sev {
		case "critical", "warning", "waiting":
			next.MinSeverity = sev
		default:
			writeError(w, http.StatusBadRequest, "min_severity must be critical, warning, or waiting")
			return
		}
	}
	saved, err := h.store.UpsertUserAlertPrefs(r.Context(), next)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	subs, _ := h.store.ListWebPushSubscriptions(r.Context(), user.ID)
	out := alertPrefsPublic{
		BrowserEnabled:    saved.BrowserEnabled,
		PushEnabled:       saved.PushEnabled,
		MinSeverity:       saved.MinSeverity,
		PushConfigured:    h.push != nil && h.push.Configured(),
		SubscriptionCount: len(subs),
	}
	if out.PushConfigured {
		out.VAPIDPublicKey = h.push.PublicKey()
	}
	writeJSON(w, http.StatusOK, map[string]any{"prefs": out})
}

func (h *Handler) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.push == nil || !h.push.Configured() {
		writeError(w, http.StatusServiceUnavailable, "web push not configured")
		return
	}
	var body pushSubscribeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	sub, err := h.store.UpsertWebPushSubscription(
		r.Context(),
		user.ID,
		body.Endpoint,
		body.Keys.P256dh,
		body.Keys.Auth,
		r.UserAgent(),
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Opt the user into push when they successfully subscribe.
	prefs, _ := h.store.GetUserAlertPrefs(r.Context(), user.ID)
	if prefs == nil {
		prefs = &store.UserAlertPrefs{UserID: user.ID, MinSeverity: "critical"}
	}
	prefs.BrowserEnabled = true
	prefs.PushEnabled = true
	_, _ = h.store.UpsertUserAlertPrefs(r.Context(), *prefs)
	writeJSON(w, http.StatusOK, map[string]any{"id": sub.ID})
}

func (h *Handler) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body pushUnsubscribeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.store.DeleteWebPushSubscriptionForUser(r.Context(), user.ID, body.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) sendTestBrowserAlert(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.push == nil || !h.push.Configured() {
		writeError(w, http.StatusServiceUnavailable, "web push not configured")
		return
	}
	deepLink := "/settings#notifications"
	if h.settings != nil {
		if base := strings.TrimRight(strings.TrimSpace(h.settings.ExternalURL()), "/"); base != "" {
			deepLink = base + deepLink
		}
	}
	n, err := h.push.SendTest(r.Context(), user.ID, deepLink)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": n})
}
