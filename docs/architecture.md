# Architecture

## Shape

```text
┌─────────────┐     API + webhooks      ┌──────────────────────────────┐
│   Gitea /   │ ◄──────────────────────► │  gitseer (Go single binary)     │
│   GitHub    │                          │  + embedded React SPA       │
│   (SoR)     │                          │  SQLite (default) / Postgres│
└─────────────┘                          └──────────────────────────────┘
                                                    │
                         browser ── session cookie + CSRF ──┘
```

- **Product:** GitSeer (user-facing). Binary / module / Helm IDs are `gitseer`.
- **Binary:** `cmd/gitseer` (`serve`, `version`, `install-ui`, `uninstall-ui`)
- **UI:** Vite/React SPA built into `internal/server/ui/dist` and served by the same process
- **Module:** `github.com/ncdlabs/gitseer`
- **HTTP:** chi router; migrations via goose (SQL in `migrations/`)
- **Forges:** Gitea + GitHub via `internal/forge` (`forge_type` on `instances`). GitLab / Bitbucket are Coming Soon (no clients yet).

## Package boundaries

| Package | Responsibility |
|---------|----------------|
| `internal/forge` (+ `gitea`, `github`) | Forge HTTP clients, factory, type normalization |
| `internal/store` | Persistence (hand-written SQL) |
| `internal/sync` | Periodic reconcile + history import (per-instance) |
| `internal/webhooks` | Ingest + process Gitea and GitHub events |
| `internal/api` | `/api/v1` handlers |
| `internal/auth` / `internal/authz` | Sessions, OAuth, CSRF, ACL |
| `internal/attention` | Attention rule engine |
| `internal/workflows` | Run/job graph helpers |
| `internal/realtime` | SSE hub (`/api/v1/events`) |
| `internal/settings` | DB-backed runtime settings + dual integration |
| `internal/setup` | Setup wizard backend |
| `internal/metrics` | Prometheus series |
| `internal/ratelimit` | In-process per-IP limits |
| `internal/uiinstall` | Gitea custom template install |
| `internal/retention` | Aged data purge |

Raw forge wire types stay in `internal/forge/gitea` and `internal/forge/github`. Domain models live in `internal/models`.

## Data flow

1. **Bootstrap / Settings** configure one or both forges (URL, token, webhook secret; Gitea OAuth when used).
2. **Sync** iterates enabled instances — repositories, open PRs, recent workflow runs/jobs (history window configurable).
3. **Webhooks** apply near-real-time updates (`POST /api/webhooks/gitea/{id}`, `POST /api/webhooks/github/{id}`).
4. **Attention** evaluates discrete rules on sync/webhook paths and a periodic sweep (~10m).
5. **UI** reads ACL-scoped API; **SSE** pushes events filtered by `authz.CanAccessRepo`.

## Integrity rules (high level)

- Upserts COALESCE nil timestamps
- PRs reject older `updated_at`
- Runs accept greater `run_attempt` or same attempt with non-regressing status
- Open-PR sync closes numbers absent from the open list
- Soft-delete only rows with `last_synced_at` before the sync start
- Empty repo sync does **not** soft-delete the catalog (zero-result reconcile is a no-op for deletes)
- Webhook `processing` reaper (~5m)

## Realtime and metrics

- **SSE** at `/api/v1/events` (not WebSockets; no Redis)
- **Prometheus** at `/metrics` — session cookie or optional Bearer `GITSEER_METRICS_TOKEN`; labels are status/event only (never repo names)

## Proxy / subpath

Set `server.external_url` to the public URL **including** any path prefix. Lens strips only that configured `PathPrefix()`. Client `X-Forwarded-Prefix` is ignored.

## Design constraints (current slice)

- Dual-forge: **Gitea + GitHub** in scope; **GitLab / Bitbucket** Coming Soon (ask before implementing clients)
- No Redis
- Read-first (rerun/cancel deferred)
- Capability detection for Actions APIs; degrade when missing
- Login: bootstrap + Gitea OAuth; GitHub service PAT for sync (no GitHub OAuth login yet)
