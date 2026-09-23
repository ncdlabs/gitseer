// Package gitea implements the forge.Forge interface for Gitea.
package gitea

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

	"github.com/ncdlabs/gitea-lens/internal/forge"
	lensmetrics "github.com/ncdlabs/gitea-lens/internal/metrics"
	"github.com/ncdlabs/gitea-lens/internal/models"
	"github.com/ncdlabs/gitea-lens/internal/workflows"
)

const defaultPageSize = 50

// Client talks to a Gitea instance. Raw Gitea types never leave this package.
type Client struct {
	baseURL             *url.URL
	token               string
	http                *http.Client
	allowPrivateNetwork bool
}

func New(baseURL, token string, allowPrivateNetwork bool) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse gitea url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("gitea url scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("gitea url host is required")
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
// Environment HTTP(S)_PROXY is ignored so SSRF checks apply to the Gitea target, not a proxy hop.
// Hostnames are resolved once and dialed by pinned IP to reduce DNS-rebinding TOCTOU.
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

func validateURL(u *url.URL, allowPrivate bool) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("blocked scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}
	return validateHost(u.String(), host, allowPrivate)
}

func validateHost(displayURL, host string, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return privateAddressError(displayURL, ip)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("dns lookup for %s: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("dns lookup for %s returned no addresses", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return privateAddressError(displayURL, ip)
		}
	}
	return nil
}

// PrivateNetworkOptionLabel is the UI checkbox label referenced in user-facing errors.
const PrivateNetworkOptionLabel = "Allow Private Network Addresses"

func privateAddressError(_displayURL string, ip net.IP) error {
	return fmt.Errorf(
		"This Gitea URL points to a private network address (%s). Check %q to allow Lens to connect.",
		ip,
		PrivateNetworkOptionLabel,
	)
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// AWS/GCP/Azure metadata and CGNAT
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

type versionResponse struct {
	Version string `json:"version"`
}

type repoSearchResponse struct {
	OK   bool        `json:"ok"`
	Data []giteaRepo `json:"data"`
}

type giteaRepo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
	Empty         bool   `json:"empty"`
	Archived      bool   `json:"archived"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Owner         *struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		FullName  string `json:"full_name"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
}

type giteaOrg struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FullName  string `json:"full_name"`
	AvatarURL string `json:"avatar_url"`
}

type giteaPR struct {
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

type giteaRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	WorkflowID int64  `json:"workflow_id"`
	HTMLURL    string `json:"html_url"`
	URL        string `json:"url"`
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

type giteaRunsResponse struct {
	WorkflowRuns []giteaRun `json:"workflow_runs"`
	TotalCount   int        `json:"total_count"`
}

type giteaJob struct {
	ID          int64  `json:"id"`
	RunID       int64  `json:"run_id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	HTMLURL     string `json:"html_url"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
	RunnerID    *int64 `json:"runner_id"`
	RunnerName  string `json:"runner_name"`
	Steps       json.RawMessage `json:"steps"`
}

type giteaJobsResponse struct {
	Jobs       []giteaJob `json:"jobs"`
	TotalCount int        `json:"total_count"`
}

type giteaUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	FullName  string `json:"full_name"`
	AvatarURL string `json:"avatar_url"`
	IsAdmin   bool   `json:"is_admin"`
}

func (c *Client) apiURL(path string, query url.Values) string {
	u := *c.baseURL
	u.Path = strings.TrimRight(c.baseURL.Path, "/") + "/api/v1" + path
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, token string) (*http.Response, error) {
	return c.doBody(ctx, method, path, query, token, nil, "")
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, payload any) (*http.Response, error) {
	var raw []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return c.doBody(ctx, method, path, query, "", raw, "application/json")
}

func (c *Client) doBody(ctx context.Context, method, path string, query url.Values, token string, body []byte, contentType string) (*http.Response, error) {
	reqURL := c.apiURL(path, query)
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
			req.Header.Set("Authorization", "token "+tok)
		}
		req.Header.Set("Accept", "application/json")
		if contentType != "" && body != nil {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err = c.http.Do(req)
		if err != nil {
			lensmetrics.GiteaAPIErrorsTotal.Inc()
			return nil, err
		}
		status := strconv.Itoa(resp.StatusCode/100) + "xx"
		lensmetrics.GiteaAPIRequestsTotal.WithLabelValues(status).Inc()
		if resp.StatusCode >= 500 {
			lensmetrics.GiteaAPIErrorsTotal.Inc()
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
		return fmt.Errorf("gitea api not found: %s", resp.Request.URL.Path)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea api %s: %s", resp.Status, truncate(string(body), 200))
	}
	if dst == nil {
		return nil
	}
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode gitea response: %w", err)
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
	var vr versionResponse
	if err := decodeJSON(resp, &vr); err != nil {
		return nil, err
	}
	return &models.InstanceInfo{Version: vr.Version}, nil
}

func (c *Client) DetectCapabilities(ctx context.Context) (*models.Capabilities, error) {
	info, err := c.GetInstance(ctx)
	if err != nil {
		return nil, err
	}
	caps := &models.Capabilities{
		Version:            info.Version,
		OAuthProvider:      true,
		WorkflowRunWebhook: true,
		WorkflowJobWebhook: true,
	}
	if err := c.probeOK(ctx, http.MethodGet, "/admin/hooks", url.Values{"limit": {"1"}, "type": {"system"}}); err == nil {
		caps.SystemHooksAPI = true
	}
	resp, err := c.do(ctx, http.MethodGet, "/repos/search", url.Values{"limit": {"1"}, "page": {"1"}}, "")
	if err != nil {
		return caps, nil
	}
	var raw repoSearchResponse
	if err := decodeJSON(resp, &raw); err != nil {
		return caps, nil
	}
	if len(raw.Data) == 0 {
		return caps, nil
	}
	r := raw.Data[0]
	owner := ""
	if r.Owner != nil {
		owner = r.Owner.Login
	}
	if owner == "" && r.FullName != "" {
		if i := strings.IndexByte(r.FullName, '/'); i > 0 {
			owner = r.FullName[:i]
		}
	}
	if owner == "" || r.Name == "" {
		return caps, nil
	}
	runsPath := fmt.Sprintf("/repos/%s/%s/actions/runs", url.PathEscape(owner), url.PathEscape(r.Name))
	runsResp, err := c.do(ctx, http.MethodGet, runsPath, url.Values{"limit": {"1"}, "page": {"1"}}, "")
	if err != nil {
		return caps, nil
	}
	defer runsResp.Body.Close()
	if runsResp.StatusCode >= 200 && runsResp.StatusCode < 300 {
		caps.ActionsAPI = true
		caps.JobLogsAPI = true
	}
	return caps, nil
}

func (c *Client) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	var out []models.Organization
	for page := 1; ; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(defaultPageSize)}}
		resp, err := c.do(ctx, http.MethodGet, "/user/orgs", q, "")
		if err != nil {
			return nil, err
		}
		var orgs []giteaOrg
		if err := decodeJSON(resp, &orgs); err != nil {
			return nil, err
		}
		for _, o := range orgs {
			out = append(out, models.Organization{
				ExternalID: o.ID,
				Name:       o.Username,
				FullName:   o.FullName,
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
	q := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)}}
	// User ACL: /user/repos matches collaborator access. Sync token uses search for broader visibility.
	if token != "" {
		resp, err := c.do(ctx, http.MethodGet, "/user/repos", q, token)
		if err != nil {
			return forge.Page[models.Repository]{}, err
		}
		var raw []giteaRepo
		if err := decodeJSON(resp, &raw); err != nil {
			return forge.Page[models.Repository]{}, err
		}
		items := make([]models.Repository, 0, len(raw))
		for _, r := range raw {
			items = append(items, mapRepo(r))
		}
		return forge.Page[models.Repository]{Items: items, Page: page, HasMore: len(raw) >= limit}, nil
	}
	resp, err := c.do(ctx, http.MethodGet, "/repos/search", q, "")
	if err != nil {
		return forge.Page[models.Repository]{}, err
	}
	var raw repoSearchResponse
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.Repository]{}, err
	}
	items := make([]models.Repository, 0, len(raw.Data))
	for _, r := range raw.Data {
		items = append(items, mapRepo(r))
	}
	return forge.Page[models.Repository]{Items: items, Page: page, HasMore: len(raw.Data) >= limit}, nil
}

func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
	path := fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	var raw giteaRepo
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
	q := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)}, "state": {state}}
	path := fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	resp, err := c.do(ctx, http.MethodGet, path, q, "")
	if err != nil {
		return forge.Page[models.PullRequest]{}, err
	}
	var raw []giteaPR
	if err := decodeJSON(resp, &raw); err != nil {
		return forge.Page[models.PullRequest]{}, err
	}
	items := make([]models.PullRequest, 0, len(raw))
	for _, p := range raw {
		items = append(items, mapPR(p))
	}
	return forge.Page[models.PullRequest]{Items: items, Page: page, HasMore: len(raw) >= limit}, nil
}

// GetCombinedCommitStatus returns the normalized Lens ci_state for a commit/ref.
// Empty string means no statuses were reported.
func (c *Client) GetCombinedCommitStatus(ctx context.Context, repo models.RepoRef, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/status", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(ref))
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gitea api %s: %s", resp.Status, truncate(string(body), 200))
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

func (c *Client) ListWorkflowRuns(ctx context.Context, repo models.RepoRef, opts forge.RunOpts) (forge.Page[models.WorkflowRun], error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	limit := opts.PageSize
	if limit < 1 {
		limit = defaultPageSize
	}
	q := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)}}
	path := fmt.Sprintf("/repos/%s/%s/actions/runs", url.PathEscape(repo.Owner), url.PathEscape(repo.Name))
	resp, err := c.do(ctx, http.MethodGet, path, q, "")
	if err != nil {
		return forge.Page[models.WorkflowRun]{}, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return forge.Page[models.WorkflowRun]{Page: page}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return forge.Page[models.WorkflowRun]{}, fmt.Errorf("gitea api %s: %s", resp.Status, truncate(string(body), 200))
	}
	var wrapped giteaRunsResponse
	var flat []giteaRun
	if err := json.Unmarshal(body, &wrapped); err == nil && (wrapped.WorkflowRuns != nil || wrapped.TotalCount > 0 || len(body) > 2 && body[0] == '{') {
		if wrapped.WorkflowRuns == nil {
			// try alternate key
			var alt struct {
				Runs []giteaRun `json:"runs"`
			}
			_ = json.Unmarshal(body, &alt)
			if alt.Runs != nil {
				wrapped.WorkflowRuns = alt.Runs
			}
		}
	} else if err := json.Unmarshal(body, &flat); err == nil {
		wrapped.WorkflowRuns = flat
	} else {
		return forge.Page[models.WorkflowRun]{}, fmt.Errorf("decode workflow runs: %w", err)
	}
	items := make([]models.WorkflowRun, 0, len(wrapped.WorkflowRuns))
	for _, r := range wrapped.WorkflowRuns {
		items = append(items, mapRun(r))
	}
	return forge.Page[models.WorkflowRun]{Items: items, Page: page, HasMore: len(wrapped.WorkflowRuns) >= limit}, nil
}

func (c *Client) ListJobs(ctx context.Context, repo models.RepoRef, runExternalID int64) ([]models.Job, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), runExternalID)
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitea api %s: %s", resp.Status, truncate(string(body), 200))
	}
	var wrapped giteaJobsResponse
	var flat []giteaJob
	if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Jobs != nil {
		// ok
	} else if err := json.Unmarshal(body, &flat); err == nil {
		wrapped.Jobs = flat
	} else {
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
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("gitea api %s: %s", resp.Status, truncate(string(b), 200))
	}
	return resp.Body, nil
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
	apiPath := fmt.Sprintf("/repos/%s/%s/raw/%s", url.PathEscape(repo.Owner), url.PathEscape(repo.Name), strings.TrimPrefix(path, "/"))
	resp, err := c.do(ctx, http.MethodGet, apiPath, q, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitea api %s: %s", resp.Status, truncate(string(body), 200))
	}
	return body, nil
}

func (c *Client) GetAuthenticatedUser(ctx context.Context, userToken string) (*models.User, error) {
	resp, err := c.do(ctx, http.MethodGet, "/user", nil, userToken)
	if err != nil {
		return nil, err
	}
	var u giteaUser
	if err := decodeJSON(resp, &u); err != nil {
		return nil, err
	}
	gid := u.ID
	return &models.User{
		GiteaUserID: &gid,
		Login:       u.Login,
		Email:       u.Email,
		DisplayName: u.FullName,
		AvatarURL:   u.AvatarURL,
	}, nil
}

// UserSettings is the subset of Gitea /user/settings used by Lens.
type UserSettings struct {
	Theme string `json:"theme"`
}

// GetUserSettings returns the authenticated user's Gitea settings (including theme).
func (c *Client) GetUserSettings(ctx context.Context, userToken string) (*UserSettings, error) {
	resp, err := c.do(ctx, http.MethodGet, "/user/settings", nil, userToken)
	if err != nil {
		return nil, err
	}
	var s UserSettings
	if err := decodeJSON(resp, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func mapRepo(r giteaRepo) models.Repository {
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
		Empty:         r.Empty,
		Fork:          r.Fork,
		HTMLURL:       r.HTMLURL,
	}
}

func mapPR(p giteaPR) models.PullRequest {
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

func mapRun(r giteaRun) models.WorkflowRun {
	name := r.Name
	if name == "" {
		name = r.Title
	}
	workflowPath := workflows.NormalizeWorkflowPath(r.Path)
	name = workflows.DisplayWorkflowName(name, r.Path)
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
	html := r.HTMLURL
	if html == "" {
		html = r.URL
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
		HTMLURL:            html,
		WorkflowPath:       workflowPath,
		StartedAt:          parseOptionalTime(r.CreatedAt),
		CompletedAt:        parseOptionalTime(r.UpdatedAt),
		RunAttempt:         attempt,
	}
}

func mapJob(j giteaJob) models.Job {
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
