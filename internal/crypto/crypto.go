// Package crypto provides AES-256-GCM envelope helpers for secrets at rest.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext with a 32-byte key. Returns base64(nonce|ciphertext).
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes")
	}
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

// Decrypt reverses Encrypt.
func Decrypt(key []byte, encoded string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// LooksLikeCiphertext reports whether stored is plausibly an Encrypt() output
// (base64 of nonce‖ciphertext‖tag). Used to fail closed when the encryption key is missing.
func LooksLikeCiphertext(stored string) bool {
	raw, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return false
	}
	// AES-GCM default nonce is 12 bytes; auth tag is 16 bytes → minimum sealed blob length.
	const minSealed = 12 + 16
	return len(raw) >= minSealed
}

// KeyFromString derives a 32-byte AES key via SHA-256.
// Rejects empty input; does not zero-pad short secrets.
func KeyFromString(s string) ([]byte, error) {
	if s == "" {
		return nil, fmt.Errorf("encryption key is empty")
	}
	if len(s) < 16 {
		return nil, fmt.Errorf("encryption key must be at least 16 characters")
	}
	sum := sha256.Sum256([]byte(s))
	key := make([]byte, 32)
	copy(key, sum[:])
	return key, nil
}
