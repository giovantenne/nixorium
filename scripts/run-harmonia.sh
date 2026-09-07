#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
if [[ -n "${LAB_REPO_ROOT:-}" ]]; then
  REPO_ROOT="${LAB_REPO_ROOT}"
else
  REPO_ROOT=$(cd "${SCRIPT_DIR}/.." && pwd)
fi
SECRET_KEY="${REPO_ROOT}/secret-key"

# shellcheck source=/home/admin/nixorium/scripts/lib/lab-meta.sh
source "${SCRIPT_DIR}/lib/lab-meta.sh"
load_lab_meta "${REPO_ROOT}"

CACHE_PORT="${LAB_CACHE_PORT}"

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
