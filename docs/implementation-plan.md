# GitSeer — Implementation Plan

**Status:** Living implementation blueprint (keep current; use git history for point-in-time snapshots)  
**Source of truth (product intent):** [`./docs/prd-spec.md`](./prd-spec.md)  
**As-built context:** `PROJECT_SHARED_STATE.md`, operator guides under `docs/`, `CHANGELOG.md`  
**Date:** 2026-09-20 (amended 2026-09-27 multi-forge; 2026-09-26 ADR-031/032; 2026-09-28 browser/OS Web Push + as-built refresh)  
**License target:** Apache-2.0  

This document is the implementation blueprint agents and contributors should follow. Do not silently reinterpret major PRD requirements. When repository reality conflicts with this plan, update **this file**, [`prd-spec.md`](./prd-spec.md), and `PROJECT_SHARED_STATE.md`.

---

## 1. Repository Assessment

### Current state (as of 1.0.3)

| Path | Status |
| --- | --- |
| `docs/prd-spec.md` | Living product requirements |
| `cmd/gitseer`, `internal/*`, `web/`, `migrations/` (through `00018`), `deploy/`, `integrations/` | **Present** — shipping multi-forge app |
| `go.mod`, `package.json`, Containerfile, Compose, Helm | **Present** |
| `README.md`, `LICENSE`, `SECURITY.md`, CI / release workflows | **Present** |
| `.git` | Active history on `main` |

### Findings

1. **As-built product.** GitSeer is a Go single-binary + embedded React SPA with SQLite/Postgres, multi-forge sync/webhooks, attention, inbox, outbound notifications, and in-app browser/OS alerts (SSE + Web Push).
2. **PRD and this plan are living docs** — amend them when behavior changes; do not leave greenfield fiction in place.
3. **Further forge types** beyond ADR-029 require an explicit ask.
4. **Deferred:** Redis, WebSockets, multi-arch/signed public images (see PRD §53).

### What not to do

- Forge adapters: Gitea, GitHub, GitLab, Bitbucket, Forgejo (ADR-029). Ask before adding further forge types.
- Do not add Redis for V1 (PRD ADR-011).
- Do not use WebSockets unless SSE is proven insufficient (PRD §24).
- Do not fork or patch forge source.
- Do not require per-repository GitSeer config files.
- Do not call external ncdLabs notification relays (ADR-015 / ADR-032).

---

## 2. PRD Summary and Non-Negotiable Requirements

### Product

**GitSeer** is a **self-hosted CI/CD and pull-request operations console** for configured forge instances (**Gitea, Forgejo, GitHub, GitLab, Bitbucket Cloud**). It answers: **what requires attention right now?** across accessible repositories without per-repo setup (PRD §1–§5).

### Non-negotiables (must not be weakened)

| ID | Requirement | PRD ref |
| --- | --- | --- |
| N1 | External companion; no forge fork / no forge source edits | §18, ADR-001 |
| N2 | Zero required repository configuration | §5, ADR-004 |
| N3 | Forges are systems of record; GitSeer caches/indexes | §5, ADR-002 |
| N4 | Webhooks + periodic reconciliation | §26, ADR-003 |
| N5 | Forge OAuth/OIDC identity (PKCE); optional bootstrap admin for first-run only (not forge passwords) | §19, ADR-005 |
| N6 | Server-side per-user repository authorization (not client filter) | §20, ADR-006 |
| N7 | Aggregate counters/search must not leak hidden repos | §20 |
| N8 | Go backend + React/TS frontend | §24, ADR-007/008 |
| N9 | SQLite default; PostgreSQL supported | §24, ADR-009/010 |
| N10 | No Redis requirement for V1 | ADR-011 |
| N11 | Workflow topology from YAML when runtime data insufficient | ADR-012 |
| N12 | API/UI consume GitSeer models, never raw forge payloads | §25, ADR-013 |
| N13 | Gitea UI via `extra_links.tmpl` / `extra_tabs.tmpl` only; preserve admin customizations | §18, ADR-014 |
| N14 | No ncdLabs-hosted dependency; telemetry off by default; no shared notification relay | §39, ADR-015 |
| N15 | Logs fetched on demand; not persisted by default | §43 |
| N16 | Capability detection + graceful degradation | §5 |
| N17 | Target Gitea API family ~1.26 (docs cite 1.26.4); other forges degrade by capability | §5 |
| N18 | Subpath reverse-proxy is first-class | §46 |
| N19 | Write ops (rerun/cancel) allowed with user-token rules; no silent service-PAT fallback for non-admin | §32 (amended) |
| N20 | Apache-2.0 recommended | §50 |

### MVP vs V1.0

- **MVP** (PRD §52): connect, OAuth, discovery, sync, core UI, DAG, logs, filters, search, dark mode, Gitea nav/tab, SQLite, Docker, binary, health, docs.
- **V1.0** (PRD §53): + PostgreSQL, Helm, migrations tooling, backup/restore docs, permission validation at scale, SBOM, signing, multi-arch, compatibility matrix, security review.

---

## 3. Proposed Architecture

Align with PRD §23–§25:

```text
Browser (React SPA, embedded in binary)
        │  REST /api/v1 + SSE /api/v1/events (+ Web Push for closed-tab alerts)
        ▼
┌───────────────────────────────────────────┐
│  GitSeer / gitseer (Go)                   │
│  HTTP API · Auth · Authz · Realtime SSE   │
│  Sync · Webhooks · Attention · Notify     │
│  Web Push (VAPID) · Workflows             │
│  Forge adapters: gitea/github/gitlab/     │
│                  bitbucket/forgejo        │
└───────────────────┬───────────────────────┘
                    │ encrypted secrets, normalized rows
                    ▼
              SQLite / PostgreSQL
                    ▲
         API + webhooks (per-instance tokens)
                    │
     Gitea / Forgejo / GitHub / GitLab / Bitbucket
```

### Runtime components

| Component | Responsibility |
| --- | --- |
| `cmd/gitseer` | Process entry: config, DB, HTTP, workers |
| Gitea adapter | Version/capability probe; translate Gitea ↔ GitSeer models |
| Synchronizer | Initial import, incremental sync, reconciliation loop |
| Webhook engine | Validate, dedupe, enqueue, idempotent apply |
| Attention engine | Rule eval open/update/resolve |
| Auth | OAuth code+PKCE, sessions, secure cookies |
| Authz | Per-user allowed repo set; query scoping |
| Query API | `/api/v1` DTOs, pagination, filters, OpenAPI |
| Realtime | SSE fan-out of entity change events (`workflow_run`, `workflow_job`, `pull_request`, `attention`) |
| Notify | Outbound SMTP/webhooks outbox + Web Push fan-out on attention open |
| Workflow package | YAML fetch at commit SHA, `needs` DAG, cache |
| Embed FS | Serve built `web/dist` from Go binary |

### Explicit non-components (current slice)

- No Redis / message broker
- No WebSocket gateway (unless a later ADR overturns)
- GitLab / Bitbucket / Forgejo forge clients are in-scope (ADR-029 amendment 2026-09-27)
- No CI log blob store by default

---

## 4. Architecture Decisions / ADRs

Adopt PRD §55 ADRs as binding. Additional decisions required **before or at start of coding**:

| ADR | Decision | Timing |
| --- | --- | --- |
| ADR-001…015 | As in PRD | Binding now |
| ADR-016 | **HTTP router:** `chi` (lightweight, middleware-friendly, common in Go ops tools) | Decide at foundation |
| ADR-017 | **Migrations:** `golang-migrate` or `pressly/goose` with SQL files in `migrations/` — prefer **goose** for embeddable single-binary UX | Foundation |
| ADR-018 | **DB access:** `database/sql` + `sqlc` for typed queries (SQLite + Postgres dialect parity). Avoid heavy ORM. | Foundation |
| ADR-019 | **Config:** YAML file + env overrides (`GITSEER_*`); secrets via `_FILE` suffix pattern | Foundation |
| ADR-020 | **Frontend:** Vite + React 18/19 + TS; TanStack Query + TanStack Table; React Flow for DAGs; CSS variables + small primitives (no heavy UI kit) | Foundation |
| ADR-021 | **Session store:** server-side sessions in DB (SQLite/PG); signed cookie session ID; no JWT-as-session for V1 | Auth phase |
| ADR-022 | **Credential encryption:** AES-256-GCM sealed envelope (`GSe` + version byte + nonce‖ciphertext); passphrase via `GITSEER_ENCRYPTION_KEY` (min 16 at config load; wizard paste min 24) hashed with SHA-256 to a 32-byte AES key (v1 derivation — not a password KDF); Decrypt accepts legacy unversioned blobs; rotate via `gitseer rotate-encryption-key` (re-seal DB secrets under current envelope, update on-disk key file when used) | Auth/sync |
| ADR-023 | **User repo ACL cache:** table `user_repository_access` refreshed on login + periodic + webhook-driven invalidation; **every** list/aggregate query joins/filters by it | Authz — critical |
| ADR-024 | **Webhook processing:** accept → persist raw envelope → `202`/`200` quickly → in-process worker pool (no Redis) | Sync |
| ADR-025 | **SSE:** one stream per authenticated session; events include `type`, `id`, `repo_id`, and for `attention` optional `title` / `severity` / `deep_link`; clients refetch via Query and may show Notification API banners | Realtime |
| ADR-026 | **Minimum Gitea version:** declare after capability matrix spike (see Open Questions); degrade Actions features when APIs missing | Before M1 exit |
| ADR-027 | **Module path:** `github.com/ncdlabs/gitseer` (binary `gitseer`, Helm chart/namespace `gitseer`, env `GITSEER_*`) | Foundation |
| ADR-028 | **Compose filename:** `compose.yaml` (project preference for `.yaml`) while keeping `docker-compose.yaml` symlink or doc alias if useful | Packaging |
| ADR-029 | **Multi-forge:** Gitea, GitHub, GitLab, Bitbucket, and Forgejo are in-scope `forge.Forge` implementations (shared inventory UI, per-instance sync/webhooks). Forgejo is a thin adapter over the Gitea client with distinct `forge_type` and webhook path. Service PAT remains required for sync on forges without user OAuth; per-user OAuth is ADR-031 (+ GitLab/Bitbucket/Forgejo OAuth). | Multi-forge |
| ADR-030 | **Write ops:** `RerunWorkflowRun` / `CancelWorkflowRun` on `forge.Forge` with API + UI. Non-admin users must use their per-instance forge token (`UserAccessTokenForInstance`); never silent service-PAT fallback. Bootstrap admin may use service PAT with explicit UI warning when no user token exists. | Stream 4 |
| ADR-031 | **Multi-forge OAuth identity:** Authorization Code + PKCE for Gitea, Forgejo, GitHub, GitLab, Bitbucket; tokens in `user_tokens` keyed by `(user_id, instance_id)`; optional link of multiple forge identities on one user (Forgejo reuses Gitea identity columns); ACL refresh only for instances with a user token. | Stream 6 |
| ADR-032 | **Notifications:** Outbound SMTP digests + Slack/Discord/generic/incident HTTPS webhooks via self-hosted `notification_outbox` (`internal/notify`). Secrets sealed with encryption key. No ncdLabs-hosted relay (reaffirms ADR-015). Triggers: attention open matching min severity (default critical); optional daily digest. **In-app browser/OS alerts:** SSE `attention` + Web Notification API; closed-tab Web Push via self-hosted VAPID (`internal/webpush`, migration `00018`); not the shared ncdLabs `notification-api` service. | Stream 7 |

**No ADR needed yet for:** additional forge types beyond ADR-029.

---

## 5. Data Model

Normalized GitSeer models (PRD §29). Gitea IDs stored as `external_id` separately from GitSeer UUIDs/`INTEGER` PKs.

### Core tables (proposed)

```text
instances
  id, name, base_url, version, capabilities_json,
  sync_token_ciphertext, webhook_secret_ciphertext,
  oauth_client_id, oauth_client_secret_ciphertext,
  external_url, created_at, updated_at

organizations
  id, instance_id, external_id, name, full_name, avatar_url, synced_at

repositories
  id, instance_id, org_id NULL, external_id, owner, name, full_name,
  default_branch, private, archived, empty, fork, html_url,
  permissions_mirror_json (service view only — NOT user authz),
  last_synced_at, deleted_at, renamed_from

pull_requests
  id, repo_id, external_id, number, title, body_excerpt,
  author_login, author_external_id, source_branch, target_branch,
  head_sha, base_sha, state, draft, mergeable, mergeable_state,
  review_state, ci_state, html_url, created_at, updated_at, closed_at, merged_at

workflows
  id, repo_id, path, name, external_id_or_path, last_seen_commit_sha

workflow_revisions / workflow_nodes
  workflow graph cached per (repo_id, path, commit_sha)
  nodes: job_key, name, needs[], raw_needs_expr, unknown_deps bool

workflow_runs
  id, repo_id, workflow_id NULL, external_id, name, event, branch,
  commit_sha, status, conclusion, upstream_status, upstream_conclusion,
  actor_login, html_url, started_at, completed_at, run_attempt

jobs
  id, run_id, repo_id, external_id, name, status, conclusion,
  upstream_*, runner_id, runner_name, html_url, started_at, completed_at,
  steps_json NULL (optional cache of API steps; not full logs)

attention_items
  id, instance_id, repo_id, type, severity, entity_type, entity_id,
  title, metadata_json, fingerprint UNIQUE, opened_at, resolved_at, updated_at

webhook_events
  id, instance_id, delivery_id, event_type, payload_hash,
  received_at, processed_at, status, error, attempts

sync_state
  id, instance_id, scope (instance|org|repo), scope_id,
  cursor_json, last_success_at, last_error, phase

users
  id, instance_id, gitea_user_id, login, email, display_name, avatar_url

sessions
  id, user_id, token_hash, expires_at, created_at, ip, user_agent

oauth_states
  state, code_verifier, redirect_to, expires_at

user_repository_access
  user_id, repo_id, permission, checked_at
  PRIMARY KEY (user_id, repo_id)

user_tokens (optional)
  user_id, access_token_ciphertext, refresh_token_ciphertext, expires_at
  — used for user-scoped Gitea calls (ACL refresh, future write ops)
```

### Status vocabulary (PRD §31)

- Execution: `queued | waiting | running | completed | unknown`
- Conclusion: `success | failure | cancelled | skipped | neutral | timed_out | action_required | unknown`
- Always retain `upstream_*` raw strings; unknown values must not fail JSON decode (use flexible string + map).

### Indexes (critical for authz + lists)

- `(instance_id, owner, name)` unique on repositories where `deleted_at IS NULL`
- `pull_requests (repo_id, number)` unique
- `workflow_runs (repo_id, external_id)` unique
- `jobs (repo_id, external_id)` unique
- `attention_items (fingerprint)` unique; partial index on `resolved_at IS NULL`
- `user_repository_access (user_id, repo_id)`
- Search: FTS5 (SQLite) / `tsvector` (Postgres) on PR title, repo name, workflow name — phase after basic LIKE/trigram if needed

---

## 6. Gitea Integration Strategy

### Boundary

```go
// internal/forge/forge.go — conceptual
type Forge interface {
    GetInstance(ctx context.Context) (*models.InstanceInfo, error)
    DetectCapabilities(ctx context.Context) (*models.Capabilities, error)

    ListOrganizations(ctx context.Context) ([]models.Organization, error)
    ListRepositories(ctx context.Context, opts ListReposOpts) (Page[models.Repository], error)
    GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error)

    ListPullRequests(ctx context.Context, repo models.RepoRef, opts PROpts) (Page[models.PullRequest], error)
    GetPullRequest(ctx context.Context, repo models.RepoRef, number int64) (*models.PullRequest, error)

    ListWorkflows(ctx context.Context, repo models.RepoRef) ([]models.Workflow, error)
    GetWorkflowYAML(ctx context.Context, repo models.RepoRef, path, ref string) ([]byte, error)

    ListWorkflowRuns(ctx context.Context, repo models.RepoRef, opts RunOpts) (Page[models.WorkflowRun], error)
    GetWorkflowRun(ctx context.Context, repo models.RepoRef, runID int64) (*models.WorkflowRun, error)
    ListJobs(ctx context.Context, repo models.RepoRef, runID int64) ([]models.Job, error)
    GetJob(ctx context.Context, repo models.RepoRef, jobID int64) (*models.Job, error)
    GetJobLogs(ctx context.Context, repo models.RepoRef, jobID int64) (io.ReadCloser, error)

    ListAccessibleReposForUser(ctx context.Context, userToken string, opts ListReposOpts) (Page[models.Repository], error)
    // Runners, hooks, OAuth app helpers as capability-gated methods
}
```

Only `internal/forge/gitea` implements this in V1. Raw `gitea` SDK or hand-rolled HTTP client stays **inside** that package.

### Capability detection (connection time + periodic)

Probe and store flags such as:

- `version`, `actions_api`, `workflow_run_webhook`, `workflow_job_webhook`
- `job_logs_api`, `job_steps_in_api`, `system_hooks_api`, `oauth_provider`
- `runners_api`, `admin_actions_jobs`

UI disables Runners / step views / etc. with explanation when false (PRD §5).

### Documented Gitea 1.26 touchpoints (verify in M1)

| Need | Likely API / mechanism |
| --- | --- |
| Version | `GET /api/v1/version` |
| Repos | `GET /api/v1/repos/search`, user/org repo lists |
| PRs | repo pull endpoints |
| Runs | `GET /repos/{owner}/{repo}/actions/runs` |
| Jobs | `GET .../actions/runs/{run}/jobs`, `GET .../actions/jobs/{id}` |
| Logs | `GET .../actions/jobs/{job_id}/logs` |
| Webhooks | system/org/repo hooks; events `workflow_run`, `workflow_job`, `pull_request`, `repository`, … |
| Signature | `X-Gitea-Signature` HMAC-SHA256 of body; `X-Gitea-Delivery` |
| OAuth | Gitea as OAuth/OIDC provider; Authorization Code + PKCE |
| Contents | raw workflow file at ref for DAG |

### Adapter rules

1. Never return Gitea swagger structs outside `forge/gitea`.
2. Map statuses through a single normalizer.
3. Treat missing fields as unknown, not errors.
4. Rate-limit / backoff on 429/`Retry-After`.
5. SSRF: validate `gitea.url` (scheme https/http allowlist, block link-local/metadata IPs unless explicitly allowed for lab mode).

---

## 7. Authentication and Authorization

### Authentication (PRD §19)

1. Setup registers OAuth application on Gitea (wizard-assisted; API create if admin token allows).
2. Login: Authorization Code + **PKCE**; store `state` + `code_verifier` server-side with short TTL.
3. Callback: validate state, exchange code, fetch user profile, upsert `users`, create `sessions`.
4. Session cookie: `HttpOnly`, `Secure` (when TLS/external HTTPS), `SameSite=Lax` (or `Strict` if subpath allows), `__Host-`/`__Secure-` prefix when possible.
5. Logout: revoke session row; clear cookie.
6. **Do not store passwords.**
7. Sync credential ≠ user session (PRD §19). Sync token is instance-level, encrypted.

### Authorization (PRD §20) — critical design

**Principals**

| Principal | Credential | Purpose |
| --- | --- | --- |
| Sync service | Instance token / app token | Discover & cache all visible data for the service account |
| End user | Session (+ optional user OAuth token) | Read API filtered to repos the **user** can access in Gitea |

**Enforcement**

1. Middleware loads `user_id` from session.
2. Repository visibility = membership in `user_repository_access` for that user (refreshed on login, on schedule, and when Gitea permission-related webhooks arrive if available).
3. **All** SQL for list/detail/summary/search/attention/SSE payloads must constrain `repo_id IN (SELECT repo_id FROM user_repository_access WHERE user_id = ?)`.
4. Direct object access by GitSeer ID: join through access table; return **404** (not 403) for hidden resources to reduce enumeration.
5. Summary counters computed only over accessible repos.
6. Never trust query params like `owner`/`repo` without authz check.
7. Admin “see all” only if explicitly modeled later; V1 = Gitea visibility only.

**ACL refresh strategy**

- On login: page through user’s accessible repos via user token; upsert access rows; delete stale.
- Background: refresh each active user every N hours (configurable).
- On sync discovering new repos: do not auto-grant to users; wait for user ACL refresh (or on-demand check for detail view).
- Optional on-demand: before serving a miss, call Gitea `GET /repos/{owner}/{repo}` with user token once and cache.

---

## 8. Synchronization and Webhooks

### Dual path (PRD §26–§28)

```text
Webhook → validate → persist webhook_events → enqueue → apply normalized mutation → attention recompute → SSE notify
Reconciliation every 1–60m (default 5m) → compare/update → same apply path
```

### Initial sync (PRD §27)

Ordered phases recorded in `sync_state`:

1. Version + capabilities  
2. Orgs  
3. Repositories (paginate)  
4. Default branches / metadata  
5. Workflow file discovery (Actions-enabled repos)  
6. Open PRs  
7. Recent runs (default **30 days**, configurable 7/30/90/none)  
8. Jobs for active/recent runs  
9. Attention backfill  
10. Register webhook at highest permitted scope (prefer **system** hook if admin)

### Webhook endpoint

`POST /api/webhooks/gitea/{instanceID}`

Requirements:

- Verify HMAC (`X-Gitea-Signature`) with constant-time compare  
- Cap body size  
- Dedupe on `(instance_id, delivery_id)` or payload hash  
- Idempotent handlers per event type  
- Fast ACK; async process  
- Record failures + retry with backoff  
- Never push UI state without DB commit first  

### Event handling (minimum)

| Event | Action |
| --- | --- |
| `workflow_run` / `workflow_job` | Upsert run/job; attention |
| `pull_request` (+ review variants) | Upsert PR; attention |
| `repository` | Create/rename/delete/transfer handling |
| `push` (optional) | Invalidate workflow YAML cache for default branch paths |

### Hard cases

- **Missed webhooks / downtime:** reconciliation heals  
- **Renames/transfers:** match `external_id`; update `owner/name`; keep GitSeer PK stable  
- **Deletes:** soft-delete repo; cascade hide from UI; purge per retention  
- **Permission loss:** remove from user ACL; sync account may still see — user must not  
- **Out-of-order webhooks:** compare `updated_at` / run attempt / monotonic fields; ignore stale  

### Workers

In-process goroutine pool + DB-backed queue table (`jobs_queue` or reuse `webhook_events` status). Single binary friendly; document multi-replica limitation (only one active reconciler via DB lease) for V1.

---

## 9. Backend API

Base: `/api/v1` (PRD §35)

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/summary` | Authz-scoped counters |
| GET | `/attention` | Filters: severity, type, repo |
| GET | `/repositories` | Pagination, search, org filter |
| GET | `/repositories/{owner}/{repo}` | 404 if unauthorized |
| GET | `/pull-requests` | Cross-repo; rich filters |
| GET | `/repositories/{owner}/{repo}/pull-requests/{number}` | Detail |
| GET | `/workflow-runs` | Cross-repo pipelines |
| GET | `/workflow-runs/{id}` | Incl. jobs + graph ref |
| GET | `/jobs/{id}` | Detail + steps if known |
| GET | `/jobs/{id}/logs` | Stream/proxy; sanitize; no default persist |
| GET | `/activity` | Chronological events |
| GET | `/runners` | Capability-gated |
| GET | `/system/status` | Gitea connectivity separate from ready |
| GET | `/events` | SSE |
| GET | `/search` | Global search |
| GET/POST | `/auth/*` | login start, callback, logout, me |
| POST | `/setup/*` | First-run wizard (bootstrap token / local-only) |

Also:

- `GET /health/live`, `GET /health/ready` (PRD §36)  
- `GET /metrics` Prometheus (PRD §41)  
- OpenAPI generated (e.g. `oapi-codegen` or `swag`) committed or CI-generated  

### Cross-cutting

- Error envelope: `{ "error": { "code", "message", "request_id" } }`  
- Pagination: `page`, `per_page`, `Link` or `{ items, total, page, per_page }`  
- Sorting whitelist only  
- Authn + authz middleware on all non-webhook/non-health routes  
- Request ID middleware  

---

## 10. Frontend Architecture

### Stack (PRD §24, ADR-020)

- Vite + React + TypeScript  
- TanStack Query (server state)  
- TanStack Table (PR/pipeline/repo tables)  
- React Flow (workflow DAG)  
- React Router matching PRD §34 routes  
- CSS variables; light/dark/system (PRD §47)  
- Embedded into Go via `embed.FS`

### App shell

- Nav: Dashboard, Inbox, Attention, Pull Requests, Pipelines, Repositories; Settings; Active Actions flyout  
- Default route: Dashboard (`/`) with Attention summary  
- Global command palette (search)  
- Link-out buttons to forge HTML URLs from models  

### Pages (as-built)

| Route | Implementation notes |
| --- | --- |
| `/` | Dashboard summary/stats + attention preview |
| `/inbox` | Personal ACL-scoped queue |
| `/attention` | Attention queue + mutes |
| `/pull-requests` | Open PR table |
| `/pipelines`, `/pipelines/:id` | Runs by action + run detail/DAG |
| `/repositories`, `/repositories/:owner/:repo` | Inventory + detail/health |
| `/actions-popout` | Detached Active Actions window |
| `/wallboard` | Token-gated read-only wallboard |
| `/settings` | Preferences / Integration / Access / Notifications / Status |
| `/setup` | First-run wizard |

### UX standards

- Loading / empty / error states required (PRD §58)  
- Responsive; WCAG 2.1 AA targets (PRD §48)  
- Status never by color alone  
- Accessible DAG alternative (table/list of jobs + needs)  
- `prefers-reduced-motion`  

### Data fetching

- Query keys scoped by filters  
- SSE invalidates relevant query keys  
- Logs: separate fetch; virtualized text view; ANSI sanitize in renderer  

---

## 11. Workflow DAG and Log Strategy

### DAG (PRD §14, §30)

1. From run’s `commit_sha` + workflow path, fetch YAML via Contents API (or git blob).  
2. Parse `jobs` and `needs` (string or array).  
3. Build directed graph → `WorkflowNode[]`.  
4. Cache by `(repo_id, path, commit_sha)`.  
5. Overlay runtime jobs by **job name** (and matrix instances carefully).  
6. Dynamic/`needs` expressions → mark edge/node `unknown`; do not invent edges.  
7. Frontend: React Flow; backend returns graph DTO with node statuses.

### Steps

- Prefer API `steps` on job objects when present (Gitea 1.26 job payload includes steps).  
- Log parsing for steps is **adapter fallback only**, never canonical (PRD §15).

### Logs

- Proxy stream from Gitea; optional short TTL cache in memory only.  
- Do not persist by default (PRD §43).  
- Sanitize ANSI / strip OSC escapes; render as text, never `dangerouslySetInnerHTML`.  
- Size limits + “open in Gitea” link.

---

## 12. Attention Engine

### Model

`AttentionItem` with stable `fingerprint` = hash(type, repo_id, entity_type, entity_id, …) so re-eval upserts.

### Initial rules (PRD §10)

| Condition | Severity |
| --- | --- |
| Failed CI on open PR | Critical |
| Failed workflow on default branch | Critical |
| Failed deployment workflow | Critical |
| Required check failed | Critical |
| Workflow awaiting manual action | Warning |
| Approved PR blocked by CI | Warning |
| PR waiting required review | Waiting |
| Approved PR behind target | Warning |
| Runner issues while queued | Warning |
| Long-running over threshold | Warning |
| Merge conflict on active PR | Warning |

### Lifecycle

- Evaluate on sync apply + webhook + periodic sweep  
- Open when condition true; update metadata; **resolve** when false (`resolved_at`)  
- Dashboard shows unresolved; history retained per retention  

### Config

V1: hardcoded thresholds in config file (e.g. long-running duration). User-configurable severity later.

---

## 13. Gitea Native UI Integration

### Artifacts under `integrations/gitea/`

- Snippets for `extra_links.tmpl`, `extra_tabs.tmpl`  
- Optional `gitseer.css`  
- Marker comments: `{{/* gitseer:begin */}}` … `{{/* gitseer:end */}}`

### CLI

```text
gitseer install-ui --gitea-custom $GITEA_CUSTOM --gitseer-url https://...
gitseer uninstall-ui --gitea-custom $GITEA_CUSTOM
```

### Rules (PRD §18)

- Detect existing files; **never silent overwrite** of non-GitSeer content  
- Insert/replace only within GitSeer markers  
- If file exists without markers, append GitSeer block or abort with instructions  
- Uninstall removes only GitSeer-marked sections  

### Repository page URL

`/repositories/{owner}/{repo}` (and `/gitseer/...` alias if reverse-proxied under Gitea path).

---

## 14. Deployment and Packaging

| Artifact | Detail |
| --- | --- |
| OCI image | `ghcr.io/ncdlabs/gitseer` multi-arch amd64/arm64 |
| Compose | `deploy/compose/compose.yaml` — port 8090, volume `/data` |
| Binary | `goreleaser` or `go build` matrix: linux/mac amd64+arm64, windows amd64 |
| Helm | `deploy/helm/gitseer` — Deployment, Service, PVC/PG options, Ingress subpath |
| Secrets | env or `*_FILE` |

First-run wizard persists config into DB/`/data/config.yaml` as designed in foundation.

---

## 15. Security

| Area | Plan |
| --- | --- |
| Secrets at rest | AES-GCM; key from env; never log | 
| Webhook auth | HMAC verification |
| OAuth | State + PKCE |
| CSRF | SameSite cookies + CSRF token for state-changing browser POSTs |
| CSP | Strict default; no inline scripts in prod builds |
| Cookies | Secure, HttpOnly, scoped path for subpath deploys |
| XSS | React text escaping; log sanitization |
| SSRF | URL validation for Gitea base URL |
| Authz | Server-side only |
| Rate limit | Login, webhook, and API abusive clients |
| Supply chain | CI scan, SBOM, signed images/releases for V1.0 |

---

## 16. Observability

- Structured JSON logs: time, level, component, request_id, delivery_id, duration, error class  
- Never log tokens, Authorization headers, or full job logs by default  
- `/metrics` as in PRD §41 — no private repo names unless opted in  
- `/health/live` process up; `/health/ready` DB + migrations + config  
- `/api/v1/system/status` includes Gitea reachability  

---

## 17. Testing Strategy

| Layer | Scope |
| --- | --- |
| Go unit | Normalizers, YAML DAG, attention, authz SQL helpers, webhook dedupe, config |
| Frontend unit | Vitest + Testing Library for tables, status components |
| Integration | Testcontainers/Podman disposable Gitea; sync + webhook fixtures |
| DB | SQLite + Postgres suite |
| OAuth | Mock OIDC server or Gitea oauth in compose |
| Authz isolation | Two users, overlapping/non-overlapping repos; counters |
| Playwright | Login, attention, PR filter, pipeline DAG, logs, repo page |
| Compat matrix | CI against supported Gitea versions (declare after spike) |

---

## 18. Open-Source Project Readiness

Create before public launch (can land progressively):

- `README.md` — positioning from PRD §51  
- `LICENSE` — Apache-2.0  
- `SECURITY.md`, `CONTRIBUTING.md`, `CHANGELOG.md`  
- `docs/architecture.md`, `docs/install.md`, `docs/upgrade.md`, `docs/backup-restore.md`  
- `.github/ISSUE_TEMPLATE/*`, `pull_request_template.md`  
- Code of conduct optional  

---

## 19. Risks and Mitigations

| Risk | Impact | Likely cause | Mitigation | When |
| --- | --- | --- | --- | --- |
| Gitea version skew | Features break or missing | Actions APIs landed recently (~1.24–1.26) | Capability flags; documented min version; degrade UI | **Before M1 exit** — version spike |
| Incomplete Actions APIs | Missing runs/jobs/org-wide lists | API gaps / permissions | Per-repo sync fallback; feature flags | Incremental |
| Step-level data thin | Job detail weak | Steps missing in some payloads | Show job status; optional log parse later | Incremental |
| DAG reconstruction hard | Wrong graph | `needs` expressions, matrix, reusable workflows | Unknown nodes; cache per commit; no guessing | Incremental; parser tests early |
| Webhook coverage gaps | Stale UI | No system hook permission; missing events | Reconciliation; setup wizard checks | Incremental |
| User authz bugs | Data leak | Filtering only in UI / wrong joins | Central authz helper; mandatory tests; 404 hidden | **Design before M4**; tests gate |
| OAuth setup friction | Install fail | Redirect URI / subpath mistakes | Wizard validation; docs | Incremental |
| Subpath proxy | Broken assets/auth redirects | Path prefix mishandled | `external_url` + prefix middleware; e2e | **Test from M0 shell** |
| Template install clobber | Admin anger | Overwrite custom tmpl | Markers + detect/abort | M8 careful design |
| Large instance sync | Slow/API ban | 1k repos × history | Incremental cursors, concurrency limits, backoff | Perf after M3 |
| History volume | DB bloat | 180d retention × many runs | Retention job; configurable depth | Incremental |
| SQLite concurrency | Write stalls | Webhook + reconcile + UI | WAL mode; short tx; single writer queue | Foundation |
| PG parity | Prod-only bugs | Dialect drift | sqlc dual dialect / integration tests both | From M2 |
| Log size | OOM / slow UI | Huge job logs | Stream + truncate UI + byte cap | M6 |
| Rate limits | Sync lag | Aggressive polling | 429 backoff; webhook-first | M3 |
| Renames/deletes | Dupes / 404s | owner/name as PK mistake | External ID as identity | M2 schema |
| Workflow changes per commit | Wrong DAG | Using default branch YAML for old run | Always fetch at run commit SHA | M6 |

---

## 20. Open Questions / Validation Required

Only genuine unknowns; verify as noted. Do not block overall sequencing.

| # | Question | How to verify |
| --- | --- | --- |
| Q1 | Exact **minimum Gitea version** for workflow_run/job APIs + webhooks usable in production | Spike against 1.22–1.26 containers; record matrix in docs |
| Q2 | Whether **system hook** creation via API is available to typical admin tokens vs UI-only | Call admin hooks API on test instance; document fallback to org/repo hooks |
| Q3 | Best **user-scoped repo list** endpoint + pagination for ACL (search vs user repos vs org loops) | Measure completeness vs Gitea UI permissions on private/org/team repos |
| Q4 | Whether job **steps** are always populated in API for failed jobs across versions | Fixture runs on matrix versions |
| Q5 | OAuth app creation via API vs manual for non-admin installers | Try `POST` oauth apps with limited token |
| Q6 | Runner list APIs sufficient for V1 Runners page or degrade to “not available” | Inspect admin/org/repo runner endpoints |
| Q7 | Subpath: does Gitea OAuth redirect allow GitSeer under `/gitseer` without extra Gitea config | End-to-end with Traefik/Caddy path strip |
| Q8 | Module/org GitHub path final (`ncdlabs/gitseer`) and GHCR permissions | Confirm org before first release |

Minor unknowns must not stall M0–M2.

---

## 21. Milestones

### M0 — Repository and architecture baseline

- **Scope:** Go module, cmd skeleton, config, logging, chi router, health, embed placeholder, Vite React shell, Makefile/task, CI lint stub, LICENSE/README draft, `compose` for GitSeer-only  
- **Key paths:** `cmd/gitseer`, `internal/config`, `internal/httpapi` (or `internal/server`), `web/`, `Dockerfile`, `.github/workflows/ci.yaml`  
- **Deps:** none  
- **Tests:** config load unit tests; health handler test  
- **Exit:** `go run ./cmd/gitseer` serves `/health/live` + empty UI shell  

### M1 — Gitea connectivity and discovery

- **Scope:** `forge` interface + gitea client; version/capabilities; list orgs/repos; setup “test connection”  
- **Key paths:** `internal/forge`, `internal/forge/gitea`  
- **Deps:** M0  
- **Tests:** client normalization with recorded HTTP fixtures (vcr/httptest)  
- **Exit:** CLI or debug endpoint prints version + repo count for a token  

### M2 — Normalized persistence

- **Scope:** migrations, SQLite, models, repos/PRs/runs/jobs tables, sqlc queries  
- **Key paths:** `migrations/`, `internal/database`, `internal/models`, `internal/store`  
- **Deps:** M0  
- **Tests:** migrate up/down SQLite; unique constraints  
- **Exit:** upsert repository round-trip  

### M3 — Synchronization and webhooks

- **Scope:** initial sync, reconcile loop, webhook endpoint, dedupe, worker, retention stub  
- **Key paths:** `internal/sync`, `internal/webhooks`  
- **Deps:** M1, M2  
- **Tests:** webhook signature + idempotency; sync against disposable Gitea (can start as fixtures)  
- **Exit:** webhook updates a run row; reconcile repairs deliberate gap  

### M4 — Authentication and authorization

- **Scope:** OAuth PKCE, sessions, ACL table, middleware, scoped summary API  
- **Key paths:** `internal/auth`, `internal/authz`  
- **Deps:** M2 (M1 for Gitea OAuth)  
- **Tests:** authz isolation tests **required**  
- **Exit:** two users see different repo sets/counters  

### M5 — Core cross-repository UI

- **Scope:** Attention shell (even if rules partial), PRs, Pipelines list, Repositories, Settings read-only, search v1, dark mode  
- **Key paths:** `web/src/pages/*`, `internal/api`  
- **Deps:** M3, M4  
- **Tests:** Playwright smoke; API handler tests  
- **Exit:** logged-in user browses authz-filtered lists  

### M6 — Workflow DAG and logs

- **Scope:** YAML loader, parser, React Flow, job detail, log stream viewer  
- **Key paths:** `internal/workflows`, pipeline detail UI  
- **Deps:** M5  
- **Tests:** parser unit tests; Playwright DAG  
- **Exit:** failed run shows graph + openable logs  

### M7 — Attention engine

- **Scope:** full rule set, resolve lifecycle, Overview prioritization  
- **Key paths:** `internal/attention`  
- **Deps:** M3, M5  
- **Tests:** rule unit tests with fixtures  
- **Exit:** fail a PR check → Critical item → fix → resolved  

### M8 — Gitea native integration

- **Scope:** install-ui/uninstall-ui, extra_links/tabs, Repository page entry, subpath docs/tests  
- **Key paths:** `integrations/gitea`, `cmd/gitseer` subcommands  
- **Deps:** M5  
- **Tests:** installer dry-run on sample custom dir with pre-existing tmpl  
- **Exit:** markers preserve admin content; tab links to GitSeer  

### M9 — Packaging and deployment

- **Scope:** multi-arch image, binaries, Helm, compose, GHCR, Goreleaser  
- **Key paths:** `deploy/`, `.goreleaser.yaml`  
- **Deps:** M5+  
- **Tests:** image boots; helm template lint  
- **Exit:** documented `podman compose up` path works  

### M10 — Hardening and V1 release

- **Scope:** Postgres parity, retention, backup docs, security review, SBOM, signing, compat matrix, upgrade test  
- **Deps:** M1–M9  
- **Exit:** PRD §53 checklist satisfied  

---

## 22. File/Package Build Map

```text
/
├── cmd/gitseer/                 # serve, version, install-ui, uninstall-ui, backup, restore, rotate-encryption-key
├── internal/
│   ├── api/                     # /api/v1 HTTP handlers
│   ├── auth/ / authz/
│   ├── attention/
│   ├── backup/
│   ├── config/
│   ├── crypto/
│   ├── database/
│   ├── forge/{gitea,github,gitlab,bitbucket,forgejo,all}/
│   ├── models/
│   ├── notify/                  # outbound outbox worker
│   ├── webpush/                 # VAPID Web Push
│   ├── realtime/                # SSE hub
│   ├── server/                  # router, embed UI
│   ├── settings/
│   ├── store/                   # sqlc + hand-written dynamic SQL
│   ├── sync/
│   ├── webhooks/
│   ├── workflows/
│   ├── retention/
│   ├── uiinstall/
│   ├── metrics/ / ratelimit/ / log/ / theme/
├── migrations/ (+ postgres/)
├── web/                         # Vite React app (+ public/sw.js)
├── integrations/gitea/
├── deploy/{docker,helm}/
├── docs/
├── .github/workflows/
├── compose.yaml
├── go.mod
└── PROJECT_SHARED_STATE.md
```

### Package responsibilities (keep boundaries boring)

- **forge:** only place that speaks forge protocols  
- **store:** only place that speaks SQL  
- **api:** HTTP + authz checks + DTO mapping from models  
- **sync/webhooks:** mutate via store; trigger attention + realtime + notify  
- **notify / webpush:** outbound channels and browser/OS push  
- **web:** consumes `/api/v1` only  

Avoid micro-packages like `internal/pr` / `internal/job` unless a file grows unwieldy.

---

## 23. Ordered Implementation Backlog

Dependencies noted as `(needs: N)`.

1. Initialize Go module `github.com/ncdlabs/gitseer`, `cmd/gitseer` stub, Makefile.  
2. Implement `internal/config` (YAML + `GITSEER_*` env overrides + `*_FILE` secrets).  
3. Structured JSON logging + request ID middleware (`internal/log`, server middleware).  
4. HTTP server skeleton with chi: `/health/live`, `/health/ready` (ready = config loaded; DB later).  
5. Database open helper + SQLite WAL; Postgres DSN path stub.  
6. Migration runner (goose) embedded; first migration `schema_migrations` only.  
7. Vite React TS shell with routing placeholders and dark/system theme toggle.  
8. Embed `web/dist` in Go; root handler serves SPA; API under `/api`.  
9. Dockerfile + `compose.yaml` mounting `/data`.  
10. CI workflow: go test, golangci-lint, frontend lint/build.  
11. `LICENSE` Apache-2.0 + minimal `README.md`.  
12. Define `internal/models` domain types (Instance, Repo, PR, Run, Job, Attention, …).  
13. Define `internal/forge.Forge` interface (Gitea-only methods needed for M1–M3).  
14. Implement Gitea HTTP client: version, rate-limit backoff, SSRF URL checks.  
15. Capability detection + store JSON on instance row.  
16. List organizations + repositories pagination → models.  
17. Migrations for `instances`, `organizations`, `repositories`, `sync_state`.  
18. Store upserts for org/repo; soft-delete/rename fields.  
19. Migrations for PRs, workflows, runs, jobs, webhook_events, attention_items.  
20. Gitea mappers for PRs, runs, jobs + status normalizer + fixture tests.  
21. Initial sync orchestrator phases 1–9 (history_days config).  
22. Reconciliation loop with interval config + DB lease.  
23. Webhook HTTP handler: signature, size limit, persist, ACK.  
24. Webhook workers: idempotent apply for workflow_* and pull_request.  
25. Deduplication + retry/error recording.  
26. API list endpoints for repositories (service auth temporary) + pagination.  
27. OAuth PKCE login/callback/logout + `users`/`sessions`/`oauth_states`.  
28. Secure cookie session middleware; `/api/v1/auth/me`.  
29. User ACL sync into `user_repository_access`.  
30. Authz scope helper; apply to all list/detail/summary queries; isolation tests.  
31. `GET /api/v1/summary` and `/attention` (even with empty rules).  
32. `GET /api/v1/pull-requests`, `/workflow-runs`, filters/sort.  
33. SSE hub + `/api/v1/events`; wire invalidate after sync apply.  
34. Frontend: auth gate, Overview/Attention, Repositories table (TanStack).  
35. Frontend: Pull Requests table + PR detail shell.  
36. Frontend: Pipelines table + run detail shell.  
37. Workflow YAML fetch at commit + `needs` parser + DAG tests.  
38. API returns graph DTO; React Flow visualization + accessible list fallback.  
39. Job detail + log proxy endpoint + ANSI-safe log viewer.  
40. Attention rule engine + resolve lifecycle + UI prioritization.  
41. Global search API + frontend palette (authz-scoped).  
42. Activity feed API + page.  
43. Runners API/UI with capability degradation.  
44. Settings pages + resync trigger + retention config.  
45. Setup wizard API + UI (connect, validate, OAuth, webhook, history depth).  
46. Subpath/`external_url` middleware; asset base path; proxy header tests.  
47. `install-ui` / `uninstall-ui` with marker-safe tmpl edits.  
48. Repository page page polish + Gitea tab link.  
49. Retention job for runs/webhooks/attention history.  
50. Prometheus `/metrics`.  
51. PostgreSQL migrations parity + CI job.  
52. Helm chart + binary release via Goreleaser.  
53. Integration suite vs disposable Gitea; Playwright critical paths.  
54. Security hardening pass (CSP, CSRF, secret scan).  
55. Docs: architecture, install, upgrade, backup/restore; SECURITY/CONTRIBUTING/CHANGELOG.  
56. Compat matrix + SBOM + signing for V1.0.  

---

## 24. Recommended First Build Slice

**Goal:** smallest vertical slice that proves the architecture.

```text
GitSeer boots
→ connects to Gitea
→ discovers repositories
→ stores normalized repository state
→ exposes an authenticated API
→ displays repositories in the frontend
```

### Backend

1. Config + logger + HTTP server + health.  
2. SQLite + migrations for `instances` + `repositories` (+ minimal `users`/`sessions` if auth in-slice).  
3. Gitea client: version + list repos.  
4. One-shot sync (or `POST /api/v1/setup/sync-repos`) upserting repositories.  
5. `GET /api/v1/repositories` with session auth **or** temporary setup token documented as interim — prefer **minimal OAuth or shared setup password** so authz path starts correctly.  
   - **Preferred:** bootstrap mode: single local admin session until OAuth configured, still storing repos under instance; mark authz “allow all for bootstrap user” explicitly.  
6. Do **not** yet build webhooks, DAG, attention rules, or Helm.

### Frontend

1. App shell + Repositories page listing `owner/name`, default branch, private flag.  
2. Basic dark mode.  
3. Loading/empty/error states.

### Database

- `instances`, `repositories` only (plus sessions if auth included).

### Tests

- Config unit test  
- Gitea repo mapper fixture test  
- Store upsert integration test (SQLite)  
- Handler test for `GET /repositories`  
- Optional: httptest against recorded Gitea list response  

### Acceptance criteria

1. `podman compose up` (or local `go run`) serves UI on `:8090`.  
2. With `GITSEER_GITEA_URL` + token, operator triggers sync.  
3. DB contains normalized rows (GitSeer IDs ≠ only Gitea IDs).  
4. UI shows repository list from GitSeer API, not direct browser→Gitea calls.  
5. Health endpoints respond; logs are JSON without secrets.  
6. No Redis, no WebSockets, no Gitea source changes.

### Explicitly out of slice

Webhooks, PRs, Actions, Attention, DAG, Helm, Postgres, Gitea template install, Playwright full suite.

---

## Appendix A — PRD development sequence mapping

| PRD §59 phase | This plan |
| --- | --- |
| Phase 1 Foundation | M0 + backlog 1–11 |
| Phase 2 Discovery | M1–M2 + 12–20 |
| Phase 3 Synchronization | M3 + 21–25 |
| Phase 4 Core UI | M5 + 31–36 |
| Phase 5 Pipeline visualization | M6 + 37–39 |
| Phase 6 Attention | M7 + 40 |
| Phase 7 Gitea integration | M4 + M8 + 27–30, 45–48 |
| Phase 8 Distribution | M9 + 52 |
| Phase 9 Hardening | M10 + 53–56 |

Note: PRD lists OAuth in Phase 7; this plan **pulls Authz to M4 before Core UI (M5)** so the UI never ships without server-side filtering. That is an intentional sequencing refinement, not a PRD contradiction.

## Appendix B — Conflicts with repository

**None.** Repository is empty aside from the PRD; plan follows PRD structure.

---

*End of implementation plan.*
