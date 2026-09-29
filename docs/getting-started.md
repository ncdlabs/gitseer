# Getting Started

## Choose an install path

| Path | When to use |
|------|-------------|
| **Public GHCR image** | Mac/PC demo or cold install (`ghcr.io/ncdlabs/gitseer`, linux/amd64 via Docker) — see root README |
| **Release binary** | Native binary from GitHub Releases (linux/darwin/windows × amd64/arm64) |
| **Guided installer** | Lab install when you already have Gitea URL/token (add other forges in wizard/Settings) |
| **Compose** | Build from source with Docker Compose (or Podman Compose) |
| **Local dev** | Contributing / iterating on UI + API (macOS/Windows OK — builds a native binary) |

In-repo detail: [docs/install.md](install.md).

## 1. Prerequisites

- Go **1.26+** and Node **22+** (binary / local builds)
- Podman Compose or Docker Compose (installer default)
- At least one forge: Gitea (~1.25+), Forgejo, GitHub / GitHub Enterprise, GitLab, and/or Bitbucket Cloud with an API token that can list repos, PRs/MRs, and CI/workflows
- Public URL for GitSeer (needed for OAuth redirect and webhook delivery)

## 2. Run the installer

```bash
git clone https://github.com/ncdlabs/gitseer.git
cd gitseer  # or local folder name if unchanged
./scripts/install.sh
# or: make install
```

Non-interactive:

```bash
cp install.example.yaml install.yaml
# edit secrets: gitea_url, gitea_token, bootstrap_password, server_external_url, …
./scripts/install.sh --config install.yaml --non-interactive
```

The installer writes gitignored `.env` + `config.yaml`, verifies dependencies, then starts GitSeer. It currently requires a Gitea URL/token; for other forges (or GitHub-only), start with Compose/binary and use `/setup` to connect.

## 3. Complete the setup wizard

On first run or when configuration is incomplete (`needs_setup`: missing encryption key, no usable forge URL+token in config/instances, or unfinished wizard), `gitseer serve` prints a Setup Wizard banner and opens the UI in a local browser when safe (interactive TTY; skipped in CI/containers or with `GITSEER_NO_BROWSER=1`). Claim Bootstrap, then GitSeer routes bootstrap admins to **`/setup`**:

1. **Prepare** — public GitSeer URL + generate or paste encryption key (required to store secrets)
2. **Forge picker** — Gitea, GitHub, GitLab, Bitbucket, or Forgejo
3. **Connect** — forge URL and provider credential (e.g. Personal Access Token)
4. **Validate** — connectivity/permission checks; create or paste webhook
5. **Sign In** — optional forge OAuth apps for logging into GitSeer (multi-select when multiple forges exist; skip for bootstrap-only)
6. **Finish** — mark setup complete when at least one forge is configured

A second forge (or more instances of the same type) can be added later under Settings → Integration. Manage OAuth under Settings → Sign In.

See [Setup Wizard](setup-wizard.md).

## 4. Sync and invite users

1. Sign in (forge OAuth — Gitea, Forgejo, GitHub, GitLab, or Bitbucket — or bootstrap password).
2. Click **Sync Now** so the catalog is populated.
3. Other users sign in with forge OAuth; ACL refreshes on login and on an interval (default 6h), scoped to instances where the user has a stored token.

## 5. Point forge webhooks at GitSeer

Prefer the **per-instance** URL from Settings → Integration:

```text
POST {external_url}/api/webhooks/gitea/{instanceID}
POST {external_url}/api/webhooks/forgejo/{instanceID}
POST {external_url}/api/webhooks/github/{instanceID}
POST {external_url}/api/webhooks/gitlab/{instanceID}
POST {external_url}/api/webhooks/bitbucket/{instanceID}
```

Legacy unscoped Gitea: `POST {external_url}/api/webhooks/gitea` (primary Gitea instance secret).

HMAC secret must match the instance (or config) webhook secret. Fail-closed when a forge URL is set in config and that forge’s secret is empty (unless unsigned webhooks are explicitly allowed for lab use).

## 6. Optional: enable browser / OS alerts

Under Settings → **Notifications** → **Browser & OS Alerts**, click **Enable Alerts** (any signed-in user). GitSeer asks for notification permission, registers a service worker, and (when VAPID is ready) a Web Push subscription. Requires **HTTPS** or localhost. Outbound SMTP/Slack channels remain bootstrap-admin under the same tab.

## Optional: Gitea navigation links

```bash
./bin/gitseer install-ui --custom-path /var/lib/gitea/custom --gitseer-url https://gitseer.example.com --instance-id 1
```

Opens GitSeer in a new tab from Gitea’s custom templates. Restart Gitea after template changes.

## Next

- [Configuration](configuration.md)
- [Authentication and ACL](authentication-and-acl.md)
- [Features](features.md)
- [Deployment](deployment.md)
