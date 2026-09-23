#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || ( "$1" != "--check" && "$1" != "--write" ) ]]; then
  echo "Usage: scripts/generate-docs.sh --check|--write" >&2
  exit 2
fi

MODE="$1"
REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

cd "$REPO_ROOT"
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell \
  --command go run ./cmd/nixorium-docs "$MODE" --repo "$REPO_ROOT"
