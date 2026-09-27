package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Default storage guardrail thresholds (bytes).
const (
	storageWarnBytes     int64 = 512 << 20 // 512 MiB
	storageCriticalBytes int64 = 2 << 30   // 2 GiB
)

func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func storageLevel(bytes int64) string {
	switch {
	case bytes >= storageCriticalBytes:
		return "critical"
	case bytes >= storageWarnBytes:
		return "warn"
	default:
		return "ok"
	}
}

func (h *Handler) storageStatus(ctx context.Context, redactSensitive bool) map[string]any {
	sqlitePath := ""
	driver := strings.ToLower(strings.TrimSpace(h.cfg.Database.Driver))
	if driver == "" || driver == "sqlite" {
		sqlitePath = strings.TrimSpace(h.cfg.Database.Path)
	}
	out := map[string]any{
		"warn_bytes":     storageWarnBytes,
		"critical_bytes": storageCriticalBytes,
		"warn_human":     formatBytes(storageWarnBytes),
		"critical_human": formatBytes(storageCriticalBytes),
	}
	info, err := h.store.DatabaseSize(ctx, sqlitePath)
	if err != nil {
		out["error"] = "unavailable"
		out["level"] = "unknown"
		return out
	}
	out["driver"] = info.Driver
	out["bytes"] = info.Bytes
	out["bytes_human"] = formatBytes(info.Bytes)
	out["method"] = info.Method
	out["estimate"] = info.Estimate
	out["level"] = storageLevel(info.Bytes)
	if !redactSensitive && info.Path != "" {
		out["path"] = info.Path
	}
	return out
}

// purgeRetention runs an immediate retention purge using current settings windows.
// POST /api/v1/admin/purge-retention — bootstrap admin + CSRF.
func (h *Handler) purgeRetention(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	if user == nil || !user.IsBootstrapAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	cfg := h.settings.Retention()
	stats, err := h.store.PurgeRetention(r.Context(), cfg.RunsDays, cfg.WebhooksDays, cfg.AttentionDays)
	if err != nil {
		h.log.Error("purge retention", "err", err)
		writeError(w, http.StatusInternalServerError, "purge failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"stats": stats,
		"windows": map[string]int{
			"runs_days":      cfg.RunsDays,
			"webhooks_days":  cfg.WebhooksDays,
			"attention_days": cfg.AttentionDays,
		},
		"storage": h.storageStatus(r.Context(), false),
	})
}
