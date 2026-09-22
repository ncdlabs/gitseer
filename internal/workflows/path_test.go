package workflows

import (
	"reflect"
	"testing"
)

func TestNormalizeWorkflowPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"ci.yaml@refs/heads/main", "ci.yaml"},
		{"ci.yaml@refs/pull/64/head", "ci.yaml"},
		{".gitea/workflows/ci.yaml", ".gitea/workflows/ci.yaml"},
		{"  deploy.yaml@refs/tags/v1  ", "deploy.yaml"},
	}
	for _, tc := range cases {
		if got := NormalizeWorkflowPath(tc.in); got != tc.want {
			t.Fatalf("NormalizeWorkflowPath(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestCandidateWorkflowPaths(t *testing.T) {
	got := CandidateWorkflowPaths("ci.yaml@refs/heads/main")
	want := []string{".gitea/workflows/ci.yaml", ".github/workflows/ci.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	got = CandidateWorkflowPaths(".gitea/workflows/deploy.yaml")
	want = []string{".gitea/workflows/deploy.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestDisplayWorkflowName(t *testing.T) {
	if got := DisplayWorkflowName("CI", "ci.yaml@refs/heads/main"); got != "CI" {
		t.Fatalf("got %q", got)
	}
	if got := DisplayWorkflowName("", "ci.yaml@refs/heads/main"); got != "ci.yaml" {
		t.Fatalf("got %q", got)
	}
	if got := DisplayWorkflowName("", ".gitea/workflows/deploy.yaml"); got != "deploy.yaml" {
		t.Fatalf("got %q", got)
	}
}
