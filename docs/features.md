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
| `/inbox` | Personal inbox (authored / review / failing CI / blocked-on-me) |
| `/attention` | Attention queue |
| `/repositories` | Repository inventory |
| `/repositories/:owner/:repo` | Repository detail (deep-link; pass `?instance_id=` when owner/name is ambiguous) |
| `/pull-requests` | Open pull requests |
| `/pipelines` | Workflow runs |
| `/pipelines/:id` | Run detail (jobs + graph) |
| `/actions-popout` | Active Actions detached window |
| `/settings` | Preferences / Integration / Access / Notifications / Status tabs |
| `/setup` | First-run wizard (bootstrap admin) |
| Login | Gitea OAuth + optional bootstrap |

## Dashboard

- Summary metrics from `GET /api/v1/summary?days=` (allowlist **0 / 1 / 7 / 30 / 90**, default **0** / Now; UI persists last choice in `gitseer-dashboard-range-days`)
- Range persisted in `gitseer-dashboard-range-days`
- Repositories count is inventory (not time-ranged); open PRs, attention, failed, and running respect the window (`Now` = current open/running state; failed = attention-linked only)
- Progressive stats from `GET /api/v1/stats?days=&section=` where `section` is `core` | `trends` | `duration` | `all` (default `all`): metrics from summary, breakdowns from `core`, trend charts + conclusions from `trends`, run-duration callout from `duration` last. `Now` skips trends/duration requests
- Background precache of all range presets (summary+core → trends → duration) after the active range settles; SSE invalidation re-warms the same layers after a short debounce
- Lightweight SVG/CSS charts (no chart library)
- Day-series JSON field is `day` (not `date`)

## Inbox

- Personal `/inbox` view from `GET /api/v1/inbox` (ACL-scoped): items tagged with reasons `author`, `requested_reviewer` (when attention metadata lists `requested_reviewers`), `failing_ci`, `blocked_on_me`
- Reason chips + forge filter; saved filter presets via `saved_filters` CRUD
- Command palette: **Inbox**, **Inbox · Failing CI**, **Inbox · Blocked On Me**, **Inbox · Review Requests**

## Attention

Discrete rule engine with severities `critical` / `warning` / `waiting`. See [Attention Engine](attention-engine.md). Attention and Inbox pages share **Save Filter** presets (`GET/POST/PUT/DELETE /api/v1/saved-filters`).
Optional on-demand **Log Tail** on attention items linked to a job/run (forge fetch, capped snippet, not stored).

## Repositories

- Inventory list/detail include a computed **health** rollup: open critical attention, failing latest default-branch run, stale open PRs (default 14d), CI fail rate over a window (default 7d), score/grade badge
- Detail page shows health summary plus **Failure Clusters** (failed jobs grouped by workflow path + job name over N days)

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
- **Rerun Workflow** / **Cancel Workflow** on pipeline detail and Active Actions (confirm dialogs). Gitea OAuth users call the forge with their user token; never silent service-PAT fallback for non-admin. Bootstrap admin may use the service PAT with an explicit UI warning (required for GitHub until OAuth login). Cancel only for `queued` / `waiting` / `running`.
- Shell **Active Actions** flyout (left rail, near Sync / account): live `queued` / `waiting` / `running` runs with job-level and step-level progress bars; fed by `workflow_run` / `workflow_job` webhooks via SSE; **Pop Out** opens a dedicated browser window at `/actions-popout` (OS-draggable; run links prefer the opener)

## Search

Header search (⌘/Ctrl+K) is a command palette over:

- **Commands** — Sync Now, Open / Pop Out Active Actions, Inbox reason shortcuts, theme switches, Settings tab jumps, Log Out
- **Pages** — Dashboard, Inbox, Attention, Pull Requests, Pipelines, Repositories, Settings
- **Inventory** — repositories, organizations, pull requests, pipelines (runs + job names), open attention via `GET /api/v1/search` (ACL-scoped; matches title, author, PR number, branch, commit SHA, workflow path)

## Settings

- Tabbed `/settings` UI: **Preferences**, **Integration**, **Access**, **Notifications**, **Status** (hash deep-links `#preferences` / `#integration` / `#access` / `#notifications` / `#status`)
- Preferences: instance name, sync history days, long-running threshold, **Attention Severity Overrides**, retention windows, **Apply Lab Preset** / **Apply Prod Preset**, **Purge Now**, public URL
- **Notifications** (bootstrap admin): SMTP + Slack/Discord/generic HTTPS webhooks; severity filter (default critical); immediate + optional daily digest; secrets write-only; **Send Test Notification**. Self-hosted only (ADR-015 / ADR-032).
- Attention queue: **Mute** / **Snooze 24h** / **Snooze 7d** / **Mute Until Resolved** per item
- Integration: multi-instance list (forge badge, URL, configured flags, webhook path `/api/webhooks/{gitea|github}/{id}`) with **Add Forge** / Edit / Remove; Gitea cards include **Download Gitea UI Snippets** (zip of marker-safe templates); Gitea and GitHub supported (GitLab / Bitbucket Coming Soon). Instance list/API is **bootstrap-admin only**; other users see forge summary on Status / `GET /settings`
- API: `GET/POST /api/v1/instances`, `PUT/DELETE /api/v1/instances/{id}` (bootstrap admin + CSRF); `GET/PUT /api/v1/settings` keeps preferences + a derived dual-forge snapshot for setup
- Status: live summary including database size / warn thresholds; when multiple forges of the same type exist, labels include instance name / id
- Inventory lists: forge filter chips are data-driven (All + types; per-instance chips when a type has multiple); badges show instance name when that type has more than one instance
- Secrets are write-only on GET (`*_configured` booleans); empty secret on PUT/update leaves unchanged; `clear_*` clears
- Dirty Cancel / Save on Preferences and instance edit dialogs
- PUT `/api/v1/settings` and instance writes are bootstrap-admin + CSRF only; apply live to sync/auth/webhooks/attention/retention
- Removing a forge instance hard-deletes cascaded inventory (confirm dialog states this)

## Setup wizard

Covered in [Setup Wizard](setup-wizard.md).

## Gitea UI integration

`gitseer install-ui --custom-path DIR --gitseer-url URL [--instance-id N]` writes markers under Gitea’s custom templates so GitSeer opens in a new tab from nav / repo tabs. Repo tab links target `/repositories/{owner}/{repo}` (with `?instance_id=` when `--instance-id` is set). Does not leave the current Gitea page.

Settings → Integration (Gitea instance): **Download Gitea UI Snippets** returns the same marker content as a zip (`templates/custom/extra_*.tmpl`) or concatenated text via `GET /api/v1/instances/{id}/gitea-ui-snippets`. CLI remains for in-place install on the Gitea host; restart Gitea after installing templates.
