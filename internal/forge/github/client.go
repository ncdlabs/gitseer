// Package github implements the forge.Forge interface for GitHub and GitHub Enterprise.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ncdlabs/gitseer/internal/forge"
	gitseermetrics "github.com/ncdlabs/gitseer/internal/metrics"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/workflows"
)

const (
	defaultPageSize = 50
	apiVersion      = "2022-11-28"
)

func init() {
	forge.Register(models.ForgeTypeGitHub, func(baseURL, token string, allowPrivateNetwork bool) (forge.Forge, error) {
		return New(baseURL, token, allowPrivateNetwork)
	})
}

// Client talks to GitHub REST. Raw GitHub types never leave this package.
type Client struct {
	baseURL             *url.URL // API root: https://api.github.com or https://ghe/api/v3
	token               string
	http                *http.Client
	allowPrivateNetwork bool
}

// New builds a GitHub forge client. baseURL may be github.com, api.github.com, or a GHE host.
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

// NewHTTPClient returns an HTTP client with redirect and dial-time private-network guards.
// Environment HTTP(S)_PROXY is ignored so SSRF checks apply to the forge target, not a proxy hop.
func NewHTTPClient(allowPrivate bool, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validateURL(req.URL, allowPrivate)
		},
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil {
					host = address
					port = ""
				}
				ips, err := resolveDialIPs(host, allowPrivate)
				if err != nil {
					return nil, err
				}
				var firstErr error
				for _, ip := range ips {
					addr := ip.String()
					if port != "" {
						addr = net.JoinHostPort(ip.String(), port)
					}
					conn, derr := dialer.DialContext(ctx, network, addr)
					if derr == nil {
						return conn, nil
					}
					if firstErr == nil {
						firstErr = derr
					}
				}
				if firstErr == nil {
					firstErr = fmt.Errorf("no addresses to dial for %s", host)
				}
				return nil, firstErr
			},
		},
	}
}

func resolveDialIPs(host string, allowPrivate bool) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !allowPrivate && isBlockedIP(ip) {
			return nil, privateAddressError(host, ip)
		}
		return []net.IP{ip}, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("dns lookup for %s: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("dns lookup for %s returned no addresses", host)
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if !allowPrivate && isBlockedIP(ip) {
			return nil, privateAddressError(host, ip)
		}
		out = append(out, ip)
	}
	return out, nil
}

type ghUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type ghOrg struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type ghRepo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
	Archived      bool   `json:"archived"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Size          int64  `json:"size"` // KB; 0 often means empty / just created
	Owner         *struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
}

type ghPR struct {
	ID             int64  `json:"id"`
	Number         int64  `json:"number"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	State          string `json:"state"`
	Draft          bool   `json:"draft"`
	Mergeable      *bool  `json:"mergeable"`
	MergeableState string `json:"mergeable_state"`
	HTMLURL        string `json:"html_url"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	ClosedAt       string `json:"closed_at"`
	MergedAt       string `json:"merged_at"`
	User           *struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"user"`
	Head *struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base *struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
}

type ghRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	RunNumber  int64  `json:"run_number"`
	RunAttempt int    `json:"run_attempt"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	Path       string `json:"path"`
	Actor      *struct {
		Login string `json:"login"`
	} `json:"actor"`
	TriggeringActor *struct {
		Login string `json:"login"`
	} `json:"triggering_actor"`
}

type ghRunsResponse struct {
	WorkflowRuns []ghRun `json:"workflow_runs"`
	TotalCount   int     `json:"total_count"`
}

type ghJob struct {
	ID          int64           `json:"id"`
	RunID       int64           `json:"run_id"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	Conclusion  string          `json:"conclusion"`
	HTMLURL     string          `json:"html_url"`
	StartedAt   string          `json:"started_at"`
	CompletedAt string          `json:"completed_at"`
	RunnerID    *int64          `json:"runner_id"`
	RunnerName  string          `json:"runner_name"`
	Steps       json.RawMessage `json:"steps"`
}

type ghJobsResponse struct {
	Jobs       []ghJob `json:"jobs"`
	TotalCount int     `json:"total_count"`
}

type ghMeta struct {
	InstalledVersion string `json:"installed_version"`
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
	return c.doBody(ctx, method, path, query, token, nil, "", "application/vnd.github+json")
}

func (c *Client) doAccept(ctx context.Context, method, path string, query url.Values, token, accept string) (*http.Response, error) {
	return c.doBody(ctx, method, path, query, token, nil, "", accept)
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
	return c.doBody(ctx, method, path, query, "", body, "application/json", "application/vnd.github+json")
}

func (c *Client) doBody(ctx context.Context, method, path string, query url.Values, token string, body []byte, contentType, accept string) (*http.Response, error) {
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
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		req.Header.Set("X-GitHub-Api-Version", apiVersion)
		if contentType != "" && body != nil {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err = c.http.Do(req)
		if err != nil {
			gitseermetrics.GitHubAPIErrorsTotal.Inc()
			return nil, err
		}
		status := strconv.Itoa(resp.StatusCode/100) + "xx"
		gitseermetrics.GitHubAPIRequestsTotal.WithLabelValues(status).Inc()
		if resp.StatusCode >= 500 {
			gitseermetrics.GitHubAPIErrorsTotal.Inc()
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
		return fmt.Errorf("github api not found: %s", resp.Request.URL.Path)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	if dst == nil {
		return nil
	}
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode github response: %w", err)
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
	resp, err := c.do(ctx, http.MethodGet, "/meta", nil, "")
	if err != nil {
		return nil, err
	}
	ver := strings.TrimSpace(resp.Header.Get("X-GitHub-Enterprise-Version"))
	var meta ghMeta
	if err := decodeJSON(resp, &meta); err != nil {
		return nil, err
	}
	if ver == "" {
		ver = strings.TrimSpace(meta.InstalledVersion)
	}
	if ver == "" {
		if isDotCom(c.baseURL) {
			ver = "github.com"
		} else {
			ver = "github-enterprise"
		}
	}
	return &models.InstanceInfo{Version: ver}, nil
}

func isDotCom(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	return h == "api.github.com" || h == "github.com"
}

func (c *Client) DetectCapabilities(ctx context.Context) (*models.Capabilities, error) {
	info, err := c.GetInstance(ctx)
	if err != nil {
		return nil, err
	}
	caps := &models.Capabilities{
		Version:            info.Version,
		OAuthProvider:      true, // GitHub OAuth Apps supported (login when client configured)
		SystemHooksAPI:     false, // no Gitea-style system hooks API
		WorkflowRunWebhook: true,
		WorkflowJobWebhook: true,
	}
	resp, err := c.do(ctx, http.MethodGet, "/user/repos", url.Values{"per_page": {"1"}, "page": {"1"}}, "")
	if err != nil {
		return caps, nil
	}
	var repos []ghRepo
	if err := decodeJSON(resp, &repos); err != nil || len(repos) == 0 {
		return caps, nil
	}
	r := repos[0]
	owner := ""
	if r.Owner != nil {
		owner = r.Owner.Login
	}
	if owner == "" || r.Name == "" {
		return caps, nil
	}
	runsPath := fmt.Sprintf("/repos/%s/%s/actions/runs", url.PathEscape(owner), url.PathEscape(r.Name))
	runsResp, err := c.do(ctx, http.MethodGet, runsPath, url.Values{"per_page": {"1"}, "page": {"1"}}, "")
	if err != nil {
		return caps, nil
	}
	defer runsResp.Body.Close()
	if runsResp.StatusCode >= 200 && runsResp.StatusCode < 300 {
		caps.ActionsAPI = true
		caps.JobLogsAPI = true
		caps.RerunWorkflowAPI = true
		caps.CancelWorkflowAPI = true
	}
	return caps, nil
}

func (c *Client) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	var out []models.Organization
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(defaultPageSize)}}
		resp, err := c.do(ctx, http.MethodGet, "/user/orgs", q, "")
		if err != nil {
			return nil, err
		}
		var orgs []ghOrg
		if err := decodeJSON(resp, &orgs); err != nil {
			return nil, err
		}
		for _, o := range orgs {
			name := o.Login
			full := o.Name
			if full == "" {
				full = name
			}
			out = append(out, models.Organization{
				ExternalID: o.ID,
				Name:       name,
				FullName:   full,
				AvatarURL:  o.AvatarURL,
			})
		}
		if len(orgs) < defaultPageSize {
			break
		}
	}
	return out, nil
}

func (c *Client) ListRepositories(ctx context.Context, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return c.listReposWithToken(ctx, "", opts)
}

func (c *Client) ListAccessibleReposForUser(ctx context.Context, userToken string, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	return c.listReposWithToken(ctx, userToken, opts)
}

func (c *Client) listReposWithToken(ctx context.Context, token string, opts forge.ListReposOpts) (forge.Page[models.Repository], error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	limit := opts.PageSize
	if limit < 1 {
		limit = defaultPageSize
	}
	q := url.Values{
		"page":     {strconv.Itoa(page)},
		"per_page": {strconv.Itoa(limit)},
		"sort":     {"updated"},
		"direction": {"desc"},
	}
	resp, err := c.do(ctx, http.MethodGet, "/user/repos", q, token)
	if err != nil {
		return forge.Page[models.Repository]{}, err
	}
	var raw []ghRepo
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.Repository]{}, err
	}
	items := make([]models.Repository, 0, len(raw))
	for _, r := range raw {
		items = append(items, mapRepo(r))
	}
	return forge.Page[models.Repository]{Items: items, Page: page, HasMore: len(raw) >= limit}, nil
}

func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
	path := fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	var raw ghRepo
	if err := decodeJSON(resp, &raw); err != nil {
		return nil, err
	}
	m := mapRepo(raw)
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
	if state == "" {
		state = "open"
	}
	q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(limit)}, "state": {state}}
	path := fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	resp, err := c.do(ctx, http.MethodGet, path, q, "")
	if err != nil {
		return forge.Page[models.PullRequest]{}, err
	}
	var raw []ghPR
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.PullRequest]{}, err
	}
	items := make([]models.PullRequest, 0, len(raw))
	for _, p := range raw {
		items = append(items, mapPR(p))
	}
	return forge.Page[models.PullRequest]{Items: items, Page: page, HasMore: len(raw) >= limit}, nil
}

// GetPullRequestReviewState aggregates PR reviews into GitSeer review_state.
func (c *Client) GetPullRequestReviewState(ctx context.Context, repo models.RepoRef, number int64) (string, error) {
	if number <= 0 {
		return "", nil
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), number)
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
		return "", fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	var raw []struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("decode reviews: %w", err)
	}
	states := make([]string, 0, len(raw))
	for _, r := range raw {
		states = append(states, r.State)
	}
	return forge.AggregateReviewState(states), nil
}

// GetCombinedCommitStatus returns the normalized GitSeer ci_state for a commit/ref,
// merging classic commit statuses with check-runs (Actions).
func (c *Client) GetCombinedCommitStatus(ctx context.Context, repo models.RepoRef, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	statusState, err := c.commitStatusState(ctx, repo, ref)
	if err != nil {
		return "", err
	}
	checkState, err := c.checkRunsState(ctx, repo, ref)
	if err != nil {
		// Checks API may be unavailable; keep classic status.
		return statusState, nil
	}
	return forge.MergeCIState(statusState, checkState), nil
}

func (c *Client) commitStatusState(ctx context.Context, repo models.RepoRef, ref string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/status", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(ref))
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	var raw struct {
		State      string `json:"state"`
		TotalCount int    `json:"total_count"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("decode commit status: %w", err)
	}
	if raw.TotalCount == 0 && raw.State == "" {
		return "", nil
	}
	return forge.NormalizeCIState(raw.State), nil
}

func (c *Client) checkRunsState(ctx context.Context, repo models.RepoRef, ref string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(ref))
	q := url.Values{"per_page": {"100"}}
	resp, err := c.doAccept(ctx, http.MethodGet, path, q, "", "application/vnd.github+json")
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
		return "", fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	var wrapped struct {
		CheckRuns []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return "", fmt.Errorf("decode check runs: %w", err)
	}
	if len(wrapped.CheckRuns) == 0 {
		return "", nil
	}
	hasPending, hasSuccess, hasFailure, hasCancelled := false, false, false, false
	for _, cr := range wrapped.CheckRuns {
		st := strings.ToLower(cr.Status)
		if st != "completed" {
			hasPending = true
			continue
		}
		switch strings.ToLower(cr.Conclusion) {
		case "failure", "timed_out", "action_required", "startup_failure":
			hasFailure = true
		case "success", "neutral", "skipped":
			hasSuccess = true
		case "cancelled", "canceled", "stale":
			hasCancelled = true
		default:
			hasPending = true
		}
	}
	if hasFailure {
		return models.CIStateFailure, nil
	}
	if hasPending {
		return models.CIStatePending, nil
	}
	if hasSuccess {
		return models.CIStateSuccess, nil
	}
	if hasCancelled {
		return models.CIStateCancelled, nil
	}
	return "", nil
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
	path := fmt.Sprintf("/repos/%s/%s/actions/runs", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	resp, err := c.do(ctx, http.MethodGet, path, q, "")
	if err != nil {
		return forge.Page[models.WorkflowRun]{}, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	resp.Body.Close()
	if err != nil {
		return forge.Page[models.WorkflowRun]{}, err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return forge.Page[models.WorkflowRun]{Page: page}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return forge.Page[models.WorkflowRun]{}, fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	var wrapped ghRunsResponse
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return forge.Page[models.WorkflowRun]{}, fmt.Errorf("decode workflow runs: %w", err)
	}
	items := make([]models.WorkflowRun, 0, len(wrapped.WorkflowRuns))
	for _, r := range wrapped.WorkflowRuns {
		items = append(items, mapRun(r))
	}
	return forge.Page[models.WorkflowRun]{Items: items, Page: page, HasMore: len(wrapped.WorkflowRuns) >= limit}, nil
}

func (c *Client) GetWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) (*models.WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), runExternalID)
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, forge.ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	var raw ghRun
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode workflow run: %w", err)
	}
	if raw.ID == 0 {
		return nil, forge.ErrNotFound
	}
	mapped := mapRun(raw)
	return &mapped, nil
}

func (c *Client) ListJobs(ctx context.Context, repo models.RepoRef, runExternalID int64) ([]models.Job, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), runExternalID)
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, forge.ErrNotFound
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	var wrapped ghJobsResponse
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("decode jobs: %w", err)
	}
	out := make([]models.Job, 0, len(wrapped.Jobs))
	for _, j := range wrapped.Jobs {
		out = append(out, mapJob(j))
	}
	return out, nil
}

func (c *Client) GetJobLogs(ctx context.Context, repo models.RepoRef, jobExternalID int64) (io.ReadCloser, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/jobs/%d/logs", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), jobExternalID)
	// Logs often 302 to signed blob storage; client follows redirects with SSRF checks.
	resp, err := c.doAccept(ctx, http.MethodGet, path, nil, "", "*/*")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github api %s: %s", resp.Status, truncate(string(b), 200))
	}
	return resp.Body, nil
}

func (c *Client) RerunWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) error {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), runExternalID)
	return c.postWorkflowWrite(ctx, path)
}

func (c *Client) CancelWorkflowRun(ctx context.Context, repo models.RepoRef, runExternalID int64) error {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/cancel", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), runExternalID)
	return c.postWorkflowWrite(ctx, path)
}

func (c *Client) postWorkflowWrite(ctx context.Context, path string) error {
	resp, err := c.do(ctx, http.MethodPost, path, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return nil
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return fmt.Errorf("%w: github api %s", forge.ErrUnsupported, resp.Status)
	default:
		return fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
}

func (c *Client) GetWorkflowYAML(ctx context.Context, repo models.RepoRef, path, ref string) ([]byte, error) {
	candidates := workflows.CandidateWorkflowPaths(path)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("empty workflow path")
	}
	var lastErr error
	for _, candidate := range candidates {
		body, err := c.getRawFile(ctx, repo, candidate, ref)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("workflow yaml not found")
	}
	return nil, lastErr
}

func (c *Client) getRawFile(ctx context.Context, repo models.RepoRef, path, ref string) ([]byte, error) {
	q := url.Values{}
	if ref != "" {
		q.Set("ref", ref)
	}
	encoded := encodeContentPath(path)
	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), encoded)
	resp, err := c.doAccept(ctx, http.MethodGet, apiPath, q, "", "application/vnd.github.raw")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github api %s: %s", resp.Status, truncate(string(body), 200))
	}
	return body, nil
}

func encodeContentPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func (c *Client) GetAuthenticatedUser(ctx context.Context, userToken string) (*models.User, error) {
	resp, err := c.do(ctx, http.MethodGet, "/user", nil, userToken)
	if err != nil {
		return nil, err
	}
	var u ghUser
	if err := decodeJSON(resp, &u); err != nil {
		return nil, err
	}
	gid := u.ID
	return &models.User{
		GiteaUserID: &gid, // reused as forge numeric user id until schema renames
		Login:       u.Login,
		Email:       u.Email,
		DisplayName: u.Name,
		AvatarURL:   u.AvatarURL,
	}, nil
}

func mapRepo(r ghRepo) models.Repository {
	owner := ""
	if r.Owner != nil {
		owner = r.Owner.Login
	}
	if owner == "" && r.FullName != "" {
		parts := strings.SplitN(r.FullName, "/", 2)
		owner = parts[0]
	}
	return models.Repository{
		ExternalID:    r.ID,
		Owner:         owner,
		Name:          r.Name,
		FullName:      r.FullName,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		Archived:      r.Archived,
		Empty:         false, // GitHub size can be 0 for brand-new non-empty repos; leave unset
		Fork:          r.Fork,
		HTMLURL:       r.HTMLURL,
	}
}

func mapPR(p ghPR) models.PullRequest {
	pr := models.PullRequest{
		ExternalID:     p.ID,
		Number:         p.Number,
		Title:          p.Title,
		BodyExcerpt:    truncate(p.Body, 500),
		State:          p.State,
		Draft:          p.Draft,
		Mergeable:      p.Mergeable,
		MergeableState: p.MergeableState,
		HTMLURL:        p.HTMLURL,
	}
	if p.User != nil {
		pr.AuthorLogin = p.User.Login
		id := p.User.ID
		pr.AuthorExternalID = &id
	}
	if p.Head != nil {
		pr.SourceBranch = p.Head.Ref
		pr.HeadSHA = p.Head.SHA
	}
	if p.Base != nil {
		pr.TargetBranch = p.Base.Ref
		pr.BaseSHA = p.Base.SHA
	}
	pr.CreatedAt = parseOptionalTime(p.CreatedAt)
	pr.UpdatedAt = parseOptionalTime(p.UpdatedAt)
	pr.ClosedAt = parseOptionalTime(p.ClosedAt)
	pr.MergedAt = parseOptionalTime(p.MergedAt)
	return pr
}

func mapRun(r ghRun) models.WorkflowRun {
	workflowPath := workflows.NormalizeWorkflowPath(r.Path)
	name := workflows.DisplayWorkflowName(r.Name, r.Path)
	st, conc := forge.NormalizeStatus(r.Status)
	if r.Conclusion != "" {
		conc = forge.NormalizeConclusion(r.Conclusion)
		if st == models.StatusUnknown || r.Status == "completed" {
			st = models.StatusCompleted
		}
	}
	actor := ""
	if r.Actor != nil {
		actor = r.Actor.Login
	} else if r.TriggeringActor != nil {
		actor = r.TriggeringActor.Login
	}
	attempt := r.RunAttempt
	if attempt == 0 {
		attempt = 1
	}
	return models.WorkflowRun{
		ExternalID:         r.ID,
		Name:               name,
		Event:              r.Event,
		Branch:             r.HeadBranch,
		CommitSHA:          r.HeadSHA,
		Status:             st,
		Conclusion:         conc,
		UpstreamStatus:     r.Status,
		UpstreamConclusion: r.Conclusion,
		ActorLogin:         actor,
		HTMLURL:            r.HTMLURL,
		WorkflowPath:       workflowPath,
		StartedAt:          parseOptionalTime(r.CreatedAt),
		CompletedAt:        parseOptionalTime(r.UpdatedAt),
		RunAttempt:         attempt,
	}
}

func mapJob(j ghJob) models.Job {
	st, conc := forge.NormalizeStatus(j.Status)
	if j.Conclusion != "" {
		conc = forge.NormalizeConclusion(j.Conclusion)
		if j.Status == "completed" || conc != models.ConclusionUnknown {
			st = models.StatusCompleted
		}
	}
	var steps *string
	if len(j.Steps) > 0 && string(j.Steps) != "null" {
		s := string(j.Steps)
		steps = &s
	}
	return models.Job{
		ExternalID:         j.ID,
		Name:               j.Name,
		Status:             st,
		Conclusion:         conc,
		UpstreamStatus:     j.Status,
		UpstreamConclusion: j.Conclusion,
		RunnerID:           j.RunnerID,
		RunnerName:         j.RunnerName,
		HTMLURL:            j.HTMLURL,
		StartedAt:          parseOptionalTime(j.StartedAt),
		CompletedAt:        parseOptionalTime(j.CompletedAt),
		StepsJSON:          steps,
	}
}

func parseOptionalTime(s string) *time.Time {
	if s == "" || s == "null" {
		return nil
	}
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}
