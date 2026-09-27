# Authentication and ACL

## Login methods

### Gitea OAuth (preferred)

- Authorization Code + PKCE
- Start: `GET /api/v1/auth/login`
- Callback: `GET /api/v1/auth/callback`
- Redirect URI must be `{external_url}/api/v1/auth/callback`
- Prefer a **public** Gitea OAuth client; client secret optional for public PKCE
- Post-login redirect only accepts same-app relative paths (`auth.SafeRedirectPath`); absolute / `//` URLs are dropped
- OAuth uses the [primary Gitea instance](#primary-gitea-instance) (client id/secret from that instance or legacy auth config)

### Bootstrap password (lab / first admin)

- Enabled only when `GITSEER_AUTH_BOOTSTRAP_PASSWORD` (or file) is set
- `POST /api/v1/auth/bootstrap/login` with CSRF
- Creates/uses bootstrap admin with allow-all repository access
- Leave empty in Compose by default to disable

GitHub inventory uses a **service PAT** on the GitHub instance for sync/job logs (bootstrap). There is no GitHub OAuth login in this slice; ordinary OAuth users are **not** auto-granted GitHub repository ACL.

## Primary Gitea instance

When more than one Gitea instance exists, OAuth login and the legacy unscoped webhook `POST /api/webhooks/gitea` prefer:

1. The lowest-id Gitea instance that has both OAuth client id and secret configured, else  
2. The lowest-id Gitea instance

Per-instance webhooks (`/api/webhooks/gitea/{instanceID}`) always use that instance’s own secret.

ACL refresh for OAuth users is scoped to the user’s `instance_id` (not every Gitea instance).

## Sessions

- HTTP session cookie after successful login
- TTL from `auth.session_ttl` (default 24h)
- `GET /api/v1/auth/me` returns current user (and may include mapped Gitea theme when OAuth token is stored)
- `POST /api/v1/auth/logout` (CSRF)

## CSRF

Double-submit cookie `gitseer_csrf` (non-HttpOnly) + header `X-CSRF-Token` on state-changing `/api/v1` writes:

- Bootstrap login, logout
- Settings PUT
- Instance POST / PUT / DELETE
- Setup POSTs (encryption, test-connection, create-webhook, create-oauth, complete, sync-repos)

Issued via `/api/v1/ui-config` and rotated on session create / `/auth/me`.

**Exempt:** OAuth GET callback, webhooks, `/health/*`, metrics, GETs (including `GET /setup/encryption`).

## ACL

- Table: `user_repository_access`
- Enforced for non-bootstrap users on all repository-scoped reads and SSE fan-out
- **Never trust client-side repository filtering for authorization**
- Bootstrap admins see all repos
- Sync does **not** auto-grant ACL rows
- ACL refresh: on OAuth login and every `auth.acl_refresh_interval` (default **6h**) for users with decryptable OAuth tokens, scoped to that user’s Gitea `instance_id`
- The same refresh **clears** any prior GitHub ACL rows for that user (GitHub has no per-user OAuth in this slice; bootstrap admins keep allow-all)
- OAuth access tokens are refreshed via `refresh_token` when expiry is within ~2 minutes
- Token / integration-secret persistence in the DB requires an encryption key (seal fail-closed without it). Env/config min length **16**; wizard paste min **24** — see [Configuration](configuration.md#encryption-key). The setup wizard **Prepare** step can generate or paste a key when env/config is unset.
- Gitea login `bootstrap` is reserved; OAuth cannot inherit the bootstrap-admin row
- GitSeer users are keyed by `(instance_id, gitea_user_id)` (login is mutable); orphaned users (`instance_id` NULL after instance delete) re-bind on next OAuth login

## Rate limits

In-process per-IP (no Redis):

- Bootstrap login and OAuth login start: 20 / minute
- Webhook POST: 120 / minute
- Client IP from `X-Forwarded-For` / `X-Real-IP` only when the peer is in `server.trusted_proxies` / `GITSEER_SERVER_TRUSTED_PROXIES`; otherwise peer `RemoteAddr` only

## Metrics auth

`GET /metrics` requires the same session cookie as the API, or `Authorization: Bearer <token>` when `server.metrics_token` / `GITSEER_METRICS_TOKEN` is set.
