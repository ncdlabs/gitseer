# Features

## UI shell

- Flat ops-console layout (IBM Plex; themes: system / light / dark / gruvbox / terminal via `data-theme`; terminal uses ANSI/ASCII graphics)
- Light/dark palettes align with Gitea built-ins (`gitea-light` / `gitea-dark`)
- Optional theme sync from Gitea user settings for OAuth users (mapped themes only); manual ThemePicker stops sync
- List pages share a table/card view toggle (`gitseer-view-mode` in localStorage)

## Routes

| Path | Page |
|------|------|
| `/` | Dashboard |
| `/attention` | Attention queue |
| `/repositories` | Repository inventory |
| `/repositories/:owner/:repo` | Repository detail (deep-link; pass `?instance_id=` when owner/name is ambiguous) |
| `/pull-requests` | Open pull requests |
| `/pipelines` | Workflow runs |
| `/pipelines/:id` | Run detail (jobs + graph) |
| `/actions-popout` | Active Actions detached window |
| `/settings` | Preferences / Integration / Status tabs |
| `/setup` | First-run wizard (bootstrap admin) |
| Login | Gitea OAuth + optional bootstrap |

## Dashboard

- Summary metrics from `GET /api/v1/summary?days=` (allowlist **0 / 1 / 7 / 30 / 90**, default **0** / Now; UI persists last choice in `gitseer-dashboard-range-days`)
- Range persisted in `gitseer-dashboard-range-days`
- Repositories count is inventory (not time-ranged); open PRs, attention, failed, and running respect the window (`Now` = current open/running state; failed = attention-linked only)
- Trends/breakdowns from `GET /api/v1/stats?days=` — lightweight SVG/CSS charts (no chart library); `Now` hides trends and returns current-state breakdowns only
- Day-series JSON field is `day` (not `date`)

## Attention

Discrete rule engine with severities `critical` / `warning` / `waiting`. See [Attention Engine](attention-engine.md).

## Pull requests

- Open PRs across accessible repos
- CI state from forge commit status / checks on sync (Gitea combined status; GitHub status + Checks API via the forge client), with fallback from indexed workflow runs
- Review state (`approved` / `changes_requested`) from forge PR reviews on sync and `pull_request_review` webhooks
- Live updates on `workflow_run` / status / check webhooks
- Pass / fail / pending CI badges and review badges on the Pull Requests tab

## Pipelines

- Workflow runs grouped by action (`repo` + `workflow_path`, fallback name)
- Expand an action to list individual runs; open a run for jobs/graph/logs
- Job logs fetched on demand (`GET /api/v1/jobs/{id}/logs`) via the repo’s forge instance client: Gitea OAuth users use their stored token; GitHub (and bootstrap) use the instance service PAT
- Shell **Active Actions** flyout (left rail, near Sync / account): live `queued` / `waiting` / `running` runs with job-level and step-level progress bars; fed by `workflow_run` / `workflow_job` webhooks via SSE; **Pop Out** opens a dedicated browser window at `/actions-popout` (OS-draggable; run links prefer the opener)

## Search

Header search (⌘/Ctrl+K) is a command palette over:

- **Commands** — Sync Now, Open / Pop Out Active Actions, theme switches, Settings tab jumps, Log Out
- **Pages** — Dashboard, Attention, Pull Requests, Pipelines, Repositories, Settings
- **Inventory** — repositories, organizations, pull requests, pipelines (runs + job names), open attention via `GET /api/v1/search` (ACL-scoped; matches title, author, PR number, branch, commit SHA, workflow path)

## Settings

- Tabbed `/settings` UI: **Preferences**, **Integration**, **Status** (hash deep-links `#preferences` / `#integration` / `#status`)
- Preferences: instance name, sync history days, long-running threshold, retention windows, public URL
- Integration: multi-instance list (forge badge, URL, configured flags, webhook path `/api/webhooks/{gitea|github}/{id}`) with **Add Forge** / Edit / Remove; Gitea and GitHub supported (GitLab / Bitbucket Coming Soon). Instance list/API is **bootstrap-admin only**; other users see forge summary on Status / `GET /settings`
- API: `GET/POST /api/v1/instances`, `PUT/DELETE /api/v1/instances/{id}` (bootstrap admin + CSRF); `GET/PUT /api/v1/settings` keeps preferences + a derived dual-forge snapshot for setup
- Status: live summary; when multiple forges of the same type exist, labels include instance name / id
- Inventory lists: forge filter chips are data-driven (All + types; per-instance chips when a type has multiple); badges show instance name when that type has more than one instance
- Secrets are write-only on GET (`*_configured` booleans); empty secret on PUT/update leaves unchanged; `clear_*` clears
- Dirty Cancel / Save on Preferences and instance edit dialogs
- PUT `/api/v1/settings` and instance writes are bootstrap-admin + CSRF only; apply live to sync/auth/webhooks/attention/retention
- Removing a forge instance hard-deletes cascaded inventory (confirm dialog states this)

## Setup wizard

Covered in [Setup Wizard](setup-wizard.md).

## Gitea UI integration

`gitseer install-ui --custom-path DIR --gitseer-url URL [--instance-id N]` writes markers under Gitea’s custom templates so GitSeer opens in a new tab from nav / repo tabs. Repo tab links target `/repositories/{owner}/{repo}` (with `?instance_id=` when `--instance-id` is set). Does not leave the current Gitea page.
