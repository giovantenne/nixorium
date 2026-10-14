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

mkdir -p "$MOCK_BIN" "${TARGET_ROOT}/home/admin" \
  "${TEST_ROOT}/gnome-target/home/admin" "${TEST_ROOT}/gnome-fail-target/home/admin" \
  "${TEST_ROOT}/ssh-target/home/admin"

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
    printf 'controllerBootstrapVersion = %s;\n' "${BOOTSTRAP_CAPABILITY_VERSION:-2}"
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
if [[ "${*: -1}" == "-" ]]; then
  while IFS= read -r line; do printf 'overlay: %s\n' "$line" >> "$BOOTSTRAP_CALL_LOG"; done
fi
if [[ "${BOOTSTRAP_LOADKEYS_FAIL:-false}" == "true" ]]; then
  exit 1
fi
EOF
chmod +x "${MOCK_BIN}/loadkeys"

cat > "${MOCK_BIN}/gsettings" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'gsettings %s\n' "$*" >> "$BOOTSTRAP_CALL_LOG"
if [[ "${BOOTSTRAP_GSETTINGS_FAIL:-false}" == "true" ]]; then
  exit 1
fi
EOF
chmod +x "${MOCK_BIN}/gsettings"

cat > "${MOCK_BIN}/sudo" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "chown" ]]; then
  exit 0
fi
printf 'sudo %s\n' "$*" >> "$BOOTSTRAP_CALL_LOG"
exec "$@"
EOF
chmod +x "${MOCK_BIN}/sudo"

export PATH="${MOCK_BIN}:$PATH"
export DISPLAY=""
export WAYLAND_DISPLAY=""
unset SSH_CONNECTION SSH_TTY
export BOOTSTRAP_CALL_LOG="$CALL_LOG"
export BOOTSTRAP_INSTALLER_LOG="$INSTALLER_LOG"
export BOOTSTRAP_REVISION="$REVISION"
export BOOTSTRAP_TEMPLATE="${REPO_ROOT}/templates/site"
export NIXORIUM_INSTALLER_EFI_DIRECTORY="${TEST_ROOT}/efi"
mkdir -p "$NIXORIUM_INSTALLER_EFI_DIRECTORY"

printf '%s\n' \
  '3' 'Europe/Rome' '' '' \
  'admin-secret' 'admin-secret' \
  'teacher-secret' 'teacher-secret' \
  'student-secret' 'student-secret' \
  '' '3' '' > "$BOOTSTRAP_INPUT"

if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_INSTALLER_REF="v2.0.0" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master >"${TEST_ROOT}/mismatch.out" 2>&1; then
  echo "mismatched installer ref was accepted" >&2
  exit 1
fi
grep -F "must match the selected release" "${TEST_ROOT}/mismatch.out" >/dev/null
test ! -e "$CALL_LOG"

# GNOME live session: the layout is set through gsettings as the user, never
# through sudo, and the graphical terminal is no longer refused.
GNOME_INPUT="${TEST_ROOT}/gnome-input"
printf '%s\n' \
  'it' 'Europe/Rome' '' '' \
  'admin-secret' 'admin-secret' \
  'teacher-secret' 'teacher-secret' \
  'student-secret' 'student-secret' \
  '' '3' '' > "$GNOME_INPUT"
NIXORIUM_TARGET_ROOT="${TEST_ROOT}/gnome-target" \
  NIXORIUM_BOOTSTRAP_TTY="$GNOME_INPUT" \
  WAYLAND_DISPLAY=wayland-0 \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/gnome.out" 2>&1 || { cat "${TEST_ROOT}/gnome.out" >&2; exit 1; }
grep -F "gsettings set org.gnome.desktop.input-sources sources [('xkb', 'it')]" "$CALL_LOG" >/dev/null
grep -F "gsettings set org.gnome.desktop.input-sources current 0" "$CALL_LOG" >/dev/null
if grep -E '^sudo gsettings|loadkeys' "$CALL_LOG" >/dev/null; then
  echo "GNOME keyboard activation used sudo or loadkeys" >&2
  exit 1
fi
if grep -F "graphical terminal" "${TEST_ROOT}/gnome.out" >/dev/null; then
  echo "bootstrap still refuses graphical terminals" >&2
  exit 1
fi
grep -F "Use the same keyboard you will use at the login screen." "${TEST_ROOT}/gnome.out" >/dev/null
: > "$CALL_LOG"

# gsettings failure: ask for the GNOME top bar menu (Enter) and carry on.
sed '1a\
' "$GNOME_INPUT" > "${TEST_ROOT}/gnome-fail-input"
NIXORIUM_TARGET_ROOT="${TEST_ROOT}/gnome-fail-target" \
  NIXORIUM_BOOTSTRAP_TTY="${TEST_ROOT}/gnome-fail-input" \
  DISPLAY=:0 \
  BOOTSTRAP_GSETTINGS_FAIL=true \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/gnome-fail.out" 2>&1 || { cat "${TEST_ROOT}/gnome-fail.out" >&2; exit 1; }
grep -F "keyboard menu in the GNOME top bar" "${TEST_ROOT}/gnome-fail.out" >/dev/null
grep -F "The controller is installed." "${TEST_ROOT}/gnome-fail.out" >/dev/null
: > "$CALL_LOG"

# SSH without a graphical session: no loadkeys, explanatory text.
printf '%s\n' 'it' 'Europe/Rome' '' '' \
  'admin-secret' 'admin-secret' 'teacher-secret' 'teacher-secret' \
  'student-secret' 'student-secret' '' '3' '' > "${TEST_ROOT}/ssh-input"
NIXORIUM_TARGET_ROOT="${TEST_ROOT}/ssh-target" \
  NIXORIUM_BOOTSTRAP_TTY="${TEST_ROOT}/ssh-input" \
  SSH_CONNECTION="192.0.2.1 1 192.0.2.2 22" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/ssh.out" 2>&1 || { cat "${TEST_ROOT}/ssh.out" >&2; exit 1; }
grep -F "connected over SSH" "${TEST_ROOT}/ssh.out" >/dev/null
if grep -E 'loadkeys|gsettings' "$CALL_LOG" >/dev/null; then
  echo "SSH session tried to change the keyboard" >&2
  exit 1
fi
: > "$CALL_LOG"

# The keyboard is a readable list: invalid answers are refused, a number and a
# code both work, and the layout is applied immediately with no confirmation
# step before the next question.
printf '%s\n' '9' 'xx' '2' > "${TEST_ROOT}/keyboard-list-input"
if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_BOOTSTRAP_TTY="${TEST_ROOT}/keyboard-list-input" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/keyboard-list.out" 2>&1; then
  echo "bootstrap accepted truncated input after the keyboard list" >&2
  exit 1
fi
grep -F "1  English (US)" "${TEST_ROOT}/keyboard-list.out" >/dev/null
grep -F "3  Italian" "${TEST_ROOT}/keyboard-list.out" >/dev/null
test "$(grep -c -F 'Type a number from 1 to 6' "${TEST_ROOT}/keyboard-list.out")" -eq 2
grep -F "loadkeys uk" "$CALL_LOG" >/dev/null
if grep -E "Does it match|Test line|Received:" "${TEST_ROOT}/keyboard-list.out" >/dev/null; then
  echo "bootstrap still asks for a keyboard test line" >&2
  exit 1
fi
if grep -F "Step 3 of 5 · Passwords" "${TEST_ROOT}/keyboard-list.out" >/dev/null; then
  echo "bootstrap asked for passwords before the time zone" >&2
  exit 1
fi
: > "$CALL_LOG"

# Each layout loads its closest console keymap; Italian also gets the AltGr
# overlay on stdin so { } [ ] ~ and the euro sign match the XKB layout.
for CASE in '1:us' '2:uk' '3:it' '4:fr-pc' '5:de-latin1' '6:es'; do
  printf '%s\n' "${CASE%%:*}" > "${TEST_ROOT}/keymap-case-input"
  NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
    NIXORIUM_BOOTSTRAP_TTY="${TEST_ROOT}/keymap-case-input" \
    timeout --foreground --kill-after=2s 10s \
    "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
    >/dev/null 2>&1 || true
  grep -E "^loadkeys ${CASE#*:}( -)?\$" "$CALL_LOG" >/dev/null
done
grep -F "overlay: altgr keycode 8 = braceleft" "$CALL_LOG" >/dev/null
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
grep -F "Setup input ended before all questions were answered" \
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
grep -F "Could not activate console keymap 'it'" \
  "${TEST_ROOT}/keymap-failure.out" >/dev/null
if grep -F "Step 3 of 5 · Passwords" "${TEST_ROOT}/keymap-failure.out" >/dev/null || \
  grep -F "mkpasswd" "$CALL_LOG" >/dev/null; then
  echo "bootstrap requested or hashed a password before keyboard activation" >&2
  exit 1
fi
: > "$CALL_LOG"

printf '%s\n' \
  '3' 'Europe/Rome' '' '' \
  'admin-secret' 'admin-secret' \
  'teacher-secret' 'teacher-secret' \
  'student-secret' 'student-secret' \
  '' '' 'n' > "${TEST_ROOT}/profile-cancel-input"
rm -f "$INSTALLER_LOG"
if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_BOOTSTRAP_TTY="${TEST_ROOT}/profile-cancel-input" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/profile-cancel.out" 2>&1; then
  echo "bootstrap accepted cancelled software profile" >&2
  exit 1
fi
grep -F "Check the applications" "${TEST_ROOT}/profile-cancel.out" >/dev/null
grep -F "Choose again" "${TEST_ROOT}/profile-cancel.out" >/dev/null
grep -F "Nothing on this computer was changed" "${TEST_ROOT}/profile-cancel.out" >/dev/null
test ! -e "$INSTALLER_LOG"
: > "$CALL_LOG"

# Answering no to the review asks the questions again instead of cancelling.
printf '%s\n' \
  '3' 'Europe/Rome' '' '' \
  'admin-secret' 'admin-secret' \
  'teacher-secret' 'teacher-secret' \
  'student-secret' 'student-secret' \
  'n' > "${TEST_ROOT}/review-again-input"
if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_BOOTSTRAP_TTY="${TEST_ROOT}/review-again-input" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/review-again.out" 2>&1; then
  echo "bootstrap continued after the answers were rejected" >&2
  exit 1
fi
grep -F "Let's go through them again" "${TEST_ROOT}/review-again.out" >/dev/null
test "$(grep -c -F 'Step 1 of 5 · Keyboard and time zone' "${TEST_ROOT}/review-again.out")" -eq 2
test ! -e "$INSTALLER_LOG"
: > "$CALL_LOG"

if NIXORIUM_TARGET_ROOT="${TEST_ROOT}/v1-target" \
  NIXORIUM_BOOTSTRAP_TTY="$BOOTSTRAP_INPUT" \
  BOOTSTRAP_CAPABILITY_VERSION=1 \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/v1-install.out" 2>&1; then
  echo "bootstrap accepted a release without capability version 2" >&2
  exit 1
fi
grep -F "too old for this installer" "${TEST_ROOT}/v1-install.out" >/dev/null
test ! -e "$INSTALLER_LOG"
: > "$CALL_LOG"

if ! NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_BOOTSTRAP_TTY="$BOOTSTRAP_INPUT" \
  timeout --foreground --kill-after=2s 10s \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/install.out" 2>&1; then
  cat "${TEST_ROOT}/install.out" >&2
  exit 1
fi

grep -F "Finding the exact source of Nixorium master" "${TEST_ROOT}/install.out" >/dev/null
grep -F "Using Nixorium master, revision ${REVISION}" "${TEST_ROOT}/install.out" >/dev/null
grep -F "Time zone [Europe/Rome]" "${TEST_ROOT}/install.out" >/dev/null
grep -F "Recommended environment: official NixOS ISO (Minimal or Graphical) in UEFI mode." \
  "${TEST_ROOT}/install.out" >/dev/null
for UI_TEXT in \
  "Controller setup for a NixOS computer lab" \
  "Nothing on this computer changes until you type ERASE." \
  "Step 1 of 5 · Keyboard and time zone" \
  "Step 2 of 5 · Account names" \
  "Step 3 of 5 · Passwords" \
  "Check your answers" \
  "Preparing the lab configuration" \
  "Step 4 of 5 · Applications" \
  "All done" \
  "The controller is installed." \
  "Installation > Network boot (PXE)"; do
  grep -F "$UI_TEXT" "${TEST_ROOT}/install.out" >/dev/null
done
grep -F "> Keyboard layout" "${TEST_ROOT}/install.out" >/dev/null
grep -F "> Time zone" "${TEST_ROOT}/install.out" >/dev/null
grep -F "[....] Finding the exact source of Nixorium master" \
  "${TEST_ROOT}/install.out" >/dev/null
if grep -F 'package IDs' "${TEST_ROOT}/install.out" >/dev/null; then
  echo "bootstrap exposed package IDs to the operator" >&2
  exit 1
fi
grep -F "commits/master" "$CALL_LOG" >/dev/null
grep -F "raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/scripts/install-controller.sh" "$CALL_LOG" >/dev/null
grep -F "raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/lib/disko-layout.nix" "$CALL_LOG" >/dev/null
grep -F "flake init -t github:giovantenne/nixorium/${REVISION}#site" "$CALL_LOG" >/dev/null
grep -F "raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/flake.nix" "$CALL_LOG" >/dev/null
grep -F "loadkeys it -" "$CALL_LOG" >/dev/null
KEYBOARD_PROMPT_LINE="$(grep -n -m1 -F 'Keyboard layout' "${TEST_ROOT}/install.out" | cut -d: -f1)"
TIME_ZONE_PROMPT_LINE="$(grep -n -m1 -F 'Time zone' "${TEST_ROOT}/install.out" | cut -d: -f1)"
TEACHER_PROMPT_LINE="$(grep -n -m1 -F 'Teacher username' "${TEST_ROOT}/install.out" | cut -d: -f1)"
if (( KEYBOARD_PROMPT_LINE >= TIME_ZONE_PROMPT_LINE || TIME_ZONE_PROMPT_LINE >= TEACHER_PROMPT_LINE )); then
  echo "bootstrap did not ask for keyboard and time zone before account settings" >&2
  exit 1
fi
LOADKEYS_LINE="$(grep -n -m1 -F 'loadkeys it -' "$CALL_LOG" | cut -d: -f1)"
MKPASSWD_LINE="$(grep -n -m1 -F 'mkpasswd' "$CALL_LOG" | cut -d: -f1)"
if (( LOADKEYS_LINE >= MKPASSWD_LINE )); then
  echo "bootstrap hashed a password before applying the selected keymap" >&2
  exit 1
fi
PROFILE_PROMPT_LINE="$(grep -n -m1 -F 'Applications [Essential]' "${TEST_ROOT}/install.out" | cut -d: -f1)"
SETTINGS_REVIEW_LINE="$(grep -n -m1 -F 'Are these answers correct?' "${TEST_ROOT}/install.out" | cut -d: -f1)"
INSTALL_LINE="$(grep -n -m1 -F 'Saving the lab configuration on the new disk' "${TEST_ROOT}/install.out" | cut -d: -f1)"
if (( PROFILE_PROMPT_LINE <= SETTINGS_REVIEW_LINE || PROFILE_PROMPT_LINE >= INSTALL_LINE )); then
  echo "software profile was not selected after settings and before installation" >&2
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
# The console follows the XKB layout; the deprecated key is no longer written.
if grep -F '"consoleKeyMap"' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null; then
  echo "bootstrap still writes consoleKeyMap" >&2
  exit 1
fi
grep -F '"timeZone": "Europe/Rome"' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
grep -F '"admin":"$6$testsalt$hash12"' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-credentials.json" >/dev/null
! grep -F '$6$' "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json"
test "$(stat -c '%a' "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-credentials.json")" = 600
! git -C "${TARGET_ROOT}/home/admin/nixorium-deployment" ls-files | grep -Fx lab-credentials.json
jq -e '
  .lab.deploymentMode == "controller" and
  .lab.pcCount == 0 and
  .lab.timeZone == "Europe/Rome" and
  .lab.keyboardLayout == "it" and
  (.lab | has("consoleKeyMap") | not)
' "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
# The template inherits the enabled Nix default; bootstrap preserves that omission.
# tests/eval-lab-settings.nix checks its effective Assistant value.
jq -e '.lab | has("assistant") | not' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-settings.json" >/dev/null
jq -e '
  .schemaVersion == 1 and
  all(.packages[]; .scope.kind == "shared") and
  any(.packages[]; .package == "nodejs") and
  any(.packages[]; .package == "opencode") and
  any(.packages[]; .package == "pi-coding-agent") and
  any(.packages[]; .package == "vscode") and
  any(.packages[]; .package == "vlc")
' "${TARGET_ROOT}/home/admin/nixorium-deployment/lab-software.json" >/dev/null
test -f "${TARGET_ROOT}/home/admin/nixorium-deployment/software-presets.json"
for path in \
  '.cache/opencode' \
  '.config/opencode' \
  '.local/share/opencode' \
  '.local/npm' \
  '.npm' \
  '.opencode' \
  '.pi'; do
  grep -F "\"$path\"" \
    "${TARGET_ROOT}/home/admin/nixorium-deployment/flake.nix" >/dev/null
done
if grep -R -F -e 'admin-secret' -e 'teacher-secret' -e 'student-secret' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment"; then
  echo "plaintext bootstrap password reached the deployment" >&2
  exit 1
fi
test -f "${TARGET_ROOT}/home/admin/nixorium-deployment/flake.lock"

echo "Controller bootstrap tests passed."
