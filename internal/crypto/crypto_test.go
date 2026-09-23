package crypto_test

import (
	"strings"
	"testing"

	lenscrypto "github.com/ncdlabs/gitea-lens/internal/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := lenscrypto.KeyFromString("sixteen-chars-ok")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := lenscrypto.Encrypt(key, "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	if ct == "" || ct == "super-secret" {
		t.Fatalf("ciphertext = %q", ct)
	}
	pt, err := lenscrypto.Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "super-secret" {
		t.Fatalf("plaintext = %q", pt)
	}
}

func TestLooksLikeCiphertext(t *testing.T) {
	key, err := lenscrypto.KeyFromString("sixteen-chars-ok")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := lenscrypto.Encrypt(key, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !lenscrypto.LooksLikeCiphertext(ct) {
		t.Fatalf("expected ciphertext to look encrypted: %q", ct)
	}
	if lenscrypto.LooksLikeCiphertext("plain-token-value") {
		t.Fatal("plaintext should not look encrypted")
	}
	if lenscrypto.LooksLikeCiphertext("") {
		t.Fatal("empty should not look encrypted")
	}
	// Not valid base64 of a sealed blob.
	if lenscrypto.LooksLikeCiphertext(strings.Repeat("A", 8)) {
		t.Fatal("short base64 should not look encrypted")
	}
}

func TestKeyFromStringRejectsShort(t *testing.T) {
	if _, err := lenscrypto.KeyFromString("short"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := lenscrypto.KeyFromString(""); err == nil {
		t.Fatal("expected error")
	}
}
