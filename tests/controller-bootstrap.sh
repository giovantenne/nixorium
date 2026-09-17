#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)
MOCK_BIN="${TEST_ROOT}/bin"
TARGET_ROOT="${TEST_ROOT}/target"
CALL_LOG="${TEST_ROOT}/calls.log"
INSTALLER_LOG="${TEST_ROOT}/installer.log"
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
printf 'flake=%s\nlayout=%s\nlayout_url=%s\nmaster=%s\nstudent=%s\ndisk=%s\n' \
  "$FLAKE_REF" "$DISKO_LAYOUT_FILE" "$DISKO_LAYOUT_URL" "$MASTER_HOST_NUMBER" "$STUDENT_USER" "${1:-}" \
  > "$BOOTSTRAP_INSTALLER_LOG"
grep -Fx 'layout-from-resolved-revision' "$DISKO_LAYOUT_FILE" >/dev/null
INSTALLER
    ;;
  https://raw.githubusercontent.com/giovantenne/nixorium/*/lib/disko-layout.nix)
    printf 'layout-from-resolved-revision\n' > "$output"
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
    cat > flake.nix <<'FLAKE'
{
  inputs.nixorium.url = "github:giovantenne/nixorium/master";
  outputs = { self, nixorium }: {};
}
FLAKE
    ;;
  *"flake lock --override-input nixorium github:giovantenne/nixorium/${BOOTSTRAP_REVISION}")
    printf '{"nodes":{},"root":"root","version":7}\n' > flake.lock
    ;;
  *"#labMeta.controller.number --json")
    printf '99\n'
    ;;
  *"#labMeta.users.student --raw")
    printf 'student\n'
    ;;
  *)
    echo "unexpected nix call: $*" >&2
    exit 1
    ;;
esac
EOF
chmod +x "${MOCK_BIN}/nix"

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
export BOOTSTRAP_CALL_LOG="$CALL_LOG"
export BOOTSTRAP_INSTALLER_LOG="$INSTALLER_LOG"
export BOOTSTRAP_REVISION="$REVISION"

if NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  NIXORIUM_INSTALLER_REF="v2.0.0" \
  "$REPO_ROOT/install.sh" --release master >"${TEST_ROOT}/mismatch.out" 2>&1; then
  echo "mismatched installer ref was accepted" >&2
  exit 1
fi
grep -F "must match the selected release" "${TEST_ROOT}/mismatch.out" >/dev/null
test ! -e "$CALL_LOG"

NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  "$REPO_ROOT/install.sh" --release master --disk /dev/vda \
  >"${TEST_ROOT}/install.out" 2>&1

grep -F "Resolving master to one immutable revision" "${TEST_ROOT}/install.out" >/dev/null
grep -F "Preparing Nixorium master at ${REVISION}" "${TEST_ROOT}/install.out" >/dev/null
grep -F "commits/master" "$CALL_LOG" >/dev/null
grep -F "raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/scripts/install-controller.sh" "$CALL_LOG" >/dev/null
grep -F "raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/lib/disko-layout.nix" "$CALL_LOG" >/dev/null
grep -F "flake init -t github:giovantenne/nixorium/${REVISION}#site" "$CALL_LOG" >/dev/null
grep -F "flake lock --override-input nixorium github:giovantenne/nixorium/${REVISION}" "$CALL_LOG" >/dev/null
grep -F "flake=path:" "$INSTALLER_LOG" >/dev/null
grep -F "layout_url=https://raw.githubusercontent.com/giovantenne/nixorium/${REVISION}/lib/disko-layout.nix" "$INSTALLER_LOG" >/dev/null
grep -F "master=99" "$INSTALLER_LOG" >/dev/null
grep -F "student=student" "$INSTALLER_LOG" >/dev/null
grep -F "disk=/dev/vda" "$INSTALLER_LOG" >/dev/null
grep -Fx '  inputs.nixorium.url = "github:giovantenne/nixorium/master";' \
  "${TARGET_ROOT}/home/admin/nixorium-deployment/flake.nix" >/dev/null
test -f "${TARGET_ROOT}/home/admin/nixorium-deployment/flake.lock"

echo "Controller bootstrap tests passed."
