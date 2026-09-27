# GitSeer documentation

Operator and developer guides for GitSeer (multi-forge; technical IDs are `gitseer`). Quick start: root [README](../README.md).

| Guide | Contents |
|-------|----------|
| [Getting started](getting-started.md) | Install paths, wizard, first sync |
| [Architecture](architecture.md) | Binary shape, packages, data flow |
| [Features](features.md) | UI routes and capabilities |
| [Configuration](configuration.md) | YAML, env, settings layers |
| [Authentication and ACL](authentication-and-acl.md) | OAuth, bootstrap, CSRF, ACL |
| [Setup wizard](setup-wizard.md) | Prepare (encryption) → Choose Forge → Connect → Validate → Finish |
| [Attention engine](attention-engine.md) | Rule matrix and severities |
| [API reference](api-reference.md) | `/api/v1` and webhooks |
| [Webhooks and sync](webhooks-and-sync.md) | Near-realtime + reconcile |
| [Deployment](deployment.md) | Compose, binary, Helm, proxy |
| [Upgrade](upgrade.md) | Backup, Helm/binary upgrade, rollback, schema notes |
| [Compatibility](compatibility.md) | Forge and runtime matrix for 1.0 |
| [Operations](operations.md) | Metrics, retention, backup |
| [Development](development.md) | Local toolchain and guidelines |
| [Troubleshooting](troubleshooting.md) | Common failure modes |
| [Install (detail)](install.md) | Installer flags, proxy, backup |

## Specs

| Document | Role |
|----------|------|
| [PRD / technical spec](prd-spec.md) | Product requirements |
| [Implementation plan](implementation-plan.md) | Build direction |

## Wiki

The same guides are published to the GitHub wiki ([ncdlabs/gitseer/wiki](https://github.com/ncdlabs/gitseer/wiki)). Local clone: `../gitseer.wiki`. Sync + push:

```bash
./scripts/publish-wiki.sh
```

GitHub does not create the `.wiki.git` remote until the first page exists — if clone fails, open the wiki in the GitHub UI, create a stub **Home** page, then re-run the script.
