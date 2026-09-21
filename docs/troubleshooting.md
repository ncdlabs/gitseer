# Troubleshooting

## Cannot reach private/lab Gitea

SSRF guard fails closed on DNS errors and checks dial-time private IPs. Set:

```bash
export LENS_GITEA_ALLOW_PRIVATE_NETWORK=true
```

(or `gitea.allow_private_network: true` / Settings).

## Startup fails: webhook secret required

When `gitea.url` / `LENS_GITEA_URL` is set, provide `LENS_WEBHOOK_SECRET` (or file). Lab-only escape hatch:

```bash
export LENS_WEBHOOK_ALLOW_UNSIGNED=true
```

## OAuth redirect mismatch

`server.external_url` must match the URL in the browser and the Gitea OAuth app redirect:

```text
{external_url}/api/v1/auth/callback
```

Subpath deploys must include the path in `external_url`. Client `X-Forwarded-Prefix` is ignored.

## Empty UI / no repositories

1. Confirm service token can list repos  
2. Click **Sync now** after first admin login  
3. Non-bootstrap users need ACL rows (login + ACL refresh; sync does not grant ACL)

## CSRF failures on Save / Sync

Ensure the SPA received a CSRF token (`/api/v1/ui-config` or `/auth/me`) and sends `X-CSRF-Token`. Hard-refresh after login if the cookie/header pair is stale.

## Bootstrap login unavailable

Empty `LENS_AUTH_BOOTSTRAP_PASSWORD` disables bootstrap auth. Set a password or use OAuth.

## Embed / blank production UI

`make frontend` must populate `internal/server/ui/dist` before `make build-go` / image build.

## Actions APIs missing

Lens detects capabilities and degrades when Gitea Actions endpoints return 404. Job/run JSON shapes vary; the client accepts wrapped or flat arrays.

## Postgres

Insert paths use `RETURNING id` for some flows; broader Postgres production readiness is still behind SQLite. Prefer SQLite unless you are intentionally validating Postgres.

## Metrics 401

`/metrics` requires a logged-in session cookie — not an open scrape target.
