# API Reference

Base path: `/api/v1` unless noted. JSON request/response. Session cookie auth unless noted. State-changing routes need `X-CSRF-Token` matching `gitseer_csrf`.

**Bootstrap admin** = the bootstrap password user (`IsBootstrapAdmin`). Several setup/settings/instance routes require it even when a session exists.

## Health (outside `/api/v1`)

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| GET | `/health/live` | — | Process up |
| GET | `/health/ready` | — | Ready when DB is reachable |

## Auth

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| GET | `/auth/login` | — | Start OAuth (rate limited) |
| GET | `/auth/callback` | — | OAuth callback |
| POST | `/auth/bootstrap/login` | CSRF | Bootstrap password |
| POST | `/auth/logout` | CSRF | End session |
| GET | `/auth/me` | session | Current user (+ optional theme) |

## System and settings

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| GET | `/system/status` | session | Version / ops status; `forges[]` with checklist, sync/lease/webhook stats, capability matrix; `encryption_healthy`; `storage` size + warn thresholds |
| GET | `/settings` | session | Preferences + dual-forge snapshot (`*_configured`, `encryption_configured`, embedded status) |
| PUT | `/settings` | bootstrap admin + CSRF | Live-apply preferences / snapshot |
| GET | `/notifications/settings` | bootstrap admin | Outbound notification config (secrets as `*_configured` only) |
| PUT | `/notifications/settings` | bootstrap admin + CSRF | Update channels / severity / digest; seal webhook/SMTP secrets |
| POST | `/notifications/test` | bootstrap admin + CSRF | Enqueue test on every enabled channel |
| GET | `/instances` | bootstrap admin | List forge instances (public fields; **403** for non-admins) |
| POST | `/instances` | bootstrap admin + CSRF | Create instance (insert; cannot flip `forge_type` later) |
| PUT | `/instances/{id}` | bootstrap admin + CSRF | Update instance (leave-blank secrets; optional `clear_*`) |
| DELETE | `/instances/{id}` | bootstrap admin + CSRF | Delete instance (cascades inventory) |
| POST | `/instances/{id}/ensure-webhook` | bootstrap admin + CSRF | Create/update forge webhook (Gitea system hook; GitHub org/repo when PAT allows) |
| POST | `/instances/{id}/verify-webhook` | bootstrap admin + CSRF | Arm delivery verification or `{ "confirm": true }` after a recent Ping |
| GET | `/instances/{id}/gitea-ui-snippets` | bootstrap admin | Download Gitea custom template snippets (`?format=zip` default or `text`); Gitea instances only |
| POST | `/admin/purge-retention` | bootstrap admin + CSRF | Immediate retention purge using saved windows; returns deleted row counts + storage snapshot |

Non-bootstrap users should use `GET /settings` / `GET /system/status` `forges[]` for read-only forge visibility — not `GET /instances`.

## Setup

All setup routes require **bootstrap admin**. Mutating `POST`s also require CSRF. `GET /setup/encryption` is auth-only (no CSRF).

| Method | Path |
|--------|------|
| POST | `/setup/encryption` |
| GET | `/setup/encryption` |
| POST | `/setup/check-gitea-url` |
| POST | `/setup/test-connection` |
| POST | `/setup/create-webhook` |
| POST | `/setup/create-oauth` |
| POST | `/setup/complete` |
| POST | `/setup/sync-repos` |

`POST /setup/complete` requires an encryption key and at least one forge (unless `GITSEER_ALLOW_SKIP_SETUP`).

## Data (ACL-scoped)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/summary?days=` | Allowlist 0/1/7/30/90 (`0` = Now snapshot); default 0 |
| GET | `/stats?days=` | Same allowlist; `0` = current-state breakdowns only; day series field `day` |
| GET | `/attention` | `q`, `forge_type`, `instance_id`, severity, type, limit; excludes muted fingerprints for the caller |
| POST | `/attention/{id}/mute` | CSRF; body `{ until: "24h"\|"7d"\|"resolved", reason? }` — bootstrap-admin = global mute |
| DELETE | `/attention/{id}/mute` | CSRF; clear mute(s) for that item’s fingerprint |
| GET | `/attention/{id}/log-snippet` | On-demand forge log tail for a failed job linked to the item (`max_bytes` cap, default 4KiB); not stored |
| GET | `/attention/rule-overrides` | Current overrides + default severities |
| PUT | `/attention/rule-overrides` | Bootstrap-admin + CSRF; replace overrides `{ overrides: [{ rule_type, severity }] }` |
| GET | `/inbox` | Personal queue: authored PRs / attention, requested reviewer when `requested_reviewers` is in attention metadata, failing CI on user’s PRs, `blocked_on_me` heuristic; `reason`, `q`, `forge_type`, `instance_id`, limit |
| GET | `/saved-filters` | Current user’s named filter presets |
| POST | `/saved-filters` | CSRF; body `{ name, query }` — `query` is a JSON object (e.g. `q`, `reason`, `page`) |
| PUT | `/saved-filters/{id}` | CSRF; update name and/or query |
| DELETE | `/saved-filters/{id}` | CSRF; delete preset |
| GET | `/repositories` | `q`, `forge_type`, `instance_id`, `limit`; each item includes computed `health` |
| GET | `/repositories/{owner}/{repo}` | One repo + `health`; pass `instance_id` when the same owner/name exists on multiple forges (**409** otherwise) |
| GET | `/repositories/{owner}/{repo}/health` | Health rollup only (`days`, `stale_days`) |
| GET | `/repositories/{owner}/{repo}/failure-clusters` | Failed jobs grouped by `(workflow_path, job_name)` over `days` (default 7) |
| GET | `/pull-requests` | e.g. `state=open`; `forge_type`, `instance_id` |
| GET | `/workflow-runs` | Run list; `forge_type`, `instance_id` |
| GET | `/workflow-runs/active` | In-flight runs (`queued`/`waiting`/`running`) with jobs |
| GET | `/workflow-runs/{id}` | Run + jobs + graph |
| POST | `/workflow-runs/{id}/rerun` | CSRF + ACL; Gitea OAuth users use user token; bootstrap admin may use service PAT (`used_service_pat`); GitHub non-admin **403** until OAuth |
| POST | `/workflow-runs/{id}/cancel` | CSRF + ACL; same token rules; **409** if run not in progress |
| GET | `/jobs/{id}` | Job |
| GET | `/jobs/{id}/logs` | On-demand logs (forge client for that repo’s instance) |
| GET | `/search` | Cross search: repos, orgs, PRs, workflow runs (incl. job-name hits), open attention; ACL-scoped |

| GET | `/events` | SSE stream |

## Webhooks (outside `/api/v1`)

| Method | Path |
|--------|------|
| POST | `/api/webhooks/gitea` |
| POST | `/api/webhooks/gitea/{instanceID}` |
| POST | `/api/webhooks/github/{instanceID}` |

HMAC-verified; rate limited. See [Webhooks and Sync](webhooks-and-sync.md).

## Metrics

| Method | Path | Notes |
|--------|------|-------|
| GET | `/metrics` | Prometheus; session when no scrape token; **Bearer-only** when `server.metrics_token` / `GITSEER_METRICS_TOKEN` is set |

Series include repository/PR/run gauges, webhook counters, sync histograms, and Gitea API counters (`gitseer_gitea_api_*`). Labels are status/event oriented — never repository names. GitHub API request counters are not exported yet.

## UI config

The SPA also loads `/api/v1/ui-config` for CSRF issuance and client bootstrap (see `web/src/api/client.ts`). Fields: `base_path`, `oauth_enabled`, `bootstrap_enabled` (false after setup unless skip-setup), `allow_skip_setup` (true only when `dev.allow_skip_setup` / `GITSEER_ALLOW_SKIP_SETUP` is set — local `npm run start`), `dev_bootstrap_password` (only when skip-setup is on, `external_url` is loopback, **and** the TCP peer is loopback), `csrf_token`.
