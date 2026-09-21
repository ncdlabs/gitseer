# Development

## Toolchain

- Go **1.26+** (`go.mod`)
- Node **22+**
- Optional: Podman for Compose

## Common commands

```bash
make deps              # go mod tidy + web npm install
make test              # go test ./...
npm run start          # API :8090 + Vite :5173; prints URLs + bootstrap creds
npm run stop
npm run restart
make frontend          # build SPA into internal/server/ui/dist
make build-go          # bin/lens without rebuilding frontend
make build             # frontend + go binary
make install           # scripts/install.sh
```

Local start prefers `config.yaml`, else `config.example.yaml` (`LENS_CONFIG` override). Default bootstrap password when unset: `lens-local`.

## Guidelines

- Raw Gitea types stay in `internal/forge/gitea`
- Enforce repository authorization server-side (`user_repository_access` or bootstrap allow-all)
- Prefer `.yaml` for new config files
- Do not add Redis or WebSocket dependencies for V1
- Ask before removing documented product features
- UI headings/buttons use Title Case

## Tests to prefer when touching areas

- Authz and ACL
- Forge normalization
- Store upserts / integrity
- Attention rule fingerprints
- CSRF on mutating routes

## Specs

- [docs/prd-spec.md](https://github.com/ncdlabs/gitea-lens/blob/main/docs/prd-spec.md)
- [docs/implementation-plan.md](https://github.com/ncdlabs/gitea-lens/blob/main/docs/implementation-plan.md)
- [CONTRIBUTING.md](https://github.com/ncdlabs/gitea-lens/blob/main/CONTRIBUTING.md)
