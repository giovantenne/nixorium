#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
MOCK_BIN="${TEST_ROOT}/bin"
ACTION_LOG="${TEST_ROOT}/actions.log"
LAYOUT="${TEST_ROOT}/disko-layout.nix"
CONFIRM="${TEST_ROOT}/confirm"
TARGET_ROOT="${TEST_ROOT}/target"
DEPLOYMENT="${TEST_ROOT}/deployment"

cleanup() {
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

mkdir -p "$MOCK_BIN" "$TARGET_ROOT" "$DEPLOYMENT"
printf '%s\n' '{ device, studentUser }: {}' > "$LAYOUT"
printf '%s\n' 'YES' > "$CONFIRM"

cat > "${MOCK_BIN}/lsblk" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  "-nrpo MOUNTPOINT /dev/vda") ;;
  "-dn -o PATH,TYPE -P") printf '%s\n' 'PATH="/dev/vda" TYPE="disk"' ;;
  *) echo "unexpected lsblk call: $*" >&2; exit 1 ;;
esac
EOF

cat > "${MOCK_BIN}/nix" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
config=${NIX_CONFIG//$'\n'/;}
printf 'nix|%s|%s|%s\n' "$config" "${XDG_CACHE_HOME:-}" "$*" >> "$CONTROLLER_INSTALLER_LOG"
if [[ "$*" == *"flake lock --override-input nixorium "* ]]; then
  printf '{"nodes":{},"root":"root","version":7}\n' > "$NIXORIUM_DEPLOYMENT_PATH/flake.lock"
fi
EOF

cat > "${MOCK_BIN}/btrfs" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'btrfs|%s\n' "$*" >> "$CONTROLLER_INSTALLER_LOG"
touch "${@: -1}"
EOF

cat > "${MOCK_BIN}/swapon" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'swapon|%s\n' "$*" >> "$CONTROLLER_INSTALLER_LOG"
EOF

cat > "${MOCK_BIN}/swapoff" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'swapoff|%s\n' "$*" >> "$CONTROLLER_INSTALLER_LOG"
EOF

cat > "${MOCK_BIN}/nixos-install" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
config=${NIX_CONFIG//$'\n'/;}
printf 'nixos-install|%s|%s|%s\n' "$config" "$XDG_CACHE_HOME" "$*" >> "$CONTROLLER_INSTALLER_LOG"
EOF

cat > "${MOCK_BIN}/sudo" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
unset NIX_CONFIG XDG_CACHE_HOME
exec "$@"
EOF

chmod +x "${MOCK_BIN}/lsblk" "${MOCK_BIN}/nix" \
  "${MOCK_BIN}/nixos-install" "${MOCK_BIN}/sudo" \
  "${MOCK_BIN}/btrfs" "${MOCK_BIN}/swapon" "${MOCK_BIN}/swapoff"

PATH="${MOCK_BIN}:$PATH" \
CONTROLLER_INSTALLER_LOG="$ACTION_LOG" \
DISKO_LAYOUT_FILE="$LAYOUT" \
FLAKE_REF="path:/deployment" \
NIXORIUM_UPSTREAM_REF="github:giovantenne/nixorium/revision" \
NIXORIUM_DEPLOYMENT_PATH="$DEPLOYMENT" \
NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
NIXORIUM_INSTALLER_TTY="$CONFIRM" \
MASTER_HOST_NUMBER=99 \
STUDENT_USER=student \
  bash "${REPO_ROOT}/scripts/install-controller.sh" /dev/vda \
  > "${TEST_ROOT}/output"

test "$(grep -c '^nix|' "$ACTION_LOG")" -eq 2
test "$(grep -c -- '--dry-run' "$ACTION_LOG" || true)" -eq 0
grep -F 'max-jobs = 1' "$ACTION_LOG" >/dev/null
grep -F 'cores = 1' "$ACTION_LOG" >/dev/null
grep -F 'trusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=' \
  "$ACTION_LOG" >/dev/null
if grep -F '6NCHdD59X431o0gWypbMrAURkbJ16ZPMQX27P3FJrRo=' "$ACTION_LOG"; then
  echo "controller installer exported the obsolete cache.nixos.org signing key" >&2
  exit 1
fi
grep -F 'run github:giovantenne/nixorium/revision#disko -- --mode disko ' "$ACTION_LOG" >/dev/null
grep -F 'flake lock --override-input nixorium github:giovantenne/nixorium/revision' "$ACTION_LOG" >/dev/null
grep -F 'btrfs|filesystem mkswapfile --size 4G' "$ACTION_LOG" >/dev/null
grep -F 'swapon|' "$ACTION_LOG" >/dev/null
grep -F 'swapoff|' "$ACTION_LOG" >/dev/null
grep -F 'nixos-install|experimental-features = nix-command flakes;' "$ACTION_LOG" >/dev/null
grep -F "|${TARGET_ROOT}/var/cache/nixorium-bootstrap|--root ${TARGET_ROOT} --flake path:/deployment#pc99 --no-write-lock-file --no-root-passwd" "$ACTION_LOG" >/dev/null
test -f "$DEPLOYMENT/flake.lock"
grep -F 'downloaded into the installed disk, not the live ISO memory' "${TEST_ROOT}/output" >/dev/null

printf '%s\n' 'NO' > "$CONFIRM"
: > "$ACTION_LOG"
if PATH="${MOCK_BIN}:$PATH" \
  CONTROLLER_INSTALLER_LOG="$ACTION_LOG" \
  DISKO_LAYOUT_FILE="$LAYOUT" \
  FLAKE_REF="path:/deployment" \
  NIXORIUM_UPSTREAM_REF="github:giovantenne/nixorium/revision" \
  NIXORIUM_DEPLOYMENT_PATH="$DEPLOYMENT" \
  NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_INSTALLER_TTY="$CONFIRM" \
    bash "${REPO_ROOT}/scripts/install-controller.sh" /dev/vda \
    > "${TEST_ROOT}/cancel-output" 2>&1; then
  echo "installer accepted a cancelled destructive confirmation" >&2
  exit 1
fi
if [[ -s "$ACTION_LOG" ]]; then
  echo "cancelled installer performed a destructive action" >&2
  exit 1
fi

echo "Controller installer tests passed."
