package theme

import "testing"

func TestMapGitea(t *testing.T) {
	cases := []struct {
		in   string
		want ID
		ok   bool
	}{
		{"gitea-light", Light, true},
		{"Gitea-Dark", Dark, true},
		{"  gitea-auto  ", System, true},
		{"gitea-light-protanopia-deuteranopia", "", false},
		{"gitea-dark-tritanopia", "", false},
		{"arc-green", "", false},
		{"gruvbox", "", false},
		{"", "", false},
		{"custom-neon", "", false},
	}
	for _, tc := range cases {
		got, ok := MapGitea(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("MapGitea(%q)=(%q,%v) want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
