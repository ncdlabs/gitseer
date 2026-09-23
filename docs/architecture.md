# Architecture

## Shape

```text
┌─────────────┐     API + webhooks      ┌──────────────────────────────┐
│    Gitea    │ ◄──────────────────────► │  lens (Go single binary)     │
│  (SoR)      │                          │  + embedded React SPA       │
└─────────────┘                          │  SQLite (default) / Postgres│
                                         └──────────────────────────────┘
                                                    │
                         browser ── session cookie + CSRF ──┘
```

- **Binary:** `cmd/lens` (`serve`, `version`, `install-ui`, `uninstall-ui`)
- **UI:** Vite/React SPA built into `internal/server/ui/dist` and served by the same process
- **Module:** `github.com/ncdlabs/gitea-lens`
- **HTTP:** chi router; migrations via goose (SQL in `migrations/`)

## Package boundaries

| Package | Responsibility |
|---------|----------------|
| `internal/forge` (+ `gitea`) | Gitea HTTP client and type normalization |
| `internal/store` | Persistence (hand-written SQL) |
| `internal/sync` | Periodic reconcile + history import |
| `internal/webhooks` | Ingest + process Gitea events |
| `internal/api` | `/api/v1` handlers |
| `internal/auth` / `internal/authz` | Sessions, OAuth, CSRF, ACL |
| `internal/attention` | Attention rule engine |
| `internal/workflows` | Run/job graph helpers |
| `internal/realtime` | SSE hub (`/api/v1/events`) |
| `internal/settings` | DB-backed runtime settings |
| `internal/setup` | Setup wizard backend |
| `internal/metrics` | Prometheus series |
| `internal/ratelimit` | In-process per-IP limits |
| `internal/uiinstall` | Gitea custom template install |
| `internal/retention` | Aged data purge |

Raw Gitea wire types stay in `internal/forge/gitea`. Domain models live in `internal/models`.

## Data flow

1. **Bootstrap / Settings** configure Gitea URL, token, webhook secret, OAuth.
2. **Sync** discovers repositories, open PRs, recent workflow runs/jobs (history window configurable).
3. **Webhooks** apply near-real-time updates (`workflow_run`, `workflow_job`, PR events, etc.).
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
- **Prometheus** at `/metrics` — session cookie or optional Bearer `LENS_METRICS_TOKEN`; labels are status/event only (never repo names)

## Proxy / subpath

Set `server.external_url` to the public URL **including** any path prefix. Lens strips only that configured `PathPrefix()`. Client `X-Forwarded-Prefix` is ignored.

## Design constraints (V1)

- Gitea-only forge
- No Redis
- Read-first (rerun/cancel deferred)
- Capability detection for Actions APIs; degrade when missing
