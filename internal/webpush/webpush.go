// Package webpush delivers self-hosted Web Push (VAPID) OS notifications.
package webpush

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	webpushlib "github.com/SherClockHolmes/webpush-go"
	"github.com/ncdlabs/gitseer/internal/config"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

// Keys holds a VAPID key pair.
type Keys struct {
	Public  string `json:"public_key"`
	Private string `json:"private_key"`
	Subject string `json:"subject,omitempty"`
}

// Sender delivers Web Push payloads to stored subscriptions.
type Sender struct {
	store   *store.Store
	log     *slog.Logger
	subject string

	mu   sync.RWMutex
	keys Keys
}

// NewSender builds a sender. Call EnsureKeys before use.
func NewSender(st *store.Store, log *slog.Logger, subject string) *Sender {
	if log == nil {
		log = slog.Default()
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "mailto:gitseer@localhost"
	}
	return &Sender{store: st, log: log, subject: subject}
}

// PublicKey returns the VAPID public key, or empty if unset.
func (s *Sender) PublicKey() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keys.Public
}

// Configured reports whether VAPID keys are ready.
func (s *Sender) Configured() bool {
	return s != nil && s.PublicKey() != "" && s.privateKey() != ""
}

func (s *Sender) privateKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keys.Private
}

// EnsureKeys loads config keys, else file keys, else generates and persists.
func (s *Sender) EnsureKeys(cfg config.Config) error {
	if s == nil {
		return fmt.Errorf("webpush sender nil")
	}
	pub := strings.TrimSpace(cfg.Notifications.VAPIDPublicKey)
	priv := strings.TrimSpace(cfg.Notifications.VAPIDPrivateKey)
	if sub := strings.TrimSpace(cfg.Notifications.VAPIDSubject); sub != "" {
		s.subject = sub
	}
	if pub != "" && priv != "" {
		s.mu.Lock()
		s.keys = Keys{Public: pub, Private: priv, Subject: s.subject}
		s.mu.Unlock()
		return nil
	}
	path := DefaultVAPIDPath(cfg)
	if keys, err := loadKeysFile(path); err == nil && keys.Public != "" && keys.Private != "" {
		if keys.Subject != "" {
			s.subject = keys.Subject
		}
		s.mu.Lock()
		s.keys = Keys{Public: keys.Public, Private: keys.Private, Subject: s.subject}
		s.mu.Unlock()
		return nil
	}
	pub, priv, err := webpushlib.GenerateVAPIDKeys()
	if err != nil {
		return fmt.Errorf("generate vapid keys: %w", err)
	}
	keys := Keys{Public: pub, Private: priv, Subject: s.subject}
	if err := saveKeysFile(path, keys); err != nil {
		return err
	}
	s.mu.Lock()
	s.keys = keys
	s.mu.Unlock()
	s.log.Info("generated web push VAPID keys", "path", path)
	return nil
}

// DefaultVAPIDPath is where auto-generated VAPID keys are stored.
func DefaultVAPIDPath(cfg config.Config) string {
	if p := strings.TrimSpace(cfg.Notifications.VAPIDKeyFile); p != "" {
		return p
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Database.Driver), "postgres") {
		return filepath.Join("data", "gitseer.vapid.json")
	}
	path := strings.TrimSpace(cfg.Database.Path)
	if path == "" {
		path = "data/gitseer.db"
	}
	return filepath.Join(filepath.Dir(path), "gitseer.vapid.json")
}

func loadKeysFile(path string) (Keys, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Keys{}, err
	}
	var keys Keys
	if err := json.Unmarshal(raw, &keys); err != nil {
		return Keys{}, err
	}
	return keys, nil
}

func saveKeysFile(path string, keys Keys) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PushPayload is the JSON body delivered to the service worker.
type PushPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Severity string `json:"severity,omitempty"`
	DeepLink string `json:"deep_link,omitempty"`
	RepoFull string `json:"repo_full,omitempty"`
	Type     string `json:"type,omitempty"`
	ID       int64  `json:"id,omitempty"`
}

// NotifyAttention fans out a Web Push to users who opted in and can see the repo.
func (s *Sender) NotifyAttention(ctx context.Context, item *models.AttentionItem, deepLink string) {
	if s == nil || !s.Configured() || item == nil || s.store == nil {
		return
	}
	userIDs, err := s.store.ListUserIDsForRepoAlert(ctx, item.RepoID)
	if err != nil {
		s.log.Warn("web push recipient list failed", "err", err)
		return
	}
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "GitSeer attention"
	}
	body := item.RepoFull
	if body == "" {
		body = item.Type
	} else if item.Type != "" {
		body = body + " · " + item.Type
	}
	payload := PushPayload{
		Title:    title,
		Body:     body,
		Severity: item.Severity,
		DeepLink: deepLink,
		RepoFull: item.RepoFull,
		Type:     item.Type,
		ID:       item.ID,
	}
	raw, _ := json.Marshal(payload)

	for _, uid := range userIDs {
		prefs, err := s.store.GetUserAlertPrefs(ctx, uid)
		if err != nil || prefs == nil || !prefs.PushEnabled {
			continue
		}
		if !store.SeverityMeetsMin(item.Severity, prefs.MinSeverity) {
			continue
		}
		subs, err := s.store.ListWebPushSubscriptions(ctx, uid)
		if err != nil {
			s.log.Warn("list web push subs failed", "user_id", uid, "err", err)
			continue
		}
		for _, sub := range subs {
			if err := s.send(ctx, sub, raw); err != nil {
				s.log.Warn("web push send failed", "user_id", uid, "err", err)
			}
		}
	}
}

// SendTest pushes a test notification to the given user's subscriptions.
func (s *Sender) SendTest(ctx context.Context, userID int64, deepLink string) (int, error) {
	if s == nil || !s.Configured() {
		return 0, fmt.Errorf("web push not configured")
	}
	subs, err := s.store.ListWebPushSubscriptions(ctx, userID)
	if err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, fmt.Errorf("no push subscriptions; enable browser alerts first")
	}
	raw, _ := json.Marshal(PushPayload{
		Title:    "GitSeer test notification",
		Body:     "Browser / OS push is working.",
		Severity: "critical",
		DeepLink: deepLink,
		Type:     "test",
	})
	n := 0
	for _, sub := range subs {
		if err := s.send(ctx, sub, raw); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Sender) send(ctx context.Context, sub store.WebPushSubscription, payload []byte) error {
	resp, err := webpushlib.SendNotificationWithContext(ctx, payload, &webpushlib.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpushlib.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}, &webpushlib.Options{
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.PublicKey(),
		VAPIDPrivateKey: s.privateKey(),
		TTL:             60,
		Urgency:         webpushlib.UrgencyHigh,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 || resp.StatusCode == 410 {
		_ = s.store.DeleteWebPushSubscriptionByEndpoint(ctx, sub.Endpoint)
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("push endpoint status %d", resp.StatusCode)
	}
	return nil
}
