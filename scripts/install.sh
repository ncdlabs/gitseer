#!/usr/bin/env bash
# Gitea Lens guided installer.
#
# Interactive (default):
#   ./scripts/install.sh
#
# Non-interactive from an install YAML:
#   ./scripts/install.sh --config install.yaml --non-interactive
#
# Values are resolved in order: defaults → config file → existing .env →
# process environment → interactive prompts (skipped in --non-interactive).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

CONFIG_FILE=""
NON_INTERACTIVE=0
METHOD=""
NO_START=0
WRITE_CONFIG=1
INSTALL_UI=""
GITEA_CUSTOM_PATH=""

# Resolved settings (empty means unset)
GITEA_URL=""
GITEA_TOKEN=""
SERVER_EXTERNAL_URL=""
SERVER_LISTEN=""
OAUTH_CLIENT_ID=""
OAUTH_CLIENT_SECRET=""
BOOTSTRAP_PASSWORD=""
ALLOW_PRIVATE_NETWORK=""
INSTANCE_NAME=""
DATABASE_DRIVER=""
DATABASE_PATH=""
DATABASE_DSN=""
START_AFTER=""

usage() {
  cat <<'EOF'
Gitea Lens installer

Usage:
  ./scripts/install.sh [options]

Options:
  --config PATH           Read values from a YAML file (install or lens format)
  --non-interactive, -y   Do not prompt; fail if required values are missing
  --method MODE           compose (default) | binary
  --no-start              Configure only; do not build or start the service
  --no-write-config       Do not write config.yaml / .env (use env only)
  --install-ui            After start, run lens install-ui against Gitea custom/
  --gitea-custom PATH     Gitea custom templates root (with --install-ui)
  -h, --help              Show this help

Examples:
  ./scripts/install.sh
  ./scripts/install.sh --config install.example.yaml --non-interactive
  ./scripts/install.sh --method binary --config config.yaml -y

Required for a working install:
  gitea url, gitea token, external url, bootstrap password
  (oauth client id recommended for normal Gitea login)
EOF
}

log()  { printf '==> %s\n' "$*"; }
warn() { printf '!  %s\n' "$*" >&2; }
die()  { printf '✗ %s\n' "$*" >&2; exit 1; }
ok()   { printf '✓ %s\n' "$*"; }

have() { command -v "$1" >/dev/null 2>&1; }

is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|y|Y|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

# Set DEST only if currently empty and SRC is non-empty.
# Note: do not use `[[ ]] &&` as the last statement of a function under `set -e`.
set_if_empty() {
  local dest_name="$1"
  local src="$2"
  local cur
  cur="$(eval "printf '%s' \"\${$dest_name}\"")"
  if [[ -z "$cur" && -n "$src" ]]; then
    printf -v "$dest_name" '%s' "$src"
  fi
  return 0
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config)
        [[ $# -ge 2 ]] || die "--config requires a path"
        CONFIG_FILE="$2"
        shift 2
        ;;
      --non-interactive|-y|--yes)
        NON_INTERACTIVE=1
        shift
        ;;
      --method)
        [[ $# -ge 2 ]] || die "--method requires compose or binary"
        METHOD="$2"
        shift 2
        ;;
      --no-start)
        NO_START=1
        shift
        ;;
      --no-write-config)
        WRITE_CONFIG=0
        shift
        ;;
      --install-ui)
        INSTALL_UI=true
        shift
        ;;
      --gitea-custom)
        [[ $# -ge 2 ]] || die "--gitea-custom requires a path"
        GITEA_CUSTOM_PATH="$2"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        die "unknown option: $1 (try --help)"
        ;;
    esac
  done
}

# Load KEY=VALUE lines from .env (no export of comments/blank).
load_dotenv() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  log "Loading $file"
  local line key value
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line#"${line%%[![:space:]]*}"}"
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" == *=* ]] || continue
    key="${line%%=*}"
    value="${line#*=}"
    key="${key%"${key##*[![:space:]]}"}"
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    if [[ "$value" == \"*\" && "$value" == *\" ]]; then
      value="${value:1:${#value}-2}"
    elif [[ "$value" == \'*\' && "$value" == *\' ]]; then
      value="${value:1:${#value}-2}"
    fi
    case "$key" in
      LENS_GITEA_URL) set_if_empty GITEA_URL "$value" ;;
      LENS_GITEA_TOKEN) set_if_empty GITEA_TOKEN "$value" ;;
      LENS_SERVER_EXTERNAL_URL) set_if_empty SERVER_EXTERNAL_URL "$value" ;;
      LENS_SERVER_LISTEN) set_if_empty SERVER_LISTEN "$value" ;;
      LENS_AUTH_OAUTH_CLIENT_ID) set_if_empty OAUTH_CLIENT_ID "$value" ;;
      LENS_AUTH_OAUTH_CLIENT_SECRET) set_if_empty OAUTH_CLIENT_SECRET "$value" ;;
      LENS_AUTH_BOOTSTRAP_PASSWORD) set_if_empty BOOTSTRAP_PASSWORD "$value" ;;
      LENS_GITEA_ALLOW_PRIVATE_NETWORK) set_if_empty ALLOW_PRIVATE_NETWORK "$value" ;;
      LENS_UI_INSTANCE_NAME) set_if_empty INSTANCE_NAME "$value" ;;
      LENS_DATABASE_DRIVER) set_if_empty DATABASE_DRIVER "$value" ;;
      LENS_DATABASE_PATH) set_if_empty DATABASE_PATH "$value" ;;
      LENS_DATABASE_DSN) set_if_empty DATABASE_DSN "$value" ;;
      LENS_INSTALL_METHOD) set_if_empty METHOD "$value" ;;
      LENS_INSTALL_UI) set_if_empty INSTALL_UI "$value" ;;
      LENS_GITEA_CUSTOM_PATH) set_if_empty GITEA_CUSTOM_PATH "$value" ;;
      LENS_INSTALL_START) set_if_empty START_AFTER "$value" ;;
    esac
  done <"$file"
}

# Pull already-exported LENS_* into locals (env wins over file for prompts later).
load_process_env() {
  set_if_empty GITEA_URL "${LENS_GITEA_URL:-}"
  set_if_empty GITEA_TOKEN "${LENS_GITEA_TOKEN:-}"
  set_if_empty SERVER_EXTERNAL_URL "${LENS_SERVER_EXTERNAL_URL:-}"
  set_if_empty SERVER_LISTEN "${LENS_SERVER_LISTEN:-}"
  set_if_empty OAUTH_CLIENT_ID "${LENS_AUTH_OAUTH_CLIENT_ID:-}"
  set_if_empty OAUTH_CLIENT_SECRET "${LENS_AUTH_OAUTH_CLIENT_SECRET:-}"
  set_if_empty BOOTSTRAP_PASSWORD "${LENS_AUTH_BOOTSTRAP_PASSWORD:-}"
  set_if_empty ALLOW_PRIVATE_NETWORK "${LENS_GITEA_ALLOW_PRIVATE_NETWORK:-}"
  set_if_empty INSTANCE_NAME "${LENS_UI_INSTANCE_NAME:-}"
  set_if_empty DATABASE_DRIVER "${LENS_DATABASE_DRIVER:-}"
  set_if_empty DATABASE_PATH "${LENS_DATABASE_PATH:-}"
  set_if_empty DATABASE_DSN "${LENS_DATABASE_DSN:-}"
  set_if_empty METHOD "${LENS_INSTALL_METHOD:-}"
  set_if_empty INSTALL_UI "${LENS_INSTALL_UI:-}"
  set_if_empty GITEA_CUSTOM_PATH "${LENS_GITEA_CUSTOM_PATH:-}"
  set_if_empty START_AFTER "${LENS_INSTALL_START:-}"
}

# Prefer env over config for final apply — call after config load to re-apply env.
apply_env_overrides() {
  [[ -n "${LENS_GITEA_URL:-}" ]] && GITEA_URL="$LENS_GITEA_URL"
  [[ -n "${LENS_GITEA_TOKEN:-}" ]] && GITEA_TOKEN="$LENS_GITEA_TOKEN"
  [[ -n "${LENS_SERVER_EXTERNAL_URL:-}" ]] && SERVER_EXTERNAL_URL="$LENS_SERVER_EXTERNAL_URL"
  [[ -n "${LENS_SERVER_LISTEN:-}" ]] && SERVER_LISTEN="$LENS_SERVER_LISTEN"
  [[ -n "${LENS_AUTH_OAUTH_CLIENT_ID:-}" ]] && OAUTH_CLIENT_ID="$LENS_AUTH_OAUTH_CLIENT_ID"
  [[ -n "${LENS_AUTH_OAUTH_CLIENT_SECRET:-}" ]] && OAUTH_CLIENT_SECRET="$LENS_AUTH_OAUTH_CLIENT_SECRET"
  [[ -n "${LENS_AUTH_BOOTSTRAP_PASSWORD:-}" ]] && BOOTSTRAP_PASSWORD="$LENS_AUTH_BOOTSTRAP_PASSWORD"
  [[ -n "${LENS_GITEA_ALLOW_PRIVATE_NETWORK:-}" ]] && ALLOW_PRIVATE_NETWORK="$LENS_GITEA_ALLOW_PRIVATE_NETWORK"
  [[ -n "${LENS_UI_INSTANCE_NAME:-}" ]] && INSTANCE_NAME="$LENS_UI_INSTANCE_NAME"
  [[ -n "${LENS_DATABASE_DRIVER:-}" ]] && DATABASE_DRIVER="$LENS_DATABASE_DRIVER"
  [[ -n "${LENS_DATABASE_PATH:-}" ]] && DATABASE_PATH="$LENS_DATABASE_PATH"
  [[ -n "${LENS_DATABASE_DSN:-}" ]] && DATABASE_DSN="$LENS_DATABASE_DSN"
  [[ -n "${LENS_INSTALL_METHOD:-}" ]] && METHOD="$LENS_INSTALL_METHOD"
  [[ -n "${LENS_INSTALL_UI:-}" ]] && INSTALL_UI="$LENS_INSTALL_UI"
  [[ -n "${LENS_GITEA_CUSTOM_PATH:-}" ]] && GITEA_CUSTOM_PATH="$LENS_GITEA_CUSTOM_PATH"
  [[ -n "${LENS_INSTALL_START:-}" ]] && START_AFTER="$LENS_INSTALL_START"
  return 0
}

yaml_get() {
  # Usage: yaml_get FILE KEY...
  # Prints the scalar value for a dotted path, or empty.
  local file="$1"
  shift
  local path="$*"

  if have yq; then
    local expr val
    expr="$(printf '.%s' "${path// /.}")"
    val="$(yq -r "${expr}" "$file" 2>/dev/null || true)"
    if [[ "$val" == "null" ]]; then
      val=""
    fi
    printf '%s' "$val"
    return 0
  fi

  if have python3; then
    python3 - "$file" "$path" <<'PY' 2>/dev/null || true
import sys

def load_minimal(path):
    data = {}
    stack = [data]
    indents = [-1]
    with open(path, encoding="utf-8") as f:
        for raw in f:
            line = raw.rstrip("\n")
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            indent = len(line) - len(line.lstrip(" "))
            while len(indents) > 1 and indent <= indents[-1]:
                stack.pop()
                indents.pop()
            if ":" not in line:
                continue
            key, _, val = line.lstrip().partition(":")
            key = key.strip()
            val = val.strip().strip('"').strip("'")
            if val == "" or val in ("|", ">"):
                child = {}
                stack[-1][key] = child
                stack.append(child)
                indents.append(indent)
            else:
                low = val.lower()
                if low in ("true", "false"):
                    stack[-1][key] = low == "true"
                else:
                    stack[-1][key] = val
    return data

path_parts = sys.argv[2].split()
try:
    import yaml  # type: ignore
except ImportError:
    data = load_minimal(sys.argv[1])
else:
    with open(sys.argv[1], encoding="utf-8") as f:
        data = yaml.safe_load(f) or {}

cur = data
for p in path_parts:
    if not isinstance(cur, dict) or p not in cur:
        sys.exit(0)
    cur = cur[p]
if cur is None:
    sys.exit(0)
if isinstance(cur, bool):
    print("true" if cur else "false")
elif isinstance(cur, (dict, list)):
    sys.exit(0)
else:
    print(cur)
PY
    return 0
  fi

  die "need yq or python3 to read YAML config ($file)"
}

load_config_file() {
  local file="$1"
  [[ -f "$file" ]] || die "config file not found: $file"
  log "Reading config $file"

  # Installer-oriented flat / nested keys
  set_if_empty METHOD "$(yaml_get "$file" method)"
  set_if_empty START_AFTER "$(yaml_get "$file" start)"
  set_if_empty INSTALL_UI "$(yaml_get "$file" install_ui)"
  set_if_empty GITEA_CUSTOM_PATH "$(yaml_get "$file" gitea_custom_path)"

  set_if_empty GITEA_URL "$(yaml_get "$file" gitea_url)"
  set_if_empty GITEA_TOKEN "$(yaml_get "$file" gitea_token)"
  set_if_empty SERVER_EXTERNAL_URL "$(yaml_get "$file" server_external_url)"
  set_if_empty SERVER_LISTEN "$(yaml_get "$file" listen)"
  set_if_empty OAUTH_CLIENT_ID "$(yaml_get "$file" oauth_client_id)"
  set_if_empty OAUTH_CLIENT_SECRET "$(yaml_get "$file" oauth_client_secret)"
  set_if_empty BOOTSTRAP_PASSWORD "$(yaml_get "$file" bootstrap_password)"
  set_if_empty ALLOW_PRIVATE_NETWORK "$(yaml_get "$file" allow_private_network)"
  set_if_empty INSTANCE_NAME "$(yaml_get "$file" instance_name)"
  set_if_empty DATABASE_DRIVER "$(yaml_get "$file" database_driver)"
  set_if_empty DATABASE_PATH "$(yaml_get "$file" database_path)"
  set_if_empty DATABASE_DSN "$(yaml_get "$file" database_dsn)"

  # Lens config.example.yaml shape
  set_if_empty GITEA_URL "$(yaml_get "$file" gitea url)"
  set_if_empty GITEA_TOKEN "$(yaml_get "$file" gitea token)"
  set_if_empty ALLOW_PRIVATE_NETWORK "$(yaml_get "$file" gitea allow_private_network)"
  set_if_empty SERVER_EXTERNAL_URL "$(yaml_get "$file" server external_url)"
  set_if_empty SERVER_LISTEN "$(yaml_get "$file" server listen)"
  set_if_empty OAUTH_CLIENT_ID "$(yaml_get "$file" auth oauth_client_id)"
  set_if_empty OAUTH_CLIENT_SECRET "$(yaml_get "$file" auth oauth_client_secret)"
  set_if_empty BOOTSTRAP_PASSWORD "$(yaml_get "$file" auth bootstrap_password)"
  set_if_empty INSTANCE_NAME "$(yaml_get "$file" ui instance_name)"
  set_if_empty DATABASE_DRIVER "$(yaml_get "$file" database driver)"
  set_if_empty DATABASE_PATH "$(yaml_get "$file" database path)"
  set_if_empty DATABASE_DSN "$(yaml_get "$file" database dsn)"

  # Nested installer block (optional)
  set_if_empty METHOD "$(yaml_get "$file" install method)"
  set_if_empty START_AFTER "$(yaml_get "$file" install start)"
  set_if_empty INSTALL_UI "$(yaml_get "$file" install install_ui)"
  set_if_empty GITEA_CUSTOM_PATH "$(yaml_get "$file" install gitea_custom_path)"
}

prompt() {
  # prompt VAR "Question" [default] [--secret]
  local var="$1"
  local question="$2"
  local default="${3:-}"
  local secret=0
  [[ "${4:-}" == "--secret" ]] && secret=1

  local cur
  cur="$(eval "printf '%s' \"\${$var}\"")"
  if [[ -n "$cur" ]]; then
    if [[ $secret -eq 1 ]]; then
      printf '   %s: (set, leave blank to keep)\n' "$question"
    else
      printf '   %s [%s]: ' "$question" "$cur"
    fi
  elif [[ -n "$default" ]]; then
    printf '   %s [%s]: ' "$question" "$default"
  else
    printf '   %s: ' "$question"
  fi

  local answer
  if [[ $secret -eq 1 ]]; then
    # shellcheck disable=SC2162
    read -r -s answer
    printf '\n'
  else
    # shellcheck disable=SC2162
    read -r answer
  fi

  if [[ -z "$answer" ]]; then
    if [[ -n "$cur" ]]; then
      return 0
    fi
    if [[ -n "$default" ]]; then
      printf -v "$var" '%s' "$default"
    fi
    return 0
  fi
  printf -v "$var" '%s' "$answer"
}

require_value() {
  local var="$1"
  local label="$2"
  local cur
  cur="$(eval "printf '%s' \"\${$var}\"")"
  if [[ -z "$cur" ]]; then
    if [[ $NON_INTERACTIVE -eq 1 ]]; then
      die "missing required value: $label (set in --config, .env, or environment)"
    fi
  fi
}

generate_password() {
  if have openssl; then
    openssl rand -base64 24 | tr -d '/+=\n' | head -c 24
    return 0
  fi
  if have python3; then
    python3 -c 'import secrets,string; a=string.ascii_letters+string.digits; print("".join(secrets.choice(a) for _ in range(24)))'
    return 0
  fi
  date +%s | shasum | head -c 24
}

looks_private_url() {
  local u
  u="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$u" in
    *localhost*|*127.0.0.1*|*.local*|*192.168.*|*10.*) return 0 ;;
  esac
  [[ "$u" =~ 172\.(1[6-9]|2[0-9]|3[0-1])\. ]] && return 0
  return 1
}

gather_interactive() {
  if [[ $NON_INTERACTIVE -eq 0 ]]; then
    log "Guided setup — press Enter to accept defaults / keep existing values"
    echo
  fi

  if [[ -z "$METHOD" ]]; then
    if [[ $NON_INTERACTIVE -eq 1 ]]; then
      METHOD=compose
    else
      prompt METHOD "Install method (compose|binary)" "compose"
    fi
  fi
  case "$METHOD" in
    compose|binary) ;;
    *) die "method must be compose or binary (got: $METHOD)" ;;
  esac

  if [[ $NON_INTERACTIVE -eq 0 ]]; then
    prompt GITEA_URL "Gitea base URL (e.g. https://git.example.com)"
    prompt GITEA_TOKEN "Gitea API token" "" --secret
    prompt SERVER_EXTERNAL_URL "Public Lens URL" "http://127.0.0.1:8090"
    prompt SERVER_LISTEN "Listen address" "0.0.0.0:8090"
    prompt OAUTH_CLIENT_ID "Gitea OAuth client ID (recommended)"
    prompt OAUTH_CLIENT_SECRET "Gitea OAuth client secret (optional for public PKCE)" "" --secret
    if [[ -z "$BOOTSTRAP_PASSWORD" ]]; then
      local gen
      gen="$(generate_password)"
      prompt BOOTSTRAP_PASSWORD "Bootstrap password" "$gen" --secret
    else
      prompt BOOTSTRAP_PASSWORD "Bootstrap password" "" --secret
    fi
    prompt INSTANCE_NAME "UI instance name" "Gitea Lens"
    if [[ -z "$ALLOW_PRIVATE_NETWORK" ]]; then
      if looks_private_url "$GITEA_URL"; then
        prompt ALLOW_PRIVATE_NETWORK "Allow private/lab Gitea URL (true|false)" "true"
      else
        prompt ALLOW_PRIVATE_NETWORK "Allow private/lab Gitea URL (true|false)" "false"
      fi
    else
      prompt ALLOW_PRIVATE_NETWORK "Allow private/lab Gitea URL (true|false)" "$ALLOW_PRIVATE_NETWORK"
    fi
    prompt DATABASE_DRIVER "Database driver (sqlite|postgres)" "sqlite"
    if [[ "${DATABASE_DRIVER}" == "postgres"* ]]; then
      prompt DATABASE_DSN "Postgres DSN"
    else
      if [[ "$METHOD" == compose ]]; then
        prompt DATABASE_PATH "SQLite path (in container)" "/data/lens.db"
      else
        prompt DATABASE_PATH "SQLite path" "data/lens.db"
      fi
    fi
    if [[ -z "$START_AFTER" && $NO_START -eq 0 ]]; then
      prompt START_AFTER "Start Lens after install (true|false)" "true"
    fi
    if [[ -z "$INSTALL_UI" ]]; then
      prompt INSTALL_UI "Install Gitea UI nav links (true|false)" "false"
    fi
    if is_true "$INSTALL_UI"; then
      prompt GITEA_CUSTOM_PATH "Gitea custom path" "/var/lib/gitea/custom"
    fi
  fi
}

apply_defaults() {
  set_if_empty METHOD "compose"
  set_if_empty SERVER_EXTERNAL_URL "http://127.0.0.1:8090"
  set_if_empty SERVER_LISTEN "0.0.0.0:8090"
  set_if_empty INSTANCE_NAME "Gitea Lens"
  set_if_empty DATABASE_DRIVER "sqlite"
  set_if_empty ALLOW_PRIVATE_NETWORK "false"
  set_if_empty INSTALL_UI "false"
  if [[ -z "$START_AFTER" ]]; then
    if [[ $NO_START -eq 1 ]]; then
      START_AFTER=false
    else
      START_AFTER=true
    fi
  fi
  if [[ -z "$DATABASE_PATH" ]]; then
    if [[ "$METHOD" == compose ]]; then
      DATABASE_PATH="/data/lens.db"
    else
      DATABASE_PATH="data/lens.db"
    fi
  fi
  if [[ -z "$BOOTSTRAP_PASSWORD" ]]; then
    if [[ $NON_INTERACTIVE -eq 1 ]]; then
      die "missing required value: bootstrap_password / LENS_AUTH_BOOTSTRAP_PASSWORD"
    fi
    BOOTSTRAP_PASSWORD="$(generate_password)"
    warn "Generated bootstrap password (saved to .env / config.yaml)"
  fi
}

validate_required() {
  require_value GITEA_URL "gitea_url / LENS_GITEA_URL"
  require_value GITEA_TOKEN "gitea_token / LENS_GITEA_TOKEN"
  require_value SERVER_EXTERNAL_URL "server_external_url / LENS_SERVER_EXTERNAL_URL"
  require_value BOOTSTRAP_PASSWORD "bootstrap_password / LENS_AUTH_BOOTSTRAP_PASSWORD"

  [[ -n "$GITEA_URL" ]] || die "Gitea URL is required"
  [[ -n "$GITEA_TOKEN" ]] || die "Gitea API token is required"
  [[ -n "$SERVER_EXTERNAL_URL" ]] || die "external URL is required"
  [[ -n "$BOOTSTRAP_PASSWORD" ]] || die "bootstrap password is required"

  if [[ -z "$OAUTH_CLIENT_ID" ]]; then
    warn "OAuth client ID is empty — only bootstrap password login will work until you set LENS_AUTH_OAUTH_CLIENT_ID"
  fi

  case "$METHOD" in
    compose|binary) ;;
    *) die "method must be compose or binary" ;;
  esac
}

version_at_least() {
  # version_at_least CURRENT MAJOR [MINOR]
  local current="$1"
  local need_major="$2"
  local need_minor="${3:-0}"
  local major minor
  major="$(printf '%s' "$current" | sed -E 's/^[^0-9]*([0-9]+).*/\1/')"
  minor="$(printf '%s' "$current" | sed -E 's/^[^0-9]*[0-9]+\.([0-9]+).*/\1/')"
  [[ -z "$minor" || ! "$minor" =~ ^[0-9]+$ ]] && minor=0
  [[ -z "$major" || ! "$major" =~ ^[0-9]+$ ]] && return 1
  if (( major > need_major )); then return 0; fi
  if (( major < need_major )); then return 1; fi
  (( minor >= need_minor ))
}

ensure_compose_deps() {
  local engine=""
  if have podman; then
    engine=podman
  elif have docker; then
    engine=docker
  else
    die "need podman (preferred) or docker for compose installs"
  fi

  if "$engine" compose version >/dev/null 2>&1; then
    ok "$engine compose available"
    COMPOSE_CMD=("$engine" compose)
    return 0
  fi

  if [[ "$engine" == podman ]] && have podman-compose; then
    warn "podman compose plugin missing; falling back to podman-compose (prefer podman compose)"
    COMPOSE_CMD=(podman-compose)
    return 0
  fi

  die "$engine found but compose is not available (install the compose plugin)"
}

ensure_binary_deps() {
  have go || die "Go is required for binary installs (need Go 1.22+)"
  have node || die "Node.js is required for binary installs (need Node 22+)"
  have npm || die "npm is required for binary installs"
  have make || die "make is required for binary installs"

  local go_ver node_ver
  go_ver="$(go env GOVERSION 2>/dev/null || go version)"
  go_ver="${go_ver#go}"
  go_ver="${go_ver%% *}"
  if ! version_at_least "$go_ver" 1 22; then
    die "Go 1.22+ required (found $go_ver)"
  fi
  ok "Go $go_ver"

  node_ver="$(node -v 2>/dev/null | sed 's/^v//')"
  if ! version_at_least "$node_ver" 22; then
    die "Node.js 22+ required (found ${node_ver:-unknown})"
  fi
  ok "Node.js $node_ver"
}

# shellcheck disable=SC2034
COMPOSE_CMD=()

write_dotenv() {
  local out="$ROOT/.env"
  log "Writing $out"
  umask 077
  cat >"$out" <<EOF
# Generated by scripts/install.sh — do not commit
LENS_GITEA_URL=${GITEA_URL}
LENS_GITEA_TOKEN=${GITEA_TOKEN}
LENS_SERVER_EXTERNAL_URL=${SERVER_EXTERNAL_URL}
LENS_SERVER_LISTEN=${SERVER_LISTEN}
LENS_AUTH_OAUTH_CLIENT_ID=${OAUTH_CLIENT_ID}
LENS_AUTH_OAUTH_CLIENT_SECRET=${OAUTH_CLIENT_SECRET}
LENS_AUTH_BOOTSTRAP_PASSWORD=${BOOTSTRAP_PASSWORD}
LENS_GITEA_ALLOW_PRIVATE_NETWORK=${ALLOW_PRIVATE_NETWORK}
LENS_UI_INSTANCE_NAME=${INSTANCE_NAME}
LENS_DATABASE_DRIVER=${DATABASE_DRIVER}
LENS_DATABASE_PATH=${DATABASE_PATH}
LENS_DATABASE_DSN=${DATABASE_DSN}
LENS_INSTALL_METHOD=${METHOD}
LENS_INSTALL_UI=${INSTALL_UI}
LENS_GITEA_CUSTOM_PATH=${GITEA_CUSTOM_PATH}
EOF
  ok "Wrote .env"
}

write_config_yaml() {
  local out="$ROOT/config.yaml"
  log "Writing $out"
  umask 077
  {
    cat <<EOF
# Generated by scripts/install.sh — do not commit
server:
  listen: ${SERVER_LISTEN}
  external_url: ${SERVER_EXTERNAL_URL}

database:
  driver: ${DATABASE_DRIVER}
EOF
    if [[ "${DATABASE_DRIVER}" == postgres* ]]; then
      printf '  dsn: %s\n' "$DATABASE_DSN"
    else
      # Host path for binary; compose overrides via LENS_DATABASE_PATH
      local host_path="$DATABASE_PATH"
      if [[ "$METHOD" == compose && "$DATABASE_PATH" == /data/* ]]; then
        host_path="data/lens.db"
      fi
      printf '  path: %s\n' "$host_path"
    fi
    cat <<EOF

gitea:
  url: ${GITEA_URL}
  allow_private_network: ${ALLOW_PRIVATE_NETWORK}
  # token via LENS_GITEA_TOKEN / .env

sync:
  reconcile_interval: 5m
  history_days: 30

auth:
  provider: gitea
  session_ttl: 24h
EOF
    [[ -n "$OAUTH_CLIENT_ID" ]] && printf '  oauth_client_id: %s\n' "$OAUTH_CLIENT_ID"
    # secrets stay in env / .env
    cat <<EOF

ui:
  instance_name: ${INSTANCE_NAME}

log:
  level: info
  format: json

retention:
  runs_days: 90
  webhooks_days: 30
  attention_days: 180
EOF
  } >"$out"
  ok "Wrote config.yaml"
}

export_runtime_env() {
  export LENS_GITEA_URL="$GITEA_URL"
  export LENS_GITEA_TOKEN="$GITEA_TOKEN"
  export LENS_SERVER_EXTERNAL_URL="$SERVER_EXTERNAL_URL"
  export LENS_SERVER_LISTEN="$SERVER_LISTEN"
  export LENS_AUTH_OAUTH_CLIENT_ID="$OAUTH_CLIENT_ID"
  export LENS_AUTH_OAUTH_CLIENT_SECRET="$OAUTH_CLIENT_SECRET"
  export LENS_AUTH_BOOTSTRAP_PASSWORD="$BOOTSTRAP_PASSWORD"
  export LENS_GITEA_ALLOW_PRIVATE_NETWORK="$ALLOW_PRIVATE_NETWORK"
  export LENS_UI_INSTANCE_NAME="$INSTANCE_NAME"
  export LENS_DATABASE_DRIVER="$DATABASE_DRIVER"
  export LENS_DATABASE_PATH="$DATABASE_PATH"
  export LENS_DATABASE_DSN="$DATABASE_DSN"
}

run_compose_install() {
  ensure_compose_deps
  export_runtime_env
  if is_true "$START_AFTER"; then
    log "Building and starting with ${COMPOSE_CMD[*]}"
    "${COMPOSE_CMD[@]}" -f "$ROOT/compose.yaml" up --build -d
    ok "Compose stack is up"
    log "Open ${SERVER_EXTERNAL_URL}"
  else
    ok "Compose deps OK — start later with: ${COMPOSE_CMD[*]} -f compose.yaml up --build -d"
  fi
}

run_binary_install() {
  ensure_binary_deps
  export_runtime_env
  if is_true "$START_AFTER"; then
    log "Installing module / frontend deps and building"
    make -C "$ROOT" deps
    make -C "$ROOT" build
    ok "Built $ROOT/bin/lens"
    log "Starting ./bin/lens serve --config config.yaml"
    mkdir -p "$ROOT/data"
    nohup "$ROOT/bin/lens" serve --config "$ROOT/config.yaml" \
      >"$ROOT/data/lens.log" 2>&1 &
    echo $! >"$ROOT/data/lens.pid"
    ok "Lens started (pid $(cat "$ROOT/data/lens.pid"), log data/lens.log)"
    log "Open ${SERVER_EXTERNAL_URL}"
  else
    log "Installing module / frontend deps and building"
    make -C "$ROOT" deps
    make -C "$ROOT" build
    ok "Built $ROOT/bin/lens — start later with: ./bin/lens serve --config config.yaml"
  fi
}

maybe_install_ui() {
  is_true "$INSTALL_UI" || return 0
  [[ -n "$GITEA_CUSTOM_PATH" ]] || die "--install-ui requires gitea custom path"

  local bin="$ROOT/bin/lens"
  if [[ ! -x "$bin" ]]; then
    if [[ "$METHOD" == binary ]]; then
      die "lens binary missing; build failed?"
    fi
    log "Building lens binary for install-ui"
    if have go && have node && have make; then
      ensure_binary_deps
      make -C "$ROOT" build
    else
      die "install-ui needs a local ./bin/lens (install Go/Node and re-run, or build first)"
    fi
  fi

  log "Installing Gitea UI links → $GITEA_CUSTOM_PATH"
  "$bin" install-ui --custom-path "$GITEA_CUSTOM_PATH" --lens-url "$SERVER_EXTERNAL_URL"
  ok "Gitea UI templates updated (restart Gitea to pick them up)"
}

print_summary() {
  echo
  log "Install summary"
  echo "   method:          $METHOD"
  echo "   gitea:           $GITEA_URL"
  echo "   external url:    $SERVER_EXTERNAL_URL"
  echo "   oauth client id: ${OAUTH_CLIENT_ID:-"(not set)"}"
  echo "   private network: $ALLOW_PRIVATE_NETWORK"
  echo "   config:          $ROOT/config.yaml"
  echo "   env file:        $ROOT/.env"
  echo
  echo "   Next steps:"
  echo "   1. Open ${SERVER_EXTERNAL_URL}"
  echo "   2. Sign in (Gitea OAuth or bootstrap password)"
  echo "   3. Click Sync now"
  if [[ -n "$OAUTH_CLIENT_ID" ]]; then
    echo "   4. Confirm OAuth redirect URI is ${SERVER_EXTERNAL_URL%/}/api/v1/auth/callback"
  else
    echo "   4. Create a Gitea OAuth app and re-run with oauth_client_id set"
  fi
}

main() {
  parse_args "$@"

  echo
  log "Gitea Lens installer"
  echo "   repo: $ROOT"
  echo

  # Resolution order: defaults filled later; config → .env → env overrides → prompts
  if [[ -n "$CONFIG_FILE" ]]; then
    load_config_file "$CONFIG_FILE"
  elif [[ -f "$ROOT/config.yaml" && $NON_INTERACTIVE -eq 1 ]]; then
    load_config_file "$ROOT/config.yaml"
  fi

  load_dotenv "$ROOT/.env"
  load_process_env
  apply_env_overrides

  gather_interactive
  apply_defaults
  if [[ $NO_START -eq 1 ]]; then
    START_AFTER=false
  fi
  validate_required

  if [[ $WRITE_CONFIG -eq 1 ]]; then
    write_dotenv
    write_config_yaml
  fi

  case "$METHOD" in
    compose) run_compose_install ;;
    binary)  run_binary_install ;;
  esac

  maybe_install_ui
  print_summary
}

main "$@"
