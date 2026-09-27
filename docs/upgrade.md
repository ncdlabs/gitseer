# Upgrade

Upgrade GitSeer in place without changing repository contents on connected forges.

## Before you upgrade

1. Note the current image tag / binary version (`gitseer version` or Helm `image.tag`).
2. Back up the database and sealed secrets:

```bash
gitseer backup --out /path/to/backup
```

3. Confirm `GITSEER_ENCRYPTION_KEY` (or the on-disk `gitseer.encryption_key` file) is available for the new process — wrong or missing keys leave Status `encryption_healthy=false` and block secret decrypt.
4. For Helm: record rollback targets:

```bash
helm history gitseer -n gitseer --max 5
kubectl -n gitseer get deploy gitseer -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

## Helm (recommended)

Use a **new immutable image tag** each ship (`pullPolicy: IfNotPresent`).

```bash
# bump image.tag in values, then:
helm upgrade --install gitseer deploy/helm/gitseer \
  -n gitseer \
  -f deploy/helm/gitseer/values-k3s-home.yaml
kubectl -n gitseer rollout status deploy/gitseer --timeout=180s
```

Goose migrations run on process start. Migrations `00014`–`00017` (node IDs, wallboard, runner signals, multi-forge OAuth columns) are additive; expect a short lock during apply on SQLite.

### Rollback

```bash
helm rollback gitseer <PREV_REV> -n gitseer
kubectl -n gitseer rollout status deploy/gitseer --timeout=180s
```

Or re-set `image.tag` to the previous tag and `helm upgrade` again (cluster must still be able to pull that tag).

## Binary / Compose

1. Stop the old process.
2. Install the new binary (or rebuild the Compose image).
3. Start with the same `config.yaml` / `.env` and data directory.
4. Confirm `/health/ready` and Settings → Status.

Compose:

```bash
podman compose -f compose.yaml up --build -d
```

## After upgrade

- Hit `GET /health/ready` and Settings → Status (`encryption_healthy`, forge checklist, capability matrix).
- Trigger **Sync Now** if webhooks were quiet during the rollout.
- Re-verify OAuth login for each forge you use (callback URLs unchanged unless you changed `server.external_url`).
- Wallboard tokens and notification settings persist across upgrades; rotate tokens only if you suspect leakage.

## Schema and feature notes (1.0)

| Migration | Effect |
|-----------|--------|
| `00014` | GitHub `node_id` columns on orgs/repos/PRs/runs/jobs |
| `00015` | Incident wallboard tokens + public snapshot |
| `00016` | Job runner signal fields for attention |
| `00017` | GitLab / Bitbucket OAuth identity columns on users |

Multi-forge OAuth (Gitea, Forgejo, GitHub, GitLab, Bitbucket) is supported in 1.0. Forgejo reuses Gitea identity columns — a single user row cannot hold distinct Gitea and Forgejo forge identities simultaneously.

## Deferred release hardening

Tracked for follow-up; not blockers for this lab ship:

- Multi-architecture container images (host cross-compile is amd64-only; QEMU multi-stage `go build` is unreliable here)
- Cosign-signed release images (needs operator key/CI secrets)
- Published integration matrix CI against every supported Gitea/GitLab version

CI already runs unit tests, frontend build, `govulncheck`, and SBOM artifact generation.
