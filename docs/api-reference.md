# API Reference

Base path: `/api/v1` unless noted. JSON request/response. Session cookie auth unless noted. State-changing routes need `X-CSRF-Token` matching `lens_csrf`.

## Auth

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| GET | `/auth/login` | — | Start OAuth (rate limited) |
| GET | `/auth/callback` | — | OAuth callback |
| POST | `/auth/bootstrap/login` | CSRF | Bootstrap password |
| POST | `/auth/logout` | CSRF | End session |
| GET | `/auth/me` | session | Current user (+ optional theme) |

## System and settings

| Method | Path | Notes |
|--------|------|-------|
| GET | `/system/status` | Version / health-ish status |
| GET | `/settings` | Preferences + integration (`*_configured` for secrets) |
| PUT | `/settings` | Bootstrap admin + CSRF; live-apply |

## Setup

| Method | Path |
|--------|------|
| POST | `/setup/check-gitea-url` |
| POST | `/setup/test-connection` |
| POST | `/setup/create-webhook` |
| POST | `/setup/create-oauth` |
| POST | `/setup/complete` |
| POST | `/setup/sync-repos` |

## Data (ACL-scoped)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/summary?days=` | Allowlist 1/7/30/90; default 7 |
| GET | `/stats?days=` | Same allowlist; day series field `day` |
| GET | `/attention` | Attention items |
| GET | `/repositories` | `q`, `limit` |
| GET | `/repositories/{owner}/{repo}` | One repo |
| GET | `/pull-requests` | e.g. `state=open` |
| GET | `/workflow-runs` | Run list |
| GET | `/workflow-runs/{id}` | Run + jobs + graph |
| GET | `/jobs/{id}` | Job |
| GET | `/jobs/{id}/logs` | On-demand logs |
| GET | `/search` | Cross search |
| GET | `/events` | SSE stream |

## Webhooks (outside `/api/v1`)

| Method | Path |
|--------|------|
| POST | `/api/webhooks/gitea` |
| POST | `/api/webhooks/gitea/{instanceID}` |

HMAC-verified; rate limited.

## Metrics

| Method | Path | Notes |
|--------|------|-------|
| GET | `/metrics` | Prometheus; **requires session** |

Series include repository/PR/run gauges, webhook counters, sync histograms, and Gitea API counters. Labels are status/event oriented — never repository names.

## UI config

The SPA also loads `/api/v1/ui-config` for CSRF issuance and client bootstrap (see `web/src/api/client.ts`).
