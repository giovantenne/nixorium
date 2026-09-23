#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

mkdir -p \
  "$TEMP_DIR/skills/nixorium-maintainer/references" \
  "$TEMP_DIR/templates/site/skills/nixorium-maintainer/references" \
  "$TEMP_DIR/docs" \
  "$TEMP_DIR/templates/site"
printf '%s\n' "skill" > "$TEMP_DIR/skills/nixorium-maintainer/SKILL.md"
printf '%s\n' "reference" > "$TEMP_DIR/skills/nixorium-maintainer/references/example.md"
cp -a "$TEMP_DIR/skills/nixorium-maintainer/." "$TEMP_DIR/templates/site/skills/nixorium-maintainer/"
printf '%s\n' "troubleshooting" > "$TEMP_DIR/docs/troubleshooting.md"
printf '%s\n' "updates" > "$TEMP_DIR/docs/updates.md"
cp "$TEMP_DIR/docs/troubleshooting.md" "$TEMP_DIR/templates/site/TROUBLESHOOTING.md"
cp "$TEMP_DIR/docs/updates.md" "$TEMP_DIR/templates/site/UPDATES.md"

NIXORIUM_SYNC_ROOT="$TEMP_DIR" "$REPO_ROOT/scripts/sync-canonical-copies.sh" --check

printf '%s\n' "extra" > "$TEMP_DIR/templates/site/skills/nixorium-maintainer/extra.md"
printf '%s\n' "stale" > "$TEMP_DIR/templates/site/UPDATES.md"
if NIXORIUM_SYNC_ROOT="$TEMP_DIR" "$REPO_ROOT/scripts/sync-canonical-copies.sh" --check >/dev/null 2>&1; then
  echo "Error: copy check accepted divergent managed files." >&2
  exit 1
fi
test -e "$TEMP_DIR/templates/site/skills/nixorium-maintainer/extra.md"
grep -q stale "$TEMP_DIR/templates/site/UPDATES.md"

NIXORIUM_SYNC_ROOT="$TEMP_DIR" "$REPO_ROOT/scripts/sync-canonical-copies.sh" --write
NIXORIUM_SYNC_ROOT="$TEMP_DIR" "$REPO_ROOT/scripts/sync-canonical-copies.sh" --check
test ! -e "$TEMP_DIR/templates/site/skills/nixorium-maintainer/extra.md"
cmp "$TEMP_DIR/docs/updates.md" "$TEMP_DIR/templates/site/UPDATES.md"

rm "$TEMP_DIR/templates/site/TROUBLESHOOTING.md"
ln -s "$TEMP_DIR/docs/troubleshooting.md" "$TEMP_DIR/templates/site/TROUBLESHOOTING.md"
if NIXORIUM_SYNC_ROOT="$TEMP_DIR" "$REPO_ROOT/scripts/sync-canonical-copies.sh" --write >/dev/null 2>&1; then
  echo "Error: copy synchronization followed a destination symlink." >&2
  exit 1
fi
test -L "$TEMP_DIR/templates/site/TROUBLESHOOTING.md"

echo "Canonical copy synchronization tests passed."
