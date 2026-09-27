package gitea

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
)

type giteaRunner struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status int    `json:"status"` // 0=idle, 1=active, 2=offline (Gitea Actions)
	Busy   bool   `json:"busy"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// ListRunners returns Gitea Actions runners (admin listing preferred).
func (c *Client) ListRunners(ctx context.Context) ([]models.ForgeRunner, error) {
	for _, path := range []string{"/admin/actions/runners", "/user/actions/runners"} {
		resp, err := c.do(ctx, http.MethodGet, path, url.Values{"limit": {"100"}, "page": {"1"}}, "")
		if err != nil {
			continue
		}
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
			resp.Body.Close()
			continue
		}
		if resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			continue
		}
		var raw struct {
			Runners []giteaRunner `json:"runners"`
		}
		if err := decodeJSON(resp, &raw); err != nil {
			// Some versions return a bare array.
			resp2, err2 := c.do(ctx, http.MethodGet, path, url.Values{"limit": {"100"}}, "")
			if err2 != nil {
				return nil, err
			}
			var arr []giteaRunner
			if err2 = decodeJSON(resp2, &arr); err2 != nil {
				return nil, err
			}
			raw.Runners = arr
		}
		out := make([]models.ForgeRunner, 0, len(raw.Runners))
		for _, r := range raw.Runners {
			status := "unknown"
			switch r.Status {
			case 0:
				status = "online"
			case 1:
				status = "online"
			case 2:
				status = "offline"
			}
			if r.Busy {
				status = "online"
			}
			labels := make([]string, 0, len(r.Labels))
			for _, l := range r.Labels {
				if strings.TrimSpace(l.Name) != "" {
					labels = append(labels, l.Name)
				}
			}
			lj, _ := json.Marshal(labels)
			out = append(out, models.ForgeRunner{
				ID: r.ID, Name: r.Name, Status: status, Busy: r.Busy, LabelsJSON: string(lj),
			})
		}
		return out, nil
	}
	return nil, forge.ErrUnsupported
}
