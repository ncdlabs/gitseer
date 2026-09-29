# Install

Preferred order: **release binary** → **Docker (GHCR)** → **clone** (guided installer / Compose). Root [README quick start](../README.md#quick-start) has the copy-paste commands.

## Install with Agent

Paste this prompt into your coding agent. Fill in the bracketed values first, or leave them blank and let the agent ask.

```text
Install GitSeer (repo ncdlabs/gitseer) from https://github.com/ncdlabs/gitseer on this machine.

Goals:
1. Prefer a native GitHub Release binary (linux/darwin/windows × amd64/arm64), then `ghcr.io/ncdlabs/gitseer:1.0.9` via Docker. Fall back to cloning and `./scripts/install.sh` / Compose / `--method binary` if needed.
2. Collect or confirm: forge plan (Gitea, Forgejo, GitHub, GitLab, and/or Bitbucket), forge base URL + API token/PAT, GitSeer public URL (`server.external_url`), suggested bootstrap username + keep/remove-after-setup choice, and encryption key (env or /setup Prepare). OAuth client id/secret are optional — the Setup wizard can create or paste them later.
3. Write gitignored `.env` + `config.yaml` (or use install.yaml + `--non-interactive`). The guided installer currently requires Gitea URL/token; for other forges, use binary/image/Compose + /setup. Set GITSEER_WEBHOOK_SECRET whenever Gitea URL is set, and GITSEER_GITHUB_WEBHOOK_SECRET whenever GitHub URL is set. Use GITSEER_*_ALLOW_PRIVATE_NETWORK=true only for private/lab forge URLs.
4. Start GitSeer, open the printed URL (default http://127.0.0.1:8090), complete /setup if shown (Prepare → Choose Forge → Connect → Validate → Finish), sign in, and click **Sync Now**.
5. Report the App URL, how to stop/restart, bootstrap vs OAuth login, and any remaining manual steps (per-instance webhooks, OAuth redirect URI, optional `gitseer install-ui`).

Constraints:
- Do not commit secrets, `.env`, `config.yaml`, `install.yaml`, `compose.override.yaml`, `values-k3s-home.yaml`, or `values-secret.yaml` (use the `*.example*` copies).
- Prefer .yaml over .yml for new config files.
- Follow docs/install.md and README.md; do not invent Redis, WebSockets, or undocumented forge types.
- Ask before destructive changes or removing existing services.

My values (replace or leave blank to prompt me):
- Gitea URL: [https://git.example.com or blank to connect other forges in /setup]
- Gitea token: [paste or point to a secret]
- GitHub URL: [https://github.com or blank]
- GitHub PAT: [paste or blank]
- GitSeer public URL: [http://127.0.0.1:8090]
- Bootstrap username: [admin | other; claim password in UI on first visit]
- Bootstrap keep after setup: [true | false]
- Encryption key: [generate in Prepare | paste ≥24 chars | env]
- Install method: [binary | ghcr | compose | guided-installer]
- Private/lab forge network: [yes | no]
```

Also listed in the root [README](../README.md#e-install-with-agent).

## Release binary

```bash
curl -fsSL https://raw.githubusercontent.com/ncdlabs/gitseer/main/scripts/install-binary.sh | bash
```

Detects OS/arch, downloads `./gitseer`, and runs `serve`. Override with `VER=1.0.9` or `--no-start`. Windows: download the `.exe` from [GitHub Releases](https://github.com/ncdlabs/gitseer/releases). Script: [`scripts/install-binary.sh`](../scripts/install-binary.sh).

## Public container (Docker)

```bash
docker run --rm -p 8090:8090 \
  --platform linux/amd64 \
  -v gitseer-data:/data \
  -e GITSEER_SERVER_LISTEN=0.0.0.0:8090 \
  -e GITSEER_SERVER_EXTERNAL_URL=http://127.0.0.1:8090 \
  -e GITSEER_DATABASE_PATH=/data/gitseer.db \
  ghcr.io/ncdlabs/gitseer:1.0.9
```

Image is **linux/amd64**. Use a named volume so the non-root container user can write SQLite.

## Guided installer (clone)

```bash
./scripts/install.sh
# or: make install
# or: make install INSTALL_ARGS='--config install.yaml -y'
```

Resolution order for values: defaults → `--config` YAML → `.env` → `GITSEER_*` environment → interactive prompts (skipped with `--non-interactive` / `-y`).

| Mode | Command |
|------|---------|
| Interactive | `./scripts/install.sh` |
| Config file, still prompt for gaps | `./scripts/install.sh --config install.yaml` |
| Fully non-interactive | `./scripts/install.sh --config install.yaml --non-interactive` |
| Configure only (no start) | add `--no-start` |
| Binary instead of Compose | add `--method binary` |

Copy `install.example.yaml` → `install.yaml` (gitignored) and fill secrets, or pass a gitseer-shaped `config.yaml` (`server` / `gitea` / `github` / `auth` keys). Secrets may also live in `.env` or the environment.

The installer writes gitignored `.env` + `config.yaml`, checks dependencies, then runs Compose (`docker compose` preferred; Podman Compose also works) or builds `./bin/gitseer`. It currently collects **Gitea** URL + token; add GitHub in `/setup` or Settings → Integration afterward.

## Binary (from source)

1. Build: `make frontend && go build -o bin/gitseer ./cmd/gitseer`
2. Optionally set `GITSEER_ENCRYPTION_KEY` (or generate in `/setup` Prepare) and `GITSEER_AUTH_BOOTSTRAP_USERNAME` (default `admin`)
3. Optionally set `GITSEER_GITEA_*` / `GITSEER_GITHUB_*` (or connect forges only in `/setup`)
4. Run: `./bin/gitseer serve --config config.example.yaml`
5. Open `:8090`, **Claim Bootstrap** (sets password), complete **Prepare → Choose Forge → Connect → Validate → Finish**, sign in, click **Sync Now**

## Compose (from source)

```bash
# Required when the matching forge URL is set (or use the forge-specific unsigned lab flags).
export GITSEER_WEBHOOK_SECRET=replace-me
# export GITSEER_GITHUB_WEBHOOK_SECRET=replace-me
# Optional suggested username (claim password in the UI). Do not set GITSEER_AUTH_BOOTSTRAP_PASSWORD for new installs.
export GITSEER_AUTH_BOOTSTRAP_USERNAME=admin
export GITSEER_GITEA_URL=https://git.example.com
export GITSEER_GITEA_TOKEN=...
# export GITSEER_GITHUB_URL=https://github.com
# export GITSEER_GITHUB_TOKEN=...
# export GITSEER_ENCRYPTION_KEY=...   # or generate in /setup Prepare
podman compose up --build
```

## Reverse proxy

Set `server.external_url` to the public URL (including subpath). GitSeer strips only that configured path prefix; client `X-Forwarded-Prefix` is ignored. Prefer root path or verify asset URLs under `/gitseer`. Set `GITSEER_SERVER_TRUSTED_PROXIES` for real client IPs on rate limits.

## Webhooks / CSRF / ACL

- HMAC: set `GITSEER_WEBHOOK_SECRET` when Gitea URL is set; set `GITSEER_GITHUB_WEBHOOK_SECRET` when GitHub URL is set. Prefer per-instance webhook URLs from Settings (`/api/webhooks/{gitea|forgejo|github|gitlab|bitbucket}/{id}`).
- Browser POSTs send `X-CSRF-Token` matching the `gitseer_csrf` cookie (issued by `/api/v1/ui-config` and rotated on login/`/auth/me`).
- OAuth user ACL refreshes on login and every `auth.acl_refresh_interval` (default 6h), scoped to forge instances where the user has a decryptable stored OAuth token (does not clear manual grants for other forges).

## Backup

Copy the SQLite file (`database.path`) or run `pg_dump` for Postgres while writers are quiet. Also back up any wizard-written `gitseer.encryption_key` beside the DB. Retention purge removes aged runs/webhooks/resolved attention on a 6h schedule (`retention.*` in config).

## OAuth

Register an OAuth application on each forge you want users to sign in with:

- **Gitea:** `{external_url}/api/v1/auth/callback` (prefer a public OAuth client with PKCE). The setup wizard can create the app or accept pasted credentials.
- **Forgejo:** `{external_url}/api/v1/auth/forgejo/callback` (Forgejo reuses Gitea identity columns on the user row).
- **GitHub:** `{external_url}/api/v1/auth/github/callback` on a GitHub OAuth App; client id/secret under Settings → Sign In. Inventory sync still uses the service PAT; per-user ACL and write ops use the linked OAuth token when present.
- **GitLab:** `{external_url}/api/v1/auth/gitlab/callback`
- **Bitbucket:** `{external_url}/api/v1/auth/bitbucket/callback`

See root `README.md` and [Authentication and ACL](authentication-and-acl.md).

## Setup wizard and Settings

When `setup_completed` is false, bootstrap admins are routed to `/setup` (Prepare → Choose Forge → Connect → Validate → Finish). Webhook and OAuth helpers call `/api/v1/setup/*`. After setup, prefer `/settings` for runtime preferences and forge instances (`/api/v1/instances`, bootstrap admin). DB-backed `app_settings` and `instances` override/seed file/env defaults once saved. Secrets on GET are write-only (`*_configured`); empty secret on PUT leaves the stored value unchanged.

## Further reading

- [Docs index](README.md)
- [Architecture](architecture.md)
- [Configuration](configuration.md)
- [Getting started](getting-started.md)
