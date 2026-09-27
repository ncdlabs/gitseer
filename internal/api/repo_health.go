package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

const (
	defaultLogSnippetBytes = 4 << 10 // 4 KiB tail
	maxLogSnippetBytes     = 16 << 10
)

type repoWithHealth struct {
	models.Repository
	Health *models.RepoHealth `json:"health,omitempty"`
}

func (h *Handler) healthOptsFromQuery(r *http.Request) store.RepoHealthOpts {
	q := r.URL.Query()
	days, _ := strconv.Atoi(q.Get("days"))
	stale, _ := strconv.Atoi(q.Get("stale_days"))
	return store.RepoHealthOpts{WindowDays: days, StalePRDays: stale}
}

func (h *Handler) resolveAccessibleRepo(w http.ResponseWriter, r *http.Request) (*models.Repository, bool) {
	user := userFromCtx(r.Context())
	owner := chi.URLParam(r, "owner")
	name := chi.URLParam(r, "repo")
	instanceID := parseQueryInt64(r.URL.Query().Get("instance_id"))

	var candidates []models.Repository
	if instanceID > 0 {
		repo, err := h.store.GetRepositoryByOwnerNameInInstance(r.Context(), owner, name, instanceID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return nil, false
		}
		candidates = []models.Repository{*repo}
	} else {
		repos, err := h.store.ListRepositoriesByOwnerName(r.Context(), owner, name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return nil, false
		}
		candidates = repos
	}

	var accessible []models.Repository
	for i := range candidates {
		ok, err := h.authz.CanAccessRepo(r.Context(), user, candidates[i].ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return nil, false
		}
		if ok {
			accessible = append(accessible, candidates[i])
		}
	}
	if len(accessible) == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return nil, false
	}
	if len(accessible) > 1 {
		writeError(w, http.StatusConflict, "multiple repositories match owner/name; pass instance_id")
		return nil, false
	}
	repo := accessible[0]
	if inst, ierr := h.store.GetInstanceByID(r.Context(), repo.InstanceID); ierr == nil && inst != nil {
		ft := inst.ForgeType
		if ft == "" {
			ft = models.ForgeTypeGitea
		}
		repo.ForgeType = ft
		repo.InstanceName = inst.Name
	}
	return &repo, true
}

func (h *Handler) attachRepoHealth(w http.ResponseWriter, r *http.Request, repos []models.Repository) ([]repoWithHealth, bool) {
	ids := make([]int64, 0, len(repos))
	for i := range repos {
		ids = append(ids, repos[i].ID)
	}
	summaries, err := h.store.RepoHealthSummaries(r.Context(), ids, h.healthOptsFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	out := make([]repoWithHealth, 0, len(repos))
	for i := range repos {
		item := repoWithHealth{Repository: repos[i]}
		if health, ok := summaries[repos[i].ID]; ok {
			hcopy := health
			item.Health = &hcopy
		}
		out = append(out, item)
	}
	return out, true
}

func (h *Handler) getRepositoryHealth(w http.ResponseWriter, r *http.Request) {
	repo, ok := h.resolveAccessibleRepo(w, r)
	if !ok {
		return
	}
	health, err := h.store.RepoHealthSummary(r.Context(), repo.ID, h.healthOptsFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repository": map[string]any{
			"id":        repo.ID,
			"full_name": repo.FullName,
			"owner":     repo.Owner,
			"name":      repo.Name,
		},
		"health": health,
	})
}

func (h *Handler) getRepositoryFailureClusters(w http.ResponseWriter, r *http.Request) {
	repo, ok := h.resolveAccessibleRepo(w, r)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = store.DefaultHealthWindowDays
	}
	if days > 90 {
		days = 90
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	clusters, err := h.store.FailureClusters(r.Context(), repo.ID, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": clusters,
		"days":  days,
		"since": since.Format(time.RFC3339),
		"repo": map[string]any{
			"id":        repo.ID,
			"full_name": repo.FullName,
		},
	})
}

func (h *Handler) getAttentionLogSnippet(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid attention id")
		return
	}
	item, err := h.store.GetAttentionByID(r.Context(), id)
	if err != nil || item == nil {
		writeError(w, http.StatusNotFound, "attention item not found")
		return
	}
	ok, err := h.authz.CanAccessRepo(r.Context(), user, item.RepoID)
	if err != nil || !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	job, err := h.resolveAttentionFailedJob(r.Context(), item)
	if err != nil {
		writeError(w, http.StatusNotFound, "no failed job for this attention item")
		return
	}
	repo, err := h.store.GetRepositoryByID(r.Context(), job.RepoID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	client, err := h.forgeClientForRepo(r.Context(), user, repo)
	if err != nil {
		if errors.Is(err, errUserTokenRequired) {
			writeError(w, http.StatusForbidden, errUserTokenRequired.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "forge not configured")
		return
	}

	maxBytes := defaultLogSnippetBytes
	if n, _ := strconv.Atoi(r.URL.Query().Get("max_bytes")); n > 0 {
		maxBytes = n
		if maxBytes > maxLogSnippetBytes {
			maxBytes = maxLogSnippetBytes
		}
	}

	rc, err := client.GetJobLogs(r.Context(), models.RepoRef{Owner: repo.Owner, Name: repo.Name}, job.ExternalID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch logs")
		return
	}
	defer rc.Close()

	// Read up to a hard cap; keep only the trailing maxBytes for the response.
	hardCap := maxLogSnippetBytes * 4
	limited, err := io.ReadAll(io.LimitReader(rc, int64(hardCap+1)))
	if err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadGateway, "failed to read logs")
		return
	}
	truncatedFetch := len(limited) > hardCap
	if truncatedFetch {
		limited = limited[:hardCap]
	}
	truncated := truncatedFetch
	buf := limited
	if len(buf) > maxBytes {
		buf = buf[len(buf)-maxBytes:]
		truncated = true
	}
	// Prefer line-aligned tail when we clipped mid-line.
	if truncated && len(buf) > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 && i+1 < len(buf) {
			buf = buf[i+1:]
		}
	}
	snippet := string(buf)
	writeJSON(w, http.StatusOK, map[string]any{
		"attention_id": item.ID,
		"job_id":       job.ID,
		"job_name":     job.Name,
		"snippet":      snippet,
		"truncated":    truncated,
		"bytes":        len(buf),
		"stored":       false,
	})
}

func (h *Handler) resolveAttentionFailedJob(ctx context.Context, item *models.AttentionItem) (*models.Job, error) {
	if item == nil {
		return nil, errors.New("nil attention")
	}
	switch item.EntityType {
	case "job":
		job, err := h.store.GetJobByID(ctx, item.EntityID)
		if err != nil {
			return nil, err
		}
		return job, nil
	case "workflow_run":
		jobs, err := h.store.ListJobsByRunID(ctx, item.EntityID)
		if err != nil {
			return nil, err
		}
		var fallback *models.Job
		for i := range jobs {
			j := &jobs[i]
			if j.Conclusion == models.ConclusionFailure || j.Conclusion == models.ConclusionTimedOut {
				return j, nil
			}
			if fallback == nil {
				fallback = j
			}
		}
		if fallback != nil {
			return fallback, nil
		}
		// metadata may carry job_id from attention open
		var meta struct {
			JobID int64 `json:"job_id"`
			RunID int64 `json:"run_id"`
		}
		_ = json.Unmarshal([]byte(item.MetadataJSON), &meta)
		if meta.JobID > 0 {
			return h.store.GetJobByID(ctx, meta.JobID)
		}
		return nil, errors.New("no jobs on run")
	default:
		var meta struct {
			JobID int64 `json:"job_id"`
			RunID int64 `json:"run_id"`
		}
		_ = json.Unmarshal([]byte(item.MetadataJSON), &meta)
		if meta.JobID > 0 {
			return h.store.GetJobByID(ctx, meta.JobID)
		}
		if meta.RunID > 0 {
			jobs, err := h.store.ListJobsByRunID(ctx, meta.RunID)
			if err != nil {
				return nil, err
			}
			for i := range jobs {
				j := &jobs[i]
				if j.Conclusion == models.ConclusionFailure || j.Conclusion == models.ConclusionTimedOut {
					return j, nil
				}
			}
			if len(jobs) > 0 {
				return &jobs[0], nil
			}
		}
		return nil, errors.New("attention entity has no job")
	}
}
