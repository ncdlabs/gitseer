#!/usr/bin/env bash
# Local `npm run start`: API + Vite, print access info, open browser.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_PORT="${GITSEER_API_PORT:-8090}"
WEB_PORT="${GITSEER_WEB_PORT:-5173}"
WEB_URL="http://127.0.0.1:${WEB_PORT}"
API_URL="http://127.0.0.1:${API_PORT}"
BOOTSTRAP_USER="${GITSEER_AUTH_BOOTSTRAP_USERNAME:-admin}"

load_dotenv() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      local key="${BASH_REMATCH[1]}"
      local val="${BASH_REMATCH[2]}"
      if [[ "$val" =~ ^\"(.*)\"$ ]]; then
        val="${BASH_REMATCH[1]}"
      elif [[ "$val" =~ ^\'(.*)\'$ ]]; then
        val="${BASH_REMATCH[1]}"
      fi
      if [[ -z "${!key+x}" ]]; then
        export "$key=$val"
      fi
    fi
  done <"$file"
}

load_dotenv "$ROOT/.env"

export GOPATH="${GOPATH:-$HOME/Library/Caches/go}"
export GOMODCACHE="${GOMODCACHE:-$HOME/Library/Caches/go/pkg/mod}"
mkdir -p "$GOPATH" "$GOMODCACHE"

# Do not set a default bootstrap password — first UI visit claims it.
# Legacy GITSEER_AUTH_BOOTSTRAP_PASSWORD from .env still works if present.
if [[ -z "${GITSEER_AUTH_BOOTSTRAP_USERNAME:-}" ]]; then
  export GITSEER_AUTH_BOOTSTRAP_USERNAME="$BOOTSTRAP_USER"
fi
BOOTSTRAP_USER="${GITSEER_AUTH_BOOTSTRAP_USERNAME}"

# Local npm start only: let bootstrap admins skip the first-run setup wizard.
export GITSEER_ALLOW_SKIP_SETUP="${GITSEER_ALLOW_SKIP_SETUP:-true}"

resolve_config() {
  if [[ -n "${GITSEER_CONFIG:-}" ]]; then
    echo "$GITSEER_CONFIG"
    return
  fi
  if [[ -f "$ROOT/config.yaml" ]]; then
    echo "$ROOT/config.yaml"
    return
  fi
  echo "$ROOT/config.example.yaml"
}

CFG="$(resolve_config)"
export GITSEER_CONFIG="$CFG"

print_access() {
  echo ""
  echo "GitSeer (local)"
  echo "  App:      ${WEB_URL}"
  echo "  API:      ${API_URL}"
  echo "  Config:   ${CFG}"
  echo "  Bootstrap suggested username: ${BOOTSTRAP_USER}"
  if [[ -n "${GITSEER_AUTH_BOOTSTRAP_PASSWORD:-}" ]]; then
    echo "  Password: (legacy env set — prefer claiming in UI on a fresh DB)"
  else
    echo "  Password: set on first visit (Claim Bootstrap)"
  fi
  if [[ -z "${GITSEER_GITEA_URL:-}" ]]; then
    echo "  OAuth:    disabled (set GITSEER_GITEA_URL + OAuth client to enable)"
  else
    echo "  OAuth:    Gitea ${GITSEER_GITEA_URL}"
  fi
  echo ""
}

wait_http() {
  local url="$1"
  local label="$2"
  local i
  for i in $(seq 1 90); do
    if curl -fsS -o /dev/null --connect-timeout 1 "$url" 2>/dev/null; then
      echo "Ready: ${label}"
      return 0
    fi
    sleep 0.5
  done
  echo "Timed out waiting for ${label} (${url})" >&2
  return 1
}

open_browser() {
  local url="$1"
  if [[ "${GITSEER_NO_BROWSER:-}" == "1" || -n "${CI:-}" ]]; then
    echo "Skipping browser open (GITSEER_NO_BROWSER or CI set)"
    return 0
  fi
  if command -v open >/dev/null 2>&1; then
    open "$url" >/dev/null 2>&1 || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" >/dev/null 2>&1 || true
  else
    echo "Open ${url} in your browser"
  fi
}

print_access

npm --prefix web install --prefer-offline --no-audit --no-fund

CONCURRENTLY="$ROOT/node_modules/.bin/concurrently"
if [[ ! -x "$CONCURRENTLY" ]]; then
  npm install --no-audit --no-fund
fi

# Job control so concurrently gets its own process group (reliable CTRL-C teardown).
set -m
"$CONCURRENTLY" -k -n api,web -c blue,green \
  "npm run start:api" \
  "npm run start:web" &
START_PID=$!

# CTRL-C / SIGTERM / script exit: stop concurrently and any orphaned
# go/vite listeners still holding :8090 / :5173.
CLEANED=0
cleanup() {
  [[ "$CLEANED" -eq 1 ]] && return 0
  CLEANED=1
  trap - EXIT INT TERM
  echo ""
  echo "Stopping GitSeer (CTRL-C / quit)…"
  if [[ -n "${START_PID:-}" ]] && kill -0 "$START_PID" 2>/dev/null; then
    kill -INT "$START_PID" 2>/dev/null || true
    # concurrently may leave `go run` / vite children; kill its process group too
    kill -INT -- "-$START_PID" 2>/dev/null || true
    wait "$START_PID" 2>/dev/null || true
  fi
  bash "$ROOT/scripts/dev-stop.sh" || true
}
trap cleanup EXIT INT TERM

if wait_http "${API_URL}/health/live" "API :${API_PORT}" \
  && wait_http "${WEB_URL}/" "UI :${WEB_PORT}"; then
  print_access
  open_browser "$WEB_URL"
else
  echo "Services failed to become ready; stopping." >&2
  exit 1
fi

wait "$START_PID"
status=$?
cleanup
exit "$status"
