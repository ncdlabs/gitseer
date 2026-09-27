package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
)

type glRunner struct {
	ID          int64    `json:"id"`
	Description string   `json:"description"`
	Name        string   `json:"name"`
	Active      bool     `json:"active"`
	Online      bool     `json:"online"`
	Status      string   `json:"status"`
	Paused      bool     `json:"paused"`
	TagList     []string `json:"tag_list"`
}

// ListRunners returns runners visible to the token (GET /runners).
func (c *Client) ListRunners(ctx context.Context) ([]models.ForgeRunner, error) {
	var out []models.ForgeRunner
	for page := 1; page <= 20; page++ {
		resp, err := c.do(ctx, http.MethodGet, "/runners", url.Values{
			"per_page": {"100"}, "page": {strconv.Itoa(page)},
		}, "")
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			if page == 1 {
				return nil, forge.ErrUnsupported
			}
			break
		}
		var raw []glRunner
		if err := decodeJSON(resp, &raw); err != nil {
			return nil, err
		}
		for _, r := range raw {
			name := strings.TrimSpace(r.Description)
			if name == "" {
				name = strings.TrimSpace(r.Name)
			}
			if name == "" {
				name = "runner-" + strconv.FormatInt(r.ID, 10)
			}
			status := strings.ToLower(strings.TrimSpace(r.Status))
			if status == "" {
				if r.Online {
					status = "online"
				} else {
					status = "offline"
				}
			}
			lj, _ := json.Marshal(r.TagList)
			out = append(out, models.ForgeRunner{
				ID: r.ID, Name: name, Status: status, Busy: false, LabelsJSON: string(lj),
			})
		}
		if len(raw) < 100 {
			break
		}
	}
	return out, nil
}
