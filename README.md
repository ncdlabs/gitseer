# GitSeer

Self-hosted CI/CD and pull-request operations console for **Gitea** and **GitHub** (dual-forge). Technical IDs: Go module `github.com/ncdlabs/gitseer`, binary `gitseer`, Helm `gitseer`, env `GITSEER_*`.

Aggregates repositories, open pull requests, Actions/workflow runs, and attention items into one ACL-aware UI. Each forge stays the system of record; GitSeer syncs via API + webhooks. Login is bootstrap password and/or Gitea OAuth (PKCE) and GitHub OAuth (PKCE). GitHub inventory sync uses a service PAT. 

Maintained by **[ncdLabs](https://ncdlabs.com)**.

**Docs:** [docs/](docs/README.md) · **Wiki:** [github.com/ncdlabs/gitseer/wiki](https://github.com/ncdlabs/gitseer/wiki) · **Spec:** [docs/prd-spec.md](docs/prd-spec.md)

![GitSeer dashboard](docs/screenshots/dashboard.png)

---

## Screenshots

Repo names, authors, and local account labels in these shots are substituted (`org-a/repo-1`, `bot`, `admin`) so the UI is readable without exposing real inventory.

**Login**

![Login](docs/screenshots/login.png)

**Attention**

![Attention](docs/screenshots/attention.png)

**Pull Requests**

![Pull Requests](docs/screenshots/pull-requests.png)

**Pipelines**

![Pipelines](docs/screenshots/pipelines.png)

**Pipeline Detail**

![Pipeline Detail](docs/screenshots/pipeline-detail.png)

### Dashboard Themes

Five themes. System follows the OS preference (shown here resolving to dark).

| Light | Dark |
|-------|------|
| ![Dashboard Light](docs/screenshots/dashboard-light.png) | ![Dashboard Dark](docs/screenshots/dashboard-dark.png) |

| Gruvbox | Terminal |
|---------|----------|
| ![Dashboard Gruvbox](docs/screenshots/dashboard-gruvbox.png) | ![Dashboard Terminal](docs/screenshots/dashboard-terminal.png) |

**System** (OS preference)

![Dashboard System](docs/screenshots/dashboard-system.png)

---

## Features

- **Dashboard** — time-scoped summary and trends (Now / 1 / 7 / 30 / 90 days)
- **Attention** — discrete rules for CI failures, review waits, long-running runs, and merge conflicts
- **Repositories, pull requests, pipelines** — ACL-scoped lists with live SSE updates; on-demand job logs
- **Setup wizard** — Prepare (public URL + encryption key) → forge picker (Gitea / GitHub; GitLab, Bitbucket, Forgejo) → Connect → Validate → Finish
- **Settings** — runtime preferences and multi-instance Gitea + GitHub integration (DB-backed secrets are write-only; instance CRUD is bootstrap-admin)
- **Gitea UI hooks** — optional `install-ui` links from Gitea nav / repo tabs
- **Ops** — Prometheus `/metrics` (session or Bearer token), retention purge, SQLite by default (Postgres experimental)

---

## Prerequisites

| Component | Version |
|-----------|---------|
| Go | 1.26+ (see `go.mod`) |
| Node.js | 22+ |
| Gitea | ~1.25+ (Actions APIs preferred; features degrade when missing) |
| Compose (installer default) | Podman Compose or Docker Compose |

---

## Quick start

### Install with Agent

Paste this prompt into your coding agent (Cursor, Codex, Claude Code, etc.). Fill in the bracketed values first, or leave them blank and let the agent ask.

```text
Install GitSeer (repo ncdlabs/gitseer) from https://github.com/ncdlabs/gitseer on this machine.

Goals:
1. Clone the repo if needed, then run the guided installer (./scripts/install.sh or make install). Prefer Compose via Podman (`podman compose`); fall back to Docker Compose or `--method binary` if Compose is unavailable.
2. Collect or confirm: forge plan (Gitea and/or GitHub), Gitea base URL + API token when using Gitea, optional GitHub URL + PAT, GitSeer public URL (server.external_url), bootstrap admin password, and encryption key (env or /setup Prepare). OAuth client id/secret are optional — the Setup wizard can create or paste them later.
3. Write gitignored `.env` + `config.yaml` (or use install.yaml + `--non-interactive`). The guided installer currently requires Gitea URL/token; for GitHub-only use Compose/binary + /setup. Set GITSEER_WEBHOOK_SECRET whenever Gitea URL is set, and GITSEER_GITHUB_WEBHOOK_SECRET whenever GitHub URL is set. Use GITSEER_*_ALLOW_PRIVATE_NETWORK=true only for private/lab forge URLs.
4. Start GitSeer, open the printed URL (default http://127.0.0.1:8090), complete /setup if shown (Prepare → Choose Forge → Connect → Validate → Finish), sign in, and click Sync now.
5. Report the App URL, how to stop/restart, bootstrap vs OAuth login, and any remaining manual steps (per-instance webhooks, OAuth redirect URI, optional `gitseer install-ui`).

Constraints:
- Do not commit secrets, .env, config.yaml, or install.yaml.
- Prefer .yaml over .yml for new config files.
- Follow docs/install.md and README.md; do not invent Redis, WebSockets, or undocumented forge types.
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

### Guided installer (recommended)

You need a Gitea URL and API token for the guided installer (add GitHub later in Settings, or use Compose/binary + `/setup` for GitHub-only). OAuth can be created in the setup wizard or beforehand.

```bash
./scripts/install.sh
# or: make install
```

Non-interactive:

```bash
cp install.example.yaml install.yaml   # fill in secrets
./scripts/install.sh --config install.yaml --non-interactive
```

The installer writes gitignored `.env` + `config.yaml`, checks dependencies, then starts GitSeer (Compose by default; `--method binary` optional).

Open the printed URL (default **http://127.0.0.1:8090**). If setup is incomplete, the **Setup** wizard walks Prepare → Choose Forge → Connect → Validate → Finish. After the first admin login, use **Sync now** so repositories exist before other users refresh access.

### Local development

```bash
make deps
npm run start          # API :8090 + Vite :5173; prints URLs + bootstrap creds
# npm run stop | npm run restart
make test
```

Default bootstrap user/password when unset: `bootstrap` / `gitseer-local` (prefer `config.yaml` over `config.example.yaml`; override with `GITSEER_CONFIG`).

Production-shaped binary:

```bash
make frontend && make build-go
./bin/gitseer serve --config config.yaml
```

### Manual Compose

```bash
export GITSEER_GITEA_URL='https://git.example.com'
export GITSEER_GITEA_TOKEN='your-gitea-api-token'
export GITSEER_WEBHOOK_SECRET='replace-me'          # required when Gitea URL is set
# export GITSEER_GITHUB_URL='https://github.com'
# export GITSEER_GITHUB_TOKEN='your-github-pat'
# export GITSEER_GITHUB_WEBHOOK_SECRET='replace-me'  # required when GitHub URL is set
export GITSEER_SERVER_EXTERNAL_URL='http://127.0.0.1:8090'
export GITSEER_AUTH_OAUTH_CLIENT_ID='your-oauth-client-id'
export GITSEER_AUTH_BOOTSTRAP_PASSWORD='change-me'  # optional first-run / lab
# export GITSEER_ENCRYPTION_KEY='…'                  # or generate in /setup Prepare
# export GITSEER_GITEA_ALLOW_PRIVATE_NETWORK=true   # private/lab Gitea URLs

podman compose -f compose.yaml up --build
```

### Optional: Gitea UI links

```bash
./bin/gitseer install-ui --custom-path /var/lib/gitea/custom --gitseer-url https://gitseer.example.com
# remove: ./bin/gitseer uninstall-ui --custom-path /var/lib/gitea/custom
```

More detail: [docs/install.md](docs/install.md) · [docs/getting-started.md](docs/getting-started.md).

---

## Configuration

Defaults live in `config.example.yaml`. Environment overrides use the `GITSEER_*` prefix — see [docs/configuration.md](docs/configuration.md).

Important:

- Set `server.external_url` to the public URL (including subpath). GitSeer strips only that configured prefix — not client `X-Forwarded-Prefix`.
- When `gitea.url` is set, `GITSEER_WEBHOOK_SECRET` is required unless `GITSEER_WEBHOOK_ALLOW_UNSIGNED=true` (lab only). Same pattern for GitHub: `GITSEER_GITHUB_WEBHOOK_SECRET` when `github.url` is set.
- Gitea OAuth redirect: `{external_url}/api/v1/auth/callback` (prefer a public client + PKCE). GitHub OAuth redirect: `{external_url}/api/v1/auth/github/callback`. GitHub inventory sync still uses a service PAT.
- `GITSEER_ENCRYPTION_KEY` (env/config min **16**, prefer ≥24; wizard paste min **24**) encrypts forge secrets and OAuth tokens at rest. Required to save integration secrets to the DB; the setup wizard can generate a key file when unset.

Runtime settings and multi-instance Gitea/GitHub integration can also be managed in the UI (`/settings`) after bootstrap login.

---

## Documentation

| Resource | Contents |
|----------|----------|
| [docs/](docs/README.md) | Architecture, features, API, auth, deploy, operations |
| [docs/install.md](docs/install.md) | Installer modes, proxy, webhooks, backup |
| [docs/prd-spec.md](docs/prd-spec.md) | Product requirements |
| [docs/implementation-plan.md](docs/implementation-plan.md) | Implementation direction |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Dev workflow and guidelines |
| [Wiki](https://github.com/ncdlabs/gitseer/wiki) | Same guides (publish after first wiki page exists) |

---

## How to contribute

1. Fork and branch.
2. `make deps && make test && npm run start`
3. Keep forge wire types in `internal/forge/gitea` / `internal/forge/github`; enforce authz server-side.
4. Open a PR with a short summary and test notes.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/development.md](docs/development.md).

---

## Attribution

**GitSeer** is an [ncdLabs](https://ncdlabs.com) open-source project (repository / module path `gitseer`).

Gitea® and GitHub® are trademarks of their respective owners. This project is not affiliated with or endorsed by the Gitea or GitHub projects.

## License

Apache-2.0 — see [LICENSE](LICENSE).
