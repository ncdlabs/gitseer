# Authentication and ACL

## Login methods

### Gitea OAuth (preferred)

- Authorization Code + PKCE
- Start: `GET /api/v1/auth/login`
- Callback: `GET /api/v1/auth/callback`
- Redirect URI must be `{external_url}/api/v1/auth/callback`
- Prefer a **public** Gitea OAuth client; client secret optional for public PKCE
- Post-login redirect only accepts same-app relative paths (`auth.SafeRedirectPath`); absolute / `//` URLs are dropped

### Bootstrap password (lab / first admin)

- Enabled only when `LENS_AUTH_BOOTSTRAP_PASSWORD` (or file) is set
- `POST /api/v1/auth/bootstrap/login` with CSRF
- Creates/uses bootstrap admin with allow-all repository access for the primary instance
- Leave empty in Compose by default to disable

## Sessions

- HTTP session cookie after successful login
- TTL from `auth.session_ttl` (default 24h)
- `GET /api/v1/auth/me` returns current user (and may include mapped Gitea theme when OAuth token is stored)
- `POST /api/v1/auth/logout` (CSRF)

## CSRF

Double-submit cookie `lens_csrf` (non-HttpOnly) + header `X-CSRF-Token` on state-changing `/api/v1` writes:

- Bootstrap login, logout, sync-repos, settings PUT, setup POSTs

Issued via `/api/v1/ui-config` and rotated on session create / `/auth/me`.

**Exempt:** OAuth GET callback, webhooks, health, metrics, GETs.

## ACL

- Table: `user_repository_access`
- Enforced for non-bootstrap users on all repository-scoped reads and SSE fan-out
- **Never trust client-side repository filtering for authorization**
- Bootstrap admins see all repos
- Sync does **not** auto-grant ACL rows
- ACL refresh: on OAuth login and every `auth.acl_refresh_interval` (default **6h**) for users with decryptable OAuth tokens
- OAuth access tokens are refreshed via `refresh_token` when expiry is within ~2 minutes
- Token / integration-secret persistence in the DB requires `LENS_ENCRYPTION_KEY` (seal fail-closed without it)
- Gitea login `bootstrap` is reserved; OAuth cannot inherit the bootstrap-admin row
- Lens users are keyed by `(instance_id, gitea_user_id)` (login is mutable)

## Rate limits

In-process per-IP (no Redis):

- Bootstrap login and OAuth login start: 20 / minute
- Webhook POST: 120 / minute
- Client IP from `X-Forwarded-For` / `X-Real-IP` only when the peer is in `server.trusted_proxies` / `LENS_SERVER_TRUSTED_PROXIES`; otherwise peer `RemoteAddr` only

## Metrics auth

`GET /metrics` requires the same session cookie as the API.
