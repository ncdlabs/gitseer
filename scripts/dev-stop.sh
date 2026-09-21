#!/usr/bin/env bash
# Stop local `npm run start` listeners (Go API + Vite).
set -euo pipefail

API_PORT="${LENS_API_PORT:-8090}"
WEB_PORT="${LENS_WEB_PORT:-5173}"

kill_port() {
  local port="$1"
  local pids
  pids="$(lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -z "$pids" ]]; then
    echo "No listener on :$port"
    return 0
  fi
  echo "Stopping listeners on :$port ($pids)"
  # shellcheck disable=SC2086
  kill $pids 2>/dev/null || true
}

force_kill_port() {
  local port="$1"
  local pids
  pids="$(lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -z "$pids" ]]; then
    return 0
  fi
  echo "Force-killing listeners on :$port ($pids)"
  # shellcheck disable=SC2086
  kill -9 $pids 2>/dev/null || true
}

kill_port "$API_PORT"
kill_port "$WEB_PORT"
sleep 0.3
force_kill_port "$API_PORT"
force_kill_port "$WEB_PORT"

echo "Stopped (API :$API_PORT, web :$WEB_PORT)"
