# Features

## UI shell

- Flat ops-console layout (IBM Plex; themes: system / light / dark / gruvbox / terminal via `data-theme`; terminal uses ANSI/ASCII graphics)
- Light/dark palettes align with Gitea built-ins (`gitea-light` / `gitea-dark`)
- Optional theme sync from Gitea user settings for OAuth users (mapped themes only); manual ThemePicker stops sync
- List pages share a table/card view toggle (`lens-view-mode` in localStorage)

## Routes

| Path | Page |
|------|------|
| `/` | Dashboard |
| `/attention` | Attention queue |
| `/repositories` | Repository inventory |
| `/pull-requests` | Open pull requests |
| `/pipelines` | Workflow runs |
| `/pipelines/:id` | Run detail (jobs + graph) |
| `/actions-popout` | Active Actions detached window |
| `/settings` | Preferences / Integration / Status tabs |
| `/setup` | First-run wizard (bootstrap admin) |
| Login | Gitea OAuth + optional bootstrap |

## Dashboard

- Summary metrics from `GET /api/v1/summary?days=` (allowlist **0 / 1 / 7 / 30 / 90**, default **0** / Now; UI persists last choice in `lens-dashboard-range-days`)
- Range persisted in `lens-dashboard-range-days`
- Repositories count is inventory (not time-ranged); open PRs, attention, failed, and running respect the window (`Now` = current open/running state; failed = attention-linked only)
- Trends/breakdowns from `GET /api/v1/stats?days=` — lightweight SVG/CSS charts (no chart library); `Now` hides trends and returns current-state breakdowns only
- Day-series JSON field is `day` (not `date`)

## Attention

Discrete rule engine with severities `critical` / `warning` / `waiting`. See [Attention Engine](attention-engine.md).

## Pull requests

- Open PRs across accessible repos
- CI state from Gitea combined commit status (`/commits/{sha}/status`) on sync, with fallback from indexed workflow runs
- Live updates on `workflow_run` webhooks
- Pass / fail / pending badges on the Pull Requests tab

## Pipelines

- Workflow runs grouped by action (`repo` + `workflow_path`, fallback name)
- Expand an action to list individual runs; open a run for jobs/graph/logs
- Job logs fetched on demand (`GET /api/v1/jobs/{id}/logs`) with the caller's Gitea OAuth token (bootstrap admin uses the service token)
- Shell **Active Actions** flyout (left rail, near Sync / account): live `queued` / `waiting` / `running` runs with job-level and step-level progress bars; fed by `workflow_run` / `workflow_job` webhooks via SSE; **Pop Out** opens a dedicated browser window at `/actions-popout` (OS-draggable; run links prefer the opener)

## Settings

- Tabbed `/settings` UI: **Preferences**, **Integration**, **Status** (hash deep-links `#preferences` / `#integration` / `#status`)
- Preferences: instance name, sync history days, long-running threshold, retention windows, public URL
- Integration: Gitea URL, service token, webhook HMAC, private-network / unsigned flags, OAuth client id/secret
- Status: live integration/runtime summary from settings payload
- Secrets are write-only on GET (`*_configured` booleans); empty secret on PUT leaves unchanged; `clear_*` clears
- Dirty Cancel / Save on edit forms (unsaved indicator on Preferences / Integration tabs)
- PUT `/api/v1/settings` is bootstrap-admin + CSRF only; applies live to sync/auth/webhooks/attention/retention

## Setup wizard

Covered in [Setup Wizard](setup-wizard.md).

## Gitea UI integration

`lens install-ui` writes markers under Gitea’s custom templates so Lens opens in a new tab from nav / repo tabs. Does not leave the current Gitea page.
