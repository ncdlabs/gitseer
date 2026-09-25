package crypto_test

import (
	"strings"
	"testing"

	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := gitseercrypto.KeyFromString("sixteen-chars-ok")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := gitseercrypto.Encrypt(key, "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	if ct == "" || ct == "super-secret" {
		t.Fatalf("ciphertext = %q", ct)
	}
	pt, err := gitseercrypto.Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "super-secret" {
		t.Fatalf("plaintext = %q", pt)
	}
}

func TestLooksLikeCiphertext(t *testing.T) {
	key, err := gitseercrypto.KeyFromString("sixteen-chars-ok")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := gitseercrypto.Encrypt(key, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !gitseercrypto.LooksLikeCiphertext(ct) {
		t.Fatalf("expected ciphertext to look encrypted: %q", ct)
	}
	if gitseercrypto.LooksLikeCiphertext("plain-token-value") {
		t.Fatal("plaintext should not look encrypted")
	}
	if gitseercrypto.LooksLikeCiphertext("") {
		t.Fatal("empty should not look encrypted")
	}
	// Not valid base64 of a sealed blob.
	if gitseercrypto.LooksLikeCiphertext(strings.Repeat("A", 8)) {
		t.Fatal("short base64 should not look encrypted")
	}
}

func TestKeyFromStringRejectsShort(t *testing.T) {
	if _, err := gitseercrypto.KeyFromString("short"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := gitseercrypto.KeyFromString(""); err == nil {
		t.Fatal("expected error")
	}
}
