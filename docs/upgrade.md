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
- **Bitbucket OAuth (1.0.1 → next):** OAuth login once hashed `bitbucket_user_id` differently from sync. After upgrade, the next Bitbucket sign-in remaps the existing user row in place (same login + instance). No manual SQL needed unless login conflict persists — then clear that user’s `bitbucket_user_id` / `bitbucket_instance_id` and sign in again.
- Wallboard tokens and notification settings persist across upgrades; rotate tokens only if you suspect leakage.

## Schema and feature notes (1.0)

| Migration | Effect |
|-----------|--------|
| `00014` | GitHub `node_id` columns on orgs/repos/PRs/runs/jobs |
| `00015` | Incident wallboard tokens + public snapshot |
| `00016` | Job runner signal fields for attention |
| `00017` | GitLab / Bitbucket OAuth identity columns on users |

Multi-forge OAuth (Gitea, Forgejo, GitHub, GitLab, Bitbucket) is supported in 1.0. Forgejo reuses Gitea identity columns — a single user row cannot hold distinct Gitea and Forgejo forge identities simultaneously.

## Cutting a public release

Public releases are **tag-gated** and cut from GitHub Actions (not locally):

1. Ensure `CHANGELOG.md` `[Unreleased]` has the notes for this release.
2. Open **Actions → Cut release → Run workflow** on `main`.
3. Choose **Bump** (`patch` / `minor` / `major`) or set an explicit **Version** (`X.Y.Z`). Use **Allow empty** only when intentionally shipping with no changelog bullets.

The [cut-release](../.github/workflows/cut-release.yaml) workflow bumps `cmd/gitseer/main.go`, Helm `Chart.yaml` (`version` / `appVersion`), and moves `CHANGELOG.md` `[Unreleased]` into `## [X.Y.Z] - date`, then commits and pushes annotated tag `vX.Y.Z`. That tag runs CI, then [`.github/workflows/release.yaml`](../.github/workflows/release.yaml), which:

- Publishes linux/amd64 images to `ghcr.io/ncdlabs/gitseer:X.Y.Z` (+ `:latest`)
- Sets the GHCR package visibility to **public** (new org packages default to private)
- Optionally also publishes to `docker.io/ncdlabs/gitseer` when Hub secrets are set
- Creates a GitHub Release with the linux/amd64 binary, checksum, and SBOM

Repo secrets:

- `RELEASE_TOKEN` (**required** for cut-release and for making the GHCR package **public**) — classic PAT with `repo` plus `read:packages` / `write:packages` (and org package admin as needed), or a fine-grained token for a repo admin who can bypass the locked / PR-required `main` branch. Used to push the version commit + tag; a PAT (not `GITHUB_TOKEN`) is also required so the tag push triggers `release.yaml`. Without packages scopes, the release image may push as a private org package and anonymous `podman pull` will fail.
- `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` — optional Docker Hub publish (skipped when unset; GHCR still publishes and is the documented public path). GHCR uses `GITHUB_TOKEN`.

```bash
gh secret set RELEASE_TOKEN --repo ncdlabs/gitseer
```

Lab / k3s-home continues to use `git.ncdlabs.com/ncdlabs/gitseer` via the deploy skill — that registry is **not** updated by this workflow. Do not bump `values-k3s-home.yaml` from cut-release.

Pull a public image:

```bash
podman pull ghcr.io/ncdlabs/gitseer:1.0.1
# Docker Hub only if DOCKERHUB_* secrets were set for that release:
# podman pull docker.io/ncdlabs/gitseer:1.0.1
```

Re-publish an existing tag (rebuild images, refresh release notes, re-assert public visibility):

```bash
gh workflow run release.yaml --repo ncdlabs/gitseer -f tag=v1.0.1
```

## Deferred release hardening

Tracked for follow-up; not blockers for public amd64 releases:

- Multi-architecture container images (host cross-compile is amd64-only; QEMU multi-stage `go build` is unreliable here)
- Cosign-signed release images (needs operator key/CI secrets)
- Published integration matrix CI against every supported Gitea/GitLab version

CI already runs unit tests, frontend build, `govulncheck`, and SBOM artifact generation. Tag pushes also publish the container + GitHub Release assets above.
