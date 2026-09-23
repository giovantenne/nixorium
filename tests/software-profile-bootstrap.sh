#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_ROOT=$(mktemp -d)

cleanup() {
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

CATALOG="$TEST_ROOT/software-presets.json"
SOFTWARE="$TEST_ROOT/lab-software.json"
cp "$REPO_ROOT/templates/site/software-presets.json" "$CATALOG"
cp "$REPO_ROOT/templates/site/lab-software.json" "$SOFTWARE"

printf '\n\n\n' | bash "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$CATALOG" "$SOFTWARE" > "$TEST_ROOT/default.out"
jq -S . "$REPO_ROOT/templates/site/lab-software.json" > "$TEST_ROOT/default-expected.json"
jq -S . "$SOFTWARE" > "$TEST_ROOT/default-actual.json"
cmp "$TEST_ROOT/default-expected.json" "$TEST_ROOT/default-actual.json"
grep -F 'Selected Essential' "$TEST_ROOT/default.out" >/dev/null

cp "$REPO_ROOT/templates/site/lab-software.json" "$SOFTWARE"
printf '3\nvlc, gcc,vlc\n\n' | bash \
  "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$CATALOG" "$SOFTWARE" > "$TEST_ROOT/programming.out"
jq -e '
  .schemaVersion == 1 and
  all(.packages[]; .scope.kind == "shared") and
  any(.packages[]; .package == "nodejs") and
  any(.packages[]; .package == "opencode") and
  any(.packages[]; .package == "pi-coding-agent") and
  any(.packages[]; .package == "vscode") and
  (any(.packages[]; .package == "vlc") | not) and
  (any(.packages[]; .package == "gcc") | not)
' "$SOFTWARE" >/dev/null
grep -F 'Programming (programming)' "$TEST_ROOT/programming.out" >/dev/null
grep -F 'Exclusions: gcc, vlc' "$TEST_ROOT/programming.out" >/dev/null

cp "$REPO_ROOT/templates/site/lab-software.json" "$SOFTWARE"
ESSENTIAL_PACKAGES="$(jq -r '.presets[] | select(.id == "essential") | .packages | join(",")' "$CATALOG")"
printf '99\nessential\n%s\n\n' "$ESSENTIAL_PACKAGES" | bash \
  "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$CATALOG" "$SOFTWARE" > "$TEST_ROOT/empty.out"
jq -e '.packages == []' "$SOFTWARE" >/dev/null
grep -F 'Choose a listed number or profile ID.' "$TEST_ROOT/empty.out" >/dev/null

cp "$REPO_ROOT/templates/site/lab-software.json" "$SOFTWARE"
BEFORE="$(sha256sum "$SOFTWARE")"
if printf '\n\nno\n' | bash \
  "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$CATALOG" "$SOFTWARE" > "$TEST_ROOT/cancel.out" 2>&1; then
  echo "cancelled software profile selection succeeded" >&2
  exit 1
fi
AFTER="$(sha256sum "$SOFTWARE")"
test "$BEFORE" = "$AFTER"
test -z "$(find "$TEST_ROOT" -maxdepth 1 -name '.lab-software.json.tmp.*' -print -quit)"
grep -F 'disk was not changed' "$TEST_ROOT/cancel.out" >/dev/null

jq '.schemaVersion = 99' "$CATALOG" > "$TEST_ROOT/invalid-catalog.json"
if printf '\n\n\n' | bash \
  "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$TEST_ROOT/invalid-catalog.json" "$SOFTWARE" > "$TEST_ROOT/invalid.out" 2>&1; then
  echo "invalid software profile catalog was accepted" >&2
  exit 1
fi
grep -F 'software-presets.json from the selected revision is invalid' "$TEST_ROOT/invalid.out" >/dev/null

echo "Software profile bootstrap tests passed."
