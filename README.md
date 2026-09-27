# GitSeer

Self-hosted CI/CD and pull-request operations console for **Gitea**, **Forgejo**, **GitHub** (github.com and Enterprise), **GitLab**, and **Bitbucket Cloud**. Technical IDs: Go module `github.com/ncdlabs/gitseer`, binary `gitseer`, Helm `gitseer`, env `GITSEER_*`.

Aggregates repositories, open pull requests, Actions/workflow runs, and attention items into one ACL-aware UI. Each forge stays the system of record; GitSeer syncs via API + webhooks. Login is bootstrap password and/or forge OAuth (PKCE). Inventory sync uses a per-instance service token / PAT.

Maintained by **[ncdLabs](https://ncdlabs.com)**.

**Docs:** [docs/](docs/README.md) · **Wiki:** [github.com/ncdlabs/gitseer/wiki](https://github.com/ncdlabs/gitseer/wiki) · **Compatibility:** [docs/compatibility.md](docs/compatibility.md)

![GitSeer dashboard](docs/screenshots/dashboard.png)

---

## Who This Is For

- Operators who want one ops console across one or more self-hosted or cloud forges
- Teams that already run Gitea/Forgejo/GitHub/GitLab/Bitbucket and want attention, pipelines, and PRs in one place
- Homelab / small-org installs that prefer a single binary or Compose stack with SQLite

## Who This Is Not For

- A replacement for the forge itself (GitSeer does not host git or runners)
- Multi-arch hosts without amd64 support (release images and binaries are **linux/amd64 only** today)
- Multi-replica SQLite deployments (use Postgres if you need more than one replica)

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
- **Setup wizard** — Prepare (public URL + encryption key) → forge picker → Connect → Validate → Finish
- **Settings** — runtime preferences and multi-instance forge integration (DB-backed secrets are write-only; instance CRUD is bootstrap-admin)
- **Gitea UI hooks** — optional `install-ui` links from Gitea nav / repo tabs
- **Ops** — Prometheus `/metrics` (session or Bearer token), retention purge, SQLite by default (Postgres supported for multi-replica)

---

## Prerequisites

| Component | Version / notes |
|-----------|-----------------|
| Container or binary | **linux/amd64** release artifacts (see [Releases](#releases)) |
| Go / Node (from source) | Go 1.26+ (`go.mod`), Node 22+ |
| Forge | At least one: Gitea ~1.25+, Forgejo, GitHub / GHE, GitLab, or Bitbucket Cloud |
| Compose (installer default) | Podman Compose or Docker Compose |

---

## Quick start

Pick one path. Detail: [docs/getting-started.md](docs/getting-started.md) · [docs/install.md](docs/install.md).

### A. Public container (fastest cold install)

```bash
mkdir -p gitseer-data && cd gitseer-data
podman run --rm -p 8090:8090 \
  -v "$PWD/data:/data" \
  -e GITSEER_SERVER_LISTEN=0.0.0.0:8090 \
  -e GITSEER_SERVER_EXTERNAL_URL=http://127.0.0.1:8090 \
  -e GITSEER_DATABASE_PATH=/data/gitseer.db \
  ghcr.io/ncdlabs/gitseer:1.0.1
```

Open **http://127.0.0.1:8090**, complete **Setup** (Prepare → Choose Forge → Connect → Validate → Finish), sign in, then **Sync Now**.

### B. Release binary

```bash
curl -fsSL -o gitseer https://github.com/ncdlabs/gitseer/releases/download/v1.0.1/gitseer_1.0.1_linux_amd64
chmod +x gitseer
./gitseer serve --config config.yaml   # or generate config via installer / wizard
```

### C. Guided installer (Gitea-oriented)

The guided installer currently requires a Gitea URL and API token. Add other forges in Setup / Settings afterward. For GitHub-only (or non-Gitea-first), use **A**, **B**, or Compose + `/setup`.

```bash
git clone https://github.com/ncdlabs/gitseer.git
cd gitseer
./scripts/install.sh
# or: make install
```

Non-interactive:

```bash
cp install.example.yaml install.yaml   # fill in secrets
./scripts/install.sh --config install.yaml --non-interactive
```

### D. Compose from source

```bash
export GITSEER_SERVER_EXTERNAL_URL='http://127.0.0.1:8090'
export GITSEER_AUTH_BOOTSTRAP_PASSWORD='change-me'
# Optional pre-seed (or connect in /setup):
# export GITSEER_GITEA_URL=… GITSEER_GITEA_TOKEN=… GITSEER_WEBHOOK_SECRET=…
# export GITSEER_GITHUB_URL=https://github.com GITSEER_GITHUB_TOKEN=… GITSEER_GITHUB_WEBHOOK_SECRET=…
podman compose -f compose.yaml up --build
```

### E. Install with Agent

Paste this prompt into your coding agent (Cursor, Codex, Claude Code, etc.). Fill in the bracketed values first, or leave them blank and let the agent ask.

```text
Install GitSeer (repo ncdlabs/gitseer) from https://github.com/ncdlabs/gitseer on this machine.

Goals:
1. Prefer the public image `ghcr.io/ncdlabs/gitseer:1.0.1` (linux/amd64) or the GitHub Release binary. Fall back to cloning and `./scripts/install.sh` / Compose / `make build-go` if building from source.
2. Collect or confirm: forge plan (Gitea, Forgejo, GitHub, GitLab, and/or Bitbucket), forge base URL + API token/PAT, GitSeer public URL (server.external_url), bootstrap admin password, and encryption key (env or /setup Prepare). OAuth client id/secret are optional — the Setup wizard can create or paste them later.
3. Guided installer currently requires Gitea URL/token; for non-Gitea-first installs use the public image/binary/Compose and complete /setup. Set the matching webhook secret whenever a forge URL is set in config. Use GITSEER_*_ALLOW_PRIVATE_NETWORK=true only for private/lab forge URLs.
4. Start GitSeer, open the printed URL (default http://127.0.0.1:8090), complete /setup if shown (Prepare → Choose Forge → Connect → Validate → Finish), sign in, and click Sync Now.
5. Report the App URL, how to stop/restart, bootstrap vs OAuth login, and any remaining manual steps (per-instance webhooks, OAuth redirect URI, optional `gitseer install-ui`).

Constraints:
- Do not commit secrets, .env, config.yaml, or install.yaml.
- Prefer .yaml over .yml for new config files.
- Follow docs/install.md and README.md; do not invent Redis, WebSockets, or undocumented forge types.
- Ask before destructive changes or removing existing services.

My values (replace or leave blank to prompt me):
- Primary forge: [gitea | forgejo | github | gitlab | bitbucket]
- Forge URL: [https://git.example.com | https://github.com | …]
- Forge token/PAT: [paste or point to a secret]
- Additional forges: [none | list]
- GitSeer public URL: [http://127.0.0.1:8090]
- Bootstrap password: [generate a strong one if blank]
- Encryption key: [generate in Prepare | paste ≥24 chars | env]
- Install method: [ghcr | binary | compose | guided-installer]
- Private/lab forge network: [yes | no]
```

### Local development

```bash
make deps
npm run start          # API :8090 + Vite :5173; prints URLs + bootstrap creds
# npm run stop | npm run restart
make test
```

Default bootstrap user/password when unset: `bootstrap` / `gitseer-local` (prefer `config.yaml` over `config.example.yaml`; override with `GITSEER_CONFIG`).

### Optional: Gitea UI links

```bash
./bin/gitseer install-ui --custom-path /var/lib/gitea/custom --gitseer-url https://gitseer.example.com
# remove: ./bin/gitseer uninstall-ui --custom-path /var/lib/gitea/custom
```

---

## Configuration

Defaults live in `config.example.yaml`. Environment overrides use the `GITSEER_*` prefix — see [docs/configuration.md](docs/configuration.md).

Important:

- Set `server.external_url` to the public URL (including subpath). GitSeer strips only that configured prefix — not client `X-Forwarded-Prefix`.
- When a forge URL is set in config, the matching webhook secret is required unless unsigned webhooks are explicitly allowed (lab only).
- OAuth redirects are per forge under `{external_url}/api/v1/auth/...` (see [docs/authentication-and-acl.md](docs/authentication-and-acl.md)). Inventory sync still uses a service token / PAT.
- `GITSEER_ENCRYPTION_KEY` (env/config min **16**, prefer ≥24; wizard paste min **24**) encrypts forge secrets and OAuth tokens at rest. Required to save integration secrets to the DB; the setup wizard can generate a key file when unset.

Runtime settings and multi-instance forge integration can also be managed in the UI (`/settings`) after bootstrap login.

---

## Documentation

| Resource | Contents |
|----------|----------|
| [docs/](docs/README.md) | Architecture, features, API, auth, deploy, operations |
| [docs/install.md](docs/install.md) | Installer modes, proxy, webhooks, backup |
| [docs/compatibility.md](docs/compatibility.md) | Forge and runtime matrix |
| [docs/prd-spec.md](docs/prd-spec.md) | Product requirements |
| [docs/implementation-plan.md](docs/implementation-plan.md) | Implementation direction |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Dev workflow and guidelines |
| [Wiki](https://github.com/ncdlabs/gitseer/wiki) | Same guides |

---

## Releases

Tagged releases publish a **linux/amd64** binary on GitHub Releases and a container image to **GHCR** (public):

```bash
podman pull ghcr.io/ncdlabs/gitseer:1.0.1
# or :latest
```

Docker Hub (`docker.io/ncdlabs/gitseer`) publishes only when `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` repo secrets are set.

Maintainers cut a release via **Actions → Cut release** (on `main`). Details: [docs/upgrade.md](docs/upgrade.md#cutting-a-public-release).

---

## How to contribute

1. Fork and branch.
2. `make deps && make test && npm run start`
3. Keep forge wire types under `internal/forge/…`; enforce authz server-side.
4. Open a PR with a short summary and test notes.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/development.md](docs/development.md).

---

## Attribution

**GitSeer** is an [ncdLabs](https://ncdlabs.com) open-source project (repository / module path `gitseer`).

Gitea®, Forgejo®, GitHub®, GitLab®, and Bitbucket® are trademarks of their respective owners. This project is not affiliated with or endorsed by those projects.

## License

Apache-2.0 — see [LICENSE](LICENSE).
