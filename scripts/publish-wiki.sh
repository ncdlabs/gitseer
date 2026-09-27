#!/usr/bin/env bash
# Sync docs/*.md into the GitHub wiki clone and push.
# Prerequisite: create the first wiki page once in the GitHub UI so
# https://github.com/ncdlabs/gitseer.wiki.git exists.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCS="$ROOT/docs"
WIKI="${GITSEER_WIKI_DIR:-$ROOT/../gitseer.wiki}"
REMOTE="${GITSEER_WIKI_REMOTE:-https://github.com/ncdlabs/gitseer.wiki.git}"

TOKEN="$(gh auth token 2>/dev/null || true)"
AUTH_REMOTE="$REMOTE"
if [[ -n "${TOKEN}" ]]; then
  AUTH_REMOTE="https://x-access-token:${TOKEN}@github.com/ncdlabs/gitseer.wiki.git"
fi

wiki_remote_ready() {
  git ls-remote "$AUTH_REMOTE" HEAD >/dev/null 2>&1
}

if ! wiki_remote_ready; then
  cat >&2 <<EOF
Wiki git remote is not initialized yet.

1. Open https://github.com/ncdlabs/gitseer/wiki while signed into GitHub
2. Create the first page titled Home (any stub content is fine)
3. Re-run: $0
EOF
  exit 1
fi

if [[ ! -d "$WIKI/.git" ]]; then
  git clone "$AUTH_REMOTE" "$WIKI"
else
  git -C "$WIKI" remote set-url origin "$AUTH_REMOTE"
  git -C "$WIKI" fetch origin 2>/dev/null || true
fi

python3 - "$DOCS" "$WIKI" <<'PY'
from pathlib import Path
import re
import sys

docs = Path(sys.argv[1])
wiki = Path(sys.argv[2])

mapping = {
    "getting-started.md": "Getting-Started.md",
    "architecture.md": "Architecture.md",
    "features.md": "Features.md",
    "configuration.md": "Configuration.md",
    "authentication-and-acl.md": "Authentication-and-ACL.md",
    "setup-wizard.md": "Setup-Wizard.md",
    "attention-engine.md": "Attention-Engine.md",
    "api-reference.md": "API-Reference.md",
    "webhooks-and-sync.md": "Webhooks-and-Sync.md",
    "deployment.md": "Deployment.md",
    "operations.md": "Operations.md",
    "development.md": "Development.md",
    "troubleshooting.md": "Troubleshooting.md",
    "install.md": "Install.md",
}

link_map = {
    "getting-started.md": "Getting-Started",
    "architecture.md": "Architecture",
    "features.md": "Features",
    "configuration.md": "Configuration",
    "authentication-and-acl.md": "Authentication-and-ACL",
    "setup-wizard.md": "Setup-Wizard",
    "attention-engine.md": "Attention-Engine",
    "api-reference.md": "API-Reference",
    "webhooks-and-sync.md": "Webhooks-and-Sync",
    "deployment.md": "Deployment",
    "operations.md": "Operations",
    "development.md": "Development",
    "troubleshooting.md": "Troubleshooting",
    "install.md": "Install",
    "README.md": "Home",
    "../README.md": "https://github.com/ncdlabs/gitseer/blob/main/README.md",
}

repo_blob = "https://github.com/ncdlabs/gitseer/blob/main"


def rewrite_links(text: str) -> str:
    def repl(m):
        label, target = m.group(1), m.group(2)
        if target.startswith(("http://", "https://", "mailto:")):
            return m.group(0)
        path, anchor = target, ""
        if "#" in target:
            path, anchor = target.split("#", 1)
            anchor = "#" + anchor
        base = path.split("/")[-1] if path else ""
        if base in link_map:
            dest = link_map[base]
            return f"[{label}]({dest}{anchor})"
        if path.startswith("../"):
            return f"[{label}]({repo_blob}/{path[3:]}{anchor})"
        if path.endswith((".md", ".yaml")) or "/" in path:
            if path.startswith("docs/"):
                return f"[{label}]({repo_blob}/{path}{anchor})"
            return f"[{label}]({repo_blob}/docs/{path}{anchor})"
        return m.group(0)

    return re.sub(r"\[([^\]]+)\]\(([^)]+)\)", repl, text)


for src_name, dest_name in mapping.items():
    body = rewrite_links((docs / src_name).read_text())
    (wiki / dest_name).write_text(body)

home = """# GitSeer

Self-hosted CI/CD and pull-request ops console for [Gitea](https://gitea.com) and [GitHub](https://github.com). Technical IDs: binary/module/Helm `gitseer`, env `GITSEER_*`.

Aggregates repositories, open PRs, Actions/workflow runs, and attention items into one ACL-scoped UI. Each forge stays the system of record; GitSeer syncs via API and webhooks. Login is bootstrap password and/or Gitea OAuth (PKCE) and GitHub OAuth (PKCE). No forge fork, no per-repo agents, no SaaS dependency. GitLab and Bitbucket are Coming Soon.

| | |
|--|--|
| **Source** | [github.com/ncdlabs/gitseer](https://github.com/ncdlabs/gitseer) |
| **License** | Apache-2.0 |
| **Maintainer** | [ncdLabs](https://ncdlabs.com) |
| **README** | [README.md](https://github.com/ncdlabs/gitseer/blob/main/README.md) |
| **In-repo docs** | [docs/](https://github.com/ncdlabs/gitseer/tree/main/docs) |
| **Live** | [gitseer.ncdlabs.com](https://gitseer.ncdlabs.com) |

## Guides

- [Getting Started](Getting-Started)
- [Install](Install)
- [Architecture](Architecture)
- [Features](Features)
- [Configuration](Configuration)
- [Authentication and ACL](Authentication-and-ACL)
- [Setup Wizard](Setup-Wizard)
- [Attention Engine](Attention-Engine)
- [API Reference](API-Reference)
- [Webhooks and Sync](Webhooks-and-Sync)
- [Deployment](Deployment)
- [Operations](Operations)
- [Development](Development)
- [Troubleshooting](Troubleshooting)

## Features

- **Dashboard** — summary and stats (Now / 1 / 7 / 30 / 90 day windows)
- **Inbox** — authored / review / failing CI / blocked-on-me queue with saved filters
- **Attention** — `critical` / `warning` / `waiting`, mute/snooze, optional log tail
- **Repositories** — health rollup and failure clusters
- **Pull requests** — CI and review badges across accessible repos
- **Pipelines** — runs by action; rerun/cancel; Active Actions flyout / pop-out
- **Notifications** — self-hosted SMTP + Slack/Discord/generic HTTPS
- **Setup wizard** — Prepare → forge picker → Connect → Validate → Finish
- **Settings** — Preferences / Integration / Access / Notifications / Status
- SSE live updates, Prometheus metrics, retention purge, backup/restore CLI
- Optional Gitea custom-template links (`install-ui` / Download Gitea UI Snippets)

## Not in scope

- Writing workflow YAML, owning runners, or replacing forge Actions
- GitLab / Bitbucket forge clients (Coming Soon)
- Redis / WebSockets
- ncdLabs-hosted notification relay

## Specs (in-repo)

- [PRD / technical spec](https://github.com/ncdlabs/gitseer/blob/main/docs/prd-spec.md)
- [Implementation plan](https://github.com/ncdlabs/gitseer/blob/main/docs/implementation-plan.md)
"""
(wiki / "Home.md").write_text(home)

(wiki / "_Sidebar.md").write_text(
    """- [Home](Home)
- [Getting Started](Getting-Started)
- [Install](Install)
- [Architecture](Architecture)
- [Features](Features)
- [Configuration](Configuration)
- [Authentication and ACL](Authentication-and-ACL)
- [Setup Wizard](Setup-Wizard)
- [Attention Engine](Attention-Engine)
- [API Reference](API-Reference)
- [Webhooks and Sync](Webhooks-and-Sync)
- [Deployment](Deployment)
- [Operations](Operations)
- [Development](Development)
- [Troubleshooting](Troubleshooting)
- **Repository**
  - [README](https://github.com/ncdlabs/gitseer/blob/main/README.md)
  - [PRD](https://github.com/ncdlabs/gitseer/blob/main/docs/prd-spec.md)
  - [Implementation plan](https://github.com/ncdlabs/gitseer/blob/main/docs/implementation-plan.md)
  - [CHANGELOG](https://github.com/ncdlabs/gitseer/blob/main/CHANGELOG.md)
  - [SECURITY](https://github.com/ncdlabs/gitseer/blob/main/SECURITY.md)
"""
)
print(f"Synced {len(mapping) + 2} wiki pages into {wiki}")
PY

cd "$WIKI"
git add -A
if ! git diff --cached --quiet; then
  git -c user.email="${GITSEER_WIKI_EMAIL:-wiki@ncdlabs.com}" \
      -c user.name="${GITSEER_WIKI_NAME:-GitSeer Docs}" \
      commit -m "docs(wiki): sync guides from docs/"
else
  echo "No local wiki file changes."
fi

branch="$(git rev-parse --abbrev-ref HEAD)"
# Ensure we see remote tip; wiki bootstrap often creates an unrelated Initial Home commit.
git fetch origin 2>/dev/null || true
if git rev-parse --verify "origin/master" >/dev/null 2>&1; then
  remote_ref="origin/master"
elif git rev-parse --verify "origin/${branch}" >/dev/null 2>&1; then
  remote_ref="origin/${branch}"
else
  remote_ref=""
fi

if [[ -n "$remote_ref" ]] && git merge-base --is-ancestor HEAD "$remote_ref" 2>/dev/null; then
  echo "Wiki already up to date on remote."
  echo "Published: https://github.com/ncdlabs/gitseer/wiki"
  exit 0
fi

if [[ -n "$remote_ref" ]] && ! git merge-base --is-ancestor "$remote_ref" HEAD 2>/dev/null; then
  # Diverged (common after first GitHub UI Home page). Prefer our synced tree.
  echo "Remote wiki history diverged; force-pushing synced docs."
  git push --force-with-lease -u origin "HEAD:master" 2>/dev/null \
    || git push --force-with-lease -u origin "HEAD:${branch}"
else
  git push -u origin "HEAD:master" 2>/dev/null || git push -u origin "HEAD:${branch}"
fi
echo "Published: https://github.com/ncdlabs/gitseer/wiki"
