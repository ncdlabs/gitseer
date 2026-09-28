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
make build-go          # bin/gitseer without rebuilding frontend
make build             # frontend + go binary
make install           # scripts/install.sh
```

Local start prefers `config.yaml`, else `config.example.yaml` (`GITSEER_CONFIG` override). No default bootstrap password — first UI visit **Claim Bootstrap** sets username + password (suggested username from `GITSEER_AUTH_BOOTSTRAP_USERNAME`, default `admin`).

## Guidelines

- Forge-specific HTTP/types stay in `internal/forge/{gitea,github,gitlab,bitbucket,forgejo}`; shared contracts on `forge.Forge`
- Enforce repository authorization server-side (`user_repository_access` or bootstrap allow-all)
- Prefer `.yaml` for new config files
- Do not add Redis or WebSocket dependencies for V1 (SSE + self-hosted Web Push are the realtime paths)
- Ask before removing documented product features
- UI headings/buttons use Title Case
- Browser alerts: `internal/webpush` + `web/public/sw.js`; do not call external notification relays

## Tests to prefer when touching areas

- Authz and ACL
- Forge normalization
- Store upserts / integrity
- Attention rule fingerprints
- CSRF on mutating routes
- Web Push subscription upsert / VAPID key load (`internal/webpush`, `internal/store`)

## Specs

- [docs/prd-spec.md](https://github.com/ncdlabs/gitseer/blob/main/docs/prd-spec.md)
- [docs/implementation-plan.md](https://github.com/ncdlabs/gitseer/blob/main/docs/implementation-plan.md)
- [CONTRIBUTING.md](https://github.com/ncdlabs/gitseer/blob/main/CONTRIBUTING.md)
