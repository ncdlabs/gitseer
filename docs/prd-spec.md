# GitSeer

**Product Requirements Document & Technical Specification**  
**Status:** Draft for Implementation  
**License:** Apache-2.0 recommended  
**Distribution:** Public open-source project  
**Working repository:** `github.com/ncdlabs/gitseer`  
**Product type:** Self-hosted Gitea CI/CD and Pull Request Operations Console

---

# 1. Executive Summary

GitSeer is an open-source companion application for Gitea that provides a single operational view of CI/CD activity, pull requests, repository health, workflow failures, approvals, and related developer activity across an entire Gitea instance.

The primary problem GitSeer solves is fragmentation.

Gitea provides repository-level views for pull requests and Actions, but users managing multiple repositories must navigate into individual projects to understand overall CI/CD and pull-request health. GitSeer aggregates that information into a unified operational console designed to answer:

> **What requires my attention right now?**

GitSeer will integrate deeply enough with Gitea to appear and behave like a native Gitea capability while remaining architecturally separate from the Gitea codebase.

The central architectural principle is:

> **Native experience. External architecture.**

GitSeer will run as an independent service, communicate with Gitea through supported APIs and webhooks, integrate into Gitea's navigation and repository UI using supported customization hooks, and authenticate users through Gitea.

It will not require a custom Gitea fork.

It will not require modification of individual repositories.

It will not require users to add workflow steps, configuration files, agents, or badges to each repository.

A new installation should require only a Gitea URL, appropriate credentials, and a GitSeer instance. Once connected, GitSeer should automatically discover accessible repositories, workflows, open pull requests, current CI status, and recent historical activity.

---

# 2. Product Vision

GitSeer should become the operational cockpit for teams running CI/CD through Gitea.

Instead of:

`Repository → Actions → Workflow → Run → Job → Logs`

repeated across many repositories, GitSeer should provide:

`Gitea Instance → Everything requiring attention`

The application should make it possible to understand the state of dozens or hundreds of repositories from one screen.

GitSeer should answer questions such as:

- Which repositories currently have failing CI?
- Which pull requests are ready to merge?
- Which pull requests are blocked by failing checks?
- Which pull requests are waiting for review?
- Which pipelines are currently running?
- Which workflow jobs failed?
- What changed since the previous successful run?
- Which jobs are unusually slow?
- Which runners appear unavailable or overloaded?
- Which deployments are waiting for approval?
- What failed most recently?
- What needs human intervention?

GitSeer is primarily an **observability and operations layer**, not a CI engine.

Gitea remains the system of record.

---

# 3. Goals

## 3.1 Primary Goals

GitSeer must provide a unified view of CI/CD activity across all repositories visible to the connected Gitea installation.

GitSeer must aggregate pull-request state across repositories.

GitSeer must automatically discover repositories and workflows.

GitSeer must update in near real time through Gitea webhooks.

GitSeer must maintain eventual consistency through API reconciliation.

GitSeer must visually represent workflow/job dependencies.

GitSeer must make failures immediately visible and easy to investigate.

GitSeer must support direct navigation back into the corresponding Gitea repository, pull request, workflow, run, commit, and job.

GitSeer must integrate with Gitea authentication.

GitSeer must respect Gitea repository permissions.

GitSeer must install without modifying Gitea source code.

GitSeer must support simple self-hosting.

GitSeer must remain useful without external SaaS dependencies.

---

# 4. Non-Goals

The initial product will not:

- replace Gitea Actions;
- execute CI/CD jobs;
- operate its own runner infrastructure;
- replace Git;
- replace Gitea pull requests;
- edit workflow YAML graphically;
- provide an independent source-control platform;
- require repository-specific configuration;
- require a cloud service controlled by the GitSeer project;
- require telemetry to function;
- require users to create GitSeer accounts separate from Gitea.

GitSeer may eventually initiate supported Gitea operations such as reruns or cancellations, but Gitea remains authoritative.

---

# 5. Product Principles

## Zero repository configuration

Repositories should appear automatically.

There should be no required:

`gitseer.yml`

repository plugin,

workflow action,

badge,

agent,

or per-repository installation procedure.

## Read-first architecture

The default installation should require the minimum privileges necessary to inspect repositories, pull requests, Actions, workflows, and relevant metadata.

Write capabilities must be separately permissioned.

## Capability detection

GitSeer should discover what a connected Gitea instance supports rather than assuming every installation behaves identically.

The initial development target should be the current Gitea 1.26 API family. The current published API documentation is 1.26.4.

GitSeer must detect Gitea version and supported API capabilities during connection.

## Graceful degradation

If a connected version of Gitea lacks a capability, GitSeer should disable that feature and explain why rather than failing globally.

## Gitea remains authoritative

GitSeer caches and indexes Gitea data to provide fast cross-repository views, but it is not the canonical source for repository or workflow state.

## Self-hosting first

GitSeer must work fully in an isolated environment without dependence on ncdLabs, GitHub, analytics platforms, cloud databases, or external identity providers.

---

# 6. User Personas

## Developer

Needs to know whether their pull requests are passing CI and whether anything needs action.

## Technical Lead

Needs visibility across a group of repositories and active pull requests.

## DevOps / Platform Engineer

Needs to understand workflow health, failing jobs, runtime duration, runners, and systemic CI issues.

## Engineering Manager

Needs a concise view of repository health, blocked PRs, and delivery activity.

## Gitea Administrator

Needs an easy-to-install application that automatically discovers the instance without extensive configuration.

---

# 7. Core User Experience

The default page is not a repository list.

The default page is **Attention**.

Example:

```
GitSeer

47 repositories     8 open PRs
3 failing           4 workflows running
2 blocked PRs       1 waiting for approval


NEEDS ATTENTION
────────────────────────────────────────────────────

CRITICAL

coThink Chat
PR #311
Android build failed
package-android
8 minutes ago


WARNING

Meridian Client
PR #184
Approved, but branch is behind main
12 minutes ago


WAITING

AttendeeSync
Deploy Production
Manual approval required
22 minutes ago


RUNNING
────────────────────────────────────────────────────

Meridian API
PR #92
Tests
4 / 7 jobs complete


RECENTLY COMPLETED
────────────────────────────────────────────────────

✓ ChangePress       4 minutes ago
✓ Assure            11 minutes ago
✓ WPAPM             19 minutes ago
```

The purpose of this screen is not to show everything.

It is to surface **what matters**.

---

# 8. Navigation

The primary GitSeer navigation should contain:


| View          | Purpose                                     |
| ------------- | ------------------------------------------- |
| Overview      | Operational summary and attention queue     |
| Pull Requests | All accessible PRs across all repositories  |
| Pipelines     | Active and historical workflow runs         |
| Repositories  | Repository health and CI summary            |
| Runners       | Runner state and activity where supported   |
| Activity      | Chronological cross-repository event stream |
| Settings      | Instance integration and GitSeer configuration |


Repository-specific GitSeer views should additionally be available when launched from Gitea repository tabs.

---

# 9. Overview Dashboard

The Overview is the main operational console.

It should display summary counters for:


| Metric                         | Example |
| ------------------------------ | ------- |
| Accessible repositories        | 47      |
| Open pull requests             | 12      |
| Pull requests requiring review | 4       |
| Blocked pull requests          | 3       |
| Failed workflows               | 2       |
| Running workflows              | 5       |
| Waiting workflows              | 1       |
| Recently successful workflows  | 18      |


The dashboard must prioritize actionable events over passive information.

---

# 10. Attention Engine

GitSeer should maintain an internal normalized **Attention Item** model.

Initial attention conditions include:


| Condition                                     | Severity |
| --------------------------------------------- | -------- |
| Workflow failure on default branch            | Critical |
| CI failure on open PR                         | Critical |
| Deployment workflow failure                   | Critical |
| Required CI check failed                      | Critical |
| Workflow awaiting manual intervention         | Warning  |
| Approved PR blocked by CI                     | Warning  |
| PR passing CI but waiting for required review | Waiting  |
| Approved PR behind target branch              | Warning  |
| Runner unavailable while jobs are queued      | Warning  |
| Long-running workflow exceeding threshold     | Warning  |
| Merge conflict on active PR                   | Warning  |


Severity rules should eventually be configurable.

Attention items must automatically resolve when their underlying condition disappears.

---

# 11. Pull Requests View

The PR view aggregates pull requests across all visible repositories.

Required columns:


| Field                |
| -------------------- |
| Repository           |
| PR number            |
| Title                |
| Author               |
| Source branch        |
| Target branch        |
| Review state         |
| CI state             |
| Merge conflict state |
| Merge readiness      |
| Last update          |


Filters must include repository, organization, author, reviewer, CI state, review state, branch, age, and attention state.

Search should match repository, PR number, title, branch, author, and commit SHA.

Example:

```
Repository       PR     Review       CI        Merge
──────────────────────────────────────────────────────
Meridian         #184   Approved     Passing   Ready
coThink Chat     #311   Approved     Failed    Blocked
AttendeeSync      #88   Waiting      Passing   Blocked
ChangePress       #47   Changes Req. Passing   Blocked
```

Selecting a PR opens the PR detail view.

---

# 12. Pull Request Detail

The detail view should combine information that currently requires moving between several Gitea pages.

Header information:

```
Meridian Client / PR #184

feature/device-enrollment → main

Author: Lou
Review: Approved
CI: Failed
Mergeability: Blocked
Updated: 4 minutes ago
```

The view should contain:


| Section  | Content                    |
| -------- | -------------------------- |
| Summary  | PR metadata                |
| Review   | Reviewers and review state |
| Checks   | Related workflows/jobs     |
| Pipeline | Visual dependency graph    |
| Commits  | Commit history             |
| Activity | Related CI and PR events   |
| Links    | Native Gitea views         |


---

# 13. Pipelines View

The Pipelines page provides cross-repository workflow visibility.

Example:

```
State   Repository       Workflow           Branch       Duration
────────────────────────────────────────────────────────────────
●       Meridian API     Test               PR-184       1m 14s
✗       coThink Chat     Android Build      main         4m 21s
✓       Assure           Release            v1.8.0       7m 02s
○       AttendeeSync     Deploy Production  main         Waiting
```

Filters should include:

repository,

organization,

workflow,

branch,

event,

status,

conclusion,

triggering user,

date range.

Historical search must not require accessing Gitea repository-by-repository at query time.

GitSeer should query its normalized local database.

---

# 14. Workflow Run Detail

The workflow run page is one of GitSeer's signature features.

It should visually represent the workflow dependency graph.

Example:

```
                 lint
                  ✓
                  │
                 test
                  ✓
                  │
                build
             ┌────┼────┐
             │    │    │
           iOS  macOS Android
            ✓     ✓     ✗
                       │
                     package
                     blocked
```

Nodes should display:

job name,

status,

conclusion,

duration,

runner,

start time,

finish time.

Selecting a node opens job details and logs.

Where runtime API data does not provide complete topology, GitSeer should inspect the repository workflow definition and derive dependencies from workflow YAML.

Runtime state comes from Gitea.

Topology comes from the workflow definition.

---

# 15. Job Detail

The job page should show:

```
Android Build

Status       Failed
Duration     3m 18s
Runner       android-arm64-01
Workflow     Release
Commit       134eab9
Triggered by Lou
```

Below that:

```
✓ Checkout             4s
✓ Setup Java           7s
✓ Install dependencies 42s
✓ Compile              1m 18s
✗ Sign APK             12s
- Upload artifact      skipped
```

The Gitea API exposes downloadable job logs through:

`GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs`.

Logs must be searchable.

GitSeer should support linking directly to the corresponding job/run in Gitea.

Step-level status may require parsing logs when the upstream API does not provide normalized step information.

Log parsing must therefore be treated as an adapter capability, not as canonical CI state.

---

# 16. Repositories View

The repository view provides one row per accessible repository.

Example:

```
Repository        Default Branch   CI       Open PRs   Last Activity
────────────────────────────────────────────────────────────────────
Meridian Client   main             ✓        3          2m ago
Meridian API      main             ●        1          20s ago
coThink Chat      main             ✗        6          8m ago
AttendeeSync      main             ✓        0          31m ago
```

Repository health should summarize rather than invent a new repository status.

Example component states:

default-branch CI,

active PR failures,

active deployments,

runner problems,

recent workflow failures.

---

# 17. Repository Page

GitSeer should support contextual entry from a Gitea repository.

Gitea supports additional repository tabs using `$GITEA_CUSTOM/templates/custom/extra_tabs.tmpl`, and additional navigation links through `extra_links.tmpl`.

The installation process should optionally add:

```
Code | Issues | Pull Requests | Actions | Projects | GitSeer
```

The GitSeer tab should link to a repository-scoped view such as:

```
/gitseer/repositories/{owner}/{repo}
```

or equivalent.

The repository page displays:

CI status,

open PR health,

recent runs,

pipeline graph,

failure history,

recent deployments,

links into native Gitea pages.

---

# 18. Native Gitea Integration

GitSeer must not maintain a modified Gitea distribution.

Supported integration should use Gitea's customization mechanisms.

Example deployment footprint:

```
$GITEA_CUSTOM/
  templates/
    custom/
      extra_links.tmpl
      extra_tabs.tmpl

  public/
    assets/
      gitseer.css
```

GitSeer must avoid replacing full upstream Gitea templates.

Template injection files should remain as small as possible to reduce upgrade sensitivity.

GitSeer should expose:

```
gitseer install-ui
```

and:

```
gitseer uninstall-ui
```

The install command should modify only GitSeer-owned customization sections or files.

Existing administrator customization must never be silently overwritten.

---

# 19. Authentication

GitSeer should use Gitea as the identity provider.

Gitea can operate as an OAuth2/OIDC provider, and current versions support granular scopes.

Users should experience:

```
Login to Gitea
      ↓
Open GitSeer
      ↓
Already authenticated or redirected through Gitea authorization
```

GitSeer must not store user passwords.

OAuth Authorization Code flow with PKCE should be preferred.

The service account used for synchronization and the individual user session must be separate concepts.

---

# 20. Authorization

GitSeer must not assume that because its synchronization account can see a repository, every logged-in user can see that repository.

For each user GitSeer must determine accessible repository scope.

User-visible data must be filtered accordingly.

At minimum:

```
GitSeer User
   │
   ├── permitted repository A
   ├── permitted repository B
   └── permitted repository C
```

Aggregated counters must also respect permissions.

If a user can access 12 of the instance's 80 repositories, their dashboard must reflect only those 12.

No side-channel counts may reveal hidden repository activity.

---

# 21. Installation Experience

The intended installation experience is:

```
Install GitSeer
    ↓
Open setup wizard
    ↓
Enter Gitea URL
    ↓
Authenticate / provide service credential
    ↓
GitSeer tests capabilities
    ↓
Configure OAuth
    ↓
Install webhook
    ↓
Optional Gitea UI integration
    ↓
Initial synchronization
    ↓
Dashboard
```

Target:

> A competent Gitea administrator should be able to install GitSeer without modifying source code or individual repositories.

---

# 22. Deployment Modes

GitSeer should ship in three primary forms.


| Method            | Purpose                              |
| ----------------- | ------------------------------------ |
| OCI/Docker image  | Default installation                 |
| Standalone binary | Bare-metal/self-contained deployment |
| Helm chart        | Kubernetes environments              |


The Docker installation should be considered the reference deployment.

Example:

```
services:
  gitseer:
    image: ghcr.io/ncdlabs/gitseer:latest
    environment:
      GITSEER_GITEA_URL: https://git.example.com
    volumes:
      - gitseer-data:/data
    ports:
      - "8090:8090"
```

Sensitive credentials should support both environment variables and file/secret references.

---

# 23. Application Architecture

Recommended architecture:

```
                        Gitea
                ┌──────────────────┐
                │ Repositories     │
                │ Pull Requests    │
                │ Actions          │
                │ Webhooks         │
                │ OAuth/OIDC       │
                └────────┬─────────┘
                         │
                  API + Webhooks
                         │
               ┌─────────▼─────────┐
               │    GitSeer     │
               │                   │
               │ Gitea Adapter     │
               │ Synchronizer      │
               │ Webhook Engine    │
               │ Attention Engine  │
               │ Query API         │
               │ Auth              │
               │ Realtime Gateway  │
               └────────┬──────────┘
                        │
                ┌───────▼────────┐
                │ Persistence    │
                │ SQLite / PG    │
                └────────────────┘
```

---

# 24. Technology Stack

## Backend

**Go**

Rationale:

single binary distribution,

strong fit with Gitea ecosystem,

low operational overhead,

good concurrent synchronization model,

simple container footprint,

frontend can be embedded into the executable.

## Frontend

**React + TypeScript**

Recommended supporting libraries:

Vite,

TanStack Query,

TanStack Table,

React Flow for workflow graphs,

small component system rather than a heavyweight design framework.

## Persistence

Default:

**SQLite**

Production/large-instance option:

**PostgreSQL**

GitSeer should use a database abstraction that allows both.

SQLite should be fully supported, not treated as a demo-only database.

## Realtime

Server-Sent Events are sufficient for most status updates.

WebSockets may be used if bidirectional realtime functionality becomes necessary.

Do not introduce Redis solely for realtime messaging in V1.

---

# 25. Gitea Adapter

All Gitea-specific operations must be encapsulated behind an internal adapter.

Conceptual interface:

```
type Forge interface {
    GetInstance(ctx context.Context) (*Instance, error)

    ListRepositories(ctx context.Context) ([]Repository, error)
    GetRepository(ctx context.Context, owner, repo string) (*Repository, error)

    ListPullRequests(ctx context.Context, repo Repository) ([]PullRequest, error)
    GetPullRequest(ctx context.Context, repo Repository, number int64) (*PullRequest, error)

    ListWorkflowRuns(ctx context.Context, repo Repository) ([]WorkflowRun, error)
    GetWorkflowRun(ctx context.Context, repo Repository, runID int64) (*WorkflowRun, error)

    ListJobs(ctx context.Context, repo Repository, runID int64) ([]Job, error)
    GetJobLogs(ctx context.Context, repo Repository, jobID int64) (io.Reader, error)
}
```

The UI and core product should not directly consume raw Gitea API structures.

This creates future flexibility for Forgejo or other providers without committing the MVP to supporting them.

---

# 26. Synchronization Model

GitSeer should combine two mechanisms:

```
Webhooks
   ↓
Immediate local updates

+

Periodic API reconciliation
   ↓
Eventual consistency
```

Neither mechanism is sufficient alone.

## Webhooks

Gitea currently exposes workflow events including `workflow_run` and `workflow_job`, with status transitions including queued, waiting, in-progress, and completed. Pull-request webhook events are also available.

GitSeer should process these events immediately.

## Reconciliation

Periodic reconciliation protects against:

missed webhooks,

GitSeer downtime,

Gitea downtime,

delivery errors,

administrator configuration mistakes,

repository additions,

eventual API inconsistencies.

Default reconciliation interval:

5 minutes.

Configurable range:

1 minute to 60 minutes.

Large installations should support incremental reconciliation.

---

# 27. Initial Synchronization

When a Gitea instance is first connected, GitSeer should:

1. identify the Gitea version;
2. query instance capabilities;
3. enumerate visible organizations;
4. enumerate visible repositories;
5. determine default branches;
6. discover workflow files;
7. import open pull requests;
8. import recent workflow runs;
9. import current workflow jobs;
10. calculate initial attention state;
11. subscribe/register webhook integration where permitted.

The Actions API includes repository workflow-run listing and individual workflow-run retrieval endpoints.

Historical import depth should be configurable.

Default:

30 days.

---

# 28. Webhook Endpoint

Example:

```
POST /api/webhooks/gitea/{instanceID}
```

Requirements:

verify webhook secret,

limit request size,

validate event type,

store event ID/hash for deduplication,

respond quickly,

process asynchronously inside the application,

record failed processing attempts,

make processing idempotent.

Webhook delivery should never directly mutate UI state without persisting the normalized state first.

---

# 29. Internal Data Model

Core entities:

```
Instance

Organization

Repository
  ├ owner
  ├ name
  ├ defaultBranch
  ├ archived
  ├ private
  └ lastSyncedAt

PullRequest
  ├ number
  ├ title
  ├ author
  ├ sourceBranch
  ├ targetBranch
  ├ headSHA
  ├ status
  ├ reviewState
  ├ mergeable
  └ updatedAt

Workflow
  ├ path
  ├ name
  └ topology

WorkflowRun
  ├ externalID
  ├ workflowID
  ├ event
  ├ branch
  ├ commitSHA
  ├ status
  ├ conclusion
  ├ startedAt
  └ completedAt

Job
  ├ externalID
  ├ runID
  ├ name
  ├ status
  ├ conclusion
  ├ runner
  ├ startedAt
  └ completedAt

WorkflowNode
  ├ jobKey
  └ dependencies[]

AttentionItem
  ├ type
  ├ severity
  ├ entityType
  ├ entityID
  ├ openedAt
  ├ resolvedAt
  └ metadata

WebhookEvent
  ├ eventType
  ├ deliveryHash
  ├ receivedAt
  ├ processedAt
  └ error
```

Gitea numeric identifiers should be stored separately from GitSeer primary keys.

---

# 30. Workflow Graph Parsing

GitSeer should retrieve workflow YAML from the repository and derive the job DAG.

For example:

```
jobs:
  test:
    runs-on: ubuntu-latest

  build:
    needs: test

  package:
    needs:
      - build
```

GitSeer normalizes this into:

```
test
  ↓
build
  ↓
package
```

Dynamic or unsupported workflow expressions should not break visualization.

Unknown dependency states should appear as unknown rather than being guessed.

The parser should retain the raw workflow revision/commit SHA so graph topology corresponds to the workflow definition that actually executed.

---

# 31. Status Normalization

Upstream state should be normalized into a small stable internal vocabulary.

## Execution Status

```
queued
waiting
running
completed
unknown
```

## Conclusion

```
success
failure
cancelled
skipped
neutral
timed_out
action_required
unknown
```

GitSeer must retain original upstream values for diagnostics.

Unknown new Gitea values must not cause deserialization failures.

---

# 32. Write Operations

V1 should primarily be read-only.

A later V1.x release may permit:

rerun workflow,

cancel workflow,

rerun failed jobs where supported.

Gitea currently exposes an endpoint to rerun an entire workflow run.

Any write operation must:

use the logged-in user's authorization where practical;

require explicit UI action;

display the operation target;

report upstream success/failure;

never silently fall back to the GitSeer service account.

---

# 33. Search

Global search should support:

repository name,

organization,

pull request number,

pull request title,

branch,

workflow,

commit SHA,

job,

author.

Search must operate against the GitSeer index/database and not issue broad Gitea API queries on every keystroke.

---

# 34. URL Design

Suggested routes:

```
/
 /attention
 /pulls
 /pulls/{owner}/{repo}/{number}

 /pipelines
 /pipelines/{owner}/{repo}/{run}

 /repositories
 /repositories/{owner}/{repo}

 /runners

 /activity

 /settings
 /settings/instance
 /settings/integration
 /settings/auth
```

URLs should be stable and bookmarkable.

---

# 35. API

GitSeer should expose its own versioned REST API.

Base:

```
/api/v1
```

Representative endpoints:

```
GET /api/v1/summary

GET /api/v1/attention

GET /api/v1/repositories
GET /api/v1/repositories/{owner}/{repo}

GET /api/v1/pull-requests
GET /api/v1/repositories/{owner}/{repo}/pull-requests/{number}

GET /api/v1/workflow-runs
GET /api/v1/workflow-runs/{id}

GET /api/v1/jobs/{id}
GET /api/v1/jobs/{id}/logs

GET /api/v1/activity

GET /api/v1/system/status
```

Internal API responses must expose GitSeer normalized models rather than raw Gitea payloads.

OpenAPI documentation should be generated.

---

# 36. Health Endpoints

Required:

```
GET /health/live
GET /health/ready
```

Readiness should verify:

database connectivity,

migration status,

required configuration.

Gitea connectivity should be represented separately rather than making the process permanently unready during a temporary Gitea outage.

Example:

```
GET /api/v1/system/status
```

---

# 37. Resilience

GitSeer must tolerate:

Gitea unavailable,

Gitea API rate limiting,

database restart,

duplicate webhooks,

out-of-order webhook delivery,

deleted repositories,

renamed repositories,

removed permissions,

workflow deletion,

unknown workflow statuses,

partial API responses,

failed OAuth refresh,

application restart during synchronization.

Synchronization operations must be idempotent.

---

# 38. Security

Credentials must be encrypted at rest where stored.

Secrets must never appear in logs.

Webhook requests must be authenticated.

OAuth state must be validated.

PKCE should be used for user authorization.

CSRF protection must be enabled for state-changing operations.

Sessions must use secure, HTTP-only cookies.

CSP should be configured.

The application must not render workflow logs as trusted HTML.

ANSI/log rendering must sanitize escape sequences capable of unsafe terminal behavior.

Repository permissions must be enforced server-side.

Client filtering is not authorization.

---

# 39. Privacy and Telemetry

No telemetry should be required.

Default:

```
Telemetry: OFF
External analytics: NONE
Crash reporting: NONE
```

If anonymous usage telemetry is ever introduced, it must be explicitly opt-in.

GitSeer should be usable in disconnected/internal environments.

---

# 40. Logging

GitSeer itself should emit structured logs.

Recommended format:

JSON.

Fields should include:

timestamp,

level,

component,

instance,

repository where appropriate,

event,

request ID,

webhook delivery ID,

duration,

error classification.

Secrets, OAuth tokens, authorization headers, and job-log content must not be emitted by default.

---

# 41. Metrics

Optional Prometheus endpoint:

```
/metrics
```

Suggested metrics:

```
gitseer_repositories_total
gitseer_open_pull_requests_total
gitseer_workflow_runs_total
gitseer_workflow_failures_total
gitseer_workflow_runs_active
gitseer_webhooks_received_total
gitseer_webhook_processing_errors_total
gitseer_sync_duration_seconds
gitseer_sync_errors_total
gitseer_gitea_api_requests_total
gitseer_gitea_api_errors_total
```

Metrics must not leak private repository names unless explicitly enabled.

---

# 42. Performance Targets

Initial design targets:


| Scale                  | Target          |
| ---------------------- | --------------- |
| Repositories           | 1,000           |
| Open PRs               | 10,000          |
| Workflow runs retained | 1,000,000       |
| Concurrent UI users    | 100             |
| Webhook processing     | <2 sec typical  |
| Dashboard API          | <500 ms typical |
| Search                 | <500 ms typical |


These are engineering targets, not guarantees.

Pagination must exist everywhere large result sets can occur.

---

# 43. Database Retention

Default retention:

workflow metadata: 180 days,

attention history: 180 days,

webhook processing records: 30 days,

logs: fetched on demand and not persistently stored by default.

Users may configure longer retention.

Persistent CI logs can become enormous remarkably quickly, because apparently failure messages require the storage characteristics of a small astronomical survey.

---

# 44. Setup Wizard

The first-run wizard should contain:

## Welcome

Explain what GitSeer will access.

## Connect to Gitea

Fields:

Gitea URL,

service credentials/authentication.

## Validate

Display:

Gitea version,

API availability,

Actions availability,

webhook permissions,

OAuth capability.

## Authentication

Guide creation/configuration of the OAuth application.

Gitea exposes OAuth application support and currently documents API creation of OAuth applications as well.

## Webhooks

Configure the highest supported scope.

## UI Integration

Offer:

```
[ ] Add GitSeer to Gitea global navigation
[ ] Add GitSeer tab to repository pages
```

## Initial Import

Allow selection:

```
7 days
30 days
90 days
No historical import
```

## Complete

Redirect to Overview.

---

# 45. Configuration

GitSeer configuration should use environment variables and an optional configuration file.

Example:

```
server:
  listen: 0.0.0.0:8090
  external_url: https://git.example.com/gitseer

database:
  driver: sqlite
  path: /data/gitseer.db

gitea:
  url: https://git.example.com
  token_file: /run/secrets/gitea_token

sync:
  reconcile_interval: 5m
  history_days: 30

auth:
  provider: gitea

ui:
  instance_name: GitSeer
```

Environment variables should override configuration-file values.

---

# 46. Reverse Proxy Support

GitSeer must support operation under:

```
https://gitseer.example.com
```

and:

```
https://git.example.com/gitseer
```

Subpath operation must be tested as a first-class deployment mode because it produces the most native integration experience.

Headers such as:

`X-Forwarded-Proto`

`X-Forwarded-Host`

`X-Forwarded-Prefix`

must be handled safely.

---

# 47. Dark Mode

GitSeer should support:

light,

dark,

system preference.

When accessed through Gitea, GitSeer should visually complement Gitea without depending on Gitea internal CSS.

Avoid copying Gitea's entire frontend stylesheet.

---

# 48. Accessibility

Target:

WCAG 2.1 AA.

Requirements include:

keyboard navigation,

visible focus state,

semantic tables,

accessible graph alternatives,

color-independent status indicators,

screen-reader labels,

reduced-motion support.

Pipeline states must never be represented by color alone.

---

# 49. Public Open-Source Repository

Recommended structure:

```
/
├── cmd/
│   └── gitseer/
├── internal/
│   ├── api/
│   ├── auth/
│   ├── attention/
│   ├── database/
│   ├── forge/
│   │   └── gitea/
│   ├── models/
│   ├── sync/
│   ├── webhooks/
│   └── workflows/
├── migrations/
├── web/
├── integrations/
│   └── gitea/
├── deploy/
│   ├── docker/
│   ├── compose/
│   └── helm/
├── docs/
├── .github/
├── Dockerfile
├── docker-compose.yml
├── LICENSE
├── README.md
├── SECURITY.md
├── CONTRIBUTING.md
└── CHANGELOG.md
```

---

# 50. License

Recommended:

**Apache License 2.0**

Reasons:

permissive,

commercial use allowed,

modification allowed,

distribution allowed,

explicit patent grant,

friendly to enterprise adoption.

MIT is acceptable, but Apache-2.0 is preferred for a developer infrastructure project.

---

# 51. Public Project Positioning

Primary headline:

> **One view for your entire Gitea instance.**

Supporting copy:

> Monitor pull requests, CI/CD pipelines, failures, approvals, and repository health across every project from a single dashboard.

Short alternative:

> **Stop checking repositories one at a time.**

README introduction:

> GitSeer is an open-source operations console for Gitea. It automatically discovers repositories, pull requests, Actions workflows, runs, and jobs and brings them together into one real-time view.

---

# 52. MVP

The MVP is complete when a user can install GitSeer, connect it to a stock supported Gitea instance, and obtain a useful cross-repository dashboard without changing any repository.

MVP features:

```
Gitea connection
Gitea OAuth authentication
Repository discovery
Organization discovery
Pull-request aggregation
Actions workflow discovery
Workflow-run synchronization
Job synchronization
Webhook processing
API reconciliation
Overview dashboard
Attention dashboard
Pull Requests view
Pipelines view
Repositories view
PR detail
Workflow-run detail
Workflow graph
Job logs
Filtering
Search
Dark mode
Gitea navigation integration
Repository page tab
SQLite
Docker image
Standalone binary
Basic metrics
Health checks
Documentation
```

---

# 53. V1.0 Exit Criteria

V1.0 should additionally require:

PostgreSQL support,

Helm chart,

migration tooling,

backup/restore documentation,

permission validation,

large-instance pagination,

resynchronization controls,

failure-recovery testing,

security review,

dependency scanning,

SBOM generation,

signed releases/container images,

multi-architecture images,

documented upgrade process,

integration tests against supported Gitea versions.

---

# 54. Post-V1 Opportunities

Future capabilities may include:

deployment environments,

runner utilization dashboards,

workflow duration trends,

failure clustering,

flaky-job detection,

release visibility,

notifications,

Slack/Discord/webhook destinations,

saved dashboard filters,

organization-specific dashboards,

team dashboards,

incident integration,

public/read-only wallboard mode,

Forgejo support,

~~GitHub support,~~ *(superseded: GitHub is in-scope dual-forge — ADR-029 in [implementation-plan.md](implementation-plan.md))*

GitLab support *(Coming Soon — ask before implementing)*,

Bitbucket support *(Coming Soon — ask before implementing)*.

These should not contaminate the original MVP architecture.

The forge abstraction supports multiple providers. **In scope now:** Gitea + GitHub. **Coming Soon:** GitLab, Bitbucket.

---

# 55. Explicit Architectural Decisions

**ADR-001:** GitSeer is an external companion application, not a Gitea fork.

**ADR-002:** Gitea remains the source of truth.

**ADR-003:** Webhooks provide immediacy; reconciliation provides correctness.

**ADR-004:** Repository configuration is optional, never required for basic operation.

**ADR-005:** Gitea OAuth provides user identity.

**ADR-006:** GitSeer independently enforces repository authorization.

**ADR-007:** Go is the backend/runtime language.

**ADR-008:** React/TypeScript is the frontend.

**ADR-009:** SQLite is the default database.

**ADR-010:** PostgreSQL is supported for larger installations.

**ADR-011:** Redis is not required for V1.

**ADR-012:** Workflow topology is derived from the workflow definition when upstream runtime data is insufficient.

**ADR-013:** The frontend consumes GitSeer models, never raw Gitea API models.

**ADR-014:** Native Gitea integration uses supported customization files rather than overwritten core templates.

**ADR-015:** All externally visible functionality must remain usable without an ncdLabs-hosted service.

**ADR-029 (amendment):** Dual-forge Gitea + GitHub is in scope for product GitSeer; GitLab/Bitbucket remain Coming Soon. See [implementation-plan.md](implementation-plan.md) §4. Product branding is GitSeer; technical IDs are `gitseer` / `GITSEER_*`.

---

# 56. Test Strategy

Testing should include:

## Unit Tests

Gitea payload normalization,

workflow YAML parsing,

DAG construction,

attention rules,

permission filtering,

status normalization,

webhook deduplication.

## Integration Tests

Automated disposable Gitea environment.

Tests should create:

repositories,

branches,

pull requests,

workflow files,

successful runs,

failed runs,

cancelled runs.

GitSeer should then verify normalized state.

## Compatibility Tests

CI matrix against each officially supported GitSeer/Gitea version.

## Security Tests

authorization isolation,

CSRF,

OAuth state validation,

webhook signature validation,

XSS through logs,

XSS through repository metadata,

path traversal,

SSRF around configured Gitea URL,

secret leakage.

## UI Tests

Playwright.

Critical workflows:

login,

dashboard,

PR filtering,

pipeline navigation,

job-log viewing,

repository-scoped GitSeer.

---

# 57. CI/CD for GitSeer

The GitSeer project itself should demonstrate the practices it visualizes.

CI should include:

```
lint
    ↓
unit tests
    ↓
integration tests
    ↓
frontend tests
    ↓
build
   ├── linux-amd64
   ├── linux-arm64
   ├── darwin-amd64
   ├── darwin-arm64
   └── windows-amd64
    ↓
container build
    ↓
security scan
    ↓
SBOM
    ↓
signed release
```

Container images should publish to GHCR initially.

---

# 58. Definition of Done

A feature is not complete until it includes:

implementation,

authorization enforcement,

error states,

empty states,

loading states,

responsive UI,

dark mode,

unit tests,

relevant integration tests,

documentation,

observability,

migration impact review where applicable.

---

# 59. Initial Development Sequence

## Phase 1 — Foundation

Go service,

configuration,

SQLite,

migrations,

React shell,

Gitea API client,

instance capability detection.

## Phase 2 — Discovery

repositories,

organizations,

workflow discovery,

pull requests,

workflow runs,

jobs.

## Phase 3 — Synchronization

webhooks,

event normalization,

reconciliation,

initial import,

incremental synchronization.

## Phase 4 — Core UI

Overview,

Repositories,

Pull Requests,

Pipelines,

details.

## Phase 5 — Pipeline Visualization

workflow YAML loader,

DAG parser,

React Flow graph,

job state overlays,

logs.

## Phase 6 — Attention

attention rules,

severity,

resolution,

dashboard prioritization.

## Phase 7 — Gitea Integration

OAuth,

permission filtering,

global navigation,

repository GitSeer tabs,

subpath/reverse-proxy support.

## Phase 8 — Distribution

Docker,

standalone binaries,

Helm,

documentation,

release automation.

## Phase 9 — Production Hardening

compatibility matrix,

security,

large installations,

failure recovery,

upgrade testing,

release candidate.

---

# 60. Success Criteria

The product succeeds if an administrator can:

```
docker compose up -d
```

connect GitSeer to Gitea,

complete initial setup,

and immediately see useful status for all accessible repositories.

A developer should be able to open GitSeer and determine within seconds:

> What is broken?

> What is running?

> What is blocked?

> What can be merged?

A Gitea administrator should not need to edit dozens of repositories to make GitSeer work.

A Gitea upgrade should not normally require a GitSeer-specific Gitea build.

A GitSeer upgrade should not change repository contents.

---

# 61. Product Boundary

GitSeer should resist becoming another DevOps platform.

The product boundary is:

> **Observe and operate the development lifecycle already occurring inside Gitea.**

Not:

> Replace every tool involved in software delivery.

That distinction is important.

There are already enough platforms attempting to become the operating system for software development. The world can survive one tool that simply shows you why the build is red.

---

# 62. Final Product Statement

**GitSeer is an open-source operational dashboard for Gitea that automatically aggregates pull requests, CI/CD pipelines, workflow jobs, failures, approvals, and repository health across an entire Gitea instance.**

It installs alongside Gitea, integrates into the Gitea user experience, authenticates through Gitea, discovers repositories automatically, and requires no repository-specific configuration.

Its central promise is simple:

> **One Gitea instance. One GitSeer. Everything that needs your attention.**

