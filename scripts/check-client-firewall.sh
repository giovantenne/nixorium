#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RULES_FILE="$(mktemp)"
INTERNET_RULES_FILE="$(mktemp)"
trap 'rm -f -- "$RULES_FILE" "$INTERNET_RULES_FILE"' EXIT
export NIXORIUM_TEST_PARENT_NETNS="$(readlink /proc/self/ns/net)"
export NIXORIUM_TEST_PARENT_USERNS="$(readlink /proc/self/ns/user)"
# Clients with an explicit interface name match it; other clients are
# configured by type and their rules must not mention any interface name.
for BY_TYPE in false true; do
  echo "== rules matching clients by $([[ "$BY_TYPE" == true ]] && echo type || echo interface name)"
  nix eval --raw --file "$REPO_ROOT/tests/client-firewall-rules.nix" \
    --apply "f: f { byType = $BY_TYPE; }" > "$RULES_FILE"
  nix eval --raw --file "$REPO_ROOT/tests/client-internet-rules.nix" \
    --apply "f: f { byType = $BY_TYPE; }" > "$INTERNET_RULES_FILE"
  if [[ "$BY_TYPE" == true ]]; then
    ! grep -Eq 'iifname "lab0"|oifname "lab0"' "$RULES_FILE" "$INTERNET_RULES_FILE"
  fi
  unshare --user --map-root-user --net \
    python3 "$REPO_ROOT/tests/client-firewall-network.py" \
    "$RULES_FILE" "$(command -v nft)" "$(command -v python3)" "$INTERNET_RULES_FILE"
done
