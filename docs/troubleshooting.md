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

`server.external_url` must match the URL in the browser and the Gitea OAuth app redirect:

```text
{external_url}/api/v1/auth/callback
```

Subpath deploys must include the path in `external_url`. Client `X-Forwarded-Prefix` is ignored. With multiple Gitea instances, OAuth uses the [primary Gitea instance](authentication-and-acl.md#primary-gitea-instance).

## Empty UI / no repositories

1. Confirm each instance’s service token can list repos  
2. Click **Sync now** after first admin login (bootstrap admin)  
3. If sync appears to no-op, another holder may hold the per-instance sync lease — wait one reconcile interval  
4. Non-bootstrap users need ACL rows (Gitea login + ACL refresh; sync does not grant ACL)

## CSRF failures on Save / Sync

Ensure the SPA received a CSRF token (`/api/v1/ui-config` or `/auth/me`) and sends `X-CSRF-Token`. Hard-refresh after login if the cookie/header pair is stale. Instance and settings writes are bootstrap-admin only.

## Bootstrap login unavailable

Empty `GITSEER_AUTH_BOOTSTRAP_PASSWORD` disables bootstrap auth. Set a password or use Gitea OAuth.

## Integration list empty / 403

`GET /api/v1/instances` is bootstrap-admin only. Non-admins use Settings → Status / `GET /settings` forge summary.

## Repository detail conflict

`GET /repositories/{owner}/{repo}` returns **409** when the same owner/name exists on more than one instance — pass `instance_id`.

## Embed / blank production UI

`make frontend` must populate `internal/server/ui/dist` before `make build-go` / image build.

## Actions APIs missing

GitSeer detects capabilities and degrades when Actions endpoints return 404 (Gitea and GitHub/GHE variance). Job/run JSON shapes vary; the client accepts wrapped or flat arrays where needed.

## Postgres

Insert paths use `RETURNING id` for some flows; broader Postgres production readiness is still behind SQLite. Prefer SQLite unless you are intentionally validating Postgres. Keep a single replica when using SQLite sync leases.

## Metrics 401

`GET /metrics` requires a logged-in session cookie or `Authorization: Bearer` when `GITSEER_METRICS_TOKEN` is configured — not an open scrape target.
