#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
MOCK_BIN="${TEST_ROOT}/bin"
ACTION_LOG="${TEST_ROOT}/actions.log"
LAYOUT="${TEST_ROOT}/disko-layout.nix"
CONFIRM="${TEST_ROOT}/confirm"

cleanup() {
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

mkdir -p "$MOCK_BIN"
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
printf 'nix|%s|%s\n' "$NIX_CONFIG" "$*" >> "$CONTROLLER_INSTALLER_LOG"
EOF

cat > "${MOCK_BIN}/nixos-install" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'nixos-install|%s\n' "$*" >> "$CONTROLLER_INSTALLER_LOG"
EOF

cat > "${MOCK_BIN}/sudo" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exec "$@"
EOF

chmod +x "${MOCK_BIN}/lsblk" "${MOCK_BIN}/nix" \
  "${MOCK_BIN}/nixos-install" "${MOCK_BIN}/sudo"

PATH="${MOCK_BIN}:$PATH" \
CONTROLLER_INSTALLER_LOG="$ACTION_LOG" \
DISKO_LAYOUT_FILE="$LAYOUT" \
FLAKE_REF="path:/deployment" \
NIXORIUM_INSTALLER_TTY="$CONFIRM" \
MASTER_HOST_NUMBER=99 \
STUDENT_USER=student \
  bash "${REPO_ROOT}/scripts/install-controller.sh" /dev/vda \
  > "${TEST_ROOT}/output"

test "$(grep -c '^nix|' "$ACTION_LOG")" -eq 3
test "$(grep -c -- '--dry-run' "$ACTION_LOG")" -eq 2
grep -F 'max-jobs = 1' "$ACTION_LOG" >/dev/null
grep -F 'cores = 1' "$ACTION_LOG" >/dev/null
grep -F 'build path:/deployment#disko --dry-run --no-link --no-write-lock-file' "$ACTION_LOG" >/dev/null
grep -F 'build path:/deployment#nixosConfigurations.pc99.config.system.build.toplevel --dry-run --no-link --no-write-lock-file' "$ACTION_LOG" >/dev/null
grep -F 'run path:/deployment#disko -- --mode disko ' "$ACTION_LOG" >/dev/null
grep -F 'nixos-install|--flake path:/deployment#pc99 --no-write-lock-file --no-root-passwd' "$ACTION_LOG" >/dev/null
grep -F 'without downloading the full controller system' "${TEST_ROOT}/output" >/dev/null
grep -F 'downloaded into the installed disk, not the live ISO memory' "${TEST_ROOT}/output" >/dev/null

printf '%s\n' 'NO' > "$CONFIRM"
: > "$ACTION_LOG"
if PATH="${MOCK_BIN}:$PATH" \
  CONTROLLER_INSTALLER_LOG="$ACTION_LOG" \
  DISKO_LAYOUT_FILE="$LAYOUT" \
  FLAKE_REF="path:/deployment" \
  NIXORIUM_INSTALLER_TTY="$CONFIRM" \
    bash "${REPO_ROOT}/scripts/install-controller.sh" /dev/vda \
    > "${TEST_ROOT}/cancel-output" 2>&1; then
  echo "installer accepted a cancelled destructive confirmation" >&2
  exit 1
fi
test "$(grep -c -- '--dry-run' "$ACTION_LOG")" -eq 2
if grep -Eq 'run .*#disko|^nixos-install\|' "$ACTION_LOG"; then
  echo "cancelled installer performed a destructive action" >&2
  exit 1
fi

echo "Controller installer tests passed."
