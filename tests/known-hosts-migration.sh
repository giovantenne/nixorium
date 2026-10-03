#!/usr/bin/env bash
set -euo pipefail
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT
owner_uid=$(id -u)
owner_gid=$(id -g)
migrate() { bash "$repo_root/scripts/migrate-known-hosts.sh" "$1" "$owner_uid" "$owner_gid"; }

# Preserve all legacy bytes and OpenSSH's usual path; repeated activation is safe.
mkdir -m 0700 "$test_root/existing"
printf '# retained comment\nexample ssh-ed25519 fixture\n' > "$test_root/existing/known_hosts"
chmod 0600 "$test_root/existing/known_hosts"
cp "$test_root/existing/known_hosts" "$test_root/expected"
migrate "$test_root/existing"
test -L "$test_root/existing/known_hosts"
cmp "$test_root/expected" "$test_root/existing/known_hosts"
test "$(stat -c %a "$test_root/existing/nixorium-known-hosts/known_hosts")" = 600
migrate "$test_root/existing"
cmp "$test_root/expected" "$test_root/existing/known_hosts"

# Fresh installations, interrupted migration, and conflicting recovery evidence.
migrate "$test_root/fresh"
test -L "$test_root/fresh/known_hosts"
test ! -s "$test_root/fresh/known_hosts"
rm "$test_root/existing/known_hosts"
cp "$test_root/expected" "$test_root/existing/known_hosts"
chmod 0600 "$test_root/existing/known_hosts"
migrate "$test_root/existing"
rm "$test_root/existing/known_hosts"
printf 'different\n' > "$test_root/existing/known_hosts"
chmod 0600 "$test_root/existing/known_hosts"
migrate "$test_root/existing" 2>/dev/null
test -L "$test_root/existing/known_hosts"
cmp "$test_root/expected" "$test_root/existing/nixorium-known-hosts/known_hosts"
kept=("$test_root"/existing/nixorium-known-hosts/known_hosts-legacy-*.bak)
test "${#kept[@]}" = 1
printf 'different\n' | cmp - "${kept[0]}"
test "$(stat -c %a "${kept[0]}")" = 600
migrate "$test_root/existing"
kept=("$test_root"/existing/nixorium-known-hosts/known_hosts-legacy-*.bak)
test "${#kept[@]}" = 1

# Never follow an unexpected SSH directory or known_hosts symlink.
ln -s "$test_root/fresh" "$test_root/linked"
if migrate "$test_root/linked" 2>/dev/null; then exit 1; fi
mkdir -m 0700 "$test_root/unsafe"
ln -s "$test_root/expected" "$test_root/unsafe/known_hosts"
if migrate "$test_root/unsafe" 2>/dev/null; then exit 1; fi
cmp "$test_root/expected" "$test_root/unsafe/known_hosts"
echo "Known-hosts migration tests passed."
