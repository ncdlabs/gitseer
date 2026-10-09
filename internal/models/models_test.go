package models

import "testing"

func TestForgeTokenLabel_Cached(t *testing.T) {
	tests := []struct {
		forgeType string
		want      string
	}{
		{ForgeTypeGitHub, "Personal Access Token"},
		{ForgeTypeGitLab, "Personal Access Token"},
		{ForgeTypeBitbucket, "HTTP Access Token"},
		{ForgeTypeGitea, "Access Token"},
		{ForgeTypeForgejo, "Access Token"},
		{"unknown", "Access Token"},
		{"", "Access Token"},
		{"  github  ", "Personal Access Token"}, // trimmed + case-insensitive
	}
	for _, tc := range tests {
		got := ForgeTokenLabel(tc.forgeType)
		if got != tc.want {
			t.Errorf("ForgeTokenLabel(%q) = %q, want %q", tc.forgeType, got, tc.want)
		}
	}
}

func TestForgeTokenLabel_ConsistentAcrossCalls(t *testing.T) {
	// Repeated calls should return the same (cached) result.
	first := ForgeTokenLabel(ForgeTypeGitHub)
	for i := 0; i < 10; i++ {
		if got := ForgeTokenLabel(ForgeTypeGitHub); got != first {
			t.Fatalf("call %d: got %q, want %q", i, got, first)
		}
	}
}
