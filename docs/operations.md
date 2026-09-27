# Operations

## Health and status

- Unauthenticated: `GET /health/live` (process), `GET /health/ready` (DB)
- Authenticated: `GET /api/v1/system/status` (`forges[]` per instance with ops checklist, sync phase, lease holder, webhook 24h stats, capability matrix, encryption health)
- Attention mutes/snoozes and severity overrides: see [Attention Engine](attention-engine.md) (`POST /api/v1/attention/{id}/mute`, Settings → Preferences → **Attention Severity Overrides**)
- Outbound notifications: Settings → **Notifications** (SMTP / Slack / Discord / generic HTTPS). Immediate alerts on attention open (min severity, default critical) and optional daily digest at `digest_hour_utc`. Delivered via `notification_outbox` worker; secrets sealed with encryption key. Self-hosted only — no ncdLabs relay. **Send Test Notification** queues one message per enabled channel.
- Settings → **Status**: pass/warn/fail checklist, **Ensure Webhook**, **Verify Delivery**, **Sync Now**
- UI header **Sync Now** for on-demand reconcile (bootstrap admin; per-instance sync leases — skips if another holder holds the lease)

### Webhook ensure / verify

| Method | Path | Notes |
|--------|------|-------|
| POST | `/api/v1/instances/{id}/ensure-webhook` | Bootstrap admin + CSRF. Gitea: system hook. GitHub: org hook when PAT has `admin:org_hook` (optional body `{ "org", "repo" }`); otherwise returns manual preview. |
| POST | `/api/v1/instances/{id}/verify-webhook` | Arms pending verification; next accepted delivery marks `webhook_verified_at`. Body `{ "confirm": true }` after a forge Ping if deliveries landed in the last 15 minutes. |

### Encryption health

Status exposes `encryption_configured`, `encryption_source`, `encryption_healthy`, and `encryption_error` (no plaintext). A decrypt probe uses one stored secret ciphertext when present, otherwise a seal/open canary. Wrong-key rotation shows as unhealthy until secrets are re-sealed.

**Rotation (CLI):** re-seal all DB secrets under a new key without re-entering each secret:

```bash
# Back up first
gitseer backup --out /path/to/backup

# Option A: provide a new passphrase
gitseer rotate-encryption-key --new-key 'your-new-high-entropy-passphrase'

# Option B: read passphrase from a file
gitseer rotate-encryption-key --new-key-file /secure/new.key

# Option C: generate one (printed once; store it securely)
gitseer rotate-encryption-key --generate
```

Behavior:

- Decrypts every sealed field (`instances`, `user_tokens`, `app_settings`, `notification_settings`) with the current key and re-seals under the new key in one transaction (**fail closed** — any decrypt/encrypt error rolls back with no writes).
- When the active key came from the on-disk `gitseer.encryption_key` file, replaces that file (0600) after a successful DB commit.
- When the key comes from `GITSEER_ENCRYPTION_KEY` / config, the CLI prints a note to update that secret and restart; the on-disk file is left alone.
- Confirm Status shows `encryption_healthy` after restart (or live apply when the file source is used).

**Manual fallback:** set the new key in env/Secret, re-enter forge tokens/webhook/OAuth secrets in Settings (or wizard), confirm Status encryption is healthy, then retire the old key.

## Metrics

Scrape `GET /metrics`:

- When `server.metrics_token` / `GITSEER_METRICS_TOKEN` is **unset**: session cookie (same auth as the API)
- When the scrape token **is** set: **Bearer-only** (`Authorization: Bearer <token>`) — session cookies are rejected (preferred for Prometheus)

Includes gauges for repos/PRs/runs, webhook counters, sync duration/errors, and Gitea API request/error counters (`gitseer_gitea_api_*`). Labels avoid repository names.

## Retention

Scheduled purge (~6h) removes aged:

- workflow runs (`retention.runs_days`, default 90)
- webhook payloads (`retention.webhooks_days`, default 30)
- resolved attention (`retention.attention_days`, default 180)

Editable in Settings → Preferences / env / config.

**Storage guardrails:** `GET /api/v1/system/status` (and Settings → Status) include `storage` with SQLite file size (main + WAL/SHM) or a Postgres `pg_database_size` estimate, plus warn (512 MiB) / critical (2 GiB) thresholds. Status surfaces a warning when size crosses those levels.

**Purge Now:** bootstrap admin + CSRF `POST /api/v1/admin/purge-retention` runs an immediate purge with the currently saved retention windows (Settings → Preferences button).

**Lab / Prod presets:** Preferences buttons **Apply Lab Preset** (history 7d · runs 14d · webhooks 7d · attention 30d) and **Apply Prod Preset** (30 / 90 / 30 / 180) fill the form; Save Changes to persist.

## Backup / restore

CLI (preferred over copying DB files by hand):

```bash
gitseer backup --out /path/to/backup [--config config.yaml]
gitseer restore --from /path/to/backup [--config config.yaml] [--force]
```

- **SQLite:** `VACUUM INTO` snapshot of `database.path`, plus `gitseer.encryption_key` when present, and `manifest.json`
- **Postgres:** writes a `pg_dump` command file; runs `pg_dump --format=custom` when `pg_dump` is on `PATH`. Restore uses `pg_restore` with `--force`
- Restore refuses to overwrite an existing DB or key file without `--force`
- Always back up `config.yaml` / Kubernetes Secret / `.env` separately — never commit secrets

### Helm CronJob example (optional)

Not shipped by default. Example pattern: mount the data PVC + Secret, run the backup binary, push the directory to object storage or a hostPath:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: gitseer-backup
  namespace: gitseer
spec:
  schedule: "0 3 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: backup
              image: git.ncdlabs.com/ncdlabs/gitseer:0.1.27
              command: ["gitseer", "backup", "--out", "/backup/$(date +%Y%m%d)"]
              envFrom:
                - secretRef:
                    name: gitseer
              volumeMounts:
                - name: data
                  mountPath: /data
                - name: backup
                  mountPath: /backup
          volumes:
            - name: data
              persistentVolumeClaim:
                claimName: gitseer-data
            - name: backup
              emptyDir: {}
```

Adapt image tag, PVC name, and offload the `/backup` tree (e.g. `rclone` sidecar) for your cluster.

## Logs

Application logs: JSON or text per `log.format` / `GITSEER_LOG_FORMAT`.

Job logs: fetched from the forge on demand through GitSeer (`/api/v1/jobs/{id}/logs`), not stored long-term as the primary log archive. Gitea OAuth users use their stored token; GitHub repos (and bootstrap) use the instance service PAT.

## Write ops (rerun / cancel)

- `POST /api/v1/workflow-runs/{id}/rerun` and `.../cancel` (CSRF + session + `CanAccessRepo`)
- Non-admin: forge call uses `UserAccessTokenForInstance` for that repo’s instance (403 if missing) — never silent service-PAT fallback; never cross-forge token reuse (legacy Gitea-only fallback when instance-scoped row is missing)
- Bootstrap admin: may use the instance service PAT; UI warns before confirm
- GitHub non-admin: uses per-user GitHub OAuth token when linked; otherwise 403
- Capability matrix rows: **Rerun Workflow** / **Cancel Workflow**; forge 404/405 → 501 unsupported
- Success publishes an SSE `workflow_run` event so Active Actions / detail refetch

## Upgrades

1. Backup DB + secrets (+ encryption key file if used) via `gitseer backup`  
2. Ship new image/binary  
3. Run migrations automatically on start (goose)  
4. Smoke: login, summary API, webhook delivery, sync, Status checklist healthy  

## Security ops notes

- Rotate webhook secrets in the forge and GitSeer together (per instance)  
- Rotate OAuth client secret via Settings (`clear_*` / replace)  
- Prefer empty bootstrap password once OAuth admins exist  
- Keep `allow_unsigned_webhooks` off outside labs (Gitea and GitHub flags are separate)  
- Removing a forge instance deletes cascaded inventory — confirm before delete  
- **Wallboard tokens** (Settings → Status): bearer or `?token=` grants **read-only** access to summary + open attention (`GET /api/v1/wallboard/snapshot`) — no writes, settings, forge secrets, or job logs. Treat like a shared TV credential: create narrowly named tokens, revoke on leak, prefer Bearer over query-string (query tokens may appear in access logs / Referer). The wallboard UI stores the token in **localStorage** deliberately for shared-display kiosks (CSP `script-src 'self'`). Snapshot uses bootstrap-wide inventory scope (not per-user ACL). 
