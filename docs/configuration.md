# Configuration

Configuration layers (later wins for most keys):

1. Built-in defaults  
2. `config.yaml` (or path from `--config` / `LENS_CONFIG`)  
3. Environment `LENS_*` (and some `*_FILE` secret paths)  
4. Database `app_settings` after UI/setup save (live overrides for integration + preferences)

Example file: [config.example.yaml](https://github.com/ncdlabs/gitea-lens/blob/main/config.example.yaml).

## YAML sections

| Section | Purpose |
|---------|---------|
| `server.listen` | Bind address (default `0.0.0.0:8090`) |
| `server.external_url` | Public URL including subpath; OAuth + webhooks |
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
| `LENS_CONFIG` | Config file path |
| `LENS_SERVER_LISTEN` | `server.listen` |
| `LENS_SERVER_EXTERNAL_URL` | `server.external_url` |
| `LENS_DATABASE_DRIVER` | `database.driver` |
| `LENS_DATABASE_PATH` | `database.path` |
| `LENS_DATABASE_DSN` | `database.dsn` |
| `LENS_GITEA_URL` | `gitea.url` |
| `LENS_GITEA_TOKEN` / `_FILE` | Service token |
| `LENS_WEBHOOK_SECRET` / `_FILE` (aliases `LENS_GITEA_WEBHOOK_*`) | Webhook HMAC |
| `LENS_WEBHOOK_ALLOW_UNSIGNED` | Unsigned webhooks |
| `LENS_GITEA_ALLOW_PRIVATE_NETWORK` | Private network allow |
| `LENS_SYNC_RECONCILE_INTERVAL` | Sync interval |
| `LENS_SYNC_HISTORY_DAYS` | History days |
| `LENS_ATTENTION_LONG_RUNNING_AFTER` | Long-running threshold |
| `LENS_AUTH_BOOTSTRAP_PASSWORD` / `_FILE` | Bootstrap login |
| `LENS_AUTH_OAUTH_CLIENT_ID` | OAuth client id |
| `LENS_AUTH_OAUTH_CLIENT_SECRET` / `_FILE` | OAuth secret |
| `LENS_ENCRYPTION_KEY` / `_FILE` | OAuth token encryption at rest (min 16 chars → SHA-256 AES key) |
| `LENS_AUTH_SESSION_TTL` | Session lifetime |
| `LENS_AUTH_ACL_REFRESH_INTERVAL` | ACL refresh (default 6h) |
| `LENS_AUTH_COOKIE_SECURE` | Session cookie Secure flag |
| `LENS_UI_INSTANCE_NAME` | Instance name |
| `LENS_LOG_LEVEL` / `LENS_LOG_FORMAT` | Logging |
| `LENS_RETENTION_RUNS_DAYS` | Run retention |
| `LENS_RETENTION_WEBHOOKS_DAYS` | Webhook retention |
| `LENS_RETENTION_ATTENTION_DAYS` | Attention retention |

Unreadable `*_FILE` paths fail config load (no silent clear).

## Webhook secret policy

When `gitea.url` is set and the webhook secret is empty, startup **fails** unless `allow_unsigned_webhooks` / `LENS_WEBHOOK_ALLOW_UNSIGNED=true`.

## Installer config

See [install.example.yaml](https://github.com/ncdlabs/gitea-lens/blob/main/install.example.yaml). Resolution order: defaults → `--config` YAML → `.env` → `LENS_*` → prompts (skipped with `--non-interactive`).
