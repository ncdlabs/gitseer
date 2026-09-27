// Package gitlab implements the forge.Forge interface for GitLab.com and self-hosted GitLab.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	forge.Register(models.ForgeTypeGitLab, func(baseURL, token string, allowPrivateNetwork bool) (forge.Forge, error) {
		return New(baseURL, token, allowPrivateNetwork)
	})
}

// Client talks to GitLab REST API v4. Raw GitLab types never leave this package.
type Client struct {
	baseURL             *url.URL // API root ending in /api/v4
	token               string
	http                *http.Client
	allowPrivateNetwork bool
}

// New builds a GitLab forge client.
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

type glUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type glGroup struct {
	ID       int64  `json:"id"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	FullPath string `json:"full_path"`
	AvatarURL string `json:"avatar_url"`
}

type glProject struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
	Archived          bool   `json:"archived"`
	EmptyRepo         bool   `json:"empty_repo"`
	ForkedFromProject *struct {
		ID int64 `json:"id"`
	} `json:"forked_from_project"`
	WebURL            string `json:"web_url"`
	Namespace         *struct {
		ID   int64  `json:"id"`
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"namespace"`
}

type glMR struct {
	ID           int64  `json:"id"`
	IID          int64  `json:"iid"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	State        string `json:"state"`
	Draft        bool   `json:"draft"`
	WorkInProgress bool `json:"work_in_progress"`
	WebURL       string `json:"web_url"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	ClosedAt     string `json:"closed_at"`
	MergedAt     string `json:"merged_at"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	SHA          string `json:"sha"`
	Author       *struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"author"`
}

type glPipeline struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Name      string `json:"name"`
	User      *struct {
		Username string `json:"username"`
	} `json:"user"`
}

type glJob struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Stage         string `json:"stage"`
	WebURL        string `json:"web_url"`
	CreatedAt     string `json:"created_at"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	FailureReason string `json:"failure_reason"`
	Pipeline      *struct {
		ID int64 `json:"id"`
	} `json:"pipeline"`
	Runner *struct {
		Description string `json:"description"`
	} `json:"runner"`
	TagList []string `json:"tag_list"`
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
			req.Header.Set("PRIVATE-TOKEN", tok)
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
		return fmt.Errorf("gitlab api %s: %s", resp.Status, truncate(string(body), 200))
	}
	if dst == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode gitlab response: %w", err)
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
	resp, err := c.do(ctx, http.MethodGet, "/version", nil, "")
	if err != nil {
		return nil, err
	}
	var ver struct {
		Version string `json:"version"`
	}
	if err := decodeJSON(resp, &ver); err != nil {
		return nil, err
	}
	return &models.InstanceInfo{Version: ver.Version}, nil
}

func (c *Client) DetectCapabilities(ctx context.Context) (*models.Capabilities, error) {
	info, err := c.GetInstance(ctx)
	caps := &models.Capabilities{
		Version:            "",
		ActionsAPI:         true, // pipelines
		OAuthProvider:      true,
		SystemHooksAPI:     false,
		JobLogsAPI:         true,
		RunnersAPI:         true,
		WorkflowRunWebhook: true,
		WorkflowJobWebhook: true,
		RerunWorkflowAPI:   true, // retry pipeline
		CancelWorkflowAPI:  true, // cancel pipeline
	}
	if err == nil && info != nil {
		caps.Version = info.Version
	}
	// Soft-probe pipelines listing on a dummy path is noisy; assume present when version works.
	_ = ctx
	return caps, nil
}

func (c *Client) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	q := url.Values{"per_page": {"100"}, "top_level_only": {"true"}}
	resp, err := c.do(ctx, http.MethodGet, "/groups", q, "")
	if err != nil {
		return nil, err
	}
	var raw []glGroup
	if err := decodeJSON(resp, &raw); err != nil {
		return nil, err
	}
	out := make([]models.Organization, 0, len(raw))
	for _, g := range raw {
		name := g.Path
		if name == "" {
			name = g.FullPath
		}
		out = append(out, models.Organization{
			ExternalID: g.ID,
			Name:       name,
			FullName:   g.Name,
			AvatarURL:  g.AvatarURL,
		})
	}
	return out, nil
}

func (c *Client) ListRepositories(ctx context.Context, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return c.listProjects(ctx, "", opts)
}

func (c *Client) ListAccessibleReposForUser(ctx context.Context, userToken string, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return c.listProjects(ctx, userToken, opts)
}

func (c *Client) listProjects(ctx context.Context, token string, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	limit := opts.PageSize
	if limit < 1 {
		limit = defaultPageSize
	}
	q := url.Values{
		"page":                   {strconv.Itoa(page)},
		"per_page":               {strconv.Itoa(limit)},
		"membership":             {"true"},
		"simple":                 {"false"},
		"order_by":               {"id"},
		"sort":                   {"asc"},
	}
	resp, err := c.do(ctx, http.MethodGet, "/projects", q, token)
	if err != nil {
		return forge.Page[models.Repository]{}, err
	}
	var raw []glProject
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.Repository]{}, err
	}
	items := make([]models.Repository, 0, len(raw))
	for _, p := range raw {
		items = append(items, mapProject(p))
	}
	return forge.Page[models.Repository]{Items: items, Page: page, HasMore: len(raw) >= limit}, nil
}

func (c *Client) projectPath(owner, repo string) string {
	return url.PathEscape(owner + "/" + repo)
}

func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
	resp, err := c.do(ctx, http.MethodGet, "/projects/"+c.projectPath(owner, repo), nil, "")
	if err != nil {
		return nil, err
	}
	var raw glProject
	if err := decodeJSON(resp, &raw); err != nil {
		return nil, err
	}
	m := mapProject(raw)
	return &m, nil
}

func (c *Client) ListPullRequests(ctx context.Context, repo models.RepoRef, opts forge.PROpts) (forge.Page[models.PullRequest], error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	limit := opts.PageSize
	if limit < 1 {
		limit = defaultPageSize
	}
	state := opts.State
	if state == "" || state == "open" {
		state = "opened"
	} else if state == "closed" {
		state = "closed"
	} else if state == "all" {
		state = "all"
	}
	q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(limit)}, "state": {state}}
	path := "/projects/" + c.projectPath(repo.Owner, repo.Name) + "/merge_requests"
	resp, err := c.do(ctx, http.MethodGet, path, q, "")
	if err != nil {
		return forge.Page[models.PullRequest]{}, err
	}
	var raw []glMR
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.PullRequest]{}, err
	}
	items := make([]models.PullRequest, 0, len(raw))
	for _, m := range raw {
		items = append(items, mapMR(m))
	}
	return forge.Page[models.PullRequest]{Items: items, Page: page, HasMore: len(raw) >= limit}, nil
}

func (c *Client) GetPullRequestReviewState(ctx context.Context, repo models.RepoRef, number int64) (string, error) {
	if number <= 0 {
		return "", nil
	}
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/approvals", c.projectPath(repo.Owner, repo.Name), number)
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil
	}
	var raw struct {
		Approved bool `json:"approved"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", nil
	}
	if raw.Approved {
		return "approved", nil
	}
	return "", nil
}

func (c *Client) GetCombinedCommitStatus(ctx context.Context, repo models.RepoRef, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	path := fmt.Sprintf("/projects/%s/repository/commits/%s/statuses", c.projectPath(repo.Owner, repo.Name), url.PathEscape(ref))
	resp, err := c.do(ctx, http.MethodGet, path, url.Values{"per_page": {"20"}}, "")
	if err != nil {
		return "", err
	}
	var raw []struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(resp, &raw); err != nil {
		if forge.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	states := make([]string, 0, len(raw))
	for _, s := range raw {
		states = append(states, mapGitLabCIState(s.Status))
	}
	out := ""
	for _, s := range states {
		out = forge.MergeCIState(out, s)
	}
	return out, nil
}

func (c *Client) ListWorkflowRuns(ctx context.Context, repo models.RepoRef, opts forge.RunOpts) (forge.Page[models.WorkflowRun], error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	limit := opts.PageSize
	if limit < 1 {
		limit = defaultPageSize
	}
	q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(limit)}}
	path := "/projects/" + c.projectPath(repo.Owner, repo.Name) + "/pipelines"
	resp, err := c.do(ctx, http.MethodGet, path, q, "")
	if err != nil {
		return forge.Page[models.WorkflowRun]{}, err
	}
	var raw []glPipeline
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.WorkflowRun]{}, err
	}
	items := make([]models.WorkflowRun, 0, len(raw))
	for _, p := range raw {
		items = append(items, mapPipeline(p))
	}
	return forge.Page[models.WorkflowRun]{Items: items, Page: page, HasMore: len(raw) >= limit}, nil
}

func (c *Client) GetWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) (*models.WorkflowRun, error) {
	path := fmt.Sprintf("/projects/%s/pipelines/%d", c.projectPath(repo.Owner, repo.Name), runExternalID)
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	var raw glPipeline
	if err := decodeJSON(resp, &raw); err != nil {
		return nil, err
	}
	m := mapPipeline(raw)
	return &m, nil
}

func (c *Client) ListJobs(ctx context.Context, repo models.RepoRef, runExternalID int64) ([]models.Job, error) {
	path := fmt.Sprintf("/projects/%s/pipelines/%d/jobs", c.projectPath(repo.Owner, repo.Name), runExternalID)
	resp, err := c.do(ctx, http.MethodGet, path, url.Values{"per_page": {"100"}}, "")
	if err != nil {
		return nil, err
	}
	var raw []glJob
	if err := decodeJSON(resp, &raw); err != nil {
		return nil, err
	}
	items := make([]models.Job, 0, len(raw))
	for _, j := range raw {
		items = append(items, mapJob(j, runExternalID))
	}
	return items, nil
}

func (c *Client) GetJobLogs(ctx context.Context, repo models.RepoRef, jobExternalID int64) (io.ReadCloser, error) {
	path := fmt.Sprintf("/projects/%s/jobs/%d/trace", c.projectPath(repo.Owner, repo.Name), jobExternalID)
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, forge.ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return nil, fmt.Errorf("gitlab api %s: %s", resp.Status, truncate(string(body), 200))
	}
	return resp.Body, nil
}

func (c *Client) RerunWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) error {
	path := fmt.Sprintf("/projects/%s/pipelines/%d/retry", c.projectPath(repo.Owner, repo.Name), runExternalID)
	resp, err := c.doJSON(ctx, http.MethodPost, path, nil, map[string]any{})
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

func (c *Client) CancelWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) error {
	path := fmt.Sprintf("/projects/%s/pipelines/%d/cancel", c.projectPath(repo.Owner, repo.Name), runExternalID)
	resp, err := c.doJSON(ctx, http.MethodPost, path, nil, map[string]any{})
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

func (c *Client) GetWorkflowYAML(ctx context.Context, repo models.RepoRef, path, ref string) ([]byte, error) {
	path = workflows.NormalizeWorkflowPath(path)
	if path == "" {
		path = ".gitlab-ci.yml"
	}
	q := url.Values{}
	if ref != "" {
		q.Set("ref", ref)
	}
	apiPath := fmt.Sprintf("/projects/%s/repository/files/%s/raw", c.projectPath(repo.Owner, repo.Name), url.PathEscape(path))
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
		return nil, fmt.Errorf("gitlab api %s: %s", resp.Status, truncate(string(body), 200))
	}
	return body, nil
}

func (c *Client) GetAuthenticatedUser(ctx context.Context, userToken string) (*models.User, error) {
	resp, err := c.do(ctx, http.MethodGet, "/user", nil, userToken)
	if err != nil {
		return nil, err
	}
	var u glUser
	if err := decodeJSON(resp, &u); err != nil {
		return nil, err
	}
	id := u.ID
	return &models.User{
		GitLabUserID: &id,
		Login:        u.Username,
		Email:        u.Email,
		DisplayName:  u.Name,
		AvatarURL:    u.AvatarURL,
	}, nil
}

func mapProject(p glProject) models.Repository {
	owner, name := "", p.Name
	if p.PathWithNamespace != "" {
		parts := strings.SplitN(p.PathWithNamespace, "/", 2)
		owner = parts[0]
		if len(parts) > 1 {
			name = parts[1]
			// nested groups: keep full path after first segment as name? Prefer last segment for Name,
			// FullName as path_with_namespace.
			segs := strings.Split(p.PathWithNamespace, "/")
			name = segs[len(segs)-1]
			owner = strings.Join(segs[:len(segs)-1], "/")
		}
	} else if p.Namespace != nil {
		owner = p.Namespace.Path
	}
	return models.Repository{
		ExternalID:    p.ID,
		Owner:         owner,
		Name:          name,
		FullName:      p.PathWithNamespace,
		DefaultBranch: p.DefaultBranch,
		Private:       p.Visibility != "public",
		Archived:      p.Archived,
		Empty:         p.EmptyRepo,
		Fork:          p.ForkedFromProject != nil,
		HTMLURL:       p.WebURL,
	}
}

func mapMR(m glMR) models.PullRequest {
	state := m.State
	switch state {
	case "opened":
		state = "open"
	case "merged", "closed":
		state = "closed"
	}
	pr := models.PullRequest{
		ExternalID:   m.ID,
		Number:       m.IID,
		Title:        m.Title,
		BodyExcerpt:  truncate(m.Description, 500),
		State:        state,
		Draft:        m.Draft || m.WorkInProgress,
		HTMLURL:      m.WebURL,
		SourceBranch: m.SourceBranch,
		TargetBranch: m.TargetBranch,
		HeadSHA:      m.SHA,
	}
	if m.Author != nil {
		pr.AuthorLogin = m.Author.Username
		id := m.Author.ID
		pr.AuthorExternalID = &id
	}
	pr.CreatedAt = parseOptionalTime(m.CreatedAt)
	pr.UpdatedAt = parseOptionalTime(m.UpdatedAt)
	pr.ClosedAt = parseOptionalTime(m.ClosedAt)
	pr.MergedAt = parseOptionalTime(m.MergedAt)
	return pr
}

func mapPipeline(p glPipeline) models.WorkflowRun {
	st, conc := mapGitLabStatus(p.Status)
	actor := ""
	if p.User != nil {
		actor = p.User.Username
	}
	name := p.Name
	if name == "" {
		name = "pipeline"
	}
	return models.WorkflowRun{
		ExternalID:         p.ID,
		Name:               name,
		Event:              p.Source,
		Branch:             p.Ref,
		CommitSHA:          p.SHA,
		Status:             st,
		Conclusion:         conc,
		UpstreamStatus:     p.Status,
		UpstreamConclusion: p.Status,
		ActorLogin:         actor,
		HTMLURL:            p.WebURL,
		WorkflowPath:       ".gitlab-ci.yml",
		StartedAt:          parseOptionalTime(p.CreatedAt),
		CompletedAt:        parseOptionalTime(p.UpdatedAt),
		RunAttempt:         1,
	}
}

func mapJob(j glJob, _ int64) models.Job {
	st, conc := mapGitLabStatus(j.Status)
	runner := ""
	if j.Runner != nil {
		runner = j.Runner.Description
	}
	var labels *string
	if len(j.TagList) > 0 {
		b, err := json.Marshal(j.TagList)
		if err == nil {
			s := string(b)
			labels = &s
		}
	}
	return models.Job{
		ExternalID:         j.ID,
		Name:               j.Name,
		Status:             st,
		Conclusion:         conc,
		UpstreamStatus:     j.Status,
		UpstreamConclusion: j.FailureReason,
		HTMLURL:            j.WebURL,
		StartedAt:          parseOptionalTime(firstNonEmpty(j.StartedAt, j.CreatedAt)),
		CompletedAt:        parseOptionalTime(j.FinishedAt),
		RunnerName:         runner,
		LabelsJSON:         labels,
		Message:            strings.TrimSpace(j.FailureReason),
	}
}

func mapGitLabStatus(s string) (status, conclusion string) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "created", "waiting_for_resource", "preparing", "pending", "manual":
		return models.StatusQueued, models.ConclusionUnknown
	case "running":
		return models.StatusRunning, models.ConclusionUnknown
	case "success":
		return models.StatusCompleted, models.ConclusionSuccess
	case "failed":
		return models.StatusCompleted, models.ConclusionFailure
	case "canceled", "cancelled":
		return models.StatusCompleted, models.ConclusionCancelled
	case "skipped":
		return models.StatusCompleted, models.ConclusionSkipped
	default:
		return forge.NormalizeStatus(s)
	}
}

func mapGitLabCIState(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "success":
		return models.CIStateSuccess
	case "failed", "failure":
		return models.CIStateFailure
	case "pending", "running", "created":
		return models.CIStatePending
	case "canceled", "cancelled", "skipped":
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
