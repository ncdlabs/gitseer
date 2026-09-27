package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/realtime"
)

// Write-ops token errors (never silent service-PAT fallback for non-admin OAuth users).
var (
	errGitHubWriteNeedsOAuth = errors.New("GitHub write operations require your GitHub OAuth token; sign in with Continue with GitHub or Link GitHub (requires GITSEER_ENCRYPTION_KEY)")
	errWriteOpsUserToken     = errors.New("write operations require your forge user token; sign in with OAuth again (requires GITSEER_ENCRYPTION_KEY)")
)

// forgeClientForWriteOps selects a forge client for rerun/cancel.
// Non-admin users must use UserAccessTokenForInstance (never service PAT).
// Bootstrap admin may use the instance service PAT (UI must warn).
func (h *Handler) forgeClientForWriteOps(ctx context.Context, user *models.User, repo *models.Repository) (client forge.Forge, usedServicePAT bool, err error) {
	if user == nil {
		return nil, false, fmt.Errorf("unauthorized")
	}
	if repo == nil || repo.InstanceID <= 0 {
		return nil, false, fmt.Errorf("repository instance unknown")
	}
	inst, err := h.store.GetInstanceByID(ctx, repo.InstanceID)
	if err != nil || inst == nil {
		return nil, false, fmt.Errorf("instance not found")
	}
	ft := inst.ForgeType
	if ft == "" {
		ft = models.ForgeTypeGitea
	}

	if user.IsBootstrapAdmin {
		tok, err := h.instanceSyncToken(ctx, *inst)
		if err != nil || tok == "" {
			if ft == models.ForgeTypeGitHub {
				gh := h.effectiveGitHub()
				if gh.URL == "" || gh.Token == "" {
					return nil, false, fmt.Errorf("github not configured")
				}
				c, err := forge.NewFromInstance(*inst, gh.Token)
				return c, true, err
			}
			if ft == models.ForgeTypeGitea {
				integ := h.effectiveIntegration()
				if integ.Token == "" {
					return nil, false, fmt.Errorf("gitea not configured")
				}
				c, err := forge.NewFromInstance(*inst, integ.Token)
				return c, true, err
			}
			// Never fall back to the Gitea integration token for GitLab/Bitbucket/Forgejo.
			return nil, false, fmt.Errorf("%s instance sync token required for write operations", ft)
		}
		c, err := forge.NewFromInstance(*inst, tok)
		return c, true, err
	}

	ut, err := h.auth.UserAccessTokenForInstance(ctx, user.ID, repo.InstanceID)
	if err != nil || ut == "" {
		// Legacy single-token Gitea users may lack a row keyed by instance_id.
		// Never cross-forge: do not use a Gitea token against GitHub/GitLab/Bitbucket/Forgejo.
		if ft == models.ForgeTypeGitea {
			ut, err = h.auth.UserAccessToken(ctx, user.ID)
		}
	}
	if err != nil || ut == "" {
		if ft == models.ForgeTypeGitHub {
			return nil, false, errGitHubWriteNeedsOAuth
		}
		return nil, false, errWriteOpsUserToken
	}
	c, err := forge.NewFromInstance(*inst, ut)
	return c, false, err
}

func (h *Handler) rerunWorkflowRun(w http.ResponseWriter, r *http.Request) {
	h.writeWorkflowRunOp(w, r, "rerun")
}

func (h *Handler) cancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	h.writeWorkflowRunOp(w, r, "cancel")
}

func (h *Handler) writeWorkflowRunOp(w http.ResponseWriter, r *http.Request, op string) {
	user := userFromCtx(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, err := h.store.GetWorkflowRunByID(r.Context(), id)
	if err != nil || run == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	ok, err := h.authz.CanAccessRepo(r.Context(), user, run.RepoID)
	if err != nil || !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	repo, err := h.store.GetRepositoryByID(r.Context(), run.RepoID)
	if err != nil || repo == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if op == "cancel" {
		switch strings.ToLower(run.Status) {
		case models.StatusQueued, models.StatusWaiting, models.StatusRunning:
			// ok
		default:
			writeError(w, http.StatusConflict, "run is not in progress; cancel only applies to queued, waiting, or running runs")
			return
		}
	}

	client, usedServicePAT, err := h.forgeClientForWriteOps(r.Context(), user, repo)
	if err != nil {
		switch {
		case errors.Is(err, errGitHubWriteNeedsOAuth), errors.Is(err, errWriteOpsUserToken), errors.Is(err, errUserTokenRequired):
			writeError(w, http.StatusForbidden, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	ref := models.RepoRef{Owner: repo.Owner, Name: repo.Name}
	switch op {
	case "rerun":
		err = client.RerunWorkflowRun(r.Context(), ref, run.ExternalID)
	case "cancel":
		err = client.CancelWorkflowRun(r.Context(), ref, run.ExternalID)
	default:
		writeError(w, http.StatusBadRequest, "unknown operation")
		return
	}
	if err != nil {
		if forge.IsUnsupported(err) {
			writeError(w, http.StatusNotImplemented, "forge does not support this operation")
			return
		}
		writeError(w, http.StatusBadGateway, truncateMsg(err.Error(), 200))
		return
	}

	if h.hub != nil {
		h.hub.Publish(realtime.Event{Type: "workflow_run", ID: run.ID, RepoID: run.RepoID})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"operation":       op,
		"run_id":          run.ID,
		"used_service_pat": usedServicePAT,
	})
}

func truncateMsg(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
