// Package config loads Lens configuration from a YAML file and LENS_* env overrides.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for Gitea Lens.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Gitea     GiteaConfig     `yaml:"gitea"`
	Sync      SyncConfig      `yaml:"sync"`
	Attention AttentionConfig `yaml:"attention"`
	Auth      AuthConfig      `yaml:"auth"`
	UI        UIConfig        `yaml:"ui"`
	Log       LogConfig       `yaml:"log"`
	Retention RetentionConfig `yaml:"retention"`
	Dev       DevConfig       `yaml:"dev"`
}

// DevConfig holds local-development-only toggles. Never enable in production.
type DevConfig struct {
	// AllowSkipSetup exposes a Skip Setup control in the wizard (npm run start sets this).
	AllowSkipSetup bool `yaml:"allow_skip_setup"`
}

type ServerConfig struct {
	Listen      string `yaml:"listen"`
	ExternalURL string `yaml:"external_url"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"` // sqlite | postgres
	Path   string `yaml:"path"`   // sqlite path
	DSN    string `yaml:"dsn"`    // postgres DSN
}

type GiteaConfig struct {
	URL                  string `yaml:"url"`
	Token                string `yaml:"token"`
	TokenFile            string `yaml:"token_file"`
	WebhookSecret        string `yaml:"webhook_secret"`
	WebhookSecretFile    string `yaml:"webhook_secret_file"`
	AllowPrivateNetwork  bool   `yaml:"allow_private_network"`
	AllowUnsignedWebhooks bool  `yaml:"allow_unsigned_webhooks"`
}

type SyncConfig struct {
	ReconcileInterval time.Duration `yaml:"reconcile_interval"`
	HistoryDays       int           `yaml:"history_days"`
}

type AttentionConfig struct {
	LongRunningAfter time.Duration `yaml:"long_running_after"`
}

type AuthConfig struct {
	Provider              string        `yaml:"provider"` // gitea | bootstrap
	BootstrapPassword     string        `yaml:"bootstrap_password"`
	BootstrapPasswordFile string        `yaml:"bootstrap_password_file"`
	SessionTTL            time.Duration `yaml:"session_ttl"`
	CookieSecure          *bool         `yaml:"cookie_secure"`
	OAuthClientID         string        `yaml:"oauth_client_id"`
	OAuthClientSecret     string        `yaml:"oauth_client_secret"`
	OAuthClientSecretFile string        `yaml:"oauth_client_secret_file"`
	EncryptionKey         string        `yaml:"encryption_key"` // 32+ char secret; used to derive AES key
	EncryptionKeyFile     string        `yaml:"encryption_key_file"`
	ACLRefreshInterval    time.Duration `yaml:"acl_refresh_interval"`
}

type UIConfig struct {
	InstanceName string `yaml:"instance_name"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // json | text
}

type RetentionConfig struct {
	RunsDays      int `yaml:"runs_days"`
	WebhooksDays  int `yaml:"webhooks_days"`
	AttentionDays int `yaml:"attention_days"`
}

// Default returns a config with V1 defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Listen: "0.0.0.0:8090",
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
			Path:   "data/lens.db",
		},
		Sync: SyncConfig{
			ReconcileInterval: 5 * time.Minute,
			HistoryDays:       30,
		},
		Attention: AttentionConfig{
			LongRunningAfter: 2 * time.Hour,
		},
		Auth: AuthConfig{
			Provider:           "gitea",
			SessionTTL:         24 * time.Hour,
			ACLRefreshInterval: 6 * time.Hour,
		},
		UI: UIConfig{
			InstanceName: "Gitea Lens",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		Retention: RetentionConfig{
			RunsDays:      90,
			WebhooksDays:  30,
			AttentionDays: 180,
		},
	}
}

// Load reads optional YAML from path (empty = defaults only), then applies env overrides.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return Config{}, fmt.Errorf("read config: %w", err)
			}
		} else {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse config: %w", err)
			}
		}
	}
	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.resolveSecrets(); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) resolveSecrets() error {
	if c.Gitea.TokenFile != "" {
		tok, err := readSecretFile(c.Gitea.TokenFile)
		if err != nil {
			return fmt.Errorf("gitea.token_file: %w", err)
		}
		c.Gitea.Token = tok
	}
	if c.Auth.BootstrapPasswordFile != "" {
		pw, err := readSecretFile(c.Auth.BootstrapPasswordFile)
		if err != nil {
			return fmt.Errorf("auth.bootstrap_password_file: %w", err)
		}
		c.Auth.BootstrapPassword = pw
	}
	if c.Auth.OAuthClientSecretFile != "" {
		sec, err := readSecretFile(c.Auth.OAuthClientSecretFile)
		if err != nil {
			return fmt.Errorf("auth.oauth_client_secret_file: %w", err)
		}
		c.Auth.OAuthClientSecret = sec
	}
	if c.Gitea.WebhookSecretFile != "" {
		sec, err := readSecretFile(c.Gitea.WebhookSecretFile)
		if err != nil {
			return fmt.Errorf("gitea.webhook_secret_file: %w", err)
		}
		c.Gitea.WebhookSecret = sec
	}
	if c.Auth.EncryptionKeyFile != "" {
		key, err := readSecretFile(c.Auth.EncryptionKeyFile)
		if err != nil {
			return fmt.Errorf("auth.encryption_key_file: %w", err)
		}
		c.Auth.EncryptionKey = key
	}
	return nil
}

func readSecretFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// Validate checks required fields for a runnable process.
func (c Config) Validate() error {
	if c.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	switch strings.ToLower(c.Database.Driver) {
	case "sqlite", "":
		if c.Database.Path == "" {
			return fmt.Errorf("database.path is required for sqlite")
		}
	case "postgres", "postgresql":
		if c.Database.DSN == "" {
			return fmt.Errorf("database.dsn is required for postgres")
		}
	default:
		return fmt.Errorf("unsupported database.driver %q", c.Database.Driver)
	}
	if c.Gitea.URL != "" && c.Gitea.WebhookSecret == "" && !c.Gitea.AllowUnsignedWebhooks {
		return fmt.Errorf("gitea.webhook_secret is required when gitea.url is set (or set gitea.allow_unsigned_webhooks / LENS_WEBHOOK_ALLOW_UNSIGNED=true for lab use)")
	}
	return nil
}

// CookieSecureResolved returns whether the session cookie should be Secure.
func (c Config) CookieSecureResolved() bool {
	if c.Auth.CookieSecure != nil {
		return *c.Auth.CookieSecure
	}
	return strings.HasPrefix(strings.ToLower(c.Server.ExternalURL), "https://")
}

// PathPrefix returns the HTTP path prefix derived from external_url (e.g. "/lens") or "".
func (c Config) PathPrefix() string {
	if c.Server.ExternalURL == "" {
		return ""
	}
	u, err := url.Parse(c.Server.ExternalURL)
	if err != nil || u.Path == "" || u.Path == "/" {
		return ""
	}
	return strings.TrimRight(u.Path, "/")
}

// CookiePath is "/" or the subpath prefix for session cookies.
func (c Config) CookiePath() string {
	if p := c.PathPrefix(); p != "" {
		return p
	}
	return "/"
}

// OAuthConfigured reports whether Gitea OAuth client settings are present.
func (c Config) OAuthConfigured() bool {
	return c.Gitea.URL != "" && c.Auth.OAuthClientID != "" && c.Server.ExternalURL != ""
}

func applyEnv(cfg *Config) error {
	setStr := func(dst *string, key string) error {
		v, ok, err := lookupEnv(key)
		if err != nil {
			return err
		}
		if ok {
			*dst = v
		}
		return nil
	}
	setBool := func(dst *bool, key string) error {
		v, ok, err := lookupEnv(key)
		if err != nil {
			return err
		}
		if !ok || v == "" {
			return nil
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*dst = b
		return nil
	}
	setInt := func(dst *int, key string) error {
		v, ok, err := lookupEnv(key)
		if err != nil {
			return err
		}
		if !ok || v == "" {
			return nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*dst = n
		return nil
	}
	setDur := func(dst *time.Duration, key string) error {
		v, ok, err := lookupEnv(key)
		if err != nil {
			return err
		}
		if !ok || v == "" {
			return nil
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*dst = d
		return nil
	}

	if err := setStr(&cfg.Server.Listen, "LENS_SERVER_LISTEN"); err != nil {
		return err
	}
	if err := setStr(&cfg.Server.ExternalURL, "LENS_SERVER_EXTERNAL_URL"); err != nil {
		return err
	}
	if err := setStr(&cfg.Database.Driver, "LENS_DATABASE_DRIVER"); err != nil {
		return err
	}
	if err := setStr(&cfg.Database.Path, "LENS_DATABASE_PATH"); err != nil {
		return err
	}
	if err := setStr(&cfg.Database.DSN, "LENS_DATABASE_DSN"); err != nil {
		return err
	}
	if err := setStr(&cfg.Gitea.URL, "LENS_GITEA_URL"); err != nil {
		return err
	}
	if err := setStr(&cfg.Gitea.Token, "LENS_GITEA_TOKEN"); err != nil {
		return err
	}
	if err := setStr(&cfg.Gitea.TokenFile, "LENS_GITEA_TOKEN_FILE"); err != nil {
		return err
	}
	if err := setStr(&cfg.Gitea.WebhookSecret, "LENS_GITEA_WEBHOOK_SECRET"); err != nil {
		return err
	}
	if err := setStr(&cfg.Gitea.WebhookSecretFile, "LENS_GITEA_WEBHOOK_SECRET_FILE"); err != nil {
		return err
	}
	if err := setStr(&cfg.Gitea.WebhookSecret, "LENS_WEBHOOK_SECRET"); err != nil {
		return err
	}
	if err := setStr(&cfg.Gitea.WebhookSecretFile, "LENS_WEBHOOK_SECRET_FILE"); err != nil {
		return err
	}
	if err := setBool(&cfg.Gitea.AllowPrivateNetwork, "LENS_GITEA_ALLOW_PRIVATE_NETWORK"); err != nil {
		return err
	}
	if err := setBool(&cfg.Gitea.AllowUnsignedWebhooks, "LENS_WEBHOOK_ALLOW_UNSIGNED"); err != nil {
		return err
	}
	if err := setDur(&cfg.Sync.ReconcileInterval, "LENS_SYNC_RECONCILE_INTERVAL"); err != nil {
		return err
	}
	if err := setInt(&cfg.Sync.HistoryDays, "LENS_SYNC_HISTORY_DAYS"); err != nil {
		return err
	}
	if err := setDur(&cfg.Attention.LongRunningAfter, "LENS_ATTENTION_LONG_RUNNING_AFTER"); err != nil {
		return err
	}
	if err := setStr(&cfg.Auth.Provider, "LENS_AUTH_PROVIDER"); err != nil {
		return err
	}
	if err := setStr(&cfg.Auth.BootstrapPassword, "LENS_AUTH_BOOTSTRAP_PASSWORD"); err != nil {
		return err
	}
	if err := setStr(&cfg.Auth.BootstrapPasswordFile, "LENS_AUTH_BOOTSTRAP_PASSWORD_FILE"); err != nil {
		return err
	}
	if err := setStr(&cfg.Auth.OAuthClientID, "LENS_AUTH_OAUTH_CLIENT_ID"); err != nil {
		return err
	}
	if err := setStr(&cfg.Auth.OAuthClientSecret, "LENS_AUTH_OAUTH_CLIENT_SECRET"); err != nil {
		return err
	}
	if err := setStr(&cfg.Auth.OAuthClientSecretFile, "LENS_AUTH_OAUTH_CLIENT_SECRET_FILE"); err != nil {
		return err
	}
	if err := setStr(&cfg.Auth.EncryptionKey, "LENS_ENCRYPTION_KEY"); err != nil {
		return err
	}
	if err := setStr(&cfg.Auth.EncryptionKeyFile, "LENS_ENCRYPTION_KEY_FILE"); err != nil {
		return err
	}
	if err := setDur(&cfg.Auth.SessionTTL, "LENS_AUTH_SESSION_TTL"); err != nil {
		return err
	}
	if err := setDur(&cfg.Auth.ACLRefreshInterval, "LENS_AUTH_ACL_REFRESH_INTERVAL"); err != nil {
		return err
	}
	if v, ok, err := lookupEnv("LENS_AUTH_COOKIE_SECURE"); err != nil {
		return err
	} else if ok && v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("LENS_AUTH_COOKIE_SECURE: %w", err)
		}
		cfg.Auth.CookieSecure = &b
	}
	if err := setStr(&cfg.UI.InstanceName, "LENS_UI_INSTANCE_NAME"); err != nil {
		return err
	}
	if err := setStr(&cfg.Log.Level, "LENS_LOG_LEVEL"); err != nil {
		return err
	}
	if err := setStr(&cfg.Log.Format, "LENS_LOG_FORMAT"); err != nil {
		return err
	}
	if err := setInt(&cfg.Retention.RunsDays, "LENS_RETENTION_RUNS_DAYS"); err != nil {
		return err
	}
	if err := setInt(&cfg.Retention.WebhooksDays, "LENS_RETENTION_WEBHOOKS_DAYS"); err != nil {
		return err
	}
	if err := setInt(&cfg.Retention.AttentionDays, "LENS_RETENTION_ATTENTION_DAYS"); err != nil {
		return err
	}
	if err := setBool(&cfg.Dev.AllowSkipSetup, "LENS_ALLOW_SKIP_SETUP"); err != nil {
		return err
	}
	return nil
}

// lookupEnv returns value for key, or for key_FILE the file contents (ADR-019).
// Unreadable *_FILE paths return an error via Load (never a silent empty overwrite).
func lookupEnv(key string) (string, bool, error) {
	if v, ok := os.LookupEnv(key); ok {
		return v, true, nil
	}
	if v, ok := os.LookupEnv(key + "_FILE"); ok && v != "" {
		b, err := os.ReadFile(v)
		if err != nil {
			return "", false, fmt.Errorf("%s_FILE: %w", key, err)
		}
		return strings.TrimSpace(string(b)), true, nil
	}
	return "", false, nil
}
