// Package crypto provides AES-256-GCM envelope helpers for secrets at rest.
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// Envelope layout (base64-encoded):
//
//	v1 (current):  magic "GSe" || version(1) || nonce || ciphertext||tag
//	legacy:        nonce || ciphertext||tag  (pre-versioning; still decryptable)
//
// Future KDF upgrades bump the version byte and may insert salt after it without
// invalidating existing rows — Decrypt switches on the version; Encrypt always
// writes the current version. rotate-encryption-key upgrades legacy → current.
var (
	envelopeMagic   = []byte{'G', 'S', 'e'} // GitSeer envelope
	envelopeVersion = byte(1)               // derivation: SHA-256 of passphrase → AES key
)

const (
	envelopeHeaderLen = 4 // magic(3) + version(1)
	aesGCMNonceSize   = 12
	aesGCMTagSize     = 16
	minLegacySealed   = aesGCMNonceSize + aesGCMTagSize
	minVersionedSealed = envelopeHeaderLen + minLegacySealed
)

// Encrypt encrypts plaintext with a 32-byte key. Returns base64(header|nonce|ciphertext).
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes")
	}
	raw, err := sealGCM(key, []byte(plaintext))
	if err != nil {
		return "", err
	}
	out := make([]byte, 0, envelopeHeaderLen+len(raw))
	out = append(out, envelopeMagic...)
	out = append(out, envelopeVersion)
	out = append(out, raw...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Reencrypt decrypts ciphertext with oldKey and seals it with newKey.
// Empty ciphertext is a no-op (returns empty). Fail-closed on decrypt errors.
// Output is always the current envelope version (upgrades legacy blobs).
func Reencrypt(oldKey, newKey []byte, ciphertext string) (string, error) {
	ciphertext = strings.TrimSpace(ciphertext)
	if ciphertext == "" {
		return "", nil
	}
	pt, err := Decrypt(oldKey, ciphertext)
	if err != nil {
		return "", err
	}
	return Encrypt(newKey, pt)
}

// Decrypt reverses Encrypt. Accepts current versioned envelopes and legacy
// unversioned base64(nonce|ciphertext) blobs from older installs.
func Decrypt(key []byte, encoded string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	payload, err := peelEnvelope(raw)
	if err != nil {
		return "", err
	}
	pt, err := openGCM(key, payload)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func peelEnvelope(raw []byte) ([]byte, error) {
	if len(raw) >= envelopeHeaderLen && bytes.Equal(raw[:3], envelopeMagic) {
		ver := raw[3]
		switch ver {
		case 1:
			if len(raw) < minVersionedSealed {
				return nil, fmt.Errorf("ciphertext too short")
			}
			return raw[envelopeHeaderLen:], nil
		default:
			return nil, fmt.Errorf("unsupported envelope version %d", ver)
		}
	}
	// Legacy unversioned nonce|ciphertext.
	if len(raw) < minLegacySealed {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return raw, nil
}

func sealGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func openGCM(key, raw []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// LooksLikeCiphertext reports whether stored is plausibly an Encrypt() output
// (base64 of optional header‖nonce‖ciphertext‖tag). Used to fail closed when the encryption key is missing.
func LooksLikeCiphertext(stored string) bool {
	raw, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return false
	}
	if len(raw) >= envelopeHeaderLen && bytes.Equal(raw[:3], envelopeMagic) {
		return len(raw) >= minVersionedSealed
	}
	return len(raw) >= minLegacySealed
}

// LooksLikeBase64Blob reports whether stored looks like base64 material that might
// be a sealed secret (or truncated ciphertext) rather than a human plaintext token.
func LooksLikeBase64Blob(stored string) bool {
	stored = strings.TrimSpace(stored)
	if len(stored) < 16 {
		return false
	}
	_, err := base64.StdEncoding.DecodeString(stored)
	return err == nil
}

// KeyFromString derives a 32-byte AES key via SHA-256 (envelope version 1).
// Rejects empty or short input. This is not a password KDF (no salt/iterations):
// operators should use a high-entropy key (wizard-generated or ≥24 random chars).
// Envelope versioning (see Encrypt) lets a future version switch derivation without
// invalidating existing rows; changing this function alone would break version-1 blobs.
// Minimum length remains 16 for compatibility with existing installs; prefer ≥24 for new keys.
//
// CodeQL may flag SHA-256 here as weak password hashing; that is a false positive —
// the input is an encryption key, not a user password.
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
