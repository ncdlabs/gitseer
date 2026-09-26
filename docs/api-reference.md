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
| GET | `/system/status` | session | Version / health-ish status; `forges[]` one row per instance |
| GET | `/settings` | session | Preferences + dual-forge snapshot (`*_configured`, `encryption_configured`, embedded status) |
| PUT | `/settings` | bootstrap admin + CSRF | Live-apply preferences / snapshot |
| GET | `/instances` | bootstrap admin | List forge instances (public fields; **403** for non-admins) |
| POST | `/instances` | bootstrap admin + CSRF | Create instance (insert; cannot flip `forge_type` later) |
| PUT | `/instances/{id}` | bootstrap admin + CSRF | Update instance (leave-blank secrets; optional `clear_*`) |
| DELETE | `/instances/{id}` | bootstrap admin + CSRF | Delete instance (cascades inventory) |

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
| GET | `/attention` | `q`, `forge_type`, `instance_id`, severity, type, limit |
| GET | `/repositories` | `q`, `forge_type`, `instance_id`, `limit` |
| GET | `/repositories/{owner}/{repo}` | One repo; pass `instance_id` when the same owner/name exists on multiple forges (**409** otherwise) |
| GET | `/pull-requests` | e.g. `state=open`; `forge_type`, `instance_id` |
| GET | `/workflow-runs` | Run list; `forge_type`, `instance_id` |
| GET | `/workflow-runs/active` | In-flight runs (`queued`/`waiting`/`running`) with jobs |
| GET | `/workflow-runs/{id}` | Run + jobs + graph |
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
| GET | `/metrics` | Prometheus; **requires session or Bearer metrics token** |

Series include repository/PR/run gauges, webhook counters, sync histograms, and Gitea API counters (`gitseer_gitea_api_*`). Labels are status/event oriented — never repository names. GitHub API request counters are not exported yet.

## UI config

The SPA also loads `/api/v1/ui-config` for CSRF issuance and client bootstrap (see `web/src/api/client.ts`). Fields: `base_path`, `oauth_enabled`, `bootstrap_enabled`, `allow_skip_setup` (true only when `dev.allow_skip_setup` / `GITSEER_ALLOW_SKIP_SETUP` is set — local `npm run start`), `dev_bootstrap_password` (loopback hosts only when skip-setup is on — login prefill), `csrf_token`.
