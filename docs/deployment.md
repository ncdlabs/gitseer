# Deployment

## Compose (default installer path)

```bash
podman compose -f compose.yaml up --build
```

Container data directory defaults to `/data`. Prefer Podman Compose (`podman compose`, not `podman-compose`).

## Binary

```bash
make frontend && make build-go
./bin/lens serve --config config.yaml
```

Commands:

```text
lens serve [--config path]
lens version
lens install-ui --custom-path DIR --lens-url URL
lens uninstall-ui --custom-path DIR
```

## Helm

Chart: [`deploy/helm/gitea-lens`](https://github.com/ncdlabs/gitea-lens/tree/main/deploy/helm/gitea-lens)

Example upgrade:

```bash
helm upgrade --install gitea-lens deploy/helm/gitea-lens \
  -n gitea-lens \
  -f deploy/helm/gitea-lens/values-k3s-home.yaml
```

Provide secrets via a Kubernetes Secret (token, bootstrap password, OAuth, webhook secret, encryption key). When `LENS_GITEA_URL` is set, `LENS_WEBHOOK_SECRET` must be present at startup.

Image builds for some environments use host cross-compile + [`deploy/docker/Containerfile.runtime`](https://github.com/ncdlabs/gitea-lens/tree/main/deploy/docker) when full multi-stage `go build` under QEMU is unreliable.

## Reverse proxy

1. Terminate TLS at the proxy.
2. Set `server.external_url` to the public URL users and Gitea see (include subpath).
3. Forward to `server.listen`.
4. Do not rely on client `X-Forwarded-Prefix`.

## Gitea companion UI

After deploy:

```bash
./bin/lens install-ui --custom-path /var/lib/gitea/custom --lens-url https://lens.example.com
```

Restart Gitea so custom templates load. Markers: `<!-- BEGIN GITEA-LENS -->` / `<!-- BEGIN GITEA-LENS-TABS -->`.

## Checklist

- [ ] `external_url` matches public scheme/host/path
- [ ] Webhook secret set and Gitea webhook registered
- [ ] OAuth app redirect matches `{external_url}/api/v1/auth/callback`
- [ ] Bootstrap password set for first admin **or** OAuth ready
- [ ] Private Gitea? `allow_private_network` / `LENS_GITEA_ALLOW_PRIVATE_NETWORK=true`
- [ ] Encryption key set if OAuth tokens must persist for ACL refresh
- [ ] First sync completed
