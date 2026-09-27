# Getting Started

## Choose an install path

| Path | When to use |
|------|-------------|
| **Install with Agent** | Paste the prompt from [docs/install.md](install.md#install-with-agent) into your coding agent |
| **Guided installer** | First production or lab install (Gitea-oriented; add GitHub in the wizard/Settings) |
| **Compose** | Container runtime already in place |
| **Binary** | Single process, no Compose; good for GitHub-only via wizard |
| **Local dev** | Contributing / iterating on UI + API |

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

The installer writes gitignored `.env` + `config.yaml`, verifies dependencies, then starts GitSeer. It currently requires a Gitea URL/token; for GitHub-only, start with Compose/binary and use `/setup` to connect GitHub.

## 3. Complete the setup wizard

On first bootstrap-admin visit (when `setup_completed` is false), GitSeer opens **`/setup`**:

1. **Prepare** — public GitSeer URL + generate or paste encryption key (required to store secrets)
2. **Forge picker** — Gitea, GitHub, GitLab, Bitbucket, or Forgejo
3. **Connect** — forge URL, service token / PAT
4. **Validate** — connectivity/permission checks; create or paste webhook (+ Gitea OAuth when applicable)  
5. **Finish** — mark setup complete when at least one forge is configured  

A second forge (or more instances of the same type) can be added later under Settings → Integration.

See [Setup Wizard](setup-wizard.md).

## 4. Sync and invite users

1. Sign in (Gitea OAuth, GitHub OAuth, or bootstrap password).
2. Click **Sync now** so the catalog is populated.
3. Other users sign in with forge OAuth; ACL refreshes on login and on an interval (default 6h), scoped to instances where the user has a stored token.

## 5. Point forge webhooks at GitSeer

Prefer the **per-instance** URL from Settings → Integration:

```text
POST {external_url}/api/webhooks/gitea/{instanceID}
POST {external_url}/api/webhooks/github/{instanceID}
```

Legacy unscoped Gitea: `POST {external_url}/api/webhooks/gitea` (primary Gitea instance secret).

HMAC secret must match the instance (or config) webhook secret. Fail-closed when a forge URL is set in config and that forge’s secret is empty (unless unsigned webhooks are explicitly allowed for lab use).

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
