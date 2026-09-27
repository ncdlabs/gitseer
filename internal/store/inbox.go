package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/ncdlabs/gitseer/internal/models"
)

// Inbox reason tags returned on GET /api/v1/inbox items.
const (
	InboxReasonAuthor            = "author"
	InboxReasonRequestedReviewer = "requested_reviewer"
	InboxReasonFailingCI         = "failing_ci"
	InboxReasonBlockedOnMe       = "blocked_on_me"
)

// InboxIdentity is the current user's forge identity used to match personal items.
type InboxIdentity struct {
	Login        string
	GiteaUserID  *int64
	GitHubUserID *int64
}

// InboxItem is an attention item or open PR relevant to the current user.
type InboxItem struct {
	Kind        string                `json:"kind"` // attention | pull_request
	Reasons     []string              `json:"reasons"`
	Attention   *models.AttentionItem `json:"attention,omitempty"`
	PullRequest *models.PullRequest   `json:"pull_request,omitempty"`
}

// ListInboxOpts scopes the personal inbox query.
type ListInboxOpts struct {
	UserID       int64
	BootstrapAll bool
	Identity     InboxIdentity
	Reason       string // optional filter: author | requested_reviewer | failing_ci | blocked_on_me
	Query        string
	ForgeType    string
	InstanceID   int64
	Limit        int
	Offset       int
}

// ListInbox returns attention/PRs relevant to the current user (author, reviewer when
// known, failing CI on authored PRs, blocked_on_me heuristic). ACL-scoped.
func (s *Store) ListInbox(ctx context.Context, opts ListInboxOpts) ([]InboxItem, int, error) {
	if err := requireListScope(opts.UserID, opts.BootstrapAll); err != nil {
		return nil, 0, err
	}
	opts.Limit = clampLimit(opts.Limit, 50, 200)
	login := strings.TrimSpace(opts.Identity.Login)
	if login == "" {
		return []InboxItem{}, 0, nil
	}

	prs, _, err := s.ListPullRequests(ctx, ListPRsOpts{
		UserID: opts.UserID, BootstrapAll: opts.BootstrapAll,
		State: "open", ForgeType: opts.ForgeType, InstanceID: opts.InstanceID,
		Query: opts.Query, Limit: 200, Offset: 0,
	})
	if err != nil {
		return nil, 0, err
	}
	attention, _, err := s.ListAttention(ctx, ListAttentionOpts{
		UserID: opts.UserID, BootstrapAll: opts.BootstrapAll,
		ForgeType: opts.ForgeType, InstanceID: opts.InstanceID,
		Query: opts.Query, OpenOnly: true, ExcludeMutedForUser: opts.UserID,
		Limit: 200, Offset: 0,
	})
	if err != nil {
		return nil, 0, err
	}

	prByID := make(map[int64]*models.PullRequest, len(prs))
	for i := range prs {
		prByID[prs[i].ID] = &prs[i]
	}

	var items []InboxItem
	seenPR := map[int64]bool{}
	seenAtt := map[int64]bool{}

	for i := range attention {
		a := &attention[i]
		var pr *models.PullRequest
		if a.EntityType == "pull_request" {
			pr = prByID[a.EntityID]
			if pr == nil {
				if loaded, err := s.GetPullRequestByID(ctx, a.EntityID); err == nil {
					pr = loaded
				}
			}
		}
		reasons := inboxReasonsForAttention(opts.Identity, a, pr)
		if len(reasons) == 0 {
			continue
		}
		if opts.Reason != "" && !containsReason(reasons, opts.Reason) {
			continue
		}
		items = append(items, InboxItem{
			Kind: "attention", Reasons: reasons, Attention: a, PullRequest: pr,
		})
		seenAtt[a.ID] = true
		if pr != nil {
			seenPR[pr.ID] = true
		}
	}

	for i := range prs {
		pr := &prs[i]
		if seenPR[pr.ID] {
			continue
		}
		reasons := inboxReasonsForPR(opts.Identity, pr, nil)
		if len(reasons) == 0 {
			continue
		}
		if opts.Reason != "" && !containsReason(reasons, opts.Reason) {
			continue
		}
		items = append(items, InboxItem{
			Kind: "pull_request", Reasons: reasons, PullRequest: pr,
		})
		seenPR[pr.ID] = true
	}

	_ = seenAtt
	sort.SliceStable(items, func(i, j int) bool {
		ti, ki, idi := inboxSortKey(items[i])
		tj, kj, idj := inboxSortKey(items[j])
		if ti != tj {
			return ti > tj
		}
		if ki != kj {
			return ki < kj
		}
		return idi < idj
	})

	total := len(items)
	if opts.Offset >= total {
		return []InboxItem{}, total, nil
	}
	end := opts.Offset + opts.Limit
	if end > total {
		end = total
	}
	return items[opts.Offset:end], total, nil
}

func containsReason(reasons []string, want string) bool {
	want = strings.TrimSpace(strings.ToLower(want))
	for _, r := range reasons {
		if strings.EqualFold(r, want) {
			return true
		}
	}
	return false
}

func loginMatches(ident InboxIdentity, login string) bool {
	a := strings.TrimSpace(strings.ToLower(ident.Login))
	b := strings.TrimSpace(strings.ToLower(login))
	return a != "" && a == b
}

func authorMatches(ident InboxIdentity, pr *models.PullRequest) bool {
	if pr == nil {
		return false
	}
	if loginMatches(ident, pr.AuthorLogin) {
		return true
	}
	if pr.AuthorExternalID == nil {
		return false
	}
	id := *pr.AuthorExternalID
	if ident.GiteaUserID != nil && *ident.GiteaUserID == id {
		return true
	}
	if ident.GitHubUserID != nil && *ident.GitHubUserID == id {
		return true
	}
	return false
}

func requestedReviewerMatches(ident InboxIdentity, metadataJSON string) bool {
	login := strings.TrimSpace(strings.ToLower(ident.Login))
	if login == "" || strings.TrimSpace(metadataJSON) == "" {
		return false
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
		return false
	}
	raw, ok := meta["requested_reviewers"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			switch t := item.(type) {
			case string:
				if strings.EqualFold(strings.TrimSpace(t), login) {
					return true
				}
			case map[string]any:
				if s, ok := t["login"].(string); ok && strings.EqualFold(strings.TrimSpace(s), login) {
					return true
				}
			}
		}
	case []string:
		for _, s := range v {
			if strings.EqualFold(strings.TrimSpace(s), login) {
				return true
			}
		}
	}
	return false
}

func isFailingCI(pr *models.PullRequest) bool {
	if pr == nil {
		return false
	}
	return strings.EqualFold(pr.CIState, models.CIStateFailure) ||
		strings.EqualFold(pr.CIState, "error") ||
		strings.EqualFold(pr.CIState, "timed_out")
}

func isBlockedOnAuthor(pr *models.PullRequest) bool {
	if pr == nil {
		return false
	}
	if strings.EqualFold(pr.ReviewState, "changes_requested") {
		return true
	}
	if pr.Mergeable != nil && !*pr.Mergeable {
		return true
	}
	ms := strings.ToLower(pr.MergeableState)
	if ms == "dirty" || ms == "conflicting" {
		return true
	}
	if isApprovedReview(pr.ReviewState) && isFailingCI(pr) {
		return true
	}
	return false
}

func isApprovedReview(state string) bool {
	return strings.EqualFold(strings.TrimSpace(state), "approved")
}

func inboxReasonsForPR(ident InboxIdentity, pr *models.PullRequest, att *models.AttentionItem) []string {
	var reasons []string
	authored := authorMatches(ident, pr)
	if authored {
		reasons = append(reasons, InboxReasonAuthor)
	}
	meta := ""
	if att != nil {
		meta = att.MetadataJSON
	}
	if requestedReviewerMatches(ident, meta) {
		reasons = append(reasons, InboxReasonRequestedReviewer)
	}
	if authored && isFailingCI(pr) {
		reasons = append(reasons, InboxReasonFailingCI)
	}
	if authored && isBlockedOnAuthor(pr) {
		reasons = append(reasons, InboxReasonBlockedOnMe)
	}
	if att != nil && authored {
		switch att.Type {
		case "pr_ci_failure", "required_check_failed", "approved_blocked_by_ci":
			if !containsReason(reasons, InboxReasonFailingCI) {
				reasons = append(reasons, InboxReasonFailingCI)
			}
			if att.Type == "approved_blocked_by_ci" && !containsReason(reasons, InboxReasonBlockedOnMe) {
				reasons = append(reasons, InboxReasonBlockedOnMe)
			}
		case "merge_conflict", "approved_behind_target":
			if !containsReason(reasons, InboxReasonBlockedOnMe) {
				reasons = append(reasons, InboxReasonBlockedOnMe)
			}
		}
	}
	if att != nil && att.Type == "awaiting_review" && requestedReviewerMatches(ident, att.MetadataJSON) {
		if !containsReason(reasons, InboxReasonBlockedOnMe) {
			reasons = append(reasons, InboxReasonBlockedOnMe)
		}
	}
	return reasons
}

func inboxReasonsForAttention(ident InboxIdentity, a *models.AttentionItem, pr *models.PullRequest) []string {
	if a == nil {
		return nil
	}
	reasons := inboxReasonsForPR(ident, pr, a)
	// Manual / long-running workflows keyed by actor in metadata when present.
	if requestedReviewerMatches(ident, a.MetadataJSON) && !containsReason(reasons, InboxReasonRequestedReviewer) {
		reasons = append(reasons, InboxReasonRequestedReviewer)
	}
	var meta map[string]any
	_ = json.Unmarshal([]byte(a.MetadataJSON), &meta)
	if actor, ok := meta["actor_login"].(string); ok && loginMatches(ident, actor) {
		if a.Type == "awaiting_manual" || a.Type == "long_running_workflow" {
			if !containsReason(reasons, InboxReasonBlockedOnMe) {
				reasons = append(reasons, InboxReasonBlockedOnMe)
			}
		}
	}
	// Authored PR attention without a loaded PR still counts via metadata author_login.
	if pr == nil {
		if author, ok := meta["author_login"].(string); ok && loginMatches(ident, author) {
			if !containsReason(reasons, InboxReasonAuthor) {
				reasons = append(reasons, InboxReasonAuthor)
			}
		}
	}
	return reasons
}

// GetPullRequestByID loads a single PR by primary key (no ACL — caller must scope).
func (s *Store) GetPullRequestByID(ctx context.Context, id int64) (*models.PullRequest, error) {
	row := s.queryRow(ctx, `
SELECT pr.id, pr.repo_id, pr.external_id, pr.number, pr.title, pr.body_excerpt, pr.author_login, pr.author_external_id,
       pr.source_branch, pr.target_branch, pr.head_sha, pr.base_sha, pr.state, pr.draft, pr.mergeable, pr.mergeable_state,
       pr.review_state, pr.ci_state, pr.html_url, pr.created_at, pr.updated_at, pr.closed_at, pr.merged_at,
       r.owner, r.name, r.full_name,
       COALESCE(NULLIF(TRIM(i.forge_type), ''), 'gitea'), r.instance_id, COALESCE(i.name, '')
FROM pull_requests pr
JOIN repositories r ON r.id = pr.repo_id
LEFT JOIN instances i ON i.id = r.instance_id
WHERE pr.id=?`, id)
	return scanPRWithForge(row)
}

func inboxSortKey(it InboxItem) (ts int64, kind string, id int64) {
	kind = it.Kind
	if it.Attention != nil {
		ts = it.Attention.OpenedAt.UnixNano()
		id = it.Attention.ID
		if ts == 0 {
			ts = it.Attention.UpdatedAt.UnixNano()
		}
		return ts, kind, id
	}
	if it.PullRequest != nil {
		if it.PullRequest.UpdatedAt != nil {
			ts = it.PullRequest.UpdatedAt.UnixNano()
		} else if it.PullRequest.CreatedAt != nil {
			ts = it.PullRequest.CreatedAt.UnixNano()
		}
		id = it.PullRequest.ID
	}
	return ts, kind, id
}
