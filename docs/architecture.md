# Architecture

## Shape

```text
┌─────────────┐     API + webhooks      ┌──────────────────────────────┐
│  Gitea /    │ ◄──────────────────────► │  gitseer (Go single binary)  │
│  Forgejo /  │                          │  + embedded React SPA        │
│  GitHub /   │                          │  SQLite (default) / Postgres │
│  GitLab /   │                          └──────────────────────────────┘
│  Bitbucket  │                                       │
└─────────────┘            browser ── session cookie + CSRF ──┘
```

- **Product:** GitSeer (user-facing). Binary / module / Helm IDs are `gitseer`.
- **Binary:** `cmd/gitseer` (`serve`, `version`, `install-ui`, `uninstall-ui`, `backup`, `restore`, `rotate-encryption-key`)
- **UI:** Vite/React SPA built into `internal/server/ui/dist` and served by the same process
- **Module:** `github.com/ncdlabs/gitseer`
- **HTTP:** chi router; migrations via goose (SQL in `migrations/` + `migrations/postgres/`)
- **Forges:** Gitea, GitHub, GitLab, Bitbucket, and Forgejo via `internal/forge` (`forge_type` on `instances`). Multiple instances per type supported via `/api/v1/instances`.

## Package boundaries

| Package | Responsibility |
|---------|----------------|
| `internal/forge` (+ `gitea`, `github`, `gitlab`, `bitbucket`, `forgejo`) | Forge HTTP clients, factory, type normalization |
| `internal/store` | Persistence (sqlc typed queries + hand-written dynamic SQL) |
| `internal/sync` | Periodic reconcile + history import (per-instance) |
| `internal/webhooks` | Ingest + process forge webhook events (HMAC / token fail-closed) |
| `internal/api` | `/api/v1` handlers (incl. setup wizard) |
| `internal/auth` / `internal/authz` | Sessions, OAuth, CSRF, ACL |
| `internal/attention` | Attention rule engine |
| `internal/notify` | Outbound SMTP / Slack / Discord / webhook outbox; fans out SSE + Web Push on attention open |
| `internal/webpush` | Self-hosted Web Push (VAPID) for browser/OS alerts |
| `internal/workflows` | Run/job graph helpers |
| `internal/realtime` | SSE hub (`/api/v1/events`) |
| `internal/settings` | DB-backed runtime settings, encryption key file, multi-instance forge CRUD |
| `internal/backup` | Backup / restore |
| `internal/config` / `internal/crypto` / `internal/database` | Config load, envelope crypto, DB open/migrate |
| `internal/metrics` | Prometheus series |
| `internal/ratelimit` | In-process per-IP limits |
| `internal/uiinstall` | Gitea custom template install |
| `internal/retention` | Aged data purge |
| `internal/server` | Router, middleware, embed UI |
| `internal/models` | Domain structs |

Raw forge wire types stay in the forge implementation packages. Domain models live in `internal/models`.

## Data flow

1. **Bootstrap / Settings** configure one or more forge instances (URL, token, webhook secret; Gitea OAuth when used). Additional instances via Settings → Integration or `/api/v1/instances`.
2. **Sync** iterates enabled instances under per-`instance_id` leases — repositories, open PRs, recent workflow runs/jobs (history window configurable).
3. **Webhooks** apply near-real-time updates (`POST /api/webhooks/{gitea|github|gitlab|bitbucket|forgejo}/{id}`; legacy unscoped Gitea uses the primary instance).
4. **Attention** evaluates discrete rules on sync/webhook paths and a periodic sweep (~10m). Newly opened/reopened items trigger outbound notify (if configured), SSE `attention` events, and optional Web Push.
5. **Repo health / failure intelligence** are computed rollups over indexed attention, runs, jobs, and PRs (no extra tables); optional log tails are on-demand forge fetches.
6. **UI** reads ACL-scoped API; **SSE** pushes events filtered by `authz.CanAccessRepo` (including browser-alert payloads on `attention`).
7. **Setup wizard** handlers live under `internal/api` (`/api/v1/setup/*`) with encryption helpers in `internal/settings`.

## Integrity rules (high level)

- Upserts COALESCE nil timestamps
- PRs reject older `updated_at`
- Runs accept greater `run_attempt` or same attempt with non-regressing status
- Open-PR sync closes numbers absent from the open list
- Soft-delete only rows with `last_synced_at` before the sync start
- Empty repo sync does **not** soft-delete the catalog (zero-result reconcile is a no-op for deletes)
- Webhook `processing` reaper (~5m)

## Realtime and metrics

- **SSE** at `/api/v1/events` (not WebSockets; no Redis). Event types include `workflow_run`, `workflow_job`, `pull_request`, and `attention` (optional `title` / `severity` / `repo_full` / `deep_link` for browser alerts).
- **Web Push** (optional per user): self-hosted VAPID in `internal/webpush`; service worker at `/sw.js`. Requires HTTPS or localhost.
- **Health** at `/health/live` (process) and `/health/ready` (DB)
- **Prometheus** at `/metrics` — session cookie or optional Bearer `GITSEER_METRICS_TOKEN`; labels are status/event only (never repo names)

## Proxy / subpath

Set `server.external_url` to the public URL **including** any path prefix. GitSeer strips only that configured `PathPrefix()`. Client `X-Forwarded-Prefix` is ignored.

## Design constraints

- Multi-forge: **Gitea, GitHub, GitLab, Bitbucket, Forgejo** in scope (ADR-029)
- No Redis
- Write ops: **Rerun Workflow** / **Cancel Workflow** via per-instance user tokens (`UserAccessTokenForInstance`); no silent service-PAT fallback for non-admin; bootstrap admin may use service PAT with UI warning when no user token
- Capability detection for Actions APIs; degrade when missing
- Login: bootstrap + forge OAuth (Gitea / Forgejo / GitHub / GitLab / Bitbucket, PKCE); service PAT for sync; ACL/grants per forge instance
