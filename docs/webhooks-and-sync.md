# Webhooks and Sync

## Webhooks

| Method | Path | Notes |
|--------|------|-------|
| POST | `/api/webhooks/gitea` | Legacy unscoped; HMAC against the [primary Gitea instance](authentication-and-acl.md#primary-gitea-instance) |
| POST | `/api/webhooks/gitea/{instanceID}` | Per-instance Gitea HMAC (`X-Gitea-Signature`) |
| POST | `/api/webhooks/github/{instanceID}` | Per-instance GitHub HMAC (`X-Hub-Signature-256`) |

- Fail closed when a forge URL is set in config and that forge’s webhook secret is empty (unless unsigned allowed for that forge — lab only)
- Per-instance routes use the secret stored on that `instances` row (never inherit the other forge’s global secret)
- In-process rate limit: 120 POSTs / IP / minute
- Events update runs, jobs, PRs (including review state), commit status / checks → PR `ci_state`, and trigger attention evaluation
- Stuck `processing` webhook rows are reaped (~5 minutes)

Recommended: create the forge webhook during the [Setup Wizard](setup-wizard.md) (or Settings → Integration), pointing at the per-instance URL shown in the UI, with the same secret GitSeer stores.

### Gitea

System or repository webhook → `{external_url}/api/webhooks/gitea/{instanceID}` (or legacy unscoped `/api/webhooks/gitea` for the primary instance). Events: `pull_request`, `pull_request_review`, `workflow_run`, `workflow_job`, `repository`, `status`.

### GitHub

Organization or repository webhook → `{external_url}/api/webhooks/github/{instanceID}`. Content type JSON; secret must match the instance webhook secret. Events: `pull_request`, `pull_request_review`, `workflow_run`, `workflow_job`, `repository`, `check_run`, `check_suite`, `status`. Startup requires `GITSEER_GITHUB_WEBHOOK_SECRET` (or file) whenever `github.url` is set unless unsigned webhooks are explicitly allowed.

## Sync / reconcile

- Periodic reconcile interval: `sync.reconcile_interval` (default **5m**)
- History window: `sync.history_days` (default **30**; editable in Settings)
- Discovers repositories visible to each instance’s service token
- Imports open PRs and recent workflow runs/jobs
- PR CI state from combined commit status (Gitea) / status+Checks API (GitHub), with fallback from indexed runs
- PR review state from forge review APIs on sync and `pull_request_review` webhooks
- Soft-delete only for rows older than the sync start; **empty catalog results do not wipe repos**
- Manual trigger: **Sync now** → `POST /api/v1/setup/sync-repos` (bootstrap admin + CSRF)

### Sync leases

Manual Sync and background reconcile both take a per-`instance_id` lease (`sync_leases`). Lease TTL matches `sync.reconcile_interval`. If another holder already has the lease, that instance’s sync is skipped until it expires. Leases are in-process/DB-backed — suitable for a single replica (`replicaCount: 1` on SQLite).

## Consistency model

Near-real-time via webhooks + eventual consistency via reconcile. Each forge remains authoritative; GitSeer stores an operational index across instances.
