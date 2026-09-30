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
PROFILE="$TEST_ROOT/workspace-profile.json"
cp "$REPO_ROOT/templates/site/software-presets.json" "$CATALOG"
cp "$REPO_ROOT/templates/site/lab-software.json" "$SOFTWARE"
cp "$REPO_ROOT/templates/site/workspace-profile.json" "$PROFILE"
cp "$REPO_ROOT/templates/site/workspace-profile.programming.example.json" "$TEST_ROOT/"

printf '\n\n' | bash "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$CATALOG" "$SOFTWARE" > "$TEST_ROOT/default.out"
jq -S . "$REPO_ROOT/templates/site/lab-software.json" > "$TEST_ROOT/default-expected.json"
jq -S . "$SOFTWARE" > "$TEST_ROOT/default-actual.json"
cmp "$TEST_ROOT/default-expected.json" "$TEST_ROOT/default-actual.json"
grep -F 'Selected Essential' "$TEST_ROOT/default.out" >/dev/null
# Essential keeps the common student profile.
cmp "$REPO_ROOT/templates/site/workspace-profile.json" "$PROFILE"

cp "$REPO_ROOT/templates/site/lab-software.json" "$SOFTWARE"
printf '3\n\n' | bash \
  "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$CATALOG" "$SOFTWARE" > "$TEST_ROOT/programming.out"
jq -e '
  .schemaVersion == 1 and
  all(.packages[]; .scope.kind == "shared") and
  any(.packages[]; .package == "nodejs") and
  any(.packages[]; .package == "opencode") and
  any(.packages[]; .package == "pi-coding-agent") and
  any(.packages[]; .package == "vscode") and
  any(.packages[]; .package == "vlc") and
  any(.packages[]; .package == "gcc")
' "$SOFTWARE" >/dev/null
grep -F 'Profile:    Programming' "$TEST_ROOT/programming.out" >/dev/null
# Programming starts from its own student profile, with editor extensions.
cmp "$REPO_ROOT/templates/site/workspace-profile.programming.example.json" "$PROFILE"
test "$(stat -c %a "$PROFILE")" = 644
test -z "$(find "$TEST_ROOT" -maxdepth 1 -name '.workspace-profile.json.tmp.*' -print -quit)"
cp "$REPO_ROOT/templates/site/workspace-profile.json" "$PROFILE"
if grep -F 'package IDs' "$TEST_ROOT/programming.out" >/dev/null; then
  echo "bootstrap exposed package IDs to the operator" >&2
  exit 1
fi

cp "$REPO_ROOT/templates/site/lab-software.json" "$SOFTWARE"
printf '99\nessential\n\n' | bash \
  "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$CATALOG" "$SOFTWARE" > "$TEST_ROOT/empty.out"
jq -e '.packages | length > 0' "$SOFTWARE" >/dev/null
grep -F 'Choose a listed number or profile ID.' "$TEST_ROOT/empty.out" >/dev/null

cp "$REPO_ROOT/templates/site/lab-software.json" "$SOFTWARE"
BEFORE="$(sha256sum "$SOFTWARE")"
if printf '\nno\n' | bash \
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
if printf '\n\n' | bash \
  "$REPO_ROOT/templates/site/scripts/configure-software-profile.sh" \
  "$TEST_ROOT/invalid-catalog.json" "$SOFTWARE" > "$TEST_ROOT/invalid.out" 2>&1; then
  echo "invalid software profile catalog was accepted" >&2
  exit 1
fi
grep -F 'software-presets.json from the selected revision is invalid' "$TEST_ROOT/invalid.out" >/dev/null

echo "Software profile bootstrap tests passed."
