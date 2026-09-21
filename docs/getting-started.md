# Getting Started

## Choose an install path

| Path | When to use |
|------|-------------|
| **Install with Agent** | Paste the prompt from [docs/install.md](install.md#install-with-agent) into your coding agent |
| **Guided installer** | First production or lab install |
| **Compose** | Container runtime already in place |
| **Binary** | Single process, no Compose |
| **Local dev** | Contributing / iterating on UI + API |

In-repo detail: [docs/install.md](install.md).

## 1. Prerequisites

- Go **1.26+** and Node **22+** (binary / local builds)
- Podman Compose or Docker Compose (installer default)
- A Gitea instance (~1.25+) with an API token that can list repos, PRs, and Actions
- Public URL for Lens (needed for OAuth redirect and webhook delivery)

## 2. Run the installer

```bash
git clone https://github.com/ncdlabs/gitea-lens.git
cd gitea-lens
./scripts/install.sh
# or: make install
```

Non-interactive:

```bash
cp install.example.yaml install.yaml
# edit secrets: gitea_url, gitea_token, bootstrap_password, server_external_url, …
./scripts/install.sh --config install.yaml --non-interactive
```

The installer writes gitignored `.env` + `config.yaml`, verifies dependencies, then starts Lens.

## 3. Complete the setup wizard

On first bootstrap-admin visit (when `setup_completed` is false), Lens opens **`/setup`**:

1. **Connect** — Gitea URL, service token, Lens public URL  
2. **Validate** — connectivity/permission checks; create or paste webhook + OAuth  
3. **Finish** — mark setup complete  

See [Setup Wizard](setup-wizard.md).

## 4. Sync and invite users

1. Sign in (Gitea OAuth or bootstrap password).
2. Click **Sync now** so the catalog is populated.
3. Other users sign in with Gitea; ACL is refreshed on login and on an interval (default 6h).

## 5. Point Gitea webhooks at Lens

Webhook endpoint:

```text
POST {external_url}/api/webhooks/gitea
```

HMAC secret must match `LENS_WEBHOOK_SECRET` / Settings. Fail-closed when Gitea URL is set and the secret is empty (unless unsigned webhooks are explicitly allowed for lab use).

## Optional: Gitea navigation links

```bash
./bin/lens install-ui --custom-path /var/lib/gitea/custom --lens-url https://lens.example.com
```

Opens Lens in a new tab from Gitea’s custom templates. Restart Gitea after template changes.

## Next

- [Configuration](configuration.md)
- [Authentication and ACL](authentication-and-acl.md)
- [Features](features.md)
- [Deployment](deployment.md)
