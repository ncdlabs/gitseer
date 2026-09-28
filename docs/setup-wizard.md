# Setup Wizard

Shown at `/setup` for bootstrap admins when `setup_completed` is false (gated in the SPA). Other users do not drive setup.

**Local skip:** When `dev.allow_skip_setup` / `GITSEER_ALLOW_SKIP_SETUP=true` (set automatically by `npm run start`), the wizard shows **Skip Setup** in the top bar. That marks setup complete without Prepare/Connect/Validate/Finish. Do not enable in production — rejected when `server.external_url` is non-local.

## Steps

### 1. Prepare

- **GitSeer public URL** (`server_external_url`) — where forges reach GitSeer for webhooks and OAuth callbacks
- **Encryption key** — seals forge tokens, webhook secrets, and OAuth client secrets in the database
  - **Generate Key** — server creates a random passphrase, persists it next to the SQLite DB (or under `data/` for Postgres), and shows it once for backup
  - **Save Key** — paste an existing passphrase (**minimum 24 characters** for the wizard API; env/config accept ≥16 — prefer a generated key)

If `GITSEER_ENCRYPTION_KEY` / `auth.encryption_key_file` is already set, the encryption section shows configured. Continue persists the public URL when set.

### 2. Choose Forge

Pick Gitea, GitHub, GitLab, Bitbucket, or Forgejo. Additional forges can be added later under Settings → Integration.

### 3. Connect

Collects forge URL and the provider’s API credential (e.g. Personal Access Token). Gitea URL blur triggers `POST /api/v1/setup/check-gitea-url`.

### 4. Validate

Auto-starts `POST /api/v1/setup/test-connection`:

- Connectivity and permission checks
- System hooks reported as **warn-if-missing** (Gitea)

Then a webhook confirmation modal:

| Modal | Actions |
|-------|---------|
| Webhook | **Create Webhook** (`POST /api/v1/setup/create-webhook`) or **I'll Add It** with forge-specific manual instructions. After setup, Settings → Status / Integration: **Ensure Webhook** + **Verify Delivery**. |

### 5. Sign In

Optional multi-forge OAuth setup for logging into GitSeer (not forge sync). Lists all connected instances; multi-select which to configure. Paste client ID/secret per forge, or **Create OAuth App** for Gitea/Forgejo (`POST /api/v1/setup/create-oauth`). Skip is always allowed (bootstrap-only login). After setup, manage under Settings → Sign In.

### 6. Finish

`POST /api/v1/setup/complete` marks setup done (requires encryption key + at least one forge). Forge credentials stay under Settings → Integration; OAuth apps under Settings → Sign In.

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
| POST | `/api/v1/instances/{id}/ensure-webhook` | Bootstrap admin + CSRF (post-setup) |
| POST | `/api/v1/instances/{id}/verify-webhook` | Bootstrap admin + CSRF (post-setup) |

See [Authentication and ACL](authentication-and-acl.md) and [API Reference](api-reference.md).
