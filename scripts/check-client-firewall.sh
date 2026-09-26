#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RULES_FILE="$(mktemp)"
trap 'rm -f -- "$RULES_FILE"' EXIT
nix eval --raw --file "$REPO_ROOT/tests/client-firewall-rules.nix" > "$RULES_FILE"
export NIXORIUM_TEST_PARENT_NETNS="$(readlink /proc/self/ns/net)"
export NIXORIUM_TEST_PARENT_USERNS="$(readlink /proc/self/ns/user)"
unshare --user --map-root-user --net \
  python3 "$REPO_ROOT/tests/client-firewall-network.py" \
  "$RULES_FILE" "$(command -v nft)" "$(command -v python3)"
