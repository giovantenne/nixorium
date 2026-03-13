#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SECRET_KEY="${REPO_ROOT}/secret-key"
CACHE_PORT=$(awk '/cachePort =/ { gsub(/[^0-9]/, ""); print; exit }' "${REPO_ROOT}/flake.nix")
CACHE_PORT="${CACHE_PORT:-5000}"

if [ ! -f "${SECRET_KEY}" ]; then
  echo "Missing ${SECRET_KEY}. Copy the secret-key into the repo root." >&2
  exit 1
fi

CONFIG_FILE=$(mktemp)
trap 'rm -f "${CONFIG_FILE}"' EXIT

cat > "${CONFIG_FILE}" <<EOF
bind = "[::]:${CACHE_PORT}"
sign_key_paths = ["${SECRET_KEY}"]
EOF

# Run without exec so the trap fires on exit
CONFIG_FILE="${CONFIG_FILE}" nix run nixpkgs#harmonia
