package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
)

type ghRunner struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Busy   bool   `json:"busy"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// ListRunners lists org/user self-hosted runners the token can see.
func (c *Client) ListRunners(ctx context.Context) ([]models.ForgeRunner, error) {
	orgs, err := c.ListMembershipOrgs(ctx)
	if err != nil {
		orgs = nil
	}
	seen := map[int64]struct{}{}
	var out []models.ForgeRunner
	for _, org := range orgs {
		page := 1
		for {
			path := fmt.Sprintf("/orgs/%s/actions/runners", url.PathEscape(org))
			resp, err := c.do(ctx, http.MethodGet, path, url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}}, "")
			if err != nil {
				break
			}
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
				resp.Body.Close()
				break
			}
			var raw struct {
				Runners []ghRunner `json:"runners"`
			}
			if err := decodeJSON(resp, &raw); err != nil {
				break
			}
			for _, r := range raw.Runners {
				if _, ok := seen[r.ID]; ok {
					continue
				}
				seen[r.ID] = struct{}{}
				labels := make([]string, 0, len(r.Labels))
				for _, l := range r.Labels {
					labels = append(labels, l.Name)
				}
				lj, _ := json.Marshal(labels)
				out = append(out, models.ForgeRunner{
					ID: r.ID, Name: r.Name, Status: r.Status, Busy: r.Busy, LabelsJSON: string(lj),
				})
			}
			if len(raw.Runners) < 100 {
				break
			}
			page++
		}
	}
	if len(out) == 0 {
		// Fall back to authenticated user runners (enterprise / personal).
		resp, err := c.do(ctx, http.MethodGet, "/user/actions/runners", url.Values{"per_page": {"100"}}, "")
		if err != nil {
			return nil, forge.ErrUnsupported
		}
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			if len(orgs) == 0 {
				return nil, forge.ErrUnsupported
			}
			return out, nil
		}
		var raw struct {
			Runners []ghRunner `json:"runners"`
		}
		if err := decodeJSON(resp, &raw); err != nil {
			return nil, err
		}
		for _, r := range raw.Runners {
			labels := make([]string, 0, len(r.Labels))
			for _, l := range r.Labels {
				labels = append(labels, l.Name)
			}
			lj, _ := json.Marshal(labels)
			out = append(out, models.ForgeRunner{
				ID: r.ID, Name: r.Name, Status: r.Status, Busy: r.Busy, LabelsJSON: string(lj),
			})
		}
	}
	return out, nil
}
