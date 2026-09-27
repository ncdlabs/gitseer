package api

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/uiinstall"
)

// downloadGiteaUISnippets returns marker-safe Gitea custom template snippets for an instance.
// GET /api/v1/instances/{id}/gitea-ui-snippets?format=zip|text
// Bootstrap admin only. CLI install-ui remains for in-place install on the Gitea host.
func (h *Handler) downloadGiteaUISnippets(w http.ResponseWriter, r *http.Request) {
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
	ft := inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}
	if ft != models.ForgeTypeGitea {
		writeError(w, http.StatusBadRequest, "gitea UI snippets are only available for Gitea instances")
		return
	}

	gitseerURL := h.webhookDeliveryURLBase()
	if gitseerURL == "" && h.settings != nil {
		gitseerURL = h.settings.ExternalURL()
	}
	if strings.TrimSpace(gitseerURL) == "" {
		writeError(w, http.StatusBadRequest, "GitSeer public URL (server.external_url) is required to render snippets")
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "zip"
	}

	files, err := uiinstall.SnippetFiles(gitseerURL, inst.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch format {
	case "text", "txt", "concat":
		body, err := uiinstall.ConcatenatedSnippets(gitseerURL, inst.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to render snippets")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="gitseer-gitea-ui-%d.txt"`, inst.ID))
		_, _ = w.Write([]byte(body))
	case "zip":
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, name := range []string{uiinstall.ExtraLinksRel, uiinstall.ExtraTabsRel} {
			content := files[name]
			fw, zerr := zw.Create("templates/custom/" + name)
			if zerr != nil {
				writeError(w, http.StatusInternalServerError, "failed to build zip")
				return
			}
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			if _, zerr = fw.Write([]byte(content)); zerr != nil {
				writeError(w, http.StatusInternalServerError, "failed to build zip")
				return
			}
		}
		readme := fmt.Sprintf(`GitSeer Gitea UI snippets (instance_id=%d)

Extract into Gitea's custom/ directory so paths become:
  custom/templates/custom/extra_links.tmpl
  custom/templates/custom/extra_tabs.tmpl

Markers are BEGIN/END GITSEER and GITSEER-TABS — merge-safe with existing admin content.
Prefer "gitseer install-ui --custom-path DIR --gitseer-url URL --instance-id %d" for in-place install on the Gitea host.
Restart Gitea after installing templates.
`, inst.ID, inst.ID)
		rf, zerr := zw.Create("README.txt")
		if zerr != nil {
			writeError(w, http.StatusInternalServerError, "failed to build zip")
			return
		}
		if _, zerr = rf.Write([]byte(readme)); zerr != nil {
			writeError(w, http.StatusInternalServerError, "failed to build zip")
			return
		}
		if err := zw.Close(); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build zip")
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="gitseer-gitea-ui-%d.zip"`, inst.ID))
		_, _ = w.Write(buf.Bytes())
	default:
		writeError(w, http.StatusBadRequest, `format must be "zip" or "text"`)
	}
}
