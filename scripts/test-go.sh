#!/usr/bin/env bash
set -euo pipefail

# Keep the normal Go build/test cache between edits, using the locked compiler.
REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$REPO_ROOT"
if [[ $# -eq 0 ]]; then
  set -- ./...
fi
exec nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell \
  --command go test "$@"
