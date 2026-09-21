# Setup Wizard

Shown at `/setup` for bootstrap admins when `setup_completed` is false (gated in the SPA). Other users do not drive setup.

**Local skip:** When `dev.allow_skip_setup` / `LENS_ALLOW_SKIP_SETUP=true` (set automatically by `npm run start`), the wizard shows **Skip Setup** in the top bar. That marks setup complete without Connect/Validate/Finish. Do not enable in production.

## Steps

### 1. Connect

Collects:

- Gitea base URL (blur triggers `POST /api/v1/setup/check-gitea-url`)
- Gitea service token
- Lens public URL (`server_external_url`)

### 2. Validate

Auto-starts `POST /api/v1/setup/test-connection`:

- Connectivity and permission checks
- System hooks + OAuth apps reported as **warn-if-missing**

Then confirmation modals:

| Modal | Actions |
|-------|---------|
| Webhook | **Create Webhook** (`POST /api/v1/setup/create-webhook`) or **I'll Add It in Gitea** |
| OAuth | **Create OAuth App** (`POST /api/v1/setup/create-oauth` via Gitea `/user/applications/oauth2`), **I'll Configure It** (paste client id/secret with Gitea instructions), or **Skip OAuth** (bootstrap-only) |

First integration persist of URL/token/secret is via create-webhook; OAuth credentials via create-oauth when chosen.

### 3. Finish

`POST /api/v1/setup/complete` marks setup done. Unsigned webhook toggle and later secret/OAuth edits also live under **Settings**.

## Related APIs

| Method | Path |
|--------|------|
| POST | `/api/v1/setup/check-gitea-url` |
| POST | `/api/v1/setup/test-connection` |
| POST | `/api/v1/setup/create-webhook` |
| POST | `/api/v1/setup/create-oauth` |
| POST | `/api/v1/setup/complete` |
| POST | `/api/v1/setup/sync-repos` |

All require auth + CSRF except as noted in [Authentication and ACL](authentication-and-acl.md).
