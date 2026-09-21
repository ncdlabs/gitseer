# Contributing

Thanks for contributing to Gitea Lens.

## Development

1. Install Go 1.22+ and Node 22+.
2. `make deps` (or `npm install` at the repo root, plus `cd web && npm install`).
3. `make test`
4. Local app: `npm run start` — prints App/API URLs and bootstrap credentials (`bootstrap` / password), opens the browser to `:5173`, Go API on `:8090`. Uses `config.yaml` when present, otherwise `config.example.yaml` (override with `LENS_CONFIG`). Default local password is `lens-local` when unset. Stop with `npm run stop`; restart with `npm run restart` (same banner + browser open).
5. Production-shaped binary: `make frontend && make build-go` then `./bin/lens serve`.

## Guidelines

- Keep the forge adapter boundary: raw Gitea types stay in `internal/forge/gitea`.
- Enforce repository authorization server-side via `user_repository_access` (or bootstrap allow-all).
- Prefer `.yaml` for new config files.
- Do not add Redis or WebSocket dependencies for V1.
- Ask before removing documented product features.

## Pull requests

- Include tests for authz, forge normalization, and store upserts when touching those areas.
- Keep changes proportional; follow `docs/implementation-plan.md`.
