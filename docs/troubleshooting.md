# Troubleshooting

## Cannot reach private/lab forge

SSRF guard fails closed on DNS errors and checks dial-time private IPs. Set:

```bash
export GITSEER_GITEA_ALLOW_PRIVATE_NETWORK=true
# and/or for GitHub Enterprise:
export GITSEER_GITHUB_ALLOW_PRIVATE_NETWORK=true
```

(or the matching YAML / Settings flags).

## Startup fails: webhook secret required

When `gitea.url` / `GITSEER_GITEA_URL` is set, provide `GITSEER_WEBHOOK_SECRET` (or file). Lab-only escape hatch:

```bash
export GITSEER_WEBHOOK_ALLOW_UNSIGNED=true
```

When `github.url` / `GITSEER_GITHUB_URL` is set, provide `GITSEER_GITHUB_WEBHOOK_SECRET` (or file). Lab-only:

```bash
export GITSEER_GITHUB_ALLOW_UNSIGNED_WEBHOOKS=true
```

## Encryption key rejected in Prepare

Env/config accept keys ≥**16** characters. The setup wizard **Save Key** path requires ≥**24**. Use **Generate Key**, or paste a longer passphrase. See [Configuration](configuration.md#encryption-key).

## OAuth redirect mismatch

`server.external_url` must match the URL in the browser and the OAuth app redirect for that forge:

```text
{external_url}/api/v1/auth/callback                 # Gitea
{external_url}/api/v1/auth/forgejo/callback
{external_url}/api/v1/auth/github/callback
{external_url}/api/v1/auth/gitlab/callback
{external_url}/api/v1/auth/bitbucket/callback
```

Subpath deploys must include the path in `external_url`. Client `X-Forwarded-Prefix` is ignored. With multiple Gitea instances, Gitea OAuth uses the [primary Gitea instance](authentication-and-acl.md#primary-gitea-instance).

## Empty UI / no repositories

1. Confirm each instance’s service token can list repos  
2. Click **Sync Now** after first admin login (bootstrap admin)  
3. If sync appears to no-op, another holder may hold the per-instance sync lease — wait one reconcile interval  
4. Non-bootstrap users need ACL rows (forge OAuth login + ACL refresh for instances with a stored user token; sync does not grant ACL)

## CSRF failures on Save / Sync

Ensure the SPA received a CSRF token (`/api/v1/ui-config` or `/auth/me`) and sends `X-CSRF-Token`. Hard-refresh after login if the cookie/header pair is stale. Instance and settings writes are bootstrap-admin only.

## Bootstrap login unavailable

Empty `GITSEER_AUTH_BOOTSTRAP_PASSWORD` disables bootstrap auth. Set a password or use forge OAuth.

## Integration list empty / 403

`GET /api/v1/instances` is bootstrap-admin only. Non-admins use Settings → Status / `GET /settings` forge summary.

## Repository detail conflict

`GET /repositories/{owner}/{repo}` returns **409** when the same owner/name exists on more than one instance — pass `instance_id`.

## Embed / blank production UI

`make frontend` must populate `internal/server/ui/dist` before `make build-go` / image build.

## Actions APIs missing

GitSeer detects capabilities and degrades when Actions endpoints return 404 (Gitea and GitHub/GHE variance). Job/run JSON shapes vary; the client accepts wrapped or flat arrays where needed. Settings → **Status** shows a per-instance **Capability Matrix** (Actions, logs, runners, workflow webhooks, Checks vs status). The **Runners API** row notes that `runner_unavailable_queued` attention opens only on a positive job conclusion/message/label/steps signal — ordinary queued jobs do not alert, and many forges omit an offline-runner signal entirely.

## Encryption unhealthy / wrong key

Settings → **Status** shows `encryption_healthy: false` with `encryption_error` when a stored secret cannot be decrypted. Fix by restoring the correct `GITSEER_ENCRYPTION_KEY` / `gitseer.encryption_key`, or re-enter forge secrets after installing a new key. Never commit keys.

## Webhooks not arriving

1. Confirm per-instance delivery URL under Integration / Status  
2. Use **Ensure Webhook** (Gitea/Forgejo system hook; GitHub org/repo when PAT allows; GitLab/Bitbucket often return a manual preview)  
3. Use **Verify Delivery** then send a Ping from the forge (or **Confirm Delivery** after a recent delivery)  
4. Check webhook 24h stats and last error on Status  

## Empty Active Actions

Status surfaces an `active_actions_hint` when there are no in-flight runs and few/no recent `workflow_*` webhooks — usually missing workflow events on the forge hook, or sync not completed yet.

## Postgres / multi-replica

Store paths use `RETURNING id` and `FOR UPDATE SKIP LOCKED` (webhook claim) on Postgres. Optional integration tests: `GITSEER_TEST_POSTGRES_DSN=… go test -tags postgres ./internal/store/ -run Postgres`.

**Multi-replica caveats:**

- Sync leases are per-`instance_id` in the DB — only one reconciler holds a given lease at a time (safe across replicas)
- SSE remains **in-process** (no Redis) — each replica has its own subscriber set; use sticky sessions or accept that clients only see events from the pod they connected to
- Prefer `replicaCount: 1` unless you accept those SSE caveats; set `replicaCount > 1` only with documented awareness

## Browser / OS alerts not appearing

1. Confirm Settings → **Notifications** → **Browser & OS Alerts** shows enabled and permission is **granted** (browser site settings).
2. Web Push needs a **secure context** (HTTPS or localhost). Plain HTTP on a LAN hostname will not register push.
3. Check logs for `generated web push VAPID keys` or `web push VAPID keys unavailable` on startup; file lives beside the DB (`gitseer.vapid.json`) unless `GITSEER_VAPID_*` overrides.
4. Open-tab banners use SSE (`attention` events) and only show when the document is **hidden**; focused tabs skip the OS banner to avoid duplicates. Closed-tab delivery needs an active push subscription (**Send Test Push**).
5. Severity filter on the prefs panel must be ≤ the attention item’s severity (default critical-only).

## Metrics 401

`GET /metrics` requires a logged-in session cookie or `Authorization: Bearer` when `GITSEER_METRICS_TOKEN` is configured — not an open scrape target.
