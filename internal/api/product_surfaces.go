package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

func (h *Handler) dashboardScopeFromQuery(r *http.Request) store.DashboardScope {
	q := r.URL.Query()
	return store.DashboardScope{
		OrgID:      parseQueryInt64(q.Get("org_id")),
		Owner:      strings.TrimSpace(q.Get("owner")),
		Team:       strings.TrimSpace(q.Get("team")),
		ForgeType:  strings.TrimSpace(q.Get("forge_type")),
		InstanceID: parseQueryInt64(q.Get("instance_id")),
	}
}

func (h *Handler) listOrganizations(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListOrganizations(r.Context(), store.ListOrganizationsOpts{
		UserID: sc.UserID, BootstrapAll: sc.BootstrapAll, Query: strings.TrimSpace(r.URL.Query().Get("q")), Limit: limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func parseLookbackDays(raw string, defaultDays int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultDays, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid days")
	}
	if n < 1 || n > 90 {
		return 0, fmt.Errorf("days must be between 1 and 90")
	}
	return n, nil
}

func (h *Handler) runnerUtilization(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	days, err := parseLookbackDays(r.URL.Query().Get("days"), 7)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rep, err := h.store.RunnerUtilization(r.Context(), sc.UserID, sc.BootstrapAll, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rep.Days = days
	h.mergeLiveForgeRunners(r.Context(), rep, sc.UserID, sc.BootstrapAll)
	writeJSON(w, http.StatusOK, rep)
}

func (h *Handler) mergeLiveForgeRunners(ctx context.Context, rep *models.RunnerUtilizationReport, userID int64, bootstrapAll bool) {
	if rep == nil {
		return
	}
	instances, err := h.store.ListInstances(ctx)
	if err != nil {
		return
	}
	var allowed map[int64]struct{}
	if !bootstrapAll {
		ids, err := h.store.ListAccessibleInstanceIDs(ctx, userID)
		if err != nil || len(ids) == 0 {
			return
		}
		allowed = make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			allowed[id] = struct{}{}
		}
	}
	byName := map[string]int{}
	for i, row := range rep.Items {
		byName[strings.ToLower(strings.TrimSpace(row.RunnerName))] = i
	}
	liveCount := 0
	for _, inst := range instances {
		if allowed != nil {
			if _, ok := allowed[inst.ID]; !ok {
				continue
			}
		}
		var caps models.Capabilities
		if strings.TrimSpace(inst.CapabilitiesJSON) != "" {
			_ = json.Unmarshal([]byte(inst.CapabilitiesJSON), &caps)
		}
		if !caps.RunnersAPI {
			continue
		}
		token, err := h.instanceSyncToken(ctx, inst)
		if err != nil || token == "" {
			continue
		}
		client, err := forge.NewFromInstance(inst, token)
		if err != nil {
			continue
		}
		runners, err := client.ListRunners(ctx)
		if err != nil {
			continue
		}
		rep.RunnersAPICapable = true
		for _, fr := range runners {
			liveCount++
			key := strings.ToLower(strings.TrimSpace(fr.Name))
			if key == "" {
				key = "runner-" + strconv.FormatInt(fr.ID, 10)
			}
			if idx, ok := byName[key]; ok {
				id := fr.ID
				rep.Items[idx].RunnerID = &id
				if rep.Items[idx].InstanceID == 0 {
					rep.Items[idx].InstanceID = inst.ID
					rep.Items[idx].InstanceName = inst.Name
					rep.Items[idx].ForgeType = inst.ForgeType
				}
				if fr.Busy {
					rep.Items[idx].BusyJobs++
				}
				continue
			}
			id := fr.ID
			busy := 0
			if fr.Busy {
				busy = 1
			}
			rep.Items = append(rep.Items, models.RunnerUtilizationRow{
				RunnerName:   fr.Name,
				RunnerID:     &id,
				BusyJobs:     busy,
				InstanceID:   inst.ID,
				InstanceName: inst.Name,
				ForgeType:    inst.ForgeType,
			})
			byName[key] = len(rep.Items) - 1
		}
	}
	rep.LiveForgeRunners = liveCount
	if liveCount > 0 {
		if rep.Source == "indexed_jobs" {
			rep.Source = "mixed"
		} else {
			rep.Source = "forge_live"
		}
		if rep.Degraded && liveCount > 0 {
			rep.Degraded = false
			rep.DegradedReason = ""
		}
	}
}


func (h *Handler) listFlakyJobs(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	days, err := parseLookbackDays(r.URL.Query().Get("days"), 14)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.FlakyJobs(r.Context(), sc.UserID, sc.BootstrapAll, 0, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "days": days})
}

func (h *Handler) getRepositoryFlakyJobs(w http.ResponseWriter, r *http.Request) {
	repo, ok := h.resolveAccessibleRepo(w, r)
	if !ok {
		return
	}
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	days, err := parseLookbackDays(r.URL.Query().Get("days"), 14)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.FlakyJobs(r.Context(), sc.UserID, sc.BootstrapAll, repo.ID, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "days": days, "repo_id": repo.ID})
}

func (h *Handler) listReleases(w http.ResponseWriter, r *http.Request) {
	user := userFromCtx(r.Context())
	sc := h.scope(user)
	days, err := parseLookbackDays(r.URL.Query().Get("days"), 30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.ListReleaseRuns(r.Context(), sc.UserID, sc.BootstrapAll, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "days": days})
}
