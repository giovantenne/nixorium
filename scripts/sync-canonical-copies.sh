#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || ( "$1" != "--check" && "$1" != "--write" ) ]]; then
  echo "Usage: scripts/sync-canonical-copies.sh --check|--write" >&2
  exit 2
fi

MODE="$1"
SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
REPO_ROOT="${NIXORIUM_SYNC_ROOT:-$SCRIPT_ROOT}"

if [[ ! -d "$REPO_ROOT" ]]; then
  echo "Error: synchronization root is not a directory: $REPO_ROOT" >&2
  exit 1
fi
REPO_ROOT=$(cd "$REPO_ROOT" && pwd -P)

reject_symlinks() {
  local path="$1"
  if [[ -L "$path" ]]; then
    echo "Error: managed path is a symlink: ${path#"$REPO_ROOT"/}" >&2
    return 1
  fi
  if [[ -d "$path" ]]; then
    local link
    link=$(find "$path" -type l -print -quit)
    if [[ -n "$link" ]]; then
      echo "Error: managed tree contains a symlink: ${link#"$REPO_ROOT"/}" >&2
      return 1
    fi
  fi
}

sync_file() {
  local source_relative="$1"
  local destination_relative="$2"
  local source="$REPO_ROOT/$source_relative"
  local destination="$REPO_ROOT/$destination_relative"
  reject_symlinks "$source"
  if [[ -e "$destination" || -L "$destination" ]]; then
    reject_symlinks "$destination"
  fi
  if [[ ! -f "$source" ]]; then
    echo "Error: canonical source is missing: $source_relative" >&2
    return 1
  fi
  if [[ "$MODE" == "--check" ]]; then
    if [[ ! -f "$destination" ]] || ! cmp -s "$source" "$destination"; then
      echo "Out of sync: $source_relative -> $destination_relative" >&2
      return 1
    fi
    return 0
  fi
  mkdir -p "$(dirname "$destination")"
  cp -p "$source" "$destination"
}

sync_tree() {
  local source_relative="$1"
  local destination_relative="$2"
  local source="$REPO_ROOT/$source_relative"
  local destination="$REPO_ROOT/$destination_relative"
  reject_symlinks "$source"
  if [[ -e "$destination" || -L "$destination" ]]; then
    reject_symlinks "$destination"
  fi
  if [[ ! -d "$source" ]]; then
    echo "Error: canonical source tree is missing: $source_relative" >&2
    return 1
  fi
  if [[ "$MODE" == "--check" ]]; then
    if [[ ! -d "$destination" ]] || ! diff -qr "$source" "$destination" >/dev/null; then
      echo "Out of sync, missing, or extra files: $source_relative -> $destination_relative" >&2
      if [[ -d "$destination" ]]; then
        diff -qr "$source" "$destination" >&2 || true
      fi
      return 1
    fi
    return 0
  fi
  local temporary
  temporary=$(mktemp -d "$REPO_ROOT/.canonical-copy.XXXXXX")
  trap 'rm -rf "$temporary"' RETURN
  cp -a "$source/." "$temporary/"
  rm -rf "$destination"
  mkdir -p "$destination"
  cp -a "$temporary/." "$destination/"
  rm -rf "$temporary"
  trap - RETURN
}

status=0
sync_tree "skills/nixorium-maintainer" "templates/site/skills/nixorium-maintainer" || status=1
sync_file "docs/troubleshooting.md" "templates/site/TROUBLESHOOTING.md" || status=1
sync_file "docs/updates.md" "templates/site/UPDATES.md" || status=1
exit "$status"
