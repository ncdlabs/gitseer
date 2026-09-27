package settings

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncdlabs/gitseer/internal/config"
	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
)

// Encryption sources reported to the setup wizard (never include key material).
const (
	EncryptionSourceConfig = "config" // GITSEER_ENCRYPTION_KEY / explicit encryption_key_file
	EncryptionSourceFile   = "file"   // wizard-persisted default path
)

// OnEncryptionChange is invoked after the AES key becomes available at runtime.
type OnEncryptionChange func(key []byte)

// DefaultEncryptionKeyPath is where the setup wizard persists the encryption passphrase
// when GITSEER_ENCRYPTION_KEY / auth.encryption_key_file are unset.
func DefaultEncryptionKeyPath(cfg config.Config) string {
	if p := strings.TrimSpace(cfg.Auth.EncryptionKeyFile); p != "" {
		return p
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Database.Driver), "postgres") {
		return filepath.Join("data", "gitseer.encryption_key")
	}
	path := strings.TrimSpace(cfg.Database.Path)
	if path == "" {
		path = "data/gitseer.db"
	}
	return filepath.Join(filepath.Dir(path), "gitseer.encryption_key")
}

func loadKeyFromConfig(cfg config.Config) (key []byte, source string) {
	if strings.TrimSpace(cfg.Auth.EncryptionKey) == "" {
		return nil, ""
	}
	k, err := gitseercrypto.KeyFromString(cfg.Auth.EncryptionKey)
	if err != nil {
		return nil, ""
	}
	return k, EncryptionSourceConfig
}

func loadKeyFromFile(path string) (key []byte, ok bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	pass := strings.TrimSpace(string(raw))
	if pass == "" {
		return nil, false
	}
	k, err := gitseercrypto.KeyFromString(pass)
	if err != nil {
		return nil, false
	}
	return k, true
}

// EncryptionConfigured reports whether secrets can be sealed.
func (m *Manager) EncryptionConfigured() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.encKey) == 32
}

// EncryptionSource returns "config", "file", or "" when unset.
func (m *Manager) EncryptionSource() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.encSource
}

// EncryptionKeyBytes returns a copy of the 32-byte AES key, or nil when unset.
func (m *Manager) EncryptionKeyBytes() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.encKey) != 32 {
		return nil
	}
	return append([]byte(nil), m.encKey...)
}

// SetOnEncryptionChange registers a live-apply callback when the key is set at runtime.
func (m *Manager) SetOnEncryptionChange(fn OnEncryptionChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onEnc = fn
}

// SetEncryptionKey validates, persists (0600 file), and applies a passphrase.
// Fails if a key is already configured (rotation is out of band).
func (m *Manager) SetEncryptionKey(passphrase string) error {
	passphrase = strings.TrimSpace(passphrase)
	if len(passphrase) < 24 {
		return fmt.Errorf("encryption key must be at least 24 characters (prefer a wizard-generated key)")
	}
	derived, err := gitseercrypto.KeyFromString(passphrase)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if len(m.encKey) == 32 {
		src := m.encSource
		m.mu.Unlock()
		if src == "" {
			src = "already set"
		}
		return fmt.Errorf("encryption key is already configured (%s)", src)
	}
	path := m.encKeyPath
	m.mu.Unlock()

	if path == "" {
		return fmt.Errorf("encryption key path is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create encryption key directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(passphrase+"\n"), 0o600); err != nil {
		return fmt.Errorf("write encryption key file: %w", err)
	}

	m.mu.Lock()
	m.encKey = derived
	m.encSource = EncryptionSourceFile
	onEnc := m.onEnc
	m.mu.Unlock()
	if onEnc != nil {
		onEnc(append([]byte(nil), derived...))
	}
	return nil
}

// GenerateEncryptionKey creates a random passphrase, persists it, and returns it once.
func (m *Manager) GenerateEncryptionKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate encryption key: %w", err)
	}
	passphrase := base64.RawURLEncoding.EncodeToString(b)
	if err := m.SetEncryptionKey(passphrase); err != nil {
		return "", err
	}
	return passphrase, nil
}

// RotateResult summarizes a successful encryption key rotation.
type RotateResult struct {
	FieldsUpdated int
	KeyFileUpdated bool
	KeyFilePath    string
	PreviousSource string
	Note           string
}

// RotateEncryptionKey re-seals all DB secrets under newPassphrase and, when the
// current key came from (or can be written to) the on-disk key file, updates that
// file. Fail-closed: DB is unchanged if any field cannot be re-sealed; the key
// file is not updated if the DB commit fails.
func (m *Manager) RotateEncryptionKey(ctx context.Context, newPassphrase string) (*RotateResult, error) {
	newPassphrase = strings.TrimSpace(newPassphrase)
	if len(newPassphrase) < 24 {
		return nil, fmt.Errorf("new encryption key must be at least 24 characters")
	}
	newKey, err := gitseercrypto.KeyFromString(newPassphrase)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	oldKey := append([]byte(nil), m.encKey...)
	src := m.encSource
	path := m.encKeyPath
	onEnc := m.onEnc
	m.mu.RUnlock()

	if len(oldKey) != 32 {
		return nil, fmt.Errorf("current encryption key is not configured")
	}
	if string(oldKey) == string(newKey) {
		return nil, fmt.Errorf("new encryption key derives to the same AES key as the current key")
	}
	if m.st == nil {
		return nil, fmt.Errorf("settings store is not configured")
	}

	n, err := m.st.RotateSealedSecrets(ctx, oldKey, newKey)
	if err != nil {
		return nil, err
	}

	result := &RotateResult{
		FieldsUpdated:  n,
		PreviousSource: src,
		KeyFilePath:    path,
	}

	updateFile := src == EncryptionSourceFile || (src == "" && path != "")
	if src == EncryptionSourceConfig {
		result.Note = "GITSEER_ENCRYPTION_KEY / auth.encryption_key is set from config; update that secret to the new passphrase and restart. On-disk key file was not modified."
	} else if path == "" {
		result.Note = "no encryption key file path configured; update GITSEER_ENCRYPTION_KEY to the new passphrase"
	} else if updateFile {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("secrets re-sealed (%d fields) but failed to prepare key directory: %w — set GITSEER_ENCRYPTION_KEY to the new passphrase immediately", n, err)
		}
		tmp := path + ".rotating"
		if err := os.WriteFile(tmp, []byte(newPassphrase+"\n"), 0o600); err != nil {
			return nil, fmt.Errorf("secrets re-sealed (%d fields) but failed to write new key file: %w — set GITSEER_ENCRYPTION_KEY to the new passphrase immediately", n, err)
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			return nil, fmt.Errorf("secrets re-sealed (%d fields) but failed to replace key file: %w — set GITSEER_ENCRYPTION_KEY to the new passphrase immediately", n, err)
		}
		result.KeyFileUpdated = true
		src = EncryptionSourceFile
	}

	m.mu.Lock()
	m.encKey = newKey
	if result.KeyFileUpdated {
		m.encSource = EncryptionSourceFile
	}
	m.mu.Unlock()
	if onEnc != nil {
		onEnc(append([]byte(nil), newKey...))
	}
	_ = src
	return result, nil
}

// RandomEncryptionPassphrase returns a high-entropy passphrase suitable for --generate.
func RandomEncryptionPassphrase() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate encryption key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
