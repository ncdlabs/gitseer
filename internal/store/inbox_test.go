package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ncdlabs/gitseer/internal/database"
	"github.com/ncdlabs/gitseer/internal/models"
)

func TestSavedFiltersCRUD(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "sf.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := NewWithDriver(db, "sqlite")
	user, err := st.EnsureBootstrapUser(ctx)
	if err != nil {
		t.Fatal(err)
	}

	created, err := st.InsertSavedFilter(ctx, user.ID, "Critical Only", json.RawMessage(`{"severity":"critical","page":"attention"}`))
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Critical Only" {
		t.Fatalf("name=%q", created.Name)
	}
	var q map[string]any
	if err := json.Unmarshal(created.Query, &q); err != nil {
		t.Fatal(err)
	}
	if q["severity"] != "critical" {
		t.Fatalf("query=%v", q)
	}

	list, err := st.ListSavedFilters(ctx, user.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}

	updated, err := st.UpdateSavedFilter(ctx, user.ID, created.ID, "Critical Attention", json.RawMessage(`{"q":"ci","page":"attention"}`))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Critical Attention" {
		t.Fatalf("updated name=%q", updated.Name)
	}

	if _, err := st.InsertSavedFilter(ctx, user.ID, "Critical Attention", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected duplicate name error")
	}

	if err := st.DeleteSavedFilter(ctx, user.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	list, err = st.ListSavedFilters(ctx, user.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("after delete list=%v err=%v", list, err)
	}
}

func TestListInboxPersonalReasons(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "inbox.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := NewWithDriver(db, "sqlite")
	user, err := st.EnsureBootstrapUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := st.UpsertInstanceByURL(ctx, "t", "https://git.example.com", "1.26", "{}")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.UpsertRepository(ctx, inst.ID, models.Repository{
		ExternalID: 1, Owner: "o", Name: "r", FullName: "o/r", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	mine, err := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 10, Number: 1, Title: "Mine failing", AuthorLogin: "bootstrap",
		State: "open", CIState: models.CIStateFailure, ReviewState: "changes_requested",
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 11, Number: 2, Title: "Needs bootstrap review", AuthorLogin: "alice",
		State: "open", CIState: models.CIStateSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertPullRequest(ctx, repo.ID, models.PullRequest{
		ExternalID: 12, Number: 3, Title: "Unrelated", AuthorLogin: "bob",
		State: "open", CIState: models.CIStateSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "pr_ci_failure", Severity: "critical",
		EntityType: "pull_request", EntityID: mine.ID, Title: "CI fail",
		Fingerprint: "inbox-ci", MetadataJSON: `{"pr_id":1}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.UpsertAttention(ctx, models.AttentionItem{
		InstanceID: inst.ID, RepoID: repo.ID, Type: "awaiting_review", Severity: "waiting",
		EntityType: "pull_request", EntityID: other.ID, Title: "Review me",
		Fingerprint: "inbox-rev",
		MetadataJSON: `{"requested_reviewers":["bootstrap"]}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	ident := InboxIdentity{Login: "bootstrap"}
	items, total, err := st.ListInbox(ctx, ListInboxOpts{
		UserID: user.ID, BootstrapAll: true, Identity: ident, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total < 2 {
		t.Fatalf("expected at least 2 inbox items, total=%d items=%+v", total, items)
	}

	var sawAuthorFail, sawReviewer bool
	for _, it := range items {
		if containsReason(it.Reasons, InboxReasonFailingCI) && containsReason(it.Reasons, InboxReasonAuthor) {
			sawAuthorFail = true
		}
		if containsReason(it.Reasons, InboxReasonRequestedReviewer) {
			sawReviewer = true
		}
		if containsReason(it.Reasons, InboxReasonBlockedOnMe) && it.Attention != nil && it.Attention.EntityID == mine.ID {
			// changes_requested on authored PR
		}
	}
	if !sawAuthorFail {
		t.Fatalf("missing author+failing_ci: %+v", items)
	}
	if !sawReviewer {
		t.Fatalf("missing requested_reviewer: %+v", items)
	}

	filtered, n, err := st.ListInbox(ctx, ListInboxOpts{
		UserID: user.ID, BootstrapAll: true, Identity: ident,
		Reason: InboxReasonRequestedReviewer, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 || len(filtered) < 1 {
		t.Fatalf("reason filter empty: n=%d", n)
	}
	for _, it := range filtered {
		if !containsReason(it.Reasons, InboxReasonRequestedReviewer) {
			t.Fatalf("unexpected item in reviewer filter: %+v", it.Reasons)
		}
	}
}
