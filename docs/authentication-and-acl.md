# Authentication and ACL

## Login methods

### Gitea OAuth (preferred when Gitea is connected)

- Authorization Code + PKCE
- Start: `GET /api/v1/auth/login`
- Callback: `GET /api/v1/auth/callback`
- Redirect URI must be `{external_url}/api/v1/auth/callback`
- Prefer a **public** Gitea OAuth client; client secret optional for public PKCE
- Post-login redirect only accepts same-app relative paths (`auth.SafeRedirectPath`); absolute / `//` URLs are dropped
- Configure client id/secret under Settings → Sign In (or setup wizard Sign In step)
- OAuth uses the [primary Gitea instance](#primary-gitea-instance) (client id/secret from that instance or legacy auth config)

### GitHub OAuth

- Authorization Code + PKCE (scopes: `read:user user:email repo`)
- Start: `GET /api/v1/auth/github/login`
- Callback: `GET /api/v1/auth/github/callback`
- Redirect URI must be `{external_url}/api/v1/auth/github/callback`
- Configure OAuth App **client id** (and secret when confidential) under Settings → Sign In for the GitHub forge instance
- Link while signed in: `GET /api/v1/auth/github/login?link=1` attaches GitHub identity + token to the current user (account menu **Link GitHub**)
- Users may hold both Gitea and GitHub identities on one GitSeer account; tokens are stored per forge `instance_id` in `user_tokens`

### Forgejo OAuth

- Same Authorization Code + PKCE shape as Gitea (`/login/oauth/authorize`)
- Start: `GET /api/v1/auth/forgejo/login` · Callback: `GET /api/v1/auth/forgejo/callback`
- Redirect URI: `{external_url}/api/v1/auth/forgejo/callback`
- Configure client id/secret under Settings → Sign In
- Primary Forgejo instance (OAuth configured preferred); identity stored in `gitea_user_id` + `instance_id` on that forgejo instance

### GitLab OAuth

- Authorization Code + PKCE (scopes: `read_api read_user read_repository`)
- Start: `GET /api/v1/auth/gitlab/login` · Callback: `GET /api/v1/auth/gitlab/callback`
- Redirect URI: `{external_url}/api/v1/auth/gitlab/callback`
- Configure client id/secret under Settings → Sign In
- Identity: `(gitlab_instance_id, gitlab_user_id)`; link with `?link=1`

### Bitbucket OAuth

- Authorization Code + PKCE (Cloud; Basic auth token exchange)
- Start: `GET /api/v1/auth/bitbucket/login` · Callback: `GET /api/v1/auth/bitbucket/callback`
- Redirect URI: `{external_url}/api/v1/auth/bitbucket/callback`
- Configure client id/secret under Settings → Sign In
- Identity: `(bitbucket_instance_id, bitbucket_user_id)` (stable hash of UUID/account_id); link with `?link=1`

### Bootstrap claim and login

- **First visit:** when no bootstrap password hash (and no legacy env password) exists, the UI shows **Claim Bootstrap** — set username + password (`POST /api/v1/auth/bootstrap/claim` with CSRF). Prefer a username other than `bootstrap`.
- **Login:** `POST /api/v1/auth/bootstrap/login` with CSRF (`username` + `password`) when login is enabled.
- **After setup:** `auth.bootstrap_keep_after_setup` / `GITSEER_AUTH_BOOTSTRAP_KEEP_AFTER_SETUP` controls whether break-glass login remains (`true`) or is removed (`false`). Elevation still works either way.
- **Become Bootstrap (sudo):** signed-in users call `POST /api/v1/auth/bootstrap/elevate` with the bootstrap password for a **5-minute** session grant (`bootstrap_elevated_until`). Effective bootstrap admin unlocks Settings writes. Settings always shows the **Bootstrap** tab; without elevation it prompts to **Become Bootstrap** for password reset (`POST /api/v1/auth/bootstrap/password`).
- Password is stored as a bcrypt hash in `app_settings`. Legacy `GITSEER_AUTH_BOOTSTRAP_PASSWORD` remains a seed until claim/override.
- Creates/uses the bootstrap admin user with allow-all repository access

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
- `GET /api/v1/auth/me` returns current user (and may include mapped Gitea theme when OAuth token is stored; also `has_gitea` / `has_github` / `has_gitlab` / `has_bitbucket`, per-forge `*_oauth_enabled`, `is_bootstrap_admin` (effective), `is_bootstrap_permanent`, `bootstrap_elevated_until`, `can_elevate_bootstrap`, `bootstrap_login_enabled`)
- `POST /api/v1/auth/logout` (CSRF)

## CSRF

Double-submit cookie `gitseer_csrf` (non-HttpOnly) + header `X-CSRF-Token` on state-changing `/api/v1` writes:

- Bootstrap login, logout
- Settings PUT; notification settings PUT / test; alert prefs + push subscribe/unsubscribe/test
- Instance POST / PUT / DELETE; ensure-webhook / verify-webhook
- User ACL grant PUT; wallboard token create/revoke
- Attention mute / unmute; rule-overrides PUT; saved-filters write
- Workflow rerun / cancel
- Setup POSTs (encryption, test-connection, create-webhook, create-oauth, complete, sync-repos)
- Admin purge-retention

Issued via `/api/v1/ui-config` and rotated on session create / `/auth/me`.

**Exempt:** OAuth GET callbacks (Gitea / Forgejo / GitHub / GitLab / Bitbucket), webhooks, `/health/*`, metrics, GETs (including `GET /setup/encryption`).

## ACL

- Table: `user_repository_access`
- Enforced for non-bootstrap users on all repository-scoped reads and SSE fan-out
- **Never trust client-side repository filtering for authorization**
- Bootstrap admins see all repos
- Sync does **not** auto-grant ACL rows
- ACL refresh: on OAuth login (any forge) and every `auth.acl_refresh_interval` (default **6h**) for users with decryptable OAuth tokens, **per forge instance that has a stored user token**
- Instances **without** a per-user token are left alone on refresh — so bootstrap-admin **manual grants** (Settings → Access) for service-PAT-only setups are preserved when another forge’s user refreshes
- When a forge user token exists, ACL for that instance is rebuilt from `ListAccessibleReposForUser`
- OAuth access tokens are refreshed via `refresh_token` when expiry is within ~2 minutes (where the forge issues refresh tokens)
- Token / integration-secret persistence in the DB requires an encryption key (seal fail-closed without it). Env/config min length **16**; wizard paste min **24** — see [Configuration](configuration.md#encryption-key). The setup wizard **Prepare** step can generate or paste a key when env/config is unset.
- Gitea login `bootstrap` is reserved; OAuth cannot inherit the bootstrap-admin row
- Identity keys: Gitea/Forgejo `(instance_id, gitea_user_id)`; GitHub `(github_instance_id, github_user_id)`; GitLab `(gitlab_instance_id, gitlab_user_id)`; Bitbucket `(bitbucket_instance_id, bitbucket_user_id)`; orphaned users re-bind on next OAuth login

### Bootstrap-admin grant fallback

- `GET /api/v1/users` (bootstrap admin)
- `GET /api/v1/users/{id}/access?instance_id=`
- `PUT /api/v1/users/{id}/access` with `{ "instance_id", "repo_ids" }` (CSRF)
- UI: Settings → **Access** — grant GitHub (or other) repos to ordinary users when GitHub OAuth is unavailable

## Rate limits

In-process per-IP (no Redis):

- Bootstrap login and OAuth login start (all forge providers): 20 / minute
- Webhook POST: 120 / minute
- Client IP from `X-Forwarded-For` / `X-Real-IP` only when the peer is in `server.trusted_proxies` / `GITSEER_SERVER_TRUSTED_PROXIES`; otherwise peer `RemoteAddr` only

## Metrics auth

When `server.metrics_token` / `GITSEER_METRICS_TOKEN` is unset, `GET /metrics` requires the same session cookie as the API.

When a scrape token **is** set, auth is **Bearer-only** (`Authorization: Bearer <token>`) — session cookies are not accepted.
