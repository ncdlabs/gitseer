// Package notify delivers outbound attention alerts via SMTP and webhooks (outbox worker).
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
	"github.com/ncdlabs/gitseer/internal/models"
	"github.com/ncdlabs/gitseer/internal/store"
)

// SecretOpener decrypts sealed notification secrets.
type SecretOpener interface {
	OpenSecret(ciphertext string) (string, error)
	EncryptionKeyBytes() []byte
	EncryptionConfigured() bool
}

// ExternalURLSource supplies the public GitSeer base URL for deep links.
type ExternalURLSource interface {
	ExternalURL() string
}

// Payload is the JSON body stored on outbox rows and sent to generic webhooks.
type Payload struct {
	Title      string `json:"title"`
	Severity   string `json:"severity"`
	Kind       string `json:"kind"`
	RepoFull   string `json:"repo_full,omitempty"`
	RepoOwner  string `json:"repo_owner,omitempty"`
	RepoName   string `json:"repo_name,omitempty"`
	Type       string `json:"type,omitempty"`
	DeepLink   string `json:"deep_link,omitempty"`
	ForgeURL   string `json:"forge_url,omitempty"`
	Count      int    `json:"count,omitempty"`
	Items      []PayloadItem `json:"items,omitempty"`
	Message    string `json:"message,omitempty"`
}

// PayloadItem is one attention row inside a digest.
type PayloadItem struct {
	Title    string `json:"title"`
	Severity string `json:"severity"`
	RepoFull string `json:"repo_full"`
	Type     string `json:"type"`
	DeepLink string `json:"deep_link,omitempty"`
}

// Service enqueues and delivers notifications.
type Service struct {
	store   *store.Store
	secrets SecretOpener
	extURL  ExternalURLSource
	log     *slog.Logger
	http    *http.Client
}

// New builds a notification service.
func New(st *store.Store, secrets SecretOpener, extURL ExternalURLSource, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:   st,
		secrets: secrets,
		extURL:  extURL,
		log:     log,
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// OnAttentionOpened is the attention-engine hook for newly opened / reopened items.
func (s *Service) OnAttentionOpened(ctx context.Context, item *models.AttentionItem) {
	if s == nil || item == nil || s.store == nil {
		return
	}
	cfg, err := s.store.GetNotificationSettings(ctx)
	if err != nil || cfg == nil || !cfg.Enabled || !cfg.ImmediateEnabled {
		return
	}
	if !store.SeverityMeetsMin(item.Severity, cfg.MinSeverity) {
		return
	}
	payload := s.payloadFromAttention(item, store.NotifyKindImmediate)
	raw, _ := json.Marshal(payload)
	dedupe := fmt.Sprintf("immediate:%s:%s", item.Fingerprint, item.OpenedAt.UTC().Format(time.RFC3339Nano))
	s.enqueueAllChannels(ctx, cfg, store.NotifyKindImmediate, string(raw), dedupe)
}

// MaybeEnqueueDigest queues a digest when due (daily at digest_hour_utc).
func (s *Service) MaybeEnqueueDigest(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	cfg, err := s.store.GetNotificationSettings(ctx)
	if err != nil || cfg == nil || !cfg.Enabled || !cfg.DigestEnabled {
		return
	}
	now := time.Now().UTC()
	if now.Hour() != cfg.DigestHourUTC {
		return
	}
	if cfg.LastDigestAt != nil {
		last := cfg.LastDigestAt.UTC()
		if last.Year() == now.Year() && last.YearDay() == now.YearDay() {
			return
		}
	}
	items, err := s.store.ListOpenAttentionForNotify(ctx, cfg.MinSeverity, 200)
	if err != nil {
		s.log.Error("notification digest list failed", "err", err)
		return
	}
	payload := Payload{
		Title:    fmt.Sprintf("GitSeer daily digest (%d open)", len(items)),
		Severity: cfg.MinSeverity,
		Kind:     store.NotifyKindDigest,
		Count:    len(items),
		Message:  fmt.Sprintf("%d open attention item(s) at or above %s", len(items), cfg.MinSeverity),
		DeepLink: s.joinURL("/attention"),
	}
	for _, it := range items {
		payload.Items = append(payload.Items, PayloadItem{
			Title:    it.Title,
			Severity: it.Severity,
			RepoFull: it.RepoFull,
			Type:     it.Type,
			DeepLink: s.deepLinkFor(&it),
		})
	}
	raw, _ := json.Marshal(payload)
	dayKey := now.Format("2006-01-02")
	dedupeBase := "digest:" + dayKey
	s.enqueueAllChannels(ctx, cfg, store.NotifyKindDigest, string(raw), dedupeBase)
	if err := s.store.MarkNotificationDigestSent(ctx, now); err != nil {
		s.log.Warn("mark digest sent failed", "err", err)
	}
}

// EnqueueTest queues a test notification on every enabled channel.
func (s *Service) EnqueueTest(ctx context.Context) (int, error) {
	cfg, err := s.store.GetNotificationSettings(ctx)
	if err != nil {
		return 0, err
	}
	if cfg == nil {
		return 0, fmt.Errorf("notification settings unavailable")
	}
	payload := Payload{
		Title:    "GitSeer test notification",
		Severity: "critical",
		Kind:     store.NotifyKindTest,
		Message:  "This is a test notification from GitSeer.",
		DeepLink: s.joinURL("/settings#notifications"),
	}
	raw, _ := json.Marshal(payload)
	dedupe := fmt.Sprintf("test:%d", time.Now().UnixNano())
	return s.enqueueAllChannels(ctx, cfg, store.NotifyKindTest, string(raw), dedupe), nil
}

func (s *Service) enqueueAllChannels(ctx context.Context, cfg *store.NotificationSettings, kind, payloadJSON, dedupeBase string) int {
	n := 0
	type ch struct {
		enabled bool
		name    string
	}
	channels := []ch{
		{cfg.SMTPEnabled && strings.TrimSpace(cfg.SMTPHost) != "" && strings.TrimSpace(cfg.SMTPTo) != "", store.NotifyChannelSMTP},
		{cfg.SlackEnabled && strings.TrimSpace(cfg.SlackWebhookCiphertext) != "", store.NotifyChannelSlack},
		{cfg.DiscordEnabled && strings.TrimSpace(cfg.DiscordWebhookCiphertext) != "", store.NotifyChannelDiscord},
		{cfg.WebhookEnabled && strings.TrimSpace(cfg.WebhookURLCiphertext) != "", store.NotifyChannelWebhook},
	}
	for _, c := range channels {
		if !c.enabled {
			continue
		}
		dedupe := ""
		if dedupeBase != "" {
			dedupe = dedupeBase + ":" + c.name
		}
		row, err := s.store.EnqueueNotification(ctx, c.name, kind, payloadJSON, dedupe)
		if err != nil {
			s.log.Warn("enqueue notification failed", "channel", c.name, "err", err)
			continue
		}
		if row != nil {
			n++
		}
	}
	return n
}

func (s *Service) payloadFromAttention(item *models.AttentionItem, kind string) Payload {
	return Payload{
		Title:     item.Title,
		Severity:  item.Severity,
		Kind:      kind,
		RepoFull:  item.RepoFull,
		RepoOwner: item.RepoOwner,
		RepoName:  item.RepoName,
		Type:      item.Type,
		DeepLink:  s.deepLinkFor(item),
		ForgeURL:  item.HTMLURL,
		Message:   item.Title,
	}
}

func (s *Service) deepLinkFor(item *models.AttentionItem) string {
	if item == nil {
		return s.joinURL("/attention")
	}
	switch item.EntityType {
	case "pull_request":
		return s.joinURL("/pull-requests")
	case "workflow_run", "job":
		if item.EntityType == "workflow_run" && item.EntityID > 0 {
			return s.joinURL(fmt.Sprintf("/pipelines/%d", item.EntityID))
		}
		return s.joinURL("/pipelines")
	default:
		return s.joinURL("/attention")
	}
}

func (s *Service) joinURL(path string) string {
	base := ""
	if s.extURL != nil {
		base = strings.TrimRight(strings.TrimSpace(s.extURL.ExternalURL()), "/")
	}
	if base == "" {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// Run processes the outbox until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	digestTicker := time.NewTicker(1 * time.Minute)
	defer digestTicker.Stop()

	s.drain(ctx)
	s.MaybeEnqueueDigest(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.drain(ctx)
		case <-digestTicker.C:
			s.MaybeEnqueueDigest(ctx)
		}
	}
}

func (s *Service) drain(ctx context.Context) {
	_, _ = s.store.ReapStaleProcessingNotifications(ctx, 5*time.Minute)
	for {
		rows, err := s.store.ClaimNotificationOutbox(ctx, 10)
		if err != nil {
			s.log.Error("claim notification outbox failed", "err", err)
			return
		}
		if len(rows) == 0 {
			return
		}
		cfg, err := s.store.GetNotificationSettings(ctx)
		if err != nil {
			s.log.Error("load notification settings failed", "err", err)
			return
		}
		for i := range rows {
			s.deliverOne(ctx, cfg, &rows[i])
		}
	}
}

func (s *Service) deliverOne(ctx context.Context, cfg *store.NotificationSettings, row *store.NotificationOutboxRow) {
	var payload Payload
	_ = json.Unmarshal([]byte(row.PayloadJSON), &payload)
	var err error
	switch row.Channel {
	case store.NotifyChannelSMTP:
		err = s.sendSMTP(cfg, payload)
	case store.NotifyChannelSlack:
		err = s.sendSlack(cfg, payload)
	case store.NotifyChannelDiscord:
		err = s.sendDiscord(cfg, payload)
	case store.NotifyChannelWebhook:
		err = s.sendGenericWebhook(ctx, cfg, payload)
	default:
		err = fmt.Errorf("unknown channel %q", row.Channel)
	}
	if err != nil {
		s.log.Warn("notification delivery failed", "id", row.ID, "channel", row.Channel, "err", err)
		_ = s.store.MarkNotificationFailed(ctx, row.ID, row.Attempts+1, err.Error())
		return
	}
	if err := s.store.MarkNotificationSent(ctx, row.ID); err != nil {
		s.log.Warn("mark notification sent failed", "id", row.ID, "err", err)
	}
}

func (s *Service) openSecret(ciphertext string) (string, error) {
	ciphertext = strings.TrimSpace(ciphertext)
	if ciphertext == "" {
		return "", fmt.Errorf("secret not configured")
	}
	if s.secrets != nil {
		return s.secrets.OpenSecret(ciphertext)
	}
	return "", fmt.Errorf("encryption unavailable")
}

func (s *Service) sendSMTP(cfg *store.NotificationSettings, payload Payload) error {
	if cfg == nil || !cfg.SMTPEnabled {
		return fmt.Errorf("smtp disabled")
	}
	host := strings.TrimSpace(cfg.SMTPHost)
	if host == "" {
		return fmt.Errorf("smtp host empty")
	}
	port := cfg.SMTPPort
	if port <= 0 {
		port = 587
	}
	from := strings.TrimSpace(cfg.SMTPFrom)
	toList := splitAddresses(cfg.SMTPTo)
	if from == "" || len(toList) == 0 {
		return fmt.Errorf("smtp from/to required")
	}
	password := ""
	if strings.TrimSpace(cfg.SMTPPasswordCiphertext) != "" {
		var err error
		password, err = s.openSecret(cfg.SMTPPasswordCiphertext)
		if err != nil {
			return fmt.Errorf("smtp password: %w", err)
		}
	}
	subject := fmt.Sprintf("[GitSeer][%s] %s", strings.ToUpper(payload.Severity), payload.Title)
	body := buildTextBody(payload)
	msg := []byte("From: " + from + "\r\n" +
		"To: " + strings.Join(toList, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" + body + "\r\n")

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	tlsMode := strings.ToLower(strings.TrimSpace(cfg.SMTPTLSMode))
	username := strings.TrimSpace(cfg.SMTPUsername)

	switch tlsMode {
	case "tls", "ssl":
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		defer conn.Close()
		c, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		if username != "" {
			auth := smtp.PlainAuth("", username, password, host)
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
		return smtpSend(c, from, toList, msg)
	case "none", "off":
		return smtp.SendMail(addr, nilAuthIfEmpty(username, password, host), from, toList, msg)
	default: // starttls
		c, err := smtp.Dial(addr)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
		if username != "" {
			auth := smtp.PlainAuth("", username, password, host)
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
		return smtpSend(c, from, toList, msg)
	}
}

func nilAuthIfEmpty(user, pass, host string) smtp.Auth {
	if strings.TrimSpace(user) == "" {
		return nil
	}
	return smtp.PlainAuth("", user, pass, host)
}

func smtpSend(c *smtp.Client, from string, to []string, msg []byte) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func (s *Service) sendSlack(cfg *store.NotificationSettings, payload Payload) error {
	urlStr, err := s.openSecret(cfg.SlackWebhookCiphertext)
	if err != nil {
		return err
	}
	text := formatChatText(payload)
	body, _ := json.Marshal(map[string]string{"text": text})
	return s.postJSON(urlStr, body)
}

func (s *Service) sendDiscord(cfg *store.NotificationSettings, payload Payload) error {
	urlStr, err := s.openSecret(cfg.DiscordWebhookCiphertext)
	if err != nil {
		return err
	}
	text := formatChatText(payload)
	body, _ := json.Marshal(map[string]string{"content": truncateRunes(text, 1900)})
	return s.postJSON(urlStr, body)
}

func (s *Service) sendGenericWebhook(ctx context.Context, cfg *store.NotificationSettings, payload Payload) error {
	urlStr, err := s.openSecret(cfg.WebhookURLCiphertext)
	if err != nil {
		return err
	}
	if err := validateHTTPSURL(urlStr); err != nil {
		return err
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GitSeer-Notify/1.0")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func (s *Service) postJSON(urlStr string, body []byte) error {
	if err := validateHTTPSURL(urlStr); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GitSeer-Notify/1.0")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func validateHTTPSURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("webhook url must be http(s)")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook url missing host")
	}
	return nil
}

func splitAddresses(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildTextBody(p Payload) string {
	var b strings.Builder
	b.WriteString(p.Title)
	b.WriteString("\n")
	if p.Severity != "" {
		b.WriteString("Severity: ")
		b.WriteString(p.Severity)
		b.WriteString("\n")
	}
	if p.RepoFull != "" {
		b.WriteString("Repo: ")
		b.WriteString(p.RepoFull)
		b.WriteString("\n")
	}
	if p.DeepLink != "" {
		b.WriteString("Link: ")
		b.WriteString(p.DeepLink)
		b.WriteString("\n")
	}
	if p.ForgeURL != "" {
		b.WriteString("Forge: ")
		b.WriteString(p.ForgeURL)
		b.WriteString("\n")
	}
	if p.Message != "" && p.Message != p.Title {
		b.WriteString("\n")
		b.WriteString(p.Message)
		b.WriteString("\n")
	}
	if len(p.Items) > 0 {
		b.WriteString("\nItems:\n")
		for _, it := range p.Items {
			b.WriteString("- [")
			b.WriteString(it.Severity)
			b.WriteString("] ")
			b.WriteString(it.RepoFull)
			b.WriteString(": ")
			b.WriteString(it.Title)
			if it.DeepLink != "" {
				b.WriteString(" (")
				b.WriteString(it.DeepLink)
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatChatText(p Payload) string {
	parts := []string{fmt.Sprintf("*%s* [%s]", p.Title, p.Severity)}
	if p.RepoFull != "" {
		parts = append(parts, "repo: "+p.RepoFull)
	}
	if p.DeepLink != "" {
		parts = append(parts, p.DeepLink)
	}
	if p.Count > 0 {
		parts = append(parts, fmt.Sprintf("count: %d", p.Count))
	}
	return strings.Join(parts, "\n")
}

func truncateRunes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// SealSecret encrypts a plaintext secret with the configured encryption key.
func SealSecret(opener SecretOpener, plaintext string) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return "", nil
	}
	if opener == nil || !opener.EncryptionConfigured() {
		return "", fmt.Errorf("encryption key required to store notification secrets")
	}
	key := opener.EncryptionKeyBytes()
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key invalid")
	}
	return gitseercrypto.Encrypt(key, plaintext)
}
