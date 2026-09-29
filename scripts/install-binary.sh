#!/usr/bin/env bash
# Download a GitSeer release binary for this OS/arch into the current directory.
#
# One-liner:
#   curl -fsSL https://raw.githubusercontent.com/ncdlabs/gitseer/main/scripts/install-binary.sh | bash
#
# Override version:
#   VER=1.0.8 curl -fsSL … | bash
#
# Download only (do not start):
#   curl -fsSL … | bash -s -- --no-start
set -euo pipefail

VER="${VER:-1.0.8}"
START=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-start) START=0 ;;
    -h|--help)
      cat <<'EOF'
Download a GitSeer release binary for this OS/arch.

Usage:
  curl -fsSL https://raw.githubusercontent.com/ncdlabs/gitseer/main/scripts/install-binary.sh | bash
  curl -fsSL … | bash -s -- --no-start
  VER=1.0.8 curl -fsSL … | bash

Options:
  --no-start   Download and chmod only; do not run ./gitseer serve
  -h, --help   Show this help
EOF
      exit 0
      ;;
    *)
      printf '✗ unknown option: %s\n' "$1" >&2
      exit 1
      ;;
  esac
  shift
done

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *)
    printf '✗ unsupported arch: %s\n' "$ARCH" >&2
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  *)
    printf '✗ unsupported OS: %s (download a Windows .exe from GitHub Releases)\n' "$OS" >&2
    exit 1
    ;;
esac

URL="https://github.com/ncdlabs/gitseer/releases/download/v${VER}/gitseer_${VER}_${OS}_${ARCH}"
printf '==> downloading %s\n' "$URL"
curl -fsSL -o gitseer "$URL"
chmod +x gitseer
printf '✓ installed ./gitseer (v%s %s/%s)\n' "$VER" "$OS" "$ARCH"

if [[ "$START" -eq 1 ]]; then
  exec ./gitseer serve --config config.yaml
fi
