# Operations

## Health and status

- Unauthenticated: `GET /health/live` (process), `GET /health/ready` (DB)
- Authenticated: `GET /api/v1/system/status` (`forges[]` per instance)
- UI **Sync now** for an on-demand reconcile (bootstrap admin; per-instance sync leases — skips if another holder holds the lease)

## Metrics

Scrape `GET /metrics` with either:

- a session cookie (same auth as the API), or
- `Authorization: Bearer <token>` when `server.metrics_token` / `GITSEER_METRICS_TOKEN` is set (preferred for Prometheus)

Includes gauges for repos/PRs/runs, webhook counters, sync duration/errors, and Gitea API request/error counters (`gitseer_gitea_api_*`). Labels avoid repository names.

## Retention

Scheduled purge (~6h) removes aged:

- workflow runs (`retention.runs_days`, default 90)
- webhook payloads (`retention.webhooks_days`, default 30)
- resolved attention (`retention.attention_days`, default 180)

Editable in Settings / env / config.

## Backup

- **SQLite:** copy `database.path` while writers are quiet (or use filesystem snapshot)
- **Postgres:** `pg_dump` the DSN database
- **Encryption key file:** if the wizard wrote `gitseer.encryption_key` beside the DB, back it up with the database

Also back up `config.yaml` / secrets store (Kubernetes Secret, `.env`) — never commit secrets.

## Logs

Application logs: JSON or text per `log.format` / `GITSEER_LOG_FORMAT`.

Job logs: fetched from the forge on demand through GitSeer (`/api/v1/jobs/{id}/logs`), not stored long-term as the primary log archive. Gitea OAuth users use their stored token; GitHub repos (and bootstrap) use the instance service PAT.

## Upgrades

1. Backup DB + secrets (+ encryption key file if used)  
2. Ship new image/binary  
3. Run migrations automatically on start (goose)  
4. Smoke: login, summary API, webhook delivery, sync  

## Security ops notes

- Rotate webhook secrets in the forge and GitSeer together (per instance)  
- Rotate OAuth client secret via Settings (`clear_*` / replace)  
- Prefer empty bootstrap password once OAuth admins exist  
- Keep `allow_unsigned_webhooks` off outside labs (Gitea and GitHub flags are separate)  
- Removing a forge instance deletes cascaded inventory — confirm before delete  
