package crypto_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := gitseercrypto.KeyFromString("twenty-four-chars-ok-key")
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
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 4 || string(raw[:3]) != "GSe" || raw[3] != 1 {
		t.Fatalf("expected versioned envelope header GSe\\x01, got %q", raw[:min(4, len(raw))])
	}
	pt, err := gitseercrypto.Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "super-secret" {
		t.Fatalf("plaintext = %q", pt)
	}
}

func TestDecryptLegacyUnversioned(t *testing.T) {
	key, err := gitseercrypto.KeyFromString("twenty-four-chars-ok-key")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := sealLegacyForTest(key, "legacy-secret")
	if err != nil {
		t.Fatal(err)
	}
	pt, err := gitseercrypto.Decrypt(key, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "legacy-secret" {
		t.Fatalf("plaintext = %q", pt)
	}
	// Reencrypt upgrades to versioned envelope.
	upgraded, err := gitseercrypto.Reencrypt(key, key, legacy)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(upgraded)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 4 || string(raw[:3]) != "GSe" || raw[3] != 1 {
		t.Fatalf("reencrypt should write versioned envelope, got %q", raw[:min(4, len(raw))])
	}
	if !gitseercrypto.LooksLikeCiphertext(legacy) {
		t.Fatal("legacy blob should look like ciphertext")
	}
}

func sealLegacyForTest(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

func TestLooksLikeCiphertext(t *testing.T) {
	key, err := gitseercrypto.KeyFromString("twenty-four-chars-ok-key")
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

func TestReencryptOldToNewKeyRoundTrip(t *testing.T) {
	oldKey, err := gitseercrypto.KeyFromString("old-encryption-key-24ch!!")
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := gitseercrypto.KeyFromString("new-encryption-key-24ch!!")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := gitseercrypto.Encrypt(oldKey, "forge-pat-secret")
	if err != nil {
		t.Fatal(err)
	}
	resealed, err := gitseercrypto.Reencrypt(oldKey, newKey, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if resealed == "" || resealed == sealed {
		t.Fatalf("resealed=%q sealed=%q", resealed, sealed)
	}
	if _, err := gitseercrypto.Decrypt(oldKey, resealed); err == nil {
		t.Fatal("old key should not open resealed ciphertext")
	}
	pt, err := gitseercrypto.Decrypt(newKey, resealed)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "forge-pat-secret" {
		t.Fatalf("plaintext=%q", pt)
	}
	empty, err := gitseercrypto.Reencrypt(oldKey, newKey, "")
	if err != nil || empty != "" {
		t.Fatalf("empty reencrypt: %q %v", empty, err)
	}
	if _, err := gitseercrypto.Reencrypt(oldKey, newKey, sealed+"corrupt"); err == nil {
		t.Fatal("expected decrypt failure on corrupt ciphertext")
	}
}
