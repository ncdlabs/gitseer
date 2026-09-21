# Gitea Lens

Self-hosted CI/CD and pull-request operations console for Gitea.

Built and maintained by **[ncdLabs](https://ncdlabs.com)**.

---

## Installation

The fastest path is the guided installer (Compose by default). You need a Gitea URL, a Gitea API token (for sync), and — for normal login — a Gitea OAuth app.

### 1. Create a Gitea OAuth app

In Gitea: **Settings → Applications → Create OAuth2 Application**

- **Redirect URI:** `{your Lens URL}/api/v1/auth/callback`  
  Examples: `http://127.0.0.1:8090/api/v1/auth/callback` or `https://lens.example.com/api/v1/auth/callback`
- Prefer a **public** client (PKCE). A client secret is optional.

### 2. Run the installer (recommended)

Interactive (prompts for anything not already in the environment, `.env`, or a config file):

```bash
./scripts/install.sh
# or: make install
```

Non-interactive from a YAML file (no prompts; fails if required values are missing):

```bash
cp install.example.yaml install.yaml   # fill in secrets
./scripts/install.sh --config install.yaml --non-interactive
```

The installer writes gitignored `.env` + `config.yaml`, checks dependencies (Podman/Docker compose, or Go 1.22+ / Node 22+ for `--method binary`), then starts Lens.

### 3. Optional: add Lens to the Gitea UI

Opens Lens in a **new tab** from Gitea’s nav / repo tabs (does not leave your current Gitea page):

```bash
./bin/lens install-ui --custom-path /var/lib/gitea/custom --lens-url https://lens.example.com
```

Or pass `--install-ui --gitea-custom /var/lib/gitea/custom` to the installer.

Remove later with `./bin/lens uninstall-ui --custom-path /var/lib/gitea/custom`.

### Manual Compose / source

```bash
export LENS_GITEA_URL='https://git.example.com'
export LENS_GITEA_TOKEN='your-gitea-api-token'
export LENS_SERVER_EXTERNAL_URL='http://127.0.0.1:8090'
export LENS_AUTH_OAUTH_CLIENT_ID='your-oauth-client-id'
export LENS_AUTH_BOOTSTRAP_PASSWORD='change-me'   # first-run / lab fallback
# export LENS_GITEA_ALLOW_PRIVATE_NETWORK=true    # only if Gitea is on a private/lab URL

podman compose -f compose.yaml up --build
# or: make build && ./bin/lens serve --config config.example.yaml
```

Open **http://127.0.0.1:8090** → **Continue with Gitea** (or use the bootstrap password).

After the first admin login, click **Sync now** so repositories exist before other users refresh access.

More detail (Postgres, reverse-proxy / subpath, backups): [docs/install.md](docs/install.md).

---

## How to contribute

Contributions are welcome.

1. Fork the repo and create a branch.
2. Install Go 1.22+ and Node 22+, then:

   ```bash
   make deps
   make test
   npm run start
   ```

   Or build the embedded binary: `make build` then `./bin/lens serve`.

3. Keep changes proportional. Enforce authz server-side; keep raw Gitea types in `internal/forge/gitea`.
4. Open a pull request with a short summary and test notes.

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines and [docs/implementation-plan.md](docs/implementation-plan.md) for architecture direction.

---

## Attribution

**Gitea Lens** is an [ncdLabs](https://ncdlabs.com) open-source project.

Gitea® is a trademark of its respective owners. This project is not affiliated with or endorsed by the Gitea project.

## License

Apache-2.0 — see [LICENSE](LICENSE).
