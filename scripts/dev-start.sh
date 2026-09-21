#!/usr/bin/env bash
# Local `npm run start`: API + Vite, print access info, open browser.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

API_PORT="${LENS_API_PORT:-8090}"
WEB_PORT="${LENS_WEB_PORT:-5173}"
WEB_URL="http://127.0.0.1:${WEB_PORT}"
API_URL="http://127.0.0.1:${API_PORT}"
BOOTSTRAP_USER="bootstrap"
DEFAULT_BOOTSTRAP_PASSWORD="lens-local"

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

if [[ -z "${LENS_AUTH_BOOTSTRAP_PASSWORD:-}" ]]; then
  if [[ -n "${LENS_AUTH_BOOTSTRAP_PASSWORD_FILE:-}" && -r "${LENS_AUTH_BOOTSTRAP_PASSWORD_FILE}" ]]; then
    LENS_AUTH_BOOTSTRAP_PASSWORD="$(tr -d '\r\n' <"${LENS_AUTH_BOOTSTRAP_PASSWORD_FILE}")"
  fi
fi
if [[ -z "${LENS_AUTH_BOOTSTRAP_PASSWORD:-}" ]]; then
  export LENS_AUTH_BOOTSTRAP_PASSWORD="$DEFAULT_BOOTSTRAP_PASSWORD"
fi
export LENS_AUTH_BOOTSTRAP_PASSWORD

resolve_config() {
  if [[ -n "${LENS_CONFIG:-}" ]]; then
    echo "$LENS_CONFIG"
    return
  fi
  if [[ -f "$ROOT/config.yaml" ]]; then
    echo "$ROOT/config.yaml"
    return
  fi
  echo "$ROOT/config.example.yaml"
}

CFG="$(resolve_config)"
export LENS_CONFIG="$CFG"

print_access() {
  echo ""
  echo "Gitea Lens (local)"
  echo "  App:      ${WEB_URL}"
  echo "  API:      ${API_URL}"
  echo "  Config:   ${CFG}"
  echo "  Username: ${BOOTSTRAP_USER}"
  echo "  Password: ${LENS_AUTH_BOOTSTRAP_PASSWORD}"
  if [[ -z "${LENS_GITEA_URL:-}" ]]; then
    echo "  OAuth:    disabled (set LENS_GITEA_URL + OAuth client to enable)"
  else
    echo "  OAuth:    Gitea ${LENS_GITEA_URL}"
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
  if [[ "${LENS_NO_BROWSER:-}" == "1" || -n "${CI:-}" ]]; then
    echo "Skipping browser open (LENS_NO_BROWSER or CI set)"
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

"$CONCURRENTLY" -k -n api,web -c blue,green \
  "npm run start:api" \
  "npm run start:web" &
START_PID=$!

cleanup() {
  if kill -0 "$START_PID" 2>/dev/null; then
    kill "$START_PID" 2>/dev/null || true
    wait "$START_PID" 2>/dev/null || true
  fi
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
trap - EXIT INT TERM
exit "$status"
