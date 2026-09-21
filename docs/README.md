# Gitea Lens documentation

Operator and developer reference for Gitea Lens. The root [README](../README.md) is the quick start; this tree holds depth.

| Guide | Contents |
|-------|----------|
| [Getting started](getting-started.md) | Install paths, wizard, first sync |
| [Architecture](architecture.md) | Binary shape, packages, data flow |
| [Features](features.md) | UI routes and capabilities |
| [Configuration](configuration.md) | YAML, env, settings layers |
| [Authentication and ACL](authentication-and-acl.md) | OAuth, bootstrap, CSRF, ACL |
| [Setup wizard](setup-wizard.md) | Connect → Validate → Finish |
| [Attention engine](attention-engine.md) | Rule matrix and severities |
| [API reference](api-reference.md) | `/api/v1` and webhooks |
| [Webhooks and sync](webhooks-and-sync.md) | Near-realtime + reconcile |
| [Deployment](deployment.md) | Compose, binary, Helm, proxy |
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

The same guides are prepared for the GitHub wiki (`../gitea-lens.wiki` sibling clone). Publish after creating the first wiki page in the GitHub UI (the `.wiki.git` remote does not exist until then): [ncdlabs/gitea-lens/wiki](https://github.com/ncdlabs/gitea-lens/wiki).
