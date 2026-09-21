# Webhooks and Sync

## Webhooks

Endpoint: `POST /api/webhooks/gitea` (optional `/{instanceID}`).

- HMAC verification against configured webhook secret
- Fail closed when Gitea URL is set and secret empty (unless unsigned allowed)
- In-process rate limit: 120 POSTs / IP / minute
- Events update runs, jobs, PRs, and trigger attention evaluation
- Stuck `processing` webhook rows are reaped (~5 minutes)

Recommended: create a **system** webhook in Gitea pointing at Lens during the [Setup Wizard](setup-wizard.md), or manually with the same secret Lens stores.

## Sync / reconcile

- Periodic reconcile interval: `sync.reconcile_interval` (default **5m**)
- History window: `sync.history_days` (default **30**; editable in Settings)
- Discovers repositories visible to the service token
- Imports open PRs and recent workflow runs/jobs
- PR CI state from combined commit status, with fallback from indexed runs
- Soft-delete only for rows older than the sync start; **empty catalog results do not wipe repos**
- Manual trigger: **Sync now** → `POST /api/v1/setup/sync-repos`

## Consistency model

Near-real-time via webhooks + eventual consistency via reconcile. Gitea remains authoritative; Lens stores an operational index.
