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
	if PathPrefixFromExternalURL("https://git.example.com/lens") != "/lens" {
		t.Fatal("expected /lens")
	}
	if PathPrefixFromExternalURL("https://lens.example.com") != "/" {
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
		"pipelines":            "/",
		"/\r\nLocation: x":     "/",
	}
	for in, want := range cases {
		if got := SafeRedirectPath(in); got != want {
			t.Fatalf("SafeRedirectPath(%q)=%q want %q", in, got, want)
		}
	}
}
