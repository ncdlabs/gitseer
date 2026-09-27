# Authentication and ACL

## Login methods

### Gitea OAuth (preferred when Gitea is connected)

- Authorization Code + PKCE
- Start: `GET /api/v1/auth/login`
- Callback: `GET /api/v1/auth/callback`
- Redirect URI must be `{external_url}/api/v1/auth/callback`
- Prefer a **public** Gitea OAuth client; client secret optional for public PKCE
- Post-login redirect only accepts same-app relative paths (`auth.SafeRedirectPath`); absolute / `//` URLs are dropped
- OAuth uses the [primary Gitea instance](#primary-gitea-instance) (client id/secret from that instance or legacy auth config)

### GitHub OAuth

- Authorization Code + PKCE (scopes: `read:user user:email repo`)
- Start: `GET /api/v1/auth/github/login`
- Callback: `GET /api/v1/auth/github/callback`
- Redirect URI must be `{external_url}/api/v1/auth/github/callback`
- Configure OAuth App **client id** (and secret when confidential) on the GitHub forge instance in Settings → Integration
- Link while signed in: `GET /api/v1/auth/github/login?link=1` attaches GitHub identity + token to the current user (account menu **Link GitHub**)
- Users may hold both Gitea and GitHub identities on one GitSeer account; tokens are stored per forge `instance_id` in `user_tokens`

### Bootstrap password (lab / first admin)

- Enabled only when `GITSEER_AUTH_BOOTSTRAP_PASSWORD` (or file) is set
- `POST /api/v1/auth/bootstrap/login` with CSRF
- Creates/uses bootstrap admin with allow-all repository access
- Leave empty in Compose by default to disable

GitHub inventory still syncs with a **service PAT** on the instance. Per-user GitHub ACL and user-scoped forge calls (logs, write ops) use the user’s stored GitHub OAuth token when present.

## Primary Gitea instance

When more than one Gitea instance exists, OAuth login and the legacy unscoped webhook `POST /api/webhooks/gitea` prefer:

1. The lowest-id Gitea instance that has both OAuth client id and secret configured, else  
2. The lowest-id Gitea instance

Per-instance webhooks (`/api/webhooks/gitea/{instanceID}`) always use that instance’s own secret.

ACL refresh for Gitea OAuth users is scoped to the user’s Gitea `instance_id` (not every Gitea instance).

## Primary GitHub instance

GitHub OAuth login prefers the lowest-id GitHub instance with OAuth client id+secret, else the lowest-id GitHub instance.

## Sessions

- HTTP session cookie after successful login
- TTL from `auth.session_ttl` (default 24h)
- `GET /api/v1/auth/me` returns current user (and may include mapped Gitea theme when OAuth token is stored; also `has_gitea` / `has_github` / `github_oauth_enabled`)
- `POST /api/v1/auth/logout` (CSRF)

## CSRF

Double-submit cookie `gitseer_csrf` (non-HttpOnly) + header `X-CSRF-Token` on state-changing `/api/v1` writes:

- Bootstrap login, logout
- Settings PUT
- Instance POST / PUT / DELETE
- User ACL grant PUT
- Setup POSTs (encryption, test-connection, create-webhook, create-oauth, complete, sync-repos)

Issued via `/api/v1/ui-config` and rotated on session create / `/auth/me`.

**Exempt:** OAuth GET callbacks (Gitea + GitHub), webhooks, `/health/*`, metrics, GETs (including `GET /setup/encryption`).

## ACL

- Table: `user_repository_access`
- Enforced for non-bootstrap users on all repository-scoped reads and SSE fan-out
- **Never trust client-side repository filtering for authorization**
- Bootstrap admins see all repos
- Sync does **not** auto-grant ACL rows
- ACL refresh: on OAuth login (Gitea and/or GitHub) and every `auth.acl_refresh_interval` (default **6h**) for users with decryptable OAuth tokens, **per forge instance that has a stored user token**
- Instances **without** a per-user token are left alone on refresh — so bootstrap-admin **manual grants** (Settings → Access) for service-PAT-only GitHub setups are preserved when a Gitea-only user refreshes
- When a GitHub user token exists, ACL for that GitHub instance is rebuilt from `ListAccessibleReposForUser`
- OAuth access tokens are refreshed via `refresh_token` when expiry is within ~2 minutes (Gitea and GitHub when issued)
- Token / integration-secret persistence in the DB requires an encryption key (seal fail-closed without it). Env/config min length **16**; wizard paste min **24** — see [Configuration](configuration.md#encryption-key). The setup wizard **Prepare** step can generate or paste a key when env/config is unset.
- Gitea login `bootstrap` is reserved; OAuth cannot inherit the bootstrap-admin row
- GitSeer Gitea users are keyed by `(instance_id, gitea_user_id)`; GitHub by `(github_instance_id, github_user_id)`; orphaned users re-bind on next OAuth login

### Bootstrap-admin grant fallback

- `GET /api/v1/users` (bootstrap admin)
- `GET /api/v1/users/{id}/access?instance_id=`
- `PUT /api/v1/users/{id}/access` with `{ "instance_id", "repo_ids" }` (CSRF)
- UI: Settings → **Access** — grant GitHub (or other) repos to ordinary users when GitHub OAuth is unavailable

## Rate limits

In-process per-IP (no Redis):

- Bootstrap login and OAuth login start (Gitea + GitHub): 20 / minute
- Webhook POST: 120 / minute
- Client IP from `X-Forwarded-For` / `X-Real-IP` only when the peer is in `server.trusted_proxies` / `GITSEER_SERVER_TRUSTED_PROXIES`; otherwise peer `RemoteAddr` only

## Metrics auth

`GET /metrics` requires the same session cookie as the API, or `Authorization: Bearer <token>` when `server.metrics_token` / `GITSEER_METRICS_TOKEN` is set.
