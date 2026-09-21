# Install

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

See root `README.md` — register redirect URI `{external_url}/api/v1/auth/callback` on Gitea and set `LENS_AUTH_OAUTH_CLIENT_ID` + `LENS_SERVER_EXTERNAL_URL`.
