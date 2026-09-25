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
lens serve [--config path]
lens version
gitseer install-ui --custom-path DIR --gitseer-url URL
gitseer uninstall-ui --custom-path DIR
```

## Helm

Chart: [`deploy/helm/gitseer`](https://github.com/ncdlabs/gitseer/tree/main/deploy/helm/gitseer)

Example upgrade:

```bash
helm upgrade --install gitseer deploy/helm/gitseer \
  -n gitseer \
  -f deploy/helm/gitseer/values-k3s-home.yaml
```

Provide secrets via a Kubernetes Secret (token, bootstrap password, OAuth, webhook secret, encryption key). When `GITSEER_GITEA_URL` is set, `GITSEER_WEBHOOK_SECRET` must be present at startup.

Image builds for some environments use host cross-compile + [`deploy/docker/Containerfile.runtime`](https://github.com/ncdlabs/gitseer/tree/main/deploy/docker) when full multi-stage `go build` under QEMU is unreliable.

## Reverse proxy

1. Terminate TLS at the proxy.
2. Set `server.external_url` to the public URL users and Gitea see (include subpath).
3. Forward to `server.listen`.
4. Do not rely on client `X-Forwarded-Prefix`.

## Gitea companion UI

After deploy:

```bash
./bin/gitseer install-ui --custom-path /var/lib/gitea/custom --gitseer-url https://gitseer.example.com
```

Restart Gitea so custom templates load. Markers: `<!-- BEGIN GITSEER -->` / `<!-- BEGIN GITSEER-TABS -->`.

## Checklist

- [ ] `external_url` matches public scheme/host/path
- [ ] Webhook secret set and Gitea webhook registered
- [ ] OAuth app redirect matches `{external_url}/api/v1/auth/callback`
- [ ] Bootstrap password set for first admin **or** OAuth ready
- [ ] Private Gitea? `allow_private_network` / `GITSEER_GITEA_ALLOW_PRIVATE_NETWORK=true`
- [ ] Encryption key set if OAuth tokens must persist for ACL refresh
- [ ] First sync completed
