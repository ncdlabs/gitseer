// Package bitbucket implements the forge.Forge interface for Bitbucket Cloud (and Workspace API 2.0).
package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/forge"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/workflows"
)

const defaultPageSize = 50

func init() {
	forge.Register(models.ForgeTypeBitbucket, func(baseURL, token string, allowPrivateNetwork bool) (forge.Forge, error) {
		return New(baseURL, token, allowPrivateNetwork)
	})
}

// Client talks to Bitbucket Cloud REST 2.0. Raw Bitbucket types never leave this package.
type Client struct {
	baseURL             *url.URL
	token               string
	http                *http.Client
	allowPrivateNetwork bool
}

// New builds a Bitbucket forge client.
func New(baseURL, token string, allowPrivateNetwork bool) (*Client, error) {
	u, err := normalizeAPIBase(baseURL)
	if err != nil {
		return nil, err
	}
	c := &Client{
		baseURL:             u,
		token:               token,
		allowPrivateNetwork: allowPrivateNetwork,
		http:                NewHTTPClient(allowPrivateNetwork, 60*time.Second),
	}
	if err := validateURL(u, allowPrivateNetwork); err != nil {
		return nil, err
	}
	return c, nil
}

// stableID maps a Bitbucket UUID (or opaque string) to a positive int64 for store external_id.
// Prefer StableID from outside this package.
func stableID(s string) int64 {
	return StableID(s)
}

// StableID maps a Bitbucket UUID (or opaque string) to a positive int64 for store external_id.
// Shared by the Bitbucket API client and webhook normalizers so IDs stay consistent.
func StableID(s string) int64 {
	s = strings.TrimSpace(strings.Trim(s, "{}"))
	if s == "" {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}

type bbUser struct {
	UUID        string `json:"uuid"`
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
	Username    string `json:"username"`
	Links       *struct {
		Avatar *struct {
			Href string `json:"href"`
		} `json:"avatar"`
	} `json:"links"`
}

type bbWorkspace struct {
	UUID  string `json:"uuid"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Links *struct {
		Avatar *struct {
			Href string `json:"href"`
		} `json:"avatar"`
	} `json:"links"`
}

type bbRepo struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Slug        string `json:"slug"`
	IsPrivate   bool   `json:"is_private"`
	Description string `json:"description"`
	MainBranch  *struct {
		Name string `json:"name"`
	} `json:"mainbranch"`
	Workspace *struct {
		Slug string `json:"slug"`
		UUID string `json:"uuid"`
	} `json:"workspace"`
	Parent *struct {
		UUID string `json:"uuid"`
	} `json:"parent"`
	Links *struct {
		HTML *struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

type bbPR struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	Draft       bool   `json:"draft"`
	CreatedOn   string `json:"created_on"`
	UpdatedOn   string `json:"updated_on"`
	Author      *bbUser `json:"author"`
	Source      *struct {
		Branch *struct {
			Name string `json:"name"`
		} `json:"branch"`
		Commit *struct {
			Hash string `json:"hash"`
		} `json:"commit"`
	} `json:"source"`
	Destination *struct {
		Branch *struct {
			Name string `json:"name"`
		} `json:"branch"`
		Commit *struct {
			Hash string `json:"hash"`
		} `json:"commit"`
	} `json:"destination"`
	Links *struct {
		HTML *struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

type bbPipeline struct {
	UUID        string `json:"uuid"`
	BuildNumber int64  `json:"build_number"`
	State       *struct {
		Name string `json:"name"`
		Result *struct {
			Name string `json:"name"`
		} `json:"result"`
		Stage *struct {
			Name string `json:"name"`
		} `json:"stage"`
	} `json:"state"`
	Target *struct {
		RefName string `json:"ref_name"`
		Commit  *struct {
			Hash string `json:"hash"`
		} `json:"commit"`
		Selector *struct {
			Pattern string `json:"pattern"`
			Type    string `json:"type"`
		} `json:"selector"`
	} `json:"target"`
	CreatedOn string `json:"created_on"`
	CompletedOn string `json:"completed_on"`
	Creator   *bbUser `json:"creator"`
}

type bbStep struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	State  *struct {
		Name   string `json:"name"`
		Result *struct {
			Name string `json:"name"`
		} `json:"result"`
	} `json:"state"`
	StartedOn   string `json:"started_on"`
	CompletedOn string `json:"completed_on"`
}

type page[T any] struct {
	Values []T    `json:"values"`
	Next   string `json:"next"`
	Size   int    `json:"size"`
}

func (c *Client) requestURL(path string, query url.Values) string {
	u := *c.baseURL
	basePath := strings.TrimRight(c.baseURL.Path, "/")
	u.Path = basePath + path
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, token string) (*http.Response, error) {
	return c.doBody(ctx, method, path, query, token, nil, "")
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, payload any) (*http.Response, error) {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	return c.doBody(ctx, method, path, query, "", body, "application/json")
}

func (c *Client) doBody(ctx context.Context, method, path string, query url.Values, token string, body []byte, contentType string) (*http.Response, error) {
	reqURL := c.requestURL(path, query)
	tok := token
	if tok == "" {
		tok = c.token
	}
	var resp *http.Response
	for attempt := 0; attempt < 4; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
		if err != nil {
			return nil, err
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		req.Header.Set("Accept", "application/json")
		if contentType != "" && body != nil {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err = c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		resp.Body.Close()
		wait := time.Duration(attempt+1) * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if sec, err := strconv.Atoi(ra); err == nil {
				wait = time.Duration(sec) * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return resp, nil
}

func decodeJSON(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return forge.ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket api %s: %s", resp.Status, truncate(string(body), 200))
	}
	if dst == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode bitbucket response: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (c *Client) GetInstance(ctx context.Context) (*models.InstanceInfo, error) {
	// Bitbucket Cloud has no /version; successful /user proves reachability.
	_, err := c.GetAuthenticatedUser(ctx, "")
	if err != nil {
		return nil, err
	}
	return &models.InstanceInfo{Version: "cloud"}, nil
}

func (c *Client) DetectCapabilities(ctx context.Context) (*models.Capabilities, error) {
	_ = ctx
	return &models.Capabilities{
		Version:            "cloud",
		ActionsAPI:         true,
		OAuthProvider:      true,
		SystemHooksAPI:     false,
		JobLogsAPI:         true,
		RunnersAPI:         false,
		WorkflowRunWebhook: true,
		WorkflowJobWebhook: true,
		RerunWorkflowAPI:   false, // Cloud pipeline rerun is limited; mark unsupported
		CancelWorkflowAPI:  true,
	}, nil
}

func (c *Client) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	resp, err := c.do(ctx, http.MethodGet, "/workspaces", url.Values{"pagelen": {"100"}}, "")
	if err != nil {
		return nil, err
	}
	var raw page[bbWorkspace]
	if err := decodeJSON(resp, &raw); err != nil {
		return nil, err
	}
	out := make([]models.Organization, 0, len(raw.Values))
	for _, w := range raw.Values {
		avatar := ""
		if w.Links != nil && w.Links.Avatar != nil {
			avatar = w.Links.Avatar.Href
		}
		out = append(out, models.Organization{
			ExternalID: stableID(w.UUID),
			Name:       w.Slug,
			FullName:   w.Name,
			AvatarURL:  avatar,
		})
	}
	return out, nil
}

func (c *Client) ListRepositories(ctx context.Context, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return c.listRepos(ctx, "", opts)
}

func (c *Client) ListAccessibleReposForUser(ctx context.Context, userToken string, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return c.listRepos(ctx, userToken, opts)
}

func (c *Client) listRepos(ctx context.Context, token string, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	pageNum := opts.Page
	if pageNum < 1 {
		pageNum = 1
	}
	limit := opts.PageSize
	if limit < 1 {
		limit = defaultPageSize
	}
	q := url.Values{"pagelen": {strconv.Itoa(limit)}, "page": {strconv.Itoa(pageNum)}, "role": {"member"}}
	resp, err := c.do(ctx, http.MethodGet, "/repositories", q, token)
	if err != nil {
		return forge.Page[models.Repository]{}, err
	}
	var raw page[bbRepo]
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.Repository]{}, err
	}
	items := make([]models.Repository, 0, len(raw.Values))
	for _, r := range raw.Values {
		items = append(items, mapRepo(r))
	}
	return forge.Page[models.Repository]{Items: items, Page: pageNum, HasMore: raw.Next != ""}, nil
}

func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
	path := fmt.Sprintf("/repositories/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	var raw bbRepo
	if err := decodeJSON(resp, &raw); err != nil {
		return nil, err
	}
	m := mapRepo(raw)
	return &m, nil
}

func (c *Client) ListPullRequests(ctx context.Context, repo models.RepoRef, opts forge.PROpts) (forge.Page[models.PullRequest], error) {
	pageNum := opts.Page
	if pageNum < 1 {
		pageNum = 1
	}
	limit := opts.PageSize
	if limit < 1 {
		limit = defaultPageSize
	}
	state := strings.ToUpper(opts.State)
	if state == "" || state == "OPEN" {
		state = "OPEN"
	} else if state == "CLOSED" {
		state = "MERGED,DECLINED,SUPERSEDED"
	} else if state == "ALL" {
		state = "OPEN,MERGED,DECLINED,SUPERSEDED"
	}
	q := url.Values{"pagelen": {strconv.Itoa(limit)}, "page": {strconv.Itoa(pageNum)}, "state": {state}}
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	resp, err := c.do(ctx, http.MethodGet, path, q, "")
	if err != nil {
		return forge.Page[models.PullRequest]{}, err
	}
	var raw page[bbPR]
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.PullRequest]{}, err
	}
	items := make([]models.PullRequest, 0, len(raw.Values))
	for _, p := range raw.Values {
		items = append(items, mapPR(p))
	}
	return forge.Page[models.PullRequest]{Items: items, Page: pageNum, HasMore: raw.Next != ""}, nil
}

func (c *Client) GetPullRequestReviewState(ctx context.Context, repo models.RepoRef, number int64) (string, error) {
	if number <= 0 {
		return "", nil
	}
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return "", err
	}
	var raw struct {
		Participants []struct {
			Approved bool   `json:"approved"`
			State    string `json:"state"`
		} `json:"participants"`
	}
	if err := decodeJSON(resp, &raw); err != nil {
		if forge.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	states := make([]string, 0, len(raw.Participants))
	for _, p := range raw.Participants {
		if p.Approved || strings.EqualFold(p.State, "approved") {
			states = append(states, "approved")
		} else if strings.EqualFold(p.State, "changes_requested") {
			states = append(states, "changes_requested")
		}
	}
	return forge.AggregateReviewState(states), nil
}

func (c *Client) GetCombinedCommitStatus(ctx context.Context, repo models.RepoRef, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	path := fmt.Sprintf("/repositories/%s/%s/commit/%s/statuses", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(ref))
	resp, err := c.do(ctx, http.MethodGet, path, url.Values{"pagelen": {"50"}}, "")
	if err != nil {
		return "", err
	}
	var raw page[struct {
		State string `json:"state"`
	}]
	if err := decodeJSON(resp, &raw); err != nil {
		if forge.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	out := ""
	for _, s := range raw.Values {
		out = forge.MergeCIState(out, mapBBCIState(s.State))
	}
	return out, nil
}

func (c *Client) ListWorkflowRuns(ctx context.Context, repo models.RepoRef, opts forge.RunOpts) (forge.Page[models.WorkflowRun], error) {
	pageNum := opts.Page
	if pageNum < 1 {
		pageNum = 1
	}
	limit := opts.PageSize
	if limit < 1 {
		limit = defaultPageSize
	}
	q := url.Values{"pagelen": {strconv.Itoa(limit)}, "page": {strconv.Itoa(pageNum)}}
	path := fmt.Sprintf("/repositories/%s/%s/pipelines/", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	resp, err := c.do(ctx, http.MethodGet, path, q, "")
	if err != nil {
		return forge.Page[models.WorkflowRun]{}, err
	}
	var raw page[bbPipeline]
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.WorkflowRun]{}, err
	}
	items := make([]models.WorkflowRun, 0, len(raw.Values))
	for _, p := range raw.Values {
		items = append(items, mapPipeline(p, repo))
	}
	return forge.Page[models.WorkflowRun]{Items: items, Page: pageNum, HasMore: raw.Next != ""}, nil
}

func (c *Client) GetWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) (*models.WorkflowRun, error) {
	// Prefer build_number filter (avoids multi-page scan when pipelines are recent).
	path := fmt.Sprintf("/repositories/%s/%s/pipelines/", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	q := url.Values{
		"pagelen": {"1"},
		"sort":    {"-created_on"},
		"q":       {fmt.Sprintf("build_number=%d", runExternalID)},
	}
	if resp, err := c.do(ctx, http.MethodGet, path, q, ""); err == nil {
		var raw page[bbPipeline]
		if decodeJSON(resp, &raw) == nil {
			for _, p := range raw.Values {
				id := p.BuildNumber
				if id == 0 {
					id = stableID(p.UUID)
				}
				if id == runExternalID || stableID(p.UUID) == runExternalID {
					run := mapPipeline(p, repo)
					return &run, nil
				}
			}
		}
	}
	// Direct UUID lookup when external id is a stable hash of the UUID.
	pageNum := 1
	for pageNum <= 5 {
		runs, err := c.ListWorkflowRuns(ctx, repo, forge.RunOpts{Page: pageNum, PageSize: 50})
		if err != nil {
			return nil, err
		}
		for i := range runs.Items {
			if runs.Items[i].ExternalID == runExternalID {
				return &runs.Items[i], nil
			}
		}
		if !runs.HasMore {
			break
		}
		pageNum++
	}
	return nil, forge.ErrNotFound
}

func (c *Client) ListJobs(ctx context.Context, repo models.RepoRef, runExternalID int64) ([]models.Job, error) {
	uuid, err := c.pipelineUUID(ctx, repo, runExternalID)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/repositories/%s/%s/pipelines/%s/steps/", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(uuid))
	resp, err := c.do(ctx, http.MethodGet, path, url.Values{"pagelen": {"100"}}, "")
	if err != nil {
		return nil, err
	}
	var raw page[bbStep]
	if err := decodeJSON(resp, &raw); err != nil {
		return nil, err
	}
	items := make([]models.Job, 0, len(raw.Values))
	for _, s := range raw.Values {
		items = append(items, mapStep(s))
	}
	return items, nil
}

func (c *Client) pipelineUUID(ctx context.Context, repo models.RepoRef, runExternalID int64) (string, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pipelines/", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	q := url.Values{
		"pagelen": {"1"},
		"q":       {fmt.Sprintf("build_number=%d", runExternalID)},
	}
	if resp, err := c.do(ctx, http.MethodGet, path, q, ""); err == nil {
		var raw page[bbPipeline]
		if decodeJSON(resp, &raw) == nil {
			for _, p := range raw.Values {
				id := p.BuildNumber
				if id == 0 {
					id = stableID(p.UUID)
				}
				if id == runExternalID || stableID(p.UUID) == runExternalID {
					return strings.Trim(p.UUID, "{}"), nil
				}
			}
		}
	}
	pageNum := 1
	for pageNum <= 5 {
		q := url.Values{"pagelen": {"50"}, "page": {strconv.Itoa(pageNum)}}
		resp, err := c.do(ctx, http.MethodGet, path, q, "")
		if err != nil {
			return "", err
		}
		var raw page[bbPipeline]
		if err := decodeJSON(resp, &raw); err != nil {
			return "", err
		}
		for _, p := range raw.Values {
			id := p.BuildNumber
			if id == 0 {
				id = stableID(p.UUID)
			}
			if id == runExternalID || stableID(p.UUID) == runExternalID {
				return strings.Trim(p.UUID, "{}"), nil
			}
		}
		if raw.Next == "" {
			break
		}
		pageNum++
	}
	return "", forge.ErrNotFound
}

func (c *Client) GetJobLogs(ctx context.Context, repo models.RepoRef, jobExternalID int64) (io.ReadCloser, error) {
	// Resolve step UUID by scanning recent pipelines' steps — expensive but acceptable for on-demand.
	pageNum := 1
	for pageNum <= 3 {
		runs, err := c.ListWorkflowRuns(ctx, repo, forge.RunOpts{Page: pageNum, PageSize: 20})
		if err != nil {
			return nil, err
		}
		for _, run := range runs.Items {
			uuid, err := c.pipelineUUID(ctx, repo, run.ExternalID)
			if err != nil {
				continue
			}
			path := fmt.Sprintf("/repositories/%s/%s/pipelines/%s/steps/", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(uuid))
			resp, err := c.do(ctx, http.MethodGet, path, url.Values{"pagelen": {"50"}}, "")
			if err != nil {
				continue
			}
			var raw page[bbStep]
			if err := decodeJSON(resp, &raw); err != nil {
				continue
			}
			for _, s := range raw.Values {
				if stableID(s.UUID) == jobExternalID {
					logPath := fmt.Sprintf("/repositories/%s/%s/pipelines/%s/steps/%s/log",
						url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(uuid), url.PathEscape(strings.Trim(s.UUID, "{}")))
					logResp, err := c.do(ctx, http.MethodGet, logPath, nil, "")
					if err != nil {
						return nil, err
					}
					if logResp.StatusCode == http.StatusNotFound {
						logResp.Body.Close()
						return nil, forge.ErrNotFound
					}
					if logResp.StatusCode < 200 || logResp.StatusCode >= 300 {
						body, _ := io.ReadAll(io.LimitReader(logResp.Body, 1<<20))
						logResp.Body.Close()
						return nil, fmt.Errorf("bitbucket api %s: %s", logResp.Status, truncate(string(body), 200))
					}
					return logResp.Body, nil
				}
			}
		}
		if !runs.HasMore {
			break
		}
		pageNum++
	}
	return nil, forge.ErrNotFound
}

func (c *Client) RerunWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) error {
	_ = ctx
	_ = repo
	_ = runExternalID
	return forge.ErrUnsupported
}

func (c *Client) CancelWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) error {
	uuid, err := c.pipelineUUID(ctx, repo, runExternalID)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/repositories/%s/%s/pipelines/%s/stopPipeline", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(uuid))
	resp, err := c.doJSON(ctx, http.MethodPost, path, nil, map[string]any{})
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

func (c *Client) GetWorkflowYAML(ctx context.Context, repo models.RepoRef, path, ref string) ([]byte, error) {
	path = workflows.NormalizeWorkflowPath(path)
	if path == "" {
		path = "bitbucket-pipelines.yml"
	}
	q := url.Values{}
	apiPath := fmt.Sprintf("/repositories/%s/%s/src/%s/%s",
		url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(firstNonEmpty(ref, "HEAD")), path)
	resp, err := c.do(ctx, http.MethodGet, apiPath, q, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, forge.ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bitbucket api %s: %s", resp.Status, truncate(string(body), 200))
	}
	return body, nil
}

func (c *Client) GetAuthenticatedUser(ctx context.Context, userToken string) (*models.User, error) {
	resp, err := c.do(ctx, http.MethodGet, "/user", nil, userToken)
	if err != nil {
		return nil, err
	}
	var u bbUser
	if err := decodeJSON(resp, &u); err != nil {
		return nil, err
	}
	login := u.Username
	if login == "" {
		login = u.Nickname
	}
	if login == "" {
		login = u.DisplayName
	}
	avatar := ""
	if u.Links != nil && u.Links.Avatar != nil {
		avatar = u.Links.Avatar.Href
	}
	id := stableID(firstNonEmpty(u.UUID, u.AccountID))
	return &models.User{
		BitbucketUserID: &id,
		Login:           login,
		DisplayName:     u.DisplayName,
		AvatarURL:       avatar,
	}, nil
}

func mapRepo(r bbRepo) models.Repository {
	owner, name := "", r.Slug
	if r.FullName != "" {
		parts := strings.SplitN(r.FullName, "/", 2)
		owner = parts[0]
		if len(parts) > 1 {
			name = parts[1]
		}
	} else if r.Workspace != nil {
		owner = r.Workspace.Slug
	}
	branch := ""
	if r.MainBranch != nil {
		branch = r.MainBranch.Name
	}
	html := ""
	if r.Links != nil && r.Links.HTML != nil {
		html = r.Links.HTML.Href
	}
	return models.Repository{
		ExternalID:    stableID(r.UUID),
		Owner:         owner,
		Name:          name,
		FullName:      r.FullName,
		DefaultBranch: branch,
		Private:       r.IsPrivate,
		Fork:          r.Parent != nil,
		HTMLURL:       html,
	}
}

func mapPR(p bbPR) models.PullRequest {
	state := strings.ToLower(p.State)
	switch state {
	case "open":
		state = "open"
	case "merged", "declined", "superseded":
		state = "closed"
	}
	pr := models.PullRequest{
		ExternalID: p.ID,
		Number:     p.ID,
		Title:      p.Title,
		BodyExcerpt: truncate(p.Description, 500),
		State:      state,
		Draft:      p.Draft,
	}
	if p.Links != nil && p.Links.HTML != nil {
		pr.HTMLURL = p.Links.HTML.Href
	}
	if p.Author != nil {
		login := p.Author.Username
		if login == "" {
			login = p.Author.Nickname
		}
		pr.AuthorLogin = login
		id := stableID(firstNonEmpty(p.Author.UUID, p.Author.AccountID))
		pr.AuthorExternalID = &id
	}
	if p.Source != nil {
		if p.Source.Branch != nil {
			pr.SourceBranch = p.Source.Branch.Name
		}
		if p.Source.Commit != nil {
			pr.HeadSHA = p.Source.Commit.Hash
		}
	}
	if p.Destination != nil {
		if p.Destination.Branch != nil {
			pr.TargetBranch = p.Destination.Branch.Name
		}
		if p.Destination.Commit != nil {
			pr.BaseSHA = p.Destination.Commit.Hash
		}
	}
	pr.CreatedAt = parseOptionalTime(p.CreatedOn)
	pr.UpdatedAt = parseOptionalTime(p.UpdatedOn)
	return pr
}

func mapPipeline(p bbPipeline, repo models.RepoRef) models.WorkflowRun {
	statusName, resultName := "", ""
	if p.State != nil {
		statusName = p.State.Name
		if p.State.Result != nil {
			resultName = p.State.Result.Name
		} else if p.State.Stage != nil {
			statusName = p.State.Stage.Name
		}
	}
	st, conc := mapBBStatus(statusName, resultName)
	id := p.BuildNumber
	if id == 0 {
		id = stableID(p.UUID)
	}
	branch, sha, name := "", "", "pipeline"
	if p.Target != nil {
		branch = p.Target.RefName
		if p.Target.Commit != nil {
			sha = p.Target.Commit.Hash
		}
		if p.Target.Selector != nil && p.Target.Selector.Pattern != "" {
			name = p.Target.Selector.Pattern
		}
	}
	actor := ""
	if p.Creator != nil {
		actor = p.Creator.Username
		if actor == "" {
			actor = p.Creator.Nickname
		}
	}
	html := fmt.Sprintf("https://bitbucket.org/%s/%s/pipelines/results/%d", repo.Owner, repo.Name, p.BuildNumber)
	return models.WorkflowRun{
		ExternalID:         id,
		NodeID:             strings.Trim(p.UUID, "{}"),
		Name:               name,
		Event:              "pipeline",
		Branch:             branch,
		CommitSHA:          sha,
		Status:             st,
		Conclusion:         conc,
		UpstreamStatus:     statusName,
		UpstreamConclusion: resultName,
		ActorLogin:         actor,
		HTMLURL:            html,
		WorkflowPath:       "bitbucket-pipelines.yml",
		StartedAt:          parseOptionalTime(p.CreatedOn),
		CompletedAt:        parseOptionalTime(p.CompletedOn),
		RunAttempt:         1,
	}
}

func mapStep(s bbStep) models.Job {
	statusName, resultName := "", ""
	if s.State != nil {
		statusName = s.State.Name
		if s.State.Result != nil {
			resultName = s.State.Result.Name
		}
	}
	st, conc := mapBBStatus(statusName, resultName)
	return models.Job{
		ExternalID:         stableID(s.UUID),
		NodeID:             strings.Trim(s.UUID, "{}"),
		Name:               s.Name,
		Status:             st,
		Conclusion:         conc,
		UpstreamStatus:     statusName,
		UpstreamConclusion: resultName,
		StartedAt:          parseOptionalTime(s.StartedOn),
		CompletedAt:        parseOptionalTime(s.CompletedOn),
	}
}

func mapBBStatus(state, result string) (status, conclusion string) {
	s := strings.ToUpper(strings.TrimSpace(state))
	r := strings.ToUpper(strings.TrimSpace(result))
	switch s {
	case "PENDING", "READY", "PARSING", "SCHEDULED":
		return models.StatusQueued, models.ConclusionUnknown
	case "IN_PROGRESS", "RUNNING":
		return models.StatusRunning, models.ConclusionUnknown
	case "COMPLETED", "SUCCESSFUL", "FAILED", "ERROR", "STOPPED", "PAUSED":
		status = models.StatusCompleted
	default:
		if r != "" {
			status = models.StatusCompleted
		} else {
			return forge.NormalizeStatus(strings.ToLower(state))
		}
	}
	switch r {
	case "SUCCESSFUL", "SUCCESS":
		conclusion = models.ConclusionSuccess
	case "FAILED", "ERROR":
		conclusion = models.ConclusionFailure
	case "STOPPED", "CANCELLED", "CANCELED":
		conclusion = models.ConclusionCancelled
	case "EXPIRED", "SKIPPED":
		conclusion = models.ConclusionSkipped
	default:
		if s == "SUCCESSFUL" {
			conclusion = models.ConclusionSuccess
		} else if s == "FAILED" || s == "ERROR" {
			conclusion = models.ConclusionFailure
		} else if s == "STOPPED" {
			conclusion = models.ConclusionCancelled
		} else {
			conclusion = models.ConclusionUnknown
		}
	}
	return status, conclusion
}

func mapBBCIState(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SUCCESSFUL", "SUCCESS":
		return models.CIStateSuccess
	case "FAILED", "ERROR":
		return models.CIStateFailure
	case "INPROGRESS", "IN_PROGRESS", "PENDING":
		return models.CIStatePending
	case "STOPPED", "STOPPEDPED":
		return models.CIStateCancelled
	default:
		return ""
	}
}

func parseOptionalTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
