# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added

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
