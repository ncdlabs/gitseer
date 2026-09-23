# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Fixed

- Webhook processing reaper ages from `processing_started_at` (claim time), not `received_at`, so backlogged events are not double-applied.
- Sync soft-delete clears repository ACL; `CanAccessRepo` rejects soft-deleted repos (including bootstrap).
- Workflow/job re-runs clear stale `completed_at` when attempt increases or status returns to in-flight; SQL `ON CONFLICT … WHERE` guards regressing updates.
- Setup wizard keeps the Finish step visible when post-setup sync fails (no silent exit).
- `safeExternalHref` rejects protocol-relative `//…` URLs.
- Header search calls `/api/v1/search` and shows results.
- `POST /api/webhooks/gitea/{instanceID}` routes to that instance.
- Workflow graph YAML fetch uses the caller's forge token (same as logs).
- SSE invalidation is typed by event; Active Actions polls every 15s as fallback; drop counter `lens_sse_events_dropped_total`.
- Helm mounts `LENS_WEBHOOK_SECRET` as required (parity with config fail-closed).

### Changed

- Attention sweep paginates through all open PRs and queued/waiting/running runs.
- Sync always fetches jobs for in-flight runs; completed-run job fetch cap raised to 50/repo.
- Postgres webhook claim uses `FOR UPDATE SKIP LOCKED`.
- Removed unused `LENS_AUTH_PROVIDER` from k3s values; dead store/API helpers cleaned up.

### Added

- Optional Prometheus Bearer scrape via `server.metrics_token` / `LENS_METRICS_TOKEN` (session cookie still works).
- Sync package tests for empty-catalog soft-delete skip, missing-repo soft-delete, and sync lease gating.
- Crypto unit tests and fail-closed decrypt when sealed secrets exist without `LENS_ENCRYPTION_KEY`.

### Changed (prior)

- Job logs fetch with the caller's Gitea OAuth token; bootstrap admins keep using the service token.
- Helm requires `LENS_ENCRYPTION_KEY` in the cluster Secret (no longer optional).
- OAuth token persist failures are logged (session still created).
- HTTP server sets `IdleTimeout` (120s); `WriteTimeout` left unset for SSE and log streaming.
- Webhook processor logs `MarkWebhookProcessed` failures.

### Added (prior)

- Discrete attention matrix (PRD §10 severities `critical` / `warning` / `waiting`) with periodic sweep; `attention.long_running_after`.
- CSRF double-submit (`lens_csrf` + `X-CSRF-Token`) on browser state-changing `/api/v1` POSTs; token via `/api/v1/ui-config` and `/auth/me`.
- Periodic ACL refresh (`auth.acl_refresh_interval`, default 6h); repository webhook ACL invalidation.
- Named Prometheus series (PRD §41) under `/metrics` (auth required).
- In-process IP rate limits on bootstrap login, OAuth start, and webhook POST.
- Webhook processing reaper for stuck `processing` rows; open-PR reconcile closes absent numbers.

### Changed

- Webhook HMAC fail-closed when `gitea.url` is set unless `gitea.allow_unsigned_webhooks` / `LENS_WEBHOOK_ALLOW_UNSIGNED=true`.
- Timestamp-preserving upserts + out-of-order PR/run guards; soft-delete respects sync-start stamp.
- Proxy prefix strips only `server.external_url` path (ignores client `X-Forwarded-Prefix`).
- Compose no longer defaults bootstrap password to `changeme`.
- Unreadable `*_FILE` env secret paths fail load instead of silently clearing.
- Store list helpers refuse unscoped queries when `UserID<=0` and not `BootstrapAll`.
- Postgres-safe `RETURNING id` for bootstrap user + webhook insert (no `LastInsertId`).

### Added (earlier)

- Initial MVP scaffold: sync, webhooks, bootstrap auth, authz-scoped API, React UI, attention rules, workflow DAG parser, install-ui, compose packaging.
