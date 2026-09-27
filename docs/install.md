# Install

## Install with Agent

Paste this prompt into your coding agent. Fill in the bracketed values first, or leave them blank and let the agent ask.

```text
Install GitSeer (repo ncdlabs/gitseer) from https://github.com/ncdlabs/gitseer on this machine.

Goals:
1. Clone the repo if needed, then run the guided installer (./scripts/install.sh or make install). Prefer Compose via Podman (`podman compose`); fall back to Docker Compose or `--method binary` if Compose is unavailable.
2. Collect or confirm: forge plan (Gitea and/or GitHub), Gitea base URL + API token when using Gitea (repo/PR/Actions read), optional GitHub URL + PAT, GitSeer public URL (server.external_url), bootstrap admin password, and GITSEER_ENCRYPTION_KEY (or plan to generate it in /setup Prepare). OAuth client id/secret are optional — the Setup wizard can create or paste them later.
3. Write gitignored `.env` + `config.yaml` (or use install.yaml + `--non-interactive`). The guided installer currently requires Gitea URL/token; for GitHub-only, use Compose/binary + /setup. Set GITSEER_WEBHOOK_SECRET whenever Gitea URL is set, and GITSEER_GITHUB_WEBHOOK_SECRET whenever GitHub URL is set. Use GITSEER_*_ALLOW_PRIVATE_NETWORK=true only for private/lab forge URLs.
4. Start GitSeer, open the printed URL (default http://127.0.0.1:8090), complete /setup if shown (Prepare → Choose Forge → Connect → Validate → Finish), sign in, and click Sync now.
5. Report the App URL, how to stop/restart, bootstrap vs OAuth login, and any remaining manual steps (per-instance webhooks, OAuth redirect URI, optional `gitseer install-ui`).

Constraints:
- Do not commit secrets, .env, config.yaml, or install.yaml.
- Prefer .yaml over .yml for new config files.
- Follow docs/install.md and README.md; do not invent Redis, WebSockets, or GitLab/Bitbucket clients (Coming Soon).
- Ask before destructive changes or removing existing services.

My values (replace or leave blank to prompt me):
- Gitea URL: [https://git.example.com or blank for GitHub-only]
- Gitea token: [paste or point to a secret]
- GitHub URL: [https://github.com or blank]
- GitHub PAT: [paste or blank]
- GitSeer public URL: [http://127.0.0.1:8090]
- Bootstrap password: [generate a strong one if blank]
- Encryption key: [generate in Prepare | paste ≥24 chars | env]
- Install method: [compose | binary]
- Private/lab forge network: [yes | no]
```

Also listed in the root [README](../README.md#install-with-agent).

## Guided installer (recommended)

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

The installer writes gitignored `.env` + `config.yaml`, checks dependencies, then runs Compose (`podman compose` preferred) or builds `./bin/gitseer`. It currently collects **Gitea** URL + token; add GitHub in `/setup` or Settings → Integration afterward.

## Binary (manual)

1. Build: `make frontend && go build -o bin/gitseer ./cmd/gitseer`
2. Set `GITSEER_AUTH_BOOTSTRAP_PASSWORD` and optionally `GITSEER_ENCRYPTION_KEY`
3. Optionally set `GITSEER_GITEA_*` / `GITSEER_GITHUB_*` (or connect forges only in `/setup`)
4. Run: `./bin/gitseer serve --config config.example.yaml`
5. Open `:8090`, complete **Prepare → Choose Forge → Connect → Validate → Finish**, sign in, click **Sync now**

## Compose (manual)

```bash
# Required when the matching forge URL is set (or use the forge-specific unsigned lab flags).
export GITSEER_WEBHOOK_SECRET=replace-me
# export GITSEER_GITHUB_WEBHOOK_SECRET=replace-me
# Optional bootstrap login (empty disables it).
export GITSEER_AUTH_BOOTSTRAP_PASSWORD=replace-me
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

- HMAC: set `GITSEER_WEBHOOK_SECRET` when Gitea URL is set; set `GITSEER_GITHUB_WEBHOOK_SECRET` when GitHub URL is set. Prefer per-instance webhook URLs from Settings.
- Browser POSTs send `X-CSRF-Token` matching the `gitseer_csrf` cookie (issued by `/api/v1/ui-config` and rotated on login/`/auth/me`).
- OAuth user ACL refreshes on login and every `auth.acl_refresh_interval` (default 6h), scoped to the user’s Gitea `instance_id`.

## Backup

Copy the SQLite file (`database.path`) or run `pg_dump` for Postgres while writers are quiet. Also back up any wizard-written `gitseer.encryption_key` beside the DB. Retention purge removes aged runs/webhooks/resolved attention on a 6h schedule (`retention.*` in config).

## OAuth

See root `README.md` and [Authentication and ACL](authentication-and-acl.md).

- **Gitea:** register redirect URI `{external_url}/api/v1/auth/callback` and set client id/secret (prefer a public OAuth client with PKCE). The setup wizard at `/setup` can create the OAuth app or accept pasted credentials.
- **GitHub:** register redirect URI `{external_url}/api/v1/auth/github/callback` on a GitHub OAuth App; configure client id/secret on the GitHub forge instance in Settings → Integration. Inventory sync still uses the service PAT; per-user ACL and write ops use the linked GitHub OAuth token when present.

## Setup wizard and Settings

When `setup_completed` is false, bootstrap admins are routed to `/setup` (Prepare → Choose Forge → Connect → Validate → Finish). Webhook and OAuth helpers call `/api/v1/setup/*`. After setup, prefer `/settings` for runtime preferences and forge instances (`/api/v1/instances`, bootstrap admin). DB-backed `app_settings` and `instances` override/seed file/env defaults once saved. Secrets on GET are write-only (`*_configured`); empty secret on PUT leaves the stored value unchanged.

## Further reading

- [Docs index](README.md)
- [Architecture](architecture.md)
- [Configuration](configuration.md)
- [Getting started](getting-started.md)
