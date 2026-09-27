package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ncdlabs/gitseer/internal/notify"
	"github.com/ncdlabs/gitseer/internal/store"
)

// NotificationSettingsPublic is the write-only-secrets API view.
type NotificationSettingsPublic struct {
	Enabled          bool   `json:"enabled"`
	MinSeverity      string `json:"min_severity"`
	ImmediateEnabled bool   `json:"immediate_enabled"`
	DigestEnabled    bool   `json:"digest_enabled"`
	DigestHourUTC    int    `json:"digest_hour_utc"`

	SMTPEnabled            bool   `json:"smtp_enabled"`
	SMTPHost               string `json:"smtp_host"`
	SMTPPort               int    `json:"smtp_port"`
	SMTPTLSMode            string `json:"smtp_tls_mode"`
	SMTPFrom               string `json:"smtp_from"`
	SMTPTo                 string `json:"smtp_to"`
	SMTPUsername           string `json:"smtp_username"`
	SMTPPasswordConfigured bool   `json:"smtp_password_configured"`

	SlackEnabled            bool `json:"slack_enabled"`
	SlackWebhookConfigured  bool `json:"slack_webhook_configured"`
	DiscordEnabled          bool `json:"discord_enabled"`
	DiscordWebhookConfigured bool `json:"discord_webhook_configured"`
	WebhookEnabled          bool `json:"webhook_enabled"`
	WebhookURLConfigured    bool `json:"webhook_url_configured"`

	IncidentEnabled          bool `json:"incident_enabled"`
	IncidentWebhookConfigured bool `json:"incident_webhook_configured"`
}

type notificationSettingsPatch struct {
	Enabled          *bool   `json:"enabled"`
	MinSeverity      *string `json:"min_severity"`
	ImmediateEnabled *bool   `json:"immediate_enabled"`
	DigestEnabled    *bool   `json:"digest_enabled"`
	DigestHourUTC    *int    `json:"digest_hour_utc"`

	SMTPEnabled   *bool   `json:"smtp_enabled"`
	SMTPHost      *string `json:"smtp_host"`
	SMTPPort      *int    `json:"smtp_port"`
	SMTPTLSMode   *string `json:"smtp_tls_mode"`
	SMTPFrom      *string `json:"smtp_from"`
	SMTPTo        *string `json:"smtp_to"`
	SMTPUsername  *string `json:"smtp_username"`
	SMTPPassword  *string `json:"smtp_password"`
	ClearSMTPPassword bool `json:"clear_smtp_password"`

	SlackEnabled       *bool   `json:"slack_enabled"`
	SlackWebhookURL    *string `json:"slack_webhook_url"`
	ClearSlackWebhook  bool    `json:"clear_slack_webhook"`
	DiscordEnabled     *bool   `json:"discord_enabled"`
	DiscordWebhookURL  *string `json:"discord_webhook_url"`
	ClearDiscordWebhook bool   `json:"clear_discord_webhook"`
	WebhookEnabled     *bool   `json:"webhook_enabled"`
	WebhookURL         *string `json:"webhook_url"`
	ClearWebhookURL    bool    `json:"clear_webhook_url"`

	IncidentEnabled       *bool   `json:"incident_enabled"`
	IncidentWebhookURL    *string `json:"incident_webhook_url"`
	ClearIncidentWebhook  bool    `json:"clear_incident_webhook"`
}

func publicNotificationSettings(ns *store.NotificationSettings) NotificationSettingsPublic {
	if ns == nil {
		return NotificationSettingsPublic{MinSeverity: "critical", ImmediateEnabled: true, DigestHourUTC: 14, SMTPPort: 587, SMTPTLSMode: "starttls"}
	}
	return NotificationSettingsPublic{
		Enabled:                  ns.Enabled,
		MinSeverity:              ns.MinSeverity,
		ImmediateEnabled:         ns.ImmediateEnabled,
		DigestEnabled:            ns.DigestEnabled,
		DigestHourUTC:            ns.DigestHourUTC,
		SMTPEnabled:              ns.SMTPEnabled,
		SMTPHost:                 ns.SMTPHost,
		SMTPPort:                 ns.SMTPPort,
		SMTPTLSMode:              ns.SMTPTLSMode,
		SMTPFrom:                 ns.SMTPFrom,
		SMTPTo:                   ns.SMTPTo,
		SMTPUsername:             ns.SMTPUsername,
		SMTPPasswordConfigured:   strings.TrimSpace(ns.SMTPPasswordCiphertext) != "",
		SlackEnabled:             ns.SlackEnabled,
		SlackWebhookConfigured:   strings.TrimSpace(ns.SlackWebhookCiphertext) != "",
		DiscordEnabled:           ns.DiscordEnabled,
		DiscordWebhookConfigured: strings.TrimSpace(ns.DiscordWebhookCiphertext) != "",
		WebhookEnabled:           ns.WebhookEnabled,
		WebhookURLConfigured:     strings.TrimSpace(ns.WebhookURLCiphertext) != "",
		IncidentEnabled:          ns.IncidentEnabled,
		IncidentWebhookConfigured: strings.TrimSpace(ns.IncidentWebhookCiphertext) != "",
	}
}

func (h *Handler) getNotificationSettings(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	ns, err := h.store.GetNotificationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": publicNotificationSettings(ns)})
}

func (h *Handler) putNotificationSettings(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body notificationSettingsPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	cur, err := h.store.GetNotificationSettings(r.Context())
	if err != nil || cur == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	next := *cur
	if body.Enabled != nil {
		next.Enabled = *body.Enabled
	}
	if body.MinSeverity != nil {
		sev := strings.ToLower(strings.TrimSpace(*body.MinSeverity))
		switch sev {
		case "critical", "warning", "waiting":
			next.MinSeverity = sev
		default:
			writeError(w, http.StatusBadRequest, `min_severity must be "critical", "warning", or "waiting"`)
			return
		}
	}
	if body.ImmediateEnabled != nil {
		next.ImmediateEnabled = *body.ImmediateEnabled
	}
	if body.DigestEnabled != nil {
		next.DigestEnabled = *body.DigestEnabled
	}
	if body.DigestHourUTC != nil {
		if *body.DigestHourUTC < 0 || *body.DigestHourUTC > 23 {
			writeError(w, http.StatusBadRequest, "digest_hour_utc must be 0–23")
			return
		}
		next.DigestHourUTC = *body.DigestHourUTC
	}
	if body.SMTPEnabled != nil {
		next.SMTPEnabled = *body.SMTPEnabled
	}
	if body.SMTPHost != nil {
		next.SMTPHost = strings.TrimSpace(*body.SMTPHost)
	}
	if body.SMTPPort != nil {
		if *body.SMTPPort < 1 || *body.SMTPPort > 65535 {
			writeError(w, http.StatusBadRequest, "smtp_port invalid")
			return
		}
		next.SMTPPort = *body.SMTPPort
	}
	if body.SMTPTLSMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*body.SMTPTLSMode))
		switch mode {
		case "none", "starttls", "tls", "ssl":
			if mode == "ssl" {
				mode = "tls"
			}
			next.SMTPTLSMode = mode
		default:
			writeError(w, http.StatusBadRequest, `smtp_tls_mode must be "none", "starttls", or "tls"`)
			return
		}
	}
	if body.SMTPFrom != nil {
		next.SMTPFrom = strings.TrimSpace(*body.SMTPFrom)
	}
	if body.SMTPTo != nil {
		next.SMTPTo = strings.TrimSpace(*body.SMTPTo)
	}
	if body.SMTPUsername != nil {
		next.SMTPUsername = strings.TrimSpace(*body.SMTPUsername)
	}
	if body.ClearSMTPPassword {
		next.SMTPPasswordCiphertext = ""
	} else if body.SMTPPassword != nil && strings.TrimSpace(*body.SMTPPassword) != "" {
		sealed, err := notify.SealSecret(h.settings, strings.TrimSpace(*body.SMTPPassword))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		next.SMTPPasswordCiphertext = sealed
	}
	if body.SlackEnabled != nil {
		next.SlackEnabled = *body.SlackEnabled
	}
	if body.ClearSlackWebhook {
		next.SlackWebhookCiphertext = ""
	} else if body.SlackWebhookURL != nil && strings.TrimSpace(*body.SlackWebhookURL) != "" {
		sealed, err := notify.SealSecret(h.settings, strings.TrimSpace(*body.SlackWebhookURL))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		next.SlackWebhookCiphertext = sealed
	}
	if body.DiscordEnabled != nil {
		next.DiscordEnabled = *body.DiscordEnabled
	}
	if body.ClearDiscordWebhook {
		next.DiscordWebhookCiphertext = ""
	} else if body.DiscordWebhookURL != nil && strings.TrimSpace(*body.DiscordWebhookURL) != "" {
		sealed, err := notify.SealSecret(h.settings, strings.TrimSpace(*body.DiscordWebhookURL))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		next.DiscordWebhookCiphertext = sealed
	}
	if body.WebhookEnabled != nil {
		next.WebhookEnabled = *body.WebhookEnabled
	}
	if body.ClearWebhookURL {
		next.WebhookURLCiphertext = ""
	} else if body.WebhookURL != nil && strings.TrimSpace(*body.WebhookURL) != "" {
		sealed, err := notify.SealSecret(h.settings, strings.TrimSpace(*body.WebhookURL))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		next.WebhookURLCiphertext = sealed
	}
	if body.IncidentEnabled != nil {
		next.IncidentEnabled = *body.IncidentEnabled
	}
	if body.ClearIncidentWebhook {
		next.IncidentWebhookCiphertext = ""
	} else if body.IncidentWebhookURL != nil && strings.TrimSpace(*body.IncidentWebhookURL) != "" {
		sealed, err := notify.SealSecret(h.settings, strings.TrimSpace(*body.IncidentWebhookURL))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		next.IncidentWebhookCiphertext = sealed
	}

	if err := h.store.UpsertNotificationSettings(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	saved, err := h.store.GetNotificationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": publicNotificationSettings(saved)})
}

func (h *Handler) sendTestNotification(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.notify == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications unavailable")
		return
	}
	n, err := h.notify.EnqueueTest(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if n == 0 {
		writeError(w, http.StatusBadRequest, "no notification channels enabled")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queued": n})
}
