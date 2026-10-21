#!/usr/bin/env bash
set -euo pipefail

# ShellCheck over every shell script, gofmt over the Go sources and errcheck
# over unchecked Go error returns, with the tools pinned by the lint shell of
# tests/source-checks.nix.
# ./scripts/validate.sh runs it there; to run it alone:
#   nix develop --file tests/source-checks.nix lint-shell --command ./scripts/lint.sh

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$REPO_ROOT"

for TOOL in shellcheck errcheck go; do
  if ! command -v "$TOOL" >/dev/null; then
    echo "Error: ${TOOL} not found; run this script from the lint shell of tests/source-checks.nix." >&2
    exit 1
  fi
done

# Tracked and new shell scripts: install.sh, setup.sh, scripts/, the site
# template and the tests.
SCRIPTS=()
while IFS= read -r -d '' FILE; do
  if [[ -f "$FILE" ]]; then
    SCRIPTS+=("$FILE")
  fi
done < <(git ls-files -z --cached --others --exclude-standard -- '*.sh')

echo "Running shellcheck on ${#SCRIPTS[@]} scripts..."
shellcheck --severity=warning "${SCRIPTS[@]}"

echo "Running gofmt..."
UNFORMATTED=$(gofmt -l cmd internal)
if [[ -n "$UNFORMATTED" ]]; then
  echo "Error: these Go files need gofmt -w:" >&2
  echo "$UNFORMATTED" >&2
  exit 1
fi

echo "Running errcheck..."
errcheck -ignoretests -exclude .errcheck-exclude ./...
