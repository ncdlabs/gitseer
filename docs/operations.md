# Operations

## Health and status

- Authenticated: `GET /api/v1/system/status`
- UI **Sync now** for an on-demand reconcile

## Metrics

Scrape `GET /metrics` with a session cookie (same auth as the API). Includes gauges for repos/PRs/runs, webhook counters, sync duration/errors, and Gitea API request/error counters. Labels avoid repository names.

## Retention

Scheduled purge (~6h) removes aged:

- workflow runs (`retention.runs_days`, default 90)
- webhook payloads (`retention.webhooks_days`, default 30)
- resolved attention (`retention.attention_days`, default 180)

Editable in Settings / env / config.

## Backup

- **SQLite:** copy `database.path` while writers are quiet (or use filesystem snapshot)
- **Postgres:** `pg_dump` the DSN database

Also back up `config.yaml` / secrets store (Kubernetes Secret, `.env`) — never commit secrets.

## Logs

Application logs: JSON or text per `log.format` / `LENS_LOG_FORMAT`.

Job logs: fetched from Gitea on demand through Lens (`/api/v1/jobs/{id}/logs`), not stored long-term as the primary log archive.

## Upgrades

1. Backup DB + secrets  
2. Ship new image/binary  
3. Run migrations automatically on start (goose)  
4. Smoke: login, summary API, webhook delivery, sync  

## Security ops notes

- Rotate webhook secret in Gitea and Lens together  
- Rotate OAuth client secret via Settings (`clear_*` / replace)  
- Prefer empty bootstrap password once OAuth admins exist  
- Keep `allow_unsigned_webhooks` off outside labs  
