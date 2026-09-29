# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added

- **Multi-arch release binaries:** GitHub Releases publish linux/darwin/windows × amd64/arm64 (plus `SHA256SUMS`); README download snippet auto-detects OS/arch.
- **Bootstrap claim:** first UI visit sets username + password (`POST /auth/bootstrap/claim`); no install-time password. Install chooses suggested username and keep/remove break-glass after setup.
- **Become Bootstrap:** `POST /auth/bootstrap/elevate` grants 5-minute admin elevation; Settings **Bootstrap** tab resets password during the grant (`POST /auth/bootstrap/password`).
- Migration `00019_bootstrap_claim` (password hash / username / keep-after-setup + `sessions.bootstrap_elevated_until`).

### Changed

- Bootstrap password stored as bcrypt in `app_settings`; `GITSEER_AUTH_BOOTSTRAP_PASSWORD` is legacy seed only.
- `ui-config` no longer returns `dev_bootstrap_password`; exposes `bootstrap_unclaimed` / `bootstrap_username` / `bootstrap_login_enabled`.
- README quick-start examples use Docker; container images remain linux/amd64.

## [1.0.3] - 2026-09-28

### Added

- **Browser & OS alerts** (self-hosted): SSE `attention` events drive the Web Notification API while a tab is connected; Web Push + service worker (`sw.js`) cover closed tabs. Per-user prefs and subscriptions (`GET/PUT /api/v1/alerts/prefs`, `POST /api/v1/alerts/push/{subscribe,unsubscribe,test}`). VAPID keys auto-persist beside the DB (`gitseer.vapid.json`) or via `notifications.vapid_*` / `GITSEER_VAPID_*`. No ncdLabs notification relay (ADR-032 amended).
- Migration `00018_web_push` (`user_alert_prefs`, `web_push_subscriptions`).

### Changed

- Attention newly opened/reopened always publishes SSE (and optional Web Push) even when outbound SMTP/webhook channels are disabled.

## [1.0.2] - 2026-09-27

### Fixed

- Bitbucket OAuth now hashes user IDs with `bitbucket.StableID` (FNV-64a), matching sync/webhooks. Next login remaps any stale 1.0.1 OAuth identity hash in place.

## [1.0.1] - 2026-09-27

### Added

- Dual-forge feature completeness: PR `review_state` from forge review APIs + `pull_request_review` webhooks; GitHub Checks/status merge into `ci_state`; status/check webhook apply.
- Repository detail route `/repositories/:owner/:repo` (optional `instance_id`) for install-ui deep links; `gitseer install-ui --instance-id`.
- Prometheus `gitseer_github_api_requests_total` / `gitseer_github_api_errors_total` (parity with Gitea).
- Review badges on the Pull Requests list; repository inventory links to the in-app detail page.
- Setup create-webhook prefers per-instance Gitea delivery URLs; Gitea webhook event set includes `status` and `pull_request_review`.
- Failed webhook applies retry with backoff (up to 5 attempts).

### Fixed

- Orphan reconcile treats only forge **404** as gone (403/other errors no longer cancel Active Actions rows); orphan run+jobs close in one transaction.
- OAuth ACL refresh no longer grants GitHub inventory from the service PAT to ordinary users; prior GitHub ACL rows are cleared on refresh (bootstrap admins unchanged).
- `GET /api/v1/ui-config` exposes `dev_bootstrap_password` only when skip-setup is on, `external_url` is loopback, **and** the TCP peer is loopback.
- Bootstrap login / `bootstrap_enabled` disabled after setup completes (unless `GITSEER_ALLOW_SKIP_SETUP`).
- When `GITSEER_METRICS_TOKEN` is set, `/metrics` accepts Bearer only (session cookie no longer bypasses).
- Non-admin `forges[]` / system status omit `allow_private_network` / `allow_unsigned`.
- Stored secrets fail closed without `GITSEER_ENCRYPTION_KEY` (no plaintext fallback).
- OAuth begin/callback bind PKCE state to an HttpOnly cookie.
- Ambiguous `owner/repo` lookup checks ACL before returning 409 (no existence leak).
- Empty `server.external_url` defaults session cookies to `Secure` (explicit `auth.cookie_secure` still wins).
- Full sync closes in-flight workflow runs that 404 on the forge (missed webhook / purged history ghosts in Active Actions), including incomplete jobs.

- Per-instance webhook routes no longer inherit the legacy global Gitea HMAC secret or global allow-unsigned (closes fail-open for GitHub/secondary forges).
- Instance upsert by `base_url` refuses to change `forge_type`; creates use insert-only (no race clobber).
- OAuth ACL refresh is scoped to the user’s Gitea `instance_id`; orphaned users re-bind after instance delete.
- `GET /repositories/{owner}/{repo}` returns 409 when owner/name is ambiguous across instances unless `instance_id` is set.
- Job logs and workflow YAML use the repository’s forge instance client (not always primary Gitea).
- Manual Sync acquires the same per-instance sync leases as background reconcile.
- Instance `forge_type` is immutable after create; SSRF/unsigned flags use optional JSON booleans.
- `GET /instances` is bootstrap-admin only; non-admin settings redact OAuth client id / allow flags.
- Bootstrap password is only prefilled in ui-config on loopback hosts (not `*.local`).
- Wizard/API new encryption keys require ≥24 characters (env load still accepts ≥16); ambiguous base64 blobs fail closed without a key.
- Webhook processing reaper ages from `processing_started_at` (claim time), not `received_at`, so backlogged events are not double-applied.
- Sync soft-delete clears repository ACL; `CanAccessRepo` rejects soft-deleted repos (including bootstrap).
- Workflow/job re-runs clear stale `completed_at` when attempt increases or status returns to in-flight; SQL `ON CONFLICT … WHERE` guards regressing updates.
- Setup wizard keeps the Finish step visible when post-setup sync fails (no silent exit).
- `safeExternalHref` rejects protocol-relative `//…` URLs.
- Header search calls `/api/v1/search` and shows results.
- `POST /api/webhooks/gitea/{instanceID}` routes to that instance.
- Workflow graph YAML fetch uses the caller's forge token (same as logs).
- SSE invalidation is typed by event; Active Actions polls every 15s as fallback; drop counter `gitseer_sse_events_dropped_total`.
- Helm mounts `GITSEER_WEBHOOK_SECRET` as required (parity with config fail-closed).

### Changed

- Setup wizard first-step label **Secure** → **Prepare** (internal step id remains `secure`); wizard step/modal copy tightened for what+why clarity.
- Attention sweep paginates through all open PRs and queued/waiting/running runs.
- Sync always fetches jobs for in-flight runs; completed-run job fetch cap raised to 50/repo.
- Postgres webhook claim uses `FOR UPDATE SKIP LOCKED`.
- Removed unused `GITSEER_AUTH_PROVIDER` from k3s values; dead store/API helpers cleaned up.

### Added (setup / metrics)

- Setup wizard **Prepare** step (first): public GitSeer URL plus generate/paste encryption key; key persists beside the DB when env/config unset (`GET`/`POST /api/v1/setup/encryption`).
- Optional Prometheus Bearer scrape via `server.metrics_token` / `GITSEER_METRICS_TOKEN` (session cookie still works).
- Sync package tests for empty-catalog soft-delete skip, missing-repo soft-delete, and sync lease gating.
- Crypto unit tests and fail-closed decrypt when sealed secrets exist without `GITSEER_ENCRYPTION_KEY`.

### Changed (prior)

- Job logs fetch with the caller's Gitea OAuth token; bootstrap admins keep using the service token.
- Helm requires `GITSEER_ENCRYPTION_KEY` in the cluster Secret (no longer optional).
- OAuth token persist failures are logged (session still created).
- HTTP server sets `IdleTimeout` (120s); `WriteTimeout` left unset for SSE and log streaming.
- Webhook processor logs `MarkWebhookProcessed` failures.

### Added (prior)

- Discrete attention matrix (PRD §10 severities `critical` / `warning` / `waiting`) with periodic sweep; `attention.long_running_after`.
- CSRF double-submit (`gitseer_csrf` + `X-CSRF-Token`) on browser state-changing `/api/v1` POSTs; token via `/api/v1/ui-config` and `/auth/me`.
- Periodic ACL refresh (`auth.acl_refresh_interval`, default 6h); repository webhook ACL invalidation.
- Named Prometheus series (PRD §41) under `/metrics` (auth required).
- In-process IP rate limits on bootstrap login, OAuth start, and webhook POST.
- Webhook processing reaper for stuck `processing` rows; open-PR reconcile closes absent numbers.

### Changed

- Webhook HMAC fail-closed when `gitea.url` is set unless `gitea.allow_unsigned_webhooks` / `GITSEER_WEBHOOK_ALLOW_UNSIGNED=true`.
- Timestamp-preserving upserts + out-of-order PR/run guards; soft-delete respects sync-start stamp.
- Proxy prefix strips only `server.external_url` path (ignores client `X-Forwarded-Prefix`).
- Compose no longer defaults bootstrap password to `changeme`.
- Unreadable `*_FILE` env secret paths fail load instead of silently clearing.
- Store list helpers refuse unscoped queries when `UserID<=0` and not `BootstrapAll`.
- Postgres-safe `RETURNING id` for bootstrap user + webhook insert (no `LastInsertId`).

### Added (earlier)

- Initial MVP scaffold: sync, webhooks, bootstrap auth, authz-scoped API, React UI, attention rules, workflow DAG parser, install-ui, compose packaging.
