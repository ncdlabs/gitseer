# Configuration

Configuration layers (later wins for most keys):

1. Built-in defaults  
2. `config.yaml` (or path from `--config` / `GITSEER_CONFIG`)  
3. Environment `GITSEER_*` (and some `*_FILE` secret paths)  
4. Database `app_settings` after UI/setup save (live overrides for integration + preferences)

Example file: [config.example.yaml](https://github.com/ncdlabs/gitseer/blob/main/config.example.yaml).

## YAML sections

| Section | Purpose |
|---------|---------|
| `server.listen` | Bind address (default `0.0.0.0:8090`) |
| `server.external_url` | Public URL including subpath; OAuth + webhooks |
| `server.trusted_proxies` | CIDRs of reverse proxies that may set `X-Forwarded-For` / `X-Real-IP` (rate limits). Empty = use peer `RemoteAddr` only. |
| `database.driver` | `sqlite` (default) or `postgres` |
| `database.path` / `dsn` | SQLite path or Postgres DSN |
| `gitea.url` | Gitea base URL |
| `gitea.token` / files | Service API token |
| `gitea.webhook_secret` | HMAC secret (required when URL set unless unsigned allowed) |
| `gitea.allow_private_network` | Allow private/lab Gitea IPs (SSRF guard) |
| `gitea.allow_unsigned_webhooks` | Lab-only unsigned webhook accept |
| `sync.reconcile_interval` | Periodic sync (default `5m`) |
| `sync.history_days` | History window (default `30`) |
| `attention.long_running_after` | Long-run threshold (default `2h`) |
| `auth.*` | OAuth, bootstrap password, session TTL, ACL refresh, encryption key |
| `ui.instance_name` | Display name |
| `log.level` / `format` | Logging |
| `retention.*` | Days for runs / webhooks / resolved attention |

## Environment reference

| Variable | Maps to |
|----------|---------|
| `GITSEER_CONFIG` | Config file path |
| `GITSEER_SERVER_LISTEN` | `server.listen` |
| `GITSEER_SERVER_EXTERNAL_URL` | `server.external_url` |
| `GITSEER_SERVER_TRUSTED_PROXIES` | `server.trusted_proxies` (comma-separated CIDRs) |
| `GITSEER_DATABASE_DRIVER` | `database.driver` |
| `GITSEER_DATABASE_PATH` | `database.path` |
| `GITSEER_DATABASE_DSN` | `database.dsn` |
| `GITSEER_GITEA_URL` | `gitea.url` |
| `GITSEER_GITEA_TOKEN` / `_FILE` | Service token |
| `GITSEER_WEBHOOK_SECRET` / `_FILE` (aliases `GITSEER_GITEA_WEBHOOK_*`) | Webhook HMAC |
| `GITSEER_WEBHOOK_ALLOW_UNSIGNED` | Unsigned webhooks |
| `GITSEER_GITEA_ALLOW_PRIVATE_NETWORK` | Private network allow |
| `GITSEER_SYNC_RECONCILE_INTERVAL` | Sync interval |
| `GITSEER_SYNC_HISTORY_DAYS` | History days |
| `GITSEER_ATTENTION_LONG_RUNNING_AFTER` | Long-running threshold |
| `GITSEER_AUTH_BOOTSTRAP_PASSWORD` / `_FILE` | Bootstrap login |
| `GITSEER_AUTH_OAUTH_CLIENT_ID` | OAuth client id |
| `GITSEER_AUTH_OAUTH_CLIENT_SECRET` / `_FILE` | OAuth secret |
| `GITSEER_ENCRYPTION_KEY` / `_FILE` | Encrypt secrets + OAuth tokens at rest (min 16 chars → SHA-256 AES key). Required to save integration secrets to the DB. |
| `GITSEER_AUTH_SESSION_TTL` | Session lifetime |
| `GITSEER_AUTH_ACL_REFRESH_INTERVAL` | ACL refresh (default 6h) |
| `GITSEER_AUTH_COOKIE_SECURE` | Session cookie Secure flag |
| `GITSEER_UI_INSTANCE_NAME` | Instance name |
| `GITSEER_LOG_LEVEL` / `GITSEER_LOG_FORMAT` | Logging |
| `GITSEER_RETENTION_RUNS_DAYS` | Run retention |
| `GITSEER_RETENTION_WEBHOOKS_DAYS` | Webhook retention |
| `GITSEER_RETENTION_ATTENTION_DAYS` | Attention retention |

Unreadable `*_FILE` paths fail config load (no silent clear).

## Webhook secret policy

When `gitea.url` is set and the webhook secret is empty, startup **fails** unless `allow_unsigned_webhooks` / `GITSEER_WEBHOOK_ALLOW_UNSIGNED=true`.

## Installer config

See [install.example.yaml](https://github.com/ncdlabs/gitseer/blob/main/install.example.yaml). Resolution order: defaults → `--config` YAML → `.env` → `GITSEER_*` → prompts (skipped with `--non-interactive`).
