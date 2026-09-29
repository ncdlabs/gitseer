# Contributing

Repository / Go module path: `gitseer`.

## Development

1. Install Go 1.26+ (see `go.mod`) and Node 22+.
2. `make deps` (or `npm install` at the repo root, plus `cd web && npm install`).
3. `make test`
4. Local app: `npm run start` — prints App/API URLs and claim-bootstrap hint (suggested username), opens the browser to `:5173`, Go API on `:8090`. Uses `config.yaml` when present, otherwise `config.example.yaml` (override with `GITSEER_CONFIG`). No default password — first visit **Claim Bootstrap**. Stop with `npm run stop`; restart with `npm run restart` (same banner + browser open).
5. Production-shaped binary: `make frontend && make build-go` then `./bin/gitseer serve`.

## Guidelines

- Keep the forge adapter boundary: raw wire types stay in `internal/forge/gitea` / `internal/forge/github` (domain models in `internal/models`).
- Enforce repository authorization server-side via `user_repository_access` (or bootstrap allow-all).
- Prefer `.yaml` for new config files.
- Do not add Redis or WebSocket dependencies for V1.
- Ask before removing documented product features. GitLab and Bitbucket forge clients are implemented (not Coming Soon).

## Pull requests

- Include tests for authz, forge normalization, and store upserts when touching those areas.
- Keep changes proportional; follow `docs/implementation-plan.md`.

## Releases (maintainers)

Public container + GitHub Release publishing is tag-gated. Cut via **Actions → Cut release**; see [docs/upgrade.md](docs/upgrade.md#cutting-a-public-release).
