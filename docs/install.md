# Install

## Install with Agent

Paste this prompt into your coding agent. Fill in the bracketed values first, or leave them blank and let the agent ask.

```text
Install Gitea Lens from https://github.com/ncdlabs/gitea-lens on this machine.

Goals:
1. Clone the repo if needed, then run the guided installer (./scripts/install.sh or make install). Prefer Compose via Podman (`podman compose`); fall back to Docker Compose or `--method binary` if Compose is unavailable.
2. Collect or confirm: Gitea base URL, Gitea API token (repo/PR/Actions read), Lens public URL (server.external_url), and a bootstrap admin password. OAuth client id/secret are optional — the in-app Setup wizard can create or paste them later.
3. Write gitignored `.env` + `config.yaml` (or use install.yaml + `--non-interactive`). Set LENS_WEBHOOK_SECRET whenever Gitea URL is set (required at startup). Use LENS_GITEA_ALLOW_PRIVATE_NETWORK=true only for private/lab Gitea URLs.
4. Start Lens, open the printed URL (default http://127.0.0.1:8090), complete /setup if shown (Connect → Validate → Finish), sign in, and click Sync now.
5. Report the App URL, how to stop/restart, bootstrap vs OAuth login, and any remaining manual steps (Gitea system webhook, OAuth redirect URI, optional `lens install-ui`).

Constraints:
- Do not commit secrets, .env, config.yaml, or install.yaml.
- Prefer .yaml over .yml for new config files.
- Follow docs/install.md and README.md; do not invent Redis, WebSockets, or multi-forge setup.
- Ask before destructive changes or removing existing services.

My values (replace or leave blank to prompt me):
- Gitea URL: [https://git.example.com]
- Gitea token: [paste or point to a secret]
- Lens public URL: [http://127.0.0.1:8090]
- Bootstrap password: [generate a strong one if blank]
- Install method: [compose | binary]
- Private/lab Gitea network: [yes | no]
```

Also listed in the root [README](../README.md#install-with-agent).

## Guided installer (recommended)

```bash
./scripts/install.sh
# or: make install
# or: make install INSTALL_ARGS='--config install.yaml -y'
```

Resolution order for values: defaults → `--config` YAML → `.env` → `LENS_*` environment → interactive prompts (skipped with `--non-interactive` / `-y`).

| Mode | Command |
|------|---------|
| Interactive | `./scripts/install.sh` |
| Config file, still prompt for gaps | `./scripts/install.sh --config install.yaml` |
| Fully non-interactive | `./scripts/install.sh --config install.yaml --non-interactive` |
| Configure only (no start) | add `--no-start` |
| Binary instead of Compose | add `--method binary` |

Copy `install.example.yaml` → `install.yaml` (gitignored) and fill secrets, or pass a lens-shaped `config.yaml` (`server` / `gitea` / `auth` keys). Secrets may also live in `.env` or the environment.

The installer writes gitignored `.env` + `config.yaml`, checks dependencies, then runs Compose (`podman compose` preferred) or builds `./bin/lens`.

## Binary (manual)

1. Build: `make frontend && go build -o bin/lens ./cmd/lens`
2. Set `LENS_AUTH_BOOTSTRAP_PASSWORD`, `LENS_GITEA_URL`, `LENS_GITEA_TOKEN`
3. Run: `./bin/lens serve --config config.example.yaml`
4. Open `:8090`, sign in, click **Sync now**

## Compose (manual)

```bash
# Required when Gitea URL is set (or set LENS_WEBHOOK_ALLOW_UNSIGNED=true for unsigned lab webhooks).
export LENS_WEBHOOK_SECRET=replace-me
# Optional bootstrap login (empty disables it).
export LENS_AUTH_BOOTSTRAP_PASSWORD=replace-me
export LENS_GITEA_URL=https://git.example.com
export LENS_GITEA_TOKEN=...
podman compose up --build
```

## Reverse proxy

Set `server.external_url` to the public URL (including subpath). Lens strips only that configured path prefix; client `X-Forwarded-Prefix` is ignored. Prefer root path or verify asset URLs under `/lens`.

## Webhooks / CSRF / ACL

- HMAC: set `LENS_WEBHOOK_SECRET`. Startup fails if `LENS_GITEA_URL` is set and the secret is empty unless `LENS_WEBHOOK_ALLOW_UNSIGNED=true`.
- Browser POSTs send `X-CSRF-Token` matching the `lens_csrf` cookie (issued by `/api/v1/ui-config` and rotated on login/`/auth/me`).
- OAuth user ACL refreshes on login and every `auth.acl_refresh_interval` (default 6h).

## Backup

Copy the SQLite file (`database.path`) or run `pg_dump` for Postgres while writers are quiet. Retention purge removes aged runs/webhooks/resolved attention on a 6h schedule (`retention.*` in config).

## OAuth

See root `README.md` — register redirect URI `{external_url}/api/v1/auth/callback` on Gitea and set `LENS_AUTH_OAUTH_CLIENT_ID` + `LENS_SERVER_EXTERNAL_URL`. Prefer a public OAuth client (PKCE). The setup wizard at `/setup` can create the OAuth app or accept pasted client id/secret.

## Setup wizard and Settings

When `setup_completed` is false, bootstrap admins are routed to `/setup` (Connect → Validate → Finish). Webhook and OAuth helpers call `/api/v1/setup/*`. After setup, prefer `/settings` for runtime preferences and Gitea integration; DB-backed values in `app_settings` override file/env defaults once saved. Secrets on GET are write-only (`*_configured`); empty secret on PUT leaves the stored value unchanged.

## Further reading

- [Docs index](README.md)
- [Architecture](architecture.md)
- [Configuration](configuration.md)
- [Getting started](getting-started.md)
