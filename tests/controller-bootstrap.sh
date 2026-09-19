#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
MOCK_BIN="${TEST_ROOT}/bin"
TARGET_ROOT="${TEST_ROOT}/target"
CALL_LOG="${TEST_ROOT}/calls.log"
INSTALLER_LOG="${TEST_ROOT}/installer.log"
BOOTSTRAP_INPUT="${TEST_ROOT}/bootstrap-input"
REVISION="0123456789abcdef0123456789abcdef01234567"

cleanup() {
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

mkdir -p "$MOCK_BIN" "${TARGET_ROOT}/home/admin"

cat > "${MOCK_BIN}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl %s\n' "$*" >> "$BOOTSTRAP_CALL_LOG"
url=""
output=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      output="$2"
      shift 2
      ;;
    -* ) shift ;;
    *)
      url="$1"
      shift
      ;;
  esac
done
case "$url" in
  https://api.github.com/repos/giovantenne/nixorium/commits/master)
    printf '{\n  "sha": "%s",\n  "commit": {}\n}\n' "$BOOTSTRAP_REVISION"
    ;;
  https://raw.githubusercontent.com/giovantenne/nixorium/*/scripts/install-controller.sh)
    cat > "$output" <<'INSTALLER'
#!/usr/bin/env bash
set -euo pipefail
printf 'flake=%s\nlayout=%s\nlayout_url=%s\nmaster=%s\nstudent=%s\ndisk=%s\nnix_config=%s\n' \
  "$FLAKE_REF" "$DISKO_LAYOUT_FILE" "$DISKO_LAYOUT_URL" "$MASTER_HOST_NUMBER" "$STUDENT_USER" "${1:-}" \
  "${NIX_CONFIG//$'\n'/;}" \
  > "$BOOTSTRAP_INSTALLER_LOG"
grep -Fx 'layout-from-resolved-revision' "$DISKO_LAYOUT_FILE" >/dev/null
printf '{"nodes":{},"root":"root","version":7}\n' > "$NIXORIUM_DEPLOYMENT_PATH/flake.lock"
INSTALLER
    ;;
  https://raw.githubusercontent.com/giovantenne/nixorium/*/lib/disko-layout.nix)
    printf 'layout-from-resolved-revision\n' > "$output"
    ;;
  https://raw.githubusercontent.com/giovantenne/nixorium/*/flake.nix)
    printf '%s\n' 'controllerBootstrapVersion = 1;'
    ;;
  *)
    echo "unexpected curl URL: $url" >&2
    exit 1
    ;;
esac
EOF
chmod +x "${MOCK_BIN}/curl"

cat > "${MOCK_BIN}/nix" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'nix %s\n' "$*" >> "$BOOTSTRAP_CALL_LOG"
case "$*" in
  *"flake init -t github:giovantenne/nixorium/${BOOTSTRAP_REVISION}#site")
    cp -a "$BOOTSTRAP_TEMPLATE/." .
    ;;
  *)
    echo "unexpected nix call: $*" >&2
    exit 1
    ;;
esac
EOF
chmod +x "${MOCK_BIN}/nix"

cat > "${MOCK_BIN}/mkpasswd" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'mkpasswd\n' >> "$BOOTSTRAP_CALL_LOG"
test "$*" = "-m sha-512 --stdin"
IFS= read -r password
printf '$6$testsalt$hash%s\n' "${#password}"
EOF
chmod +x "${MOCK_BIN}/mkpasswd"

cat > "${MOCK_BIN}/loadkeys" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'loadkeys %s\n' "$*" >> "$BOOTSTRAP_CALL_LOG"
if [[ "${BOOTSTRAP_LOADKEYS_FAIL:-false}" == "true" ]]; then
  exit 1
fi
EOF
chmod +x "${MOCK_BIN}/loadkeys"

cat > "${MOCK_BIN}/sudo" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "chown" ]]; then
  exit 0
fi
exec "$@"
EOF
chmod +x "${MOCK_BIN}/sudo"

export PATH="${MOCK_BIN}:$PATH"
export DISPLAY=""
export WAYLAND_DISPLAY=""
export BOOTSTRAP_CALL_LOG="$CALL_LOG"
export BOOTSTRAP_INSTALLER_LOG="$INSTALLER_LOG"
export BOOTSTRAP_REVISION="$REVISION"
export BOOTSTRAP_TEMPLATE="${REPO_ROOT}/templates/site"

printf '%s\n\n\n\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n' \
  'it' \
  'admin-secret' 'admin-secret' \
  'teacher-secret' 'teacher-secret' \
  'student-secret' 'student-secret' \
  '' > "$BOOTSTRAP_INPUT"

if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_INSTALLER_REF="v2.0.0" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master >"${TEST_ROOT}/mismatch.out" 2>&1; then
  echo "mismatched installer ref was accepted" >&2
  exit 1
fi
grep -F "must match the selected release" "${TEST_ROOT}/mismatch.out" >/dev/null
test ! -e "$CALL_LOG"

if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_BOOTSTRAP_TTY="$BOOTSTRAP_INPUT" \
  DISPLAY=:0 \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/graphical.out" 2>&1; then
  echo "bootstrap accepted password setup from a graphical terminal" >&2
  exit 1
fi
grep -F "cannot be verified safely from a graphical terminal" \
  "${TEST_ROOT}/graphical.out" >/dev/null
if grep -F "Set account passwords" "${TEST_ROOT}/graphical.out" >/dev/null; then
  echo "bootstrap requested passwords in an unverified graphical layout" >&2
  exit 1
fi
: > "$CALL_LOG"

printf '%s\n\n\n\n' 'us' > "${TEST_ROOT}/truncated-input"
if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_BOOTSTRAP_TTY="${TEST_ROOT}/truncated-input" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/truncated.out" 2>&1; then
  echo "bootstrap accepted truncated controller setup input" >&2
  exit 1
fi
grep -F "input ended before configuration was complete" \
  "${TEST_ROOT}/truncated.out" >/dev/null
: > "$CALL_LOG"

if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_BOOTSTRAP_TTY="$BOOTSTRAP_INPUT" \
  BOOTSTRAP_LOADKEYS_FAIL=true \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/keymap-failure.out" 2>&1; then
  echo "bootstrap continued after console keymap activation failed" >&2
  exit 1
fi
grep -F "could not activate console keymap 'it2'" \
  "${TEST_ROOT}/keymap-failure.out" >/dev/null
if grep -F "Set account passwords" "${TEST_ROOT}/keymap-failure.out" >/dev/null || \
  grep -F "mkpasswd" "$CALL_LOG" >/dev/null; then
  echo "bootstrap requested or hashed a password before keyboard activation" >&2
  exit 1
fi
: > "$CALL_LOG"

if ! NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_BOOTSTRAP_TTY="$BOOTSTRAP_INPUT" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/install.out" 2>&1; then
  cat "${TEST_ROOT}/install.out" >&2
  exit 1
fi

grep -F "Resolving master to one immutable revision" "${TEST_ROOT}/install.out" >/dev/null
grep -F "Preparing Nixorium master at ${REVISION}" "${TEST_ROOT}/install.out" >/dev/null
grep -F "Recommended environment: official NixOS Minimal ISO in UEFI mode." \
  "${TEST_ROOT}/install.out" >/dev/null
grep -F "commits/master" "$CALL_LOG" >/dev/null
grep -F "raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/scripts/install-controller.sh" "$CALL_LOG" >/dev/null
grep -F "raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/lib/disko-layout.nix" "$CALL_LOG" >/dev/null
grep -F "flake init -t github:giovantenne/nixorium/${REVISION}#site" "$CALL_LOG" >/dev/null
grep -F "raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/flake.nix" "$CALL_LOG" >/dev/null
grep -F "loadkeys it2" "$CALL_LOG" >/dev/null
KEYBOARD_PROMPT_LINE="$(grep -n -m1 -F 'Keyboard layout' "${TEST_ROOT}/install.out" | cut -d: -f1)"
TEACHER_PROMPT_LINE="$(grep -n -m1 -F 'Teacher username' "${TEST_ROOT}/install.out" | cut -d: -f1)"
if (( KEYBOARD_PROMPT_LINE >= TEACHER_PROMPT_LINE )); then
  echo "bootstrap did not ask for the keyboard before account settings" >&2
  exit 1
fi
LOADKEYS_LINE="$(grep -n -m1 -F 'loadkeys it2' "$CALL_LOG" | cut -d: -f1)"
MKPASSWD_LINE="$(grep -n -m1 -F 'mkpasswd' "$CALL_LOG" | cut -d: -f1)"
if (( LOADKEYS_LINE >= MKPASSWD_LINE )); then
  echo "bootstrap hashed a password before applying the selected keymap" >&2
  exit 1
fi
if grep -Eq 'flake lock|bootstrap configure|#lib.controllerBootstrapVersion|#labMeta' "$CALL_LOG"; then
  echo "bootstrap performed Nix work before invoking the disk installer" >&2
  exit 1
fi
grep -F "flake=path:" "$INSTALLER_LOG" >/dev/null
grep -F "layout_url=https://raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/lib/disko-layout.nix" "$INSTALLER_LOG" >/dev/null
grep -F "master=99" "$INSTALLER_LOG" >/dev/null
grep -F "student=student" "$INSTALLER_LOG" >/dev/null
grep -F "disk=/dev/vda" "$INSTALLER_LOG" >/dev/null
grep -F "trusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=" \
  "$INSTALLER_LOG" >/dev/null
if grep -F "6NCHdD59X431o0gWypbMrAURkbJ16ZPMQX27P3FJrRo=" "$INSTALLER_LOG"; then
  echo "bootstrap exported the obsolete cache.nixos.org signing key" >&2
  exit 1
fi
grep -Fx '  inputs.nixorium.url = "github:giovantenne/nixorium/master";' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/flake.nix" >/dev/null
grep -Fx '  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/flake.nix" >/dev/null
grep -Fx '  inputs.nixorium.inputs.nixpkgs.follows = "nixpkgs";' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/flake.nix" >/dev/null
grep -F '"deploymentMode": "controller"' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
grep -F '"pcCount": 0' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
grep -F '"keyboardLayout": "it"' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
grep -F '"consoleKeyMap": "it2"' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
grep -F '"adminPassword": "$6$testsalt$hash12"' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
jq -e '
  .lab.deploymentMode == "controller" and
  .lab.pcCount == 0 and
  .lab.keyboardLayout == "it" and
  .lab.consoleKeyMap == "it2"
' "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
if grep -R -F -e 'admin-secret' -e 'teacher-secret' -e 'student-secret' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment"; then
  echo "plaintext bootstrap password reached the deployment" >&2
  exit 1
fi
test -f "${TARGET_ROOT}/home/admin/nixorium-deployment/flake.lock"

echo "Controller bootstrap tests passed."
