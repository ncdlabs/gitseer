# Compatibility Matrix

Capability detection is authoritative at runtime (Settings → Status). This page is the operator-facing summary for GitSeer **1.0**.

## Supported forges

| Forge | Sync / webhooks | OAuth login | Rerun / Cancel | Live runners API | Notes |
|-------|-----------------|-------------|----------------|------------------|-------|
| Gitea | Yes | Yes | Yes (user token; bootstrap may use service PAT with UI warning) | Yes when capable | Target API family ~1.26; lab often runs 1.25.x with Actions degrade |
| Forgejo | Yes | Yes (Gitea-compatible) | Same as Gitea | Same as Gitea | Shares Gitea identity columns on `users` |
| GitHub.com / GHE | Yes | Yes | Yes | Org runners when capable | REST `node_id` stored; GHE needs normalized API base |
| GitLab | Yes | Yes | Yes | Yes when capable | Token header webhooks (`X-Gitlab-Token`) |
| Bitbucket Cloud | Yes | Yes | Cancel yes; rerun often unsupported | No public runners inventory | HMAC `X-Hub-Signature-256` |

## GitSeer runtime

| Component | Supported | Constraint |
|-----------|-----------|------------|
| SQLite | Default | `replicaCount: 1` only |
| PostgreSQL | Yes | Preferred for multi-replica sync leases |
| SSE realtime | Yes | In-process; sticky sessions or accept split streams if replicas > 1 |
| Browser Notification API | Yes | While a signed-in tab is connected; OS banner when tab backgrounded |
| Web Push (closed tab) | Yes | Self-hosted VAPID; requires HTTPS or localhost; per-user opt-in |
| Image arch | linux/amd64 | Multi-arch deferred (see [Upgrade](upgrade.md)) |
| Subpath proxy | Yes | Set `server.external_url` including path |

## Gitea Actions

Missing or incomplete Actions APIs disable affected UI (runs, jobs, rerun) via the instance capability matrix rather than failing the whole product. Workflow YAML paths often look like `ci.yaml@refs/heads/main` — GitSeer normalizes and probes `.gitea/workflows/` then `.github/workflows/`.

## Auth and secrets

- Encryption key required to persist/decrypt integration secrets and OAuth tokens (`GITSEER_ENCRYPTION_KEY` or wizard file; env/config ≥16 chars, wizard paste ≥24).
- Webhook HMAC fail-closed when forge URL set and secret empty (unless allow-unsigned). Per-instance routes never inherit the legacy global Gitea secret.
- `GITSEER_ALLOW_SKIP_SETUP` is rejected when `server.external_url` is non-local — never enable on production.
