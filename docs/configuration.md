# Configuration

Configuration layers (later wins for most keys):

1. Built-in defaults  
2. `config.yaml` (or path from `--config` / `GITSEER_CONFIG`)  
3. Environment `GITSEER_*` (and some `*_FILE` secret paths)  
4. Database overrides after UI/setup save:
   - `app_settings` — preferences + legacy dual-forge integration snapshot
   - `instances` — per-forge credentials (URL, encrypted PAT/webhook secret, Gitea OAuth); source of truth for multi-instance sync and webhooks

Example file: [config.example.yaml](https://github.com/ncdlabs/gitseer/blob/main/config.example.yaml).

## YAML sections

| Section | Purpose |
|---------|---------|
| `server.listen` | Bind address (default `0.0.0.0:8090`) |
| `server.external_url` | Public URL including subpath; OAuth + webhooks |
| `server.trusted_proxies` | CIDRs of reverse proxies that may set `X-Forwarded-For` / `X-Real-IP` (rate limits). Empty = use peer `RemoteAddr` only. |
| `server.metrics_token` / file | Optional Bearer token for `GET /metrics` |
| `database.driver` | `sqlite` (default) or `postgres` |
| `database.path` / `dsn` | SQLite path or Postgres DSN |
| `gitea.url` | Gitea base URL (optional if using GitHub only via wizard/Settings) |
| `gitea.token` / files | Service API token |
| `gitea.webhook_secret` | HMAC secret (required when URL set unless unsigned allowed) |
| `gitea.allow_private_network` | Allow private/lab Gitea IPs (SSRF guard) |
| `gitea.allow_unsigned_webhooks` | Lab-only unsigned webhook accept |
| `github.url` | GitHub.com or GitHub Enterprise base URL |
| `github.token` / files | Service PAT for GitHub inventory sync; per-user ACL/write ops use GitHub OAuth tokens on the instance when configured |
| `github.webhook_secret` | HMAC secret (required when URL set unless unsigned allowed) |
| `github.allow_private_network` | Allow private/lab GitHub Enterprise IPs |
| `github.allow_unsigned_webhooks` | Lab-only unsigned webhook accept |
| `sync.reconcile_interval` | Periodic sync (default `5m`); also used as per-instance sync lease TTL |
| `sync.history_days` | History window (default `30`) |
| `attention.long_running_after` | Long-run threshold (default `2h`) |
| `auth.*` | Provider, OAuth, bootstrap username / keep-after-setup / optional legacy password, session TTL, ACL refresh, encryption key |
| `ui.instance_name` | Display name |
| `log.level` / `format` | Logging |
| `retention.*` | Days for runs / webhooks / resolved attention. Settings presets: Lab (14/7/30) vs Prod (90/30/180) plus `sync.history_days` 7 vs 30. Status warns at 512 MiB / 2 GiB DB size; **Purge Now** via `POST /api/v1/admin/purge-retention`. |
| `dev.allow_skip_setup` | Local-only skip for `/setup` (`GITSEER_ALLOW_SKIP_SETUP`); rejected when `external_url` is non-local |

Outbound notification channels (SMTP, Slack/Discord/generic HTTPS webhooks, severity filter, digest hour) are **not** YAML keys — they live in `notification_settings` and are edited under Settings → **Notifications** (`GET/PUT /api/v1/notifications/settings`). Secrets are sealed with `GITSEER_ENCRYPTION_KEY`.

Browser / OS alerts use self-hosted Web Push VAPID keys. When `notifications.vapid_public_key` / `vapid_private_key` (or `GITSEER_VAPID_*`) are unset, GitSeer generates and stores `gitseer.vapid.json` beside the database. Optional `vapid_subject` should be a `mailto:` or `https:` contact URI.

File/env forge blocks seed or default the matching `instances` row by `(forge_type, base_url)`. Runtime multi-instance CRUD is under Settings → Integration / `/api/v1/instances` (bootstrap admin).

## Environment reference

| Variable | Maps to |
|----------|---------|
| `GITSEER_CONFIG` | Config file path |
| `GITSEER_SERVER_LISTEN` | `server.listen` |
| `GITSEER_SERVER_EXTERNAL_URL` | `server.external_url` |
| `GITSEER_SERVER_TRUSTED_PROXIES` | `server.trusted_proxies` (comma-separated CIDRs) |
| `GITSEER_METRICS_TOKEN` / `_FILE` | `server.metrics_token` |
| `GITSEER_DATABASE_DRIVER` | `database.driver` |
| `GITSEER_DATABASE_PATH` | `database.path` |
| `GITSEER_DATABASE_DSN` | `database.dsn` |
| `GITSEER_GITEA_URL` | `gitea.url` |
| `GITSEER_GITEA_TOKEN` / `_FILE` | Service token |
| `GITSEER_WEBHOOK_SECRET` / `_FILE` (aliases `GITSEER_GITEA_WEBHOOK_*`) | Gitea webhook HMAC |
| `GITSEER_WEBHOOK_ALLOW_UNSIGNED` | Gitea unsigned webhooks (lab) |
| `GITSEER_GITEA_ALLOW_PRIVATE_NETWORK` | Private network allow for Gitea |
| `GITSEER_GITHUB_URL` | `github.url` |
| `GITSEER_GITHUB_TOKEN` / `_FILE` | GitHub service PAT |
| `GITSEER_GITHUB_WEBHOOK_SECRET` / `_FILE` | GitHub webhook HMAC |
| `GITSEER_GITHUB_ALLOW_UNSIGNED_WEBHOOKS` | GitHub unsigned webhooks (lab) |
| `GITSEER_GITHUB_ALLOW_PRIVATE_NETWORK` | Private network allow for GitHub Enterprise |
| `GITSEER_SYNC_RECONCILE_INTERVAL` | Sync interval / lease TTL |
| `GITSEER_SYNC_HISTORY_DAYS` | History days |
| `GITSEER_ATTENTION_LONG_RUNNING_AFTER` | Long-running threshold |
| `GITSEER_AUTH_PROVIDER` | Legacy auth provider hint (`gitea` \| `bootstrap`); forge OAuth is enabled per configured instance |
| `GITSEER_AUTH_BOOTSTRAP_USERNAME` | Suggested username for Claim Bootstrap (default `admin`) |
| `GITSEER_AUTH_BOOTSTRAP_KEEP_AFTER_SETUP` | `true` keep break-glass login after setup; `false` remove login (Become Bootstrap only) |
| `GITSEER_AUTH_BOOTSTRAP_PASSWORD` / `_FILE` | Legacy seed only — prefer empty; claim password in the UI |
| `GITSEER_AUTH_OAUTH_CLIENT_ID` | OAuth client id |
| `GITSEER_AUTH_OAUTH_CLIENT_SECRET` / `_FILE` | OAuth secret |
| `GITSEER_ENCRYPTION_KEY` / `_FILE` | Encrypt secrets + OAuth tokens at rest (see below) |
| `GITSEER_AUTH_SESSION_TTL` | Session lifetime |
| `GITSEER_AUTH_ACL_REFRESH_INTERVAL` | ACL refresh (default 6h) |
| `GITSEER_AUTH_COOKIE_SECURE` | Session cookie Secure flag |
| `GITSEER_UI_INSTANCE_NAME` | Instance name |
| `GITSEER_LOG_LEVEL` / `GITSEER_LOG_FORMAT` | Logging |
| `GITSEER_RETENTION_RUNS_DAYS` | Run retention |
| `GITSEER_RETENTION_WEBHOOKS_DAYS` | Webhook retention |
| `GITSEER_RETENTION_ATTENTION_DAYS` | Attention retention |
| `GITSEER_ALLOW_SKIP_SETUP` | Skip `/setup` (local/dev only) |
| `GITSEER_VAPID_PUBLIC_KEY` / `GITSEER_VAPID_PRIVATE_KEY` | Optional Web Push VAPID override (else auto-generate) |
| `GITSEER_VAPID_SUBJECT` | VAPID contact URI (`mailto:` / `https:`) |
| `GITSEER_VAPID_KEY_FILE` | Path for persisted auto-generated VAPID JSON |

Unreadable `*_FILE` paths fail config load (no silent clear).

## Encryption key

`GITSEER_ENCRYPTION_KEY` (or file / wizard-persisted `gitseer.encryption_key` beside the DB) is required to **persist** forge secrets and OAuth tokens in the database. The passphrase is hashed with SHA-256 to derive the AES key (envelope version 1 — not a password KDF; use high-entropy keys). Ciphertext is base64(`GSe` ‖ version ‖ nonce ‖ ciphertext); older unversioned blobs still decrypt.

| Path | Minimum length |
|------|----------------|
| Env / `auth.encryption_key` at startup | **16** characters (prefer ≥24 for new keys) |
| Setup wizard **Save Key** (`POST /api/v1/setup/encryption`) | **24** characters |
| Wizard **Generate Key** | ~43 character base64url (32 random bytes) |

Prefer a wizard-generated key, or set `GITSEER_ENCRYPTION_KEY` before first run in production.

## Webhook secret policy

When `gitea.url` is set and the Gitea webhook secret is empty, startup **fails** unless `allow_unsigned_webhooks` / `GITSEER_WEBHOOK_ALLOW_UNSIGNED=true`.

When `github.url` is set and the GitHub webhook secret is empty, startup **fails** unless `github.allow_unsigned_webhooks` / `GITSEER_GITHUB_ALLOW_UNSIGNED_WEBHOOKS=true`.

Per-instance webhook secrets stored on `instances` are used for `POST /api/webhooks/{gitea|forgejo|github|gitlab|bitbucket}/{instanceID}`. The unscoped `POST /api/webhooks/gitea` route uses the [primary Gitea instance](authentication-and-acl.md#primary-gitea-instance) secret (legacy migration path).

## Installer config

See [install.example.yaml](https://github.com/ncdlabs/gitseer/blob/main/install.example.yaml). Resolution order: defaults → `--config` YAML → `.env` → `GITSEER_*` → prompts (skipped with `--non-interactive`).

The guided installer currently collects **Gitea** URL + token. For GitHub / GitLab / Bitbucket / Forgejo (or additional instances), use the binary/Compose start path and complete **Prepare → Choose Forge → Connect** in `/setup`, or add forges under Settings → Integration after bootstrap login.
