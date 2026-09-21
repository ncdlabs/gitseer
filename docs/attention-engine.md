# Attention Engine

Package: `internal/attention`. Implements discrete PRD §10 rules. Severities are only:

- `critical`
- `warning`
- `waiting`

Legacy fingerprints (e.g. blanket `open_pull_request`) are resolved on evaluate. Periodic sweep runs about every **10 minutes**. Long-running threshold defaults to **2 hours** (`attention.long_running_after`).

## Rule matrix

| Type key | Typical severity | Trigger (summary) |
|----------|------------------|-------------------|
| `failed_default_branch_workflow` | critical | Failed/timed-out run on repo default branch |
| `deployment_workflow_failure` | critical | Failed/timed-out run whose name/path/event looks like deploy/release/prod/cd |
| `pr_ci_failure` | critical | Open PR with CI state failure |
| `required_check_failed` | critical | Open PR `mergeable_state=unstable` with CI failure or review pressure (best-effort) |
| `approved_blocked_by_ci` | critical | Approved-ish PR blocked by failing CI |
| `awaiting_manual` | warning | Run/job waiting or `action_required` |
| `long_running_workflow` | warning | Incomplete run older than threshold |
| `awaiting_review` | waiting | Open PR waiting for required review |
| `approved_behind_target` | waiting | Approved PR behind target branch |
| `merge_conflict` | waiting | Open PR with merge conflict |
| `runner_unavailable_queued` | — | **No-op** until forge exposes runner signals |

## Evaluation entry points

- `EvaluateRun` — default-branch failure, deploy failure, awaiting manual, long-running
- `EvaluatePullRequest` — CI failure, required check, approved-blocked, awaiting review, behind target, merge conflict
- `EvaluateJob` — awaiting manual (job), runner unavailable (stub)

Items are upserted by fingerprint and resolved when the condition clears.
