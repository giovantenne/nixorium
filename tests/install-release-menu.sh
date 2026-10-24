#!/usr/bin/env bash
set -euo pipefail

# The bootstrap installer offers the latest stable release (the default), the
# latest prerelease only when it is newer, and master.
REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
eval "$(sed -n '/^release_menu_entries() {/,/^}/p' "${REPO_ROOT}/install.sh")"

check() {
  local expected="$1" releases="$2" actual
  actual="$(release_menu_entries "$releases" | paste -sd' ')"
  if [[ "$actual" != "$expected" ]]; then
    echo "release menu for [${releases//$'\n'/ }] with default ${DEFAULT_RELEASE}: got '${actual}', want '${expected}'" >&2
    exit 1
  fi
}

DEFAULT_RELEASE="v3.10.0-beta.4"
check "v3.9.0 v3.10.0-beta.4 master" $'v3.10.0-beta.4\nv3.10.0-beta.3\nv3.9.0\nv3.8.0'
check "v3.10.0 master" $'v3.10.0\nv3.10.0-beta.4\nv3.9.0'
check "v4.0.0-beta.1 master" $'v4.0.0-beta.1'
check "v3.9.0 master" $'nightly\nv3.9.0\nlatest'
check "v3.10.0-beta.4 master" ""
DEFAULT_RELEASE="v3.10.0"
check "v3.10.0 master" ""
DEFAULT_RELEASE="master"
check "master" ""

echo "Installer release menu tests passed."
