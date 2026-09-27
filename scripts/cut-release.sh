#!/usr/bin/env bash
# Bump GitSeer semver, refresh CHANGELOG, commit, and create an annotated v* tag.
# Invoked by .github/workflows/cut-release.yaml (Actions → Cut release).
# Pushing the tag triggers .github/workflows/release.yaml (GHCR + Docker Hub + GitHub Release).
#
# Usage (CI; prefer the workflow_dispatch UI):
#   ./scripts/cut-release.sh              # patch (default)
#   ./scripts/cut-release.sh minor
#   ./scripts/cut-release.sh major
#   ./scripts/cut-release.sh 1.2.3
#   ./scripts/cut-release.sh patch --push
#   ./scripts/cut-release.sh patch --allow-empty
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MAIN_GO="cmd/gitseer/main.go"
CHART="deploy/helm/gitseer/Chart.yaml"
CHANGELOG="CHANGELOG.md"

PUSH=0
ALLOW_EMPTY=0
BUMP="patch"

usage() {
  cat <<EOF
Usage: $0 [patch|minor|major|X.Y.Z] [--push] [--allow-empty]

Bumps version in $MAIN_GO, $CHART, and $CHANGELOG, commits, and tags vX.Y.Z.
With --push, also runs: git push origin HEAD --tags

Prefer cutting releases via GitHub Actions (Actions → Cut release) rather than locally.
EOF
}

for arg in "$@"; do
  case "$arg" in
    -h|--help) usage; exit 0 ;;
    --push) PUSH=1 ;;
    --allow-empty) ALLOW_EMPTY=1 ;;
    patch|minor|major) BUMP="$arg" ;;
    [0-9]*.[0-9]*.[0-9]*) BUMP="$arg" ;;
    *)
      echo "unknown argument: $arg" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -n "$(git status --porcelain)" ]]; then
  echo "working tree is dirty; commit or stash before cutting a release" >&2
  exit 1
fi

current="$(sed -n 's/^var version = "\([^"]*\)"/\1/p' "$MAIN_GO" | head -n1)"
if [[ -z "$current" ]]; then
  echo "could not read version from $MAIN_GO" >&2
  exit 1
fi

if [[ "$BUMP" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  next="$BUMP"
else
  IFS=. read -r major minor patch <<<"$current"
  case "$BUMP" in
    patch) next="${major}.${minor}.$((patch + 1))" ;;
    minor) next="${major}.$((minor + 1)).0" ;;
    major) next="$((major + 1)).0.0" ;;
    *)
      echo "invalid bump: $BUMP" >&2
      exit 1
      ;;
  esac
fi

tag="v${next}"
if git rev-parse "$tag" >/dev/null 2>&1; then
  echo "tag $tag already exists" >&2
  exit 1
fi

today="$(date +%Y-%m-%d)"

python3 - "$CHANGELOG" "$next" "$today" "$ALLOW_EMPTY" <<'PY'
import re, sys
path, ver, today, allow_empty = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4] == "1"
text = open(path, encoding="utf-8").read()
m = re.search(r"(?ms)^## \[Unreleased\]\n(.*?)(?=^## \[|\Z)", text)
if not m:
    sys.exit("CHANGELOG.md: missing ## [Unreleased] section")
body = m.group(1).strip()
# Treat section as empty if it has no bullet lines
bullets = [ln for ln in body.splitlines() if ln.strip().startswith("- ")]
if not bullets and not allow_empty:
    sys.exit(
        "CHANGELOG.md [Unreleased] has no entries; add notes or pass --allow-empty"
    )
replacement = f"## [Unreleased]\n\n## [{ver}] - {today}\n"
if body:
    replacement += "\n" + body + "\n"
else:
    replacement += "\n"
new_text = text[: m.start()] + replacement + text[m.end() :]
open(path, "w", encoding="utf-8").write(new_text)
PY

perl -i -pe "s/^var version = \".*\"/var version = \"$next\"/" "$MAIN_GO"
perl -i -pe "s/^version: .*/version: $next/; s/^appVersion: .*/appVersion: \"$next\"/" "$CHART"

git add "$MAIN_GO" "$CHART" "$CHANGELOG"
git commit -m "$(cat <<EOF
release: v${next}

EOF
)"
git tag -a "$tag" -m "GitSeer ${next}"

echo "Created commit and tag ${tag} (${current} → ${next})"
if [[ "$PUSH" -eq 1 ]]; then
  git push origin HEAD --tags
  echo "Pushed HEAD and ${tag}; release workflow should start shortly."
else
  cat <<EOF
Next:
  git push origin HEAD --tags

Or re-run with --push.
EOF
fi
