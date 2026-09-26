package forge

import (
	"testing"

	"github.com/ncdlabs/gitseer/internal/models"
)

func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		in, st, conc string
	}{
		{"queued", "queued", "unknown"},
		{"in_progress", "running", "unknown"},
		{"success", "completed", "success"},
		{"failure", "completed", "failure"},
		{"canceled", "completed", "cancelled"},
		{"", "unknown", "unknown"},
		{"weird", "unknown", "unknown"},
	}
	for _, tc := range cases {
		st, conc := NormalizeStatus(tc.in)
		if st != tc.st || conc != tc.conc {
			t.Fatalf("%q => %s/%s want %s/%s", tc.in, st, conc, tc.st, tc.conc)
		}
	}
}

func TestNormalizeCIState(t *testing.T) {
	cases := map[string]string{
		"success":  "success",
		"failure":  "failure",
		"error":    "failure",
		"pending":  "pending",
		"warning":  "pending",
		"canceled": "cancelled",
		"":         "",
		"other":    "",
	}
	for in, want := range cases {
		if got := NormalizeCIState(in); got != want {
			t.Fatalf("NormalizeCIState(%q)=%q want %q", in, got, want)
		}
	}
}

func TestAggregateCIState(t *testing.T) {
	runs := []models.WorkflowRun{
		{Status: "completed", Conclusion: "success"},
		{Status: "completed", Conclusion: "failure"},
	}
	if got := AggregateCIState(runs); got != "failure" {
		t.Fatalf("got %q", got)
	}
	runs = []models.WorkflowRun{
		{Status: "completed", Conclusion: "success"},
		{Status: "running", Conclusion: "unknown"},
	}
	if got := AggregateCIState(runs); got != "pending" {
		t.Fatalf("got %q", got)
	}
	runs = []models.WorkflowRun{
		{Status: "completed", Conclusion: "success"},
	}
	if got := AggregateCIState(runs); got != "success" {
		t.Fatalf("got %q", got)
	}
	if got := AggregateCIState(nil); got != "" {
		t.Fatalf("empty=%q", got)
	}
}

func TestAggregateReviewState(t *testing.T) {
	if got := AggregateReviewState([]string{"APPROVED", "COMMENTED"}); got != "approved" {
		t.Fatalf("approved=%q", got)
	}
	if got := AggregateReviewState([]string{"approved", "changes_requested"}); got != "changes_requested" {
		t.Fatalf("changes win=%q", got)
	}
	if got := AggregateReviewState([]string{"commented", "pending"}); got != "" {
		t.Fatalf("empty=%q", got)
	}
}

func TestMergeCIState(t *testing.T) {
	if got := MergeCIState("success", "failure"); got != "failure" {
		t.Fatalf("got %q", got)
	}
	if got := MergeCIState("pending", "success"); got != "pending" {
		t.Fatalf("got %q", got)
	}
	if got := MergeCIState("", "success"); got != "success" {
		t.Fatalf("got %q", got)
	}
}
