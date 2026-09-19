#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 0 ]]; then
  echo "Usage: scripts/check-agent-guidance.sh" >&2
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECKER="$(nix --extra-experimental-features 'nix-command flakes' build \
  --file "$REPO_ROOT/tests/source-checks.nix" guidance-check \
  --no-write-lock-file --no-link --print-out-paths)"
NIXORIUM_GUIDANCE_ROOT="$REPO_ROOT" "$CHECKER/bin/guidance-check" \
  -test.run '^TestAgentGuidance' -test.v
