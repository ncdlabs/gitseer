# Deployment

## Compose (default installer path)

```bash
podman compose -f compose.yaml up --build
```

Container data directory defaults to `/data`. Prefer Podman Compose (`podman compose`, not `podman-compose`).

## Binary

```bash
make frontend && make build-go
./bin/gitseer serve --config config.yaml
```

Commands:

```text
gitseer serve [--config path]
gitseer version
gitseer backup --out DIR [--config path]
gitseer restore --from DIR [--config path] [--force]
gitseer rotate-encryption-key [--config path] (--new-key STR | --new-key-file PATH | --generate)
gitseer install-ui --custom-path DIR --gitseer-url URL [--instance-id N]
gitseer uninstall-ui --custom-path DIR
```

Settings → Integration also offers **Download Gitea UI Snippets** (same marker content as a zip) when a Gitea instance is configured; use CLI for in-place install on the Gitea host.

## Helm

Chart: [`deploy/helm/gitseer`](https://github.com/ncdlabs/gitseer/tree/main/deploy/helm/gitseer)

Site overlays are gitignored. Start from the examples:

```bash
cp deploy/helm/gitseer/values-k3s-home.example.yaml deploy/helm/gitseer/values-k3s-home.yaml
# optional when secret.create=true:
cp deploy/helm/gitseer/values-secret.example.yaml deploy/helm/gitseer/values-secret.yaml
```

Example upgrade:

```bash
helm upgrade --install gitseer deploy/helm/gitseer \
  -n gitseer \
  -f deploy/helm/gitseer/values-k3s-home.yaml
```

Provide secrets via a Kubernetes Secret (`GITSEER_GITEA_TOKEN`, bootstrap password, OAuth, `GITSEER_WEBHOOK_SECRET`, `GITSEER_ENCRYPTION_KEY`, and GitHub vars when used), or via a local `values-secret.yaml` (gitignored). When `GITSEER_GITEA_URL` is set, `GITSEER_WEBHOOK_SECRET` must be present at startup. When `GITSEER_GITHUB_URL` is set, `GITSEER_GITHUB_WEBHOOK_SECRET` must be present unless unsigned webhooks are allowed for GitHub.

### Replicas

Default `replicaCount: 1`. Raising replicas is supported only with caveats:

- **Sync:** DB-backed per-`instance_id` leases ensure a single active reconciler per forge instance
- **SSE:** fan-out is in-process (no Redis) — use sticky sessions or accept split event streams
- **Web Push:** delivery is from whichever pod runs the notify fan-out; subscription storage is shared in the DB (Postgres-friendly). Keep `replicaCount: 1` on SQLite.
- **SQLite:** keep `replicaCount: 1` (shared SQLite across pods is unsafe)

## Public container images

Tagged releases publish linux/amd64 images to:

- `ghcr.io/ncdlabs/gitseer:<version>` (also `:latest`)
- `docker.io/ncdlabs/gitseer:<version>` (also `:latest`)

The same release also attaches native binaries for **linux / darwin / windows** × **amd64 / arm64**.

```bash
docker pull ghcr.io/ncdlabs/gitseer:1.0.8
```

Maintainers cut releases via **Actions → Cut release** — see [Upgrade](upgrade.md#cutting-a-public-release). Lab/k3s images on `git.ncdlabs.com` are separate (deploy skill) and may run ahead of the latest public GHCR cut (for example browser/OS alerts in `1.0.3` before that tag is published).

Image builds for some environments use host cross-compile + [`deploy/docker/Containerfile.runtime`](https://github.com/ncdlabs/gitseer/tree/main/deploy/docker) when full multi-stage `go build` under QEMU is unreliable.

## Reverse proxy

1. Terminate TLS at the proxy.
2. Set `server.external_url` to the public URL users and forges see (include subpath).
3. Forward to `server.listen`.
4. Do not rely on client `X-Forwarded-Prefix`.
5. Set `GITSEER_SERVER_TRUSTED_PROXIES` to the proxy CIDRs so rate limits see real client IPs — never `0.0.0.0/0`.

## Gitea companion UI

After deploy:

```bash
./bin/gitseer install-ui --custom-path /var/lib/gitea/custom --gitseer-url https://gitseer.example.com --instance-id 1
```

Restart Gitea so custom templates load. Markers: `<!-- BEGIN GITSEER -->` / `<!-- BEGIN GITSEER-TABS -->`.

## Checklist

- [ ] `external_url` matches public scheme/host/path
- [ ] Encryption key set (`GITSEER_ENCRYPTION_KEY` or wizard Prepare) before saving forge secrets
- [ ] Per-instance webhook URLs registered (Gitea / GitHub / GitLab / Bitbucket / Forgejo as used) with matching secrets
- [ ] Config URL set ⇒ matching webhook secret present at startup (or lab unsigned flag for that forge)
- [ ] OAuth app redirect matches `{external_url}/api/v1/auth/callback` (Gitea) and `/api/v1/auth/{github|gitlab|bitbucket|forgejo}/callback` when those providers are enabled
- [ ] Bootstrap password set for first admin **or** OAuth ready
- [ ] Private forge? `allow_private_network` / `GITSEER_GITEA_ALLOW_PRIVATE_NETWORK` / `GITSEER_GITHUB_ALLOW_PRIVATE_NETWORK` (and per-instance flags for other forges)
- [ ] `GITSEER_SERVER_TRUSTED_PROXIES` set to Traefik/Ingress CIDRs (never `0.0.0.0/0`); `replicaCount: 1` for SQLite
- [ ] First sync completed
- [ ] Probes: `/health/live`, `/health/ready`
