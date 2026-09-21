#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$REPO_ROOT"

command -v staticcheck >/dev/null
command -v govulncheck >/dev/null
command -v actionlint >/dev/null

echo "Validating GitHub Actions workflows..."
actionlint

echo "Running staticcheck..."
staticcheck -checks="inherit,-ST1005" ./...

echo "Running govulncheck..."
govulncheck ./...
