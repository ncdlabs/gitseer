# Setup Wizard

Shown at `/setup` for bootstrap admins when `setup_completed` is false (gated in the SPA). Other users do not drive setup.

**Local skip:** When `dev.allow_skip_setup` / `GITSEER_ALLOW_SKIP_SETUP=true` (set automatically by `npm run start`), the wizard shows **Skip Setup** in the top bar. That marks setup complete without Prepare/Connect/Validate/Finish. Do not enable in production — rejected when `server.external_url` is non-local.

## Steps

### 1. Prepare

- **GitSeer public URL** (`server_external_url`) — where forges reach GitSeer for webhooks and Gitea OAuth
- **Encryption key** — seals forge tokens, webhook secrets, and OAuth client secrets in the database
  - **Generate Key** — server creates a random passphrase, persists it next to the SQLite DB (or under `data/` for Postgres), and shows it once for backup
  - **Save Key** — paste an existing passphrase (**minimum 24 characters** for the wizard API; env/config accept ≥16 — prefer a generated key)

If `GITSEER_ENCRYPTION_KEY` / `auth.encryption_key_file` is already set, the encryption section shows configured. Continue persists the public URL when set.

### 2. Choose Forge

Pick Gitea or GitHub (GitLab / Bitbucket Coming Soon). Additional forges can be added later under Settings → Integration.

### 3. Connect

Collects forge URL and service token / PAT. Gitea URL blur triggers `POST /api/v1/setup/check-gitea-url`.

### 4. Validate

Auto-starts `POST /api/v1/setup/test-connection`:

- Connectivity and permission checks
- System hooks + OAuth apps reported as **warn-if-missing** (Gitea)

Then confirmation modals:

| Modal | Actions |
|-------|---------|
| Webhook | **Create Webhook** (`POST /api/v1/setup/create-webhook`) or **I'll Add It in Gitea** / GitHub manual instructions |
| OAuth | **Create OAuth App** (`POST /api/v1/setup/create-oauth`), **I'll Configure It**, or **Skip OAuth** (Gitea; bootstrap-only) |

### 5. Finish

`POST /api/v1/setup/complete` marks setup done (requires encryption key + at least one forge). Unsigned webhook toggle and later secret/OAuth edits also live under **Settings**.

## Related APIs

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/v1/setup/encryption` | Bootstrap admin |
| POST | `/api/v1/setup/encryption` | Bootstrap admin + CSRF |
| POST | `/api/v1/setup/check-gitea-url` | Bootstrap admin + CSRF |
| POST | `/api/v1/setup/test-connection` | Bootstrap admin + CSRF |
| POST | `/api/v1/setup/create-webhook` | Bootstrap admin + CSRF |
| POST | `/api/v1/setup/create-oauth` | Bootstrap admin + CSRF |
| POST | `/api/v1/setup/complete` | Bootstrap admin + CSRF |
| POST | `/api/v1/setup/sync-repos` | Bootstrap admin + CSRF |

See [Authentication and ACL](authentication-and-acl.md) and [API Reference](api-reference.md).
