# Attention Engine

Package: `internal/attention`. Severities are only:

- `critical`
- `warning`
- `waiting`

Legacy fingerprints (e.g. blanket `open_pull_request`) are resolved on evaluate. Periodic sweep runs about every **10 minutes**. Long-running threshold defaults to **2 hours** (`attention.long_running_after`). Bootstrap admins can override per-rule severity under Settings → Preferences (**Attention Severity Overrides**); overrides live in `attention_rule_overrides`.

## Rule matrix

| Type key | Typical severity | Trigger (summary) |
|----------|------------------|-------------------|
| `failed_default_branch_workflow` | critical | Failed/timed-out run on repo default branch |
| `deployment_workflow_failure` | critical | Failed/timed-out run whose name/path/event looks like deploy/release/prod/cd |
| `pr_ci_failure` | critical | Open PR with CI state failure |
| `required_check_failed` | critical | Open PR `mergeable_state=unstable` with CI failure or review pressure (best-effort) |
| `approved_blocked_by_ci` | warning | Approved-ish PR blocked by failing CI |
| `awaiting_manual` | warning | Run/job waiting or `action_required` |
| `long_running_workflow` | warning | Incomplete run older than threshold |
| `awaiting_review` | waiting | Open PR waiting for required review |
| `approved_behind_target` | warning | Approved PR behind target branch |
| `merge_conflict` | warning | Open PR with merge conflict |
| `runner_unavailable_queued` | warning | Best-effort: queued/waiting job whose conclusion, message, labels, or steps contain a positive offline/unavailable runner signal. Ordinary queued jobs do **not** open attention (forges often cannot signal this; see Status capability matrix) |

## Mutes / snooze

Table `attention_mutes`:

- Per-item mute by fingerprint from Attention UI: **Mute** → **Snooze 24h** / **Snooze 7d** / **Mute Until Resolved**
- Bootstrap-admin mutes are **global** (`user_id` NULL): evaluate skips opening matching fingerprints; current item is resolved immediately
- Non-admin mutes are **personal**: list hides the item for that user; evaluate still opens for everyone else
- `until_at` NULL = until resolved — cleared when the engine resolves that fingerprint
- Timed mutes expire; next evaluate reopens if the condition remains true

APIs: `POST/DELETE /api/v1/attention/{id}/mute`, `GET/PUT /api/v1/attention/rule-overrides` (PUT bootstrap-admin + CSRF).

## Log Tail (on-demand)

`GET /api/v1/attention/{id}/log-snippet` fetches a capped log tail from the forge for a failed job linked to the item (`entity_type` job/run or `job_id`/`run_id` in metadata). Default ~4 KiB, max 16 KiB; **not stored** in GitSeer by default. UI: **Log Tail** on Attention cards when a job/run is resolvable.

## Evaluation entry points

- `EvaluateRun` — default-branch failure, deploy failure, awaiting manual, long-running
- `EvaluatePullRequest` — CI failure, required check, approved-blocked, awaiting review, behind target, merge conflict
- `EvaluateJob` — awaiting manual (job), runner unavailable (best-effort positive signal)

Items are upserted by fingerprint and resolved when the condition clears. Global mutes short-circuit `open()`.

## Personal inbox

`GET /api/v1/inbox` surfaces ACL-scoped attention and open PRs for the current user with reason tags: `author`, `requested_reviewer` (when attention `metadata_json` includes `requested_reviewers`), `failing_ci`, `blocked_on_me` (changes requested / merge conflict / approved-blocked on authored PRs, or review request metadata). UI: `/inbox` with saved filter presets (`saved_filters`).
