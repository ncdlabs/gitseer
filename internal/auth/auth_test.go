package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestPKCEChallengeS256(t *testing.T) {
	// RFC 7636 appendix B
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	got := pkceChallengeS256(verifier)
	if got != want {
		t.Fatalf("challenge=%s want=%s", got, want)
	}
}

func TestRandomPKCEVerifierLength(t *testing.T) {
	v, err := randomPKCEVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if len(v) < 43 || len(v) > 128 {
		t.Fatalf("len=%d", len(v))
	}
}

func TestPathPrefixFromExternalURL(t *testing.T) {
	if PathPrefixFromExternalURL("https://git.example.com/gitseer") != "/gitseer" {
		t.Fatal("expected /gitseer")
	}
	if PathPrefixFromExternalURL("https://gitseer.example.com") != "/" {
		t.Fatal("expected /")
	}
	_ = sha256.Size
	_ = base64.RawURLEncoding
}

func TestSafeRedirectPath(t *testing.T) {
	cases := map[string]string{
		"":                     "/",
		"/":                    "/",
		"/pipelines/1":         "/pipelines/1",
		"https://evil.example": "/",
		"//evil.example":       "/",
		"/\\evil.example":      "/",
		"pipelines":            "/",
		"/\r\nLocation: x":     "/",
	}
	for in, want := range cases {
		if got := SafeRedirectPath(in); got != want {
			t.Fatalf("SafeRedirectPath(%q)=%q want %q", in, got, want)
		}
	}
}

func TestApplyPathPrefix(t *testing.T) {
	cases := []struct {
		path, prefix, want string
	}{
		{"/pipelines", "", "/pipelines"},
		{"/pipelines", "/app", "/app/pipelines"},
		{"/", "/app", "/app/"},
		{"/app/x", "/app", "/app/x"},
		{"//evil", "/app", "/app/"},
	}
	for _, tc := range cases {
		if got := ApplyPathPrefix(tc.path, tc.prefix); got != tc.want {
			t.Fatalf("ApplyPathPrefix(%q,%q)=%q want %q", tc.path, tc.prefix, got, tc.want)
		}
	}
}

func TestConstantTimeStringEqual(t *testing.T) {
	if !constantTimeStringEqual("same", "same") {
		t.Fatal("expected equal strings to match")
	}
	if constantTimeStringEqual("a", "b") {
		t.Fatal("expected different strings to mismatch")
	}
	if constantTimeStringEqual("short", "longer") {
		t.Fatal("expected different lengths to mismatch")
	}
}
