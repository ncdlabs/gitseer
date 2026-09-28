package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

const BootstrapElevateTTL = 5 * time.Minute
const bcryptCost = bcrypt.DefaultCost

var (
	ErrBootstrapAlreadyClaimed = errors.New("bootstrap already claimed")
	ErrBootstrapNotClaimed     = errors.New("bootstrap not claimed")
	ErrBootstrapLoginDisabled  = errors.New("bootstrap login disabled")
	ErrWeakPassword            = errors.New("password too weak")
	ErrInvalidUsername         = errors.New("invalid username")
)

// BootstrapAuthInfo is the public claim/login status for ui-config and /auth/me.
type BootstrapAuthInfo struct {
	Unclaimed      bool
	Username       string
	LoginEnabled   bool
	KeepAfterSetup bool
	HasPassword    bool // DB hash or legacy env password
}

// EffectiveBootstrapAdmin reports permanent bootstrap OR unexpired session elevation.
func EffectiveBootstrapAdmin(user *models.User, sess *models.Session) bool {
	if user == nil {
		return false
	}
	if user.IsBootstrapAdmin {
		return true
	}
	if sess == nil || sess.BootstrapElevatedUntil == nil {
		return false
	}
	return time.Now().UTC().Before(sess.BootstrapElevatedUntil.UTC())
}

// ApplyEffectiveBootstrap returns a user copy with IsBootstrapAdmin set when elevated.
func ApplyEffectiveBootstrap(user *models.User, sess *models.Session) *models.User {
	if user == nil {
		return nil
	}
	if user.IsBootstrapAdmin || !EffectiveBootstrapAdmin(user, sess) {
		return user
	}
	cp := *user
	cp.IsBootstrapAdmin = true
	return &cp
}

func (s *Service) bootstrapState(ctx context.Context) (*store.BootstrapAuthState, error) {
	st, err := s.store.GetBootstrapAuthState(ctx)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return &store.BootstrapAuthState{KeepAfterSetup: true}, nil
	}
	return st, nil
}

// BootstrapInfo returns claim/login status for the UI.
func (s *Service) BootstrapInfo(ctx context.Context, setupCompleted bool, allowSkipSetup bool) (BootstrapAuthInfo, error) {
	st, err := s.bootstrapState(ctx)
	if err != nil {
		return BootstrapAuthInfo{}, err
	}
	s.mu.RLock()
	envPW := s.cfg.BootstrapPassword
	cfgUser := strings.TrimSpace(s.cfg.BootstrapUsername)
	s.mu.RUnlock()

	username := strings.TrimSpace(st.Username)
	if username == "" {
		username = cfgUser
	}
	if username == "" {
		username = "bootstrap"
	}
	hasHash := strings.TrimSpace(st.PasswordHash) != ""
	hasEnv := envPW != ""
	unclaimed := !hasHash && !hasEnv
	loginEnabled := false
	if hasHash || hasEnv {
		if !setupCompleted || st.KeepAfterSetup || allowSkipSetup {
			loginEnabled = true
		}
	}
	return BootstrapAuthInfo{
		Unclaimed:      unclaimed,
		Username:       username,
		LoginEnabled:   loginEnabled,
		KeepAfterSetup: st.KeepAfterSetup,
		HasPassword:    hasHash || hasEnv,
	}, nil
}

func validateBootstrapUsername(username string) error {
	username = strings.TrimSpace(username)
	if len(username) < 2 || len(username) > 64 {
		return ErrInvalidUsername
	}
	for _, r := range username {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return ErrInvalidUsername
	}
	return nil
}

func validateBootstrapPassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	if len(password) > 256 {
		return ErrWeakPassword
	}
	return nil
}

func hashBootstrapPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *Service) verifyBootstrapPassword(ctx context.Context, password string) error {
	st, err := s.bootstrapState(ctx)
	if err != nil {
		return err
	}
	if st != nil && strings.TrimSpace(st.PasswordHash) != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(st.PasswordHash), []byte(password)); err != nil {
			return ErrInvalidCredentials
		}
		return nil
	}
	s.mu.RLock()
	envPW := s.cfg.BootstrapPassword
	s.mu.RUnlock()
	if envPW == "" {
		return ErrBootstrapNotClaimed
	}
	if !constantTimeStringEqual(password, envPW) {
		return ErrInvalidCredentials
	}
	return nil
}

// ClaimBootstrap sets the initial username + password and returns a bootstrap session.
func (s *Service) ClaimBootstrap(ctx context.Context, username, password, ip, ua string) (*models.User, string, error) {
	info, err := s.BootstrapInfo(ctx, false, true)
	if err != nil {
		return nil, "", err
	}
	if !info.Unclaimed {
		return nil, "", ErrBootstrapAlreadyClaimed
	}
	if err := validateBootstrapUsername(username); err != nil {
		return nil, "", err
	}
	if err := validateBootstrapPassword(password); err != nil {
		return nil, "", err
	}
	hash, err := hashBootstrapPassword(password)
	if err != nil {
		return nil, "", err
	}
	st, err := s.bootstrapState(ctx)
	if err != nil {
		return nil, "", err
	}
	keep := true
	if st != nil {
		keep = st.KeepAfterSetup
	}
	s.mu.RLock()
	if s.cfg.BootstrapKeepAfterSetup != nil {
		keep = *s.cfg.BootstrapKeepAfterSetup
	}
	s.mu.RUnlock()

	if err := s.store.SetBootstrapAuthClaim(ctx, username, hash, keep, true); err != nil {
		return nil, "", err
	}
	user, err := s.store.EnsureBootstrapUserWithLogin(ctx, username)
	if err != nil {
		return nil, "", err
	}
	token, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// LoginBootstrap validates username + password and creates a session.
func (s *Service) LoginBootstrap(ctx context.Context, username, password, ip, ua string) (*models.User, string, error) {
	st, err := s.bootstrapState(ctx)
	if err != nil {
		return nil, "", err
	}
	expected := "bootstrap"
	if st != nil && strings.TrimSpace(st.Username) != "" {
		expected = strings.TrimSpace(st.Username)
	} else {
		s.mu.RLock()
		if u := strings.TrimSpace(s.cfg.BootstrapUsername); u != "" {
			expected = u
		}
		s.mu.RUnlock()
	}
	if !strings.EqualFold(strings.TrimSpace(username), expected) {
		// Constant-time-ish: still verify password so timing doesn't reveal username.
		_ = s.verifyBootstrapPassword(ctx, password)
		return nil, "", ErrInvalidCredentials
	}
	if err := s.verifyBootstrapPassword(ctx, password); err != nil {
		return nil, "", err
	}
	user, err := s.store.EnsureBootstrapUserWithLogin(ctx, expected)
	if err != nil {
		return nil, "", err
	}
	token, err := s.createSession(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// ElevateBootstrap grants a 5-minute bootstrap elevation on the current session.
func (s *Service) ElevateBootstrap(ctx context.Context, sess *models.Session, password string) (time.Time, error) {
	if sess == nil {
		return time.Time{}, ErrUnauthorized
	}
	if err := s.verifyBootstrapPassword(ctx, password); err != nil {
		return time.Time{}, err
	}
	until := time.Now().UTC().Add(BootstrapElevateTTL)
	if err := s.store.SetSessionBootstrapElevatedUntil(ctx, sess.ID, &until); err != nil {
		return time.Time{}, err
	}
	sess.BootstrapElevatedUntil = &until
	return until, nil
}

// SetBootstrapPassword updates the bootstrap password (requires effective admin caller).
func (s *Service) SetBootstrapPassword(ctx context.Context, currentPassword, newPassword string) error {
	if err := s.verifyBootstrapPassword(ctx, currentPassword); err != nil {
		return err
	}
	if err := validateBootstrapPassword(newPassword); err != nil {
		return err
	}
	hash, err := hashBootstrapPassword(newPassword)
	if err != nil {
		return err
	}
	st, err := s.bootstrapState(ctx)
	if err != nil {
		return err
	}
	username := "bootstrap"
	if st != nil && strings.TrimSpace(st.Username) != "" {
		username = strings.TrimSpace(st.Username)
	}
	keep := true
	if st != nil {
		keep = st.KeepAfterSetup
	}
	return s.store.SetBootstrapAuthClaim(ctx, username, hash, keep, false)
}

// ReservedBootstrapLogin reports whether login collides with the bootstrap account name.
func (s *Service) ReservedBootstrapLogin(ctx context.Context, login string) bool {
	login = strings.TrimSpace(login)
	if strings.EqualFold(login, "bootstrap") {
		return true
	}
	st, err := s.bootstrapState(ctx)
	if err != nil || st == nil {
		return false
	}
	return strings.TrimSpace(st.Username) != "" && strings.EqualFold(login, st.Username)
}

// SeedBootstrapFromConfig writes install-time username / keep policy when unclaimed.
func (s *Service) SeedBootstrapFromConfig(ctx context.Context) error {
	s.mu.RLock()
	username := strings.TrimSpace(s.cfg.BootstrapUsername)
	keepPtr := s.cfg.BootstrapKeepAfterSetup
	s.mu.RUnlock()
	st, err := s.bootstrapState(ctx)
	if err != nil {
		return err
	}
	if st != nil && strings.TrimSpace(st.PasswordHash) != "" {
		return nil
	}
	if username != "" {
		if err := s.store.SetBootstrapUsernameSeed(ctx, username); err != nil {
			return fmt.Errorf("seed bootstrap username: %w", err)
		}
	}
	if keepPtr != nil {
		if err := s.store.SetBootstrapKeepAfterSetup(ctx, *keepPtr); err != nil {
			return fmt.Errorf("seed bootstrap keep_after_setup: %w", err)
		}
	}
	return nil
}

// UserFromRequestWithEffective returns the session user with elevation applied.
func (s *Service) UserFromRequestWithEffective(ctx context.Context, r *http.Request) (*models.User, *models.Session, error) {
	user, sess, err := s.UserFromRequest(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	return ApplyEffectiveBootstrap(user, sess), sess, nil
}
