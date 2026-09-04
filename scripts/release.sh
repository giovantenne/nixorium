#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: ./scripts/release.sh <version>" >&2
  echo "Example: ./scripts/release.sh 1.1.0" >&2
  exit 1
fi

VERSION="$1"
TAG="v${VERSION}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ ! "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "Error: '${VERSION}' is not a valid semantic version." >&2
  exit 1
fi

cd "${REPO_ROOT}"

FILE_VERSION="$(tr -d '[:space:]' < VERSION)"
if [[ "${FILE_VERSION}" != "${VERSION}" ]]; then
  echo "Error: VERSION contains '${FILE_VERSION}', expected '${VERSION}'." >&2
  exit 1
fi

if ! grep -Eq "^## \\[${VERSION//./\\.}\\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$" CHANGELOG.md; then
  echo "Error: CHANGELOG.md has no dated section for ${VERSION}." >&2
  exit 1
fi

if [[ -n "$(git status --short)" ]]; then
  echo "Error: the worktree is not clean. Commit the release metadata first." >&2
  exit 1
fi

CURRENT_BRANCH="$(git branch --show-current)"
if [[ "${CURRENT_BRANCH}" != "master" ]]; then
  echo "Error: releases must be created from master, not '${CURRENT_BRANCH}'." >&2
  exit 1
fi

if git show-ref --verify --quiet "refs/tags/${TAG}"; then
  echo "Error: tag '${TAG}' already exists." >&2
  exit 1
fi

git fetch origin master --tags

LOCAL_COMMIT="$(git rev-parse HEAD)"
REMOTE_COMMIT="$(git rev-parse origin/master)"
if [[ "${LOCAL_COMMIT}" != "${REMOTE_COMMIT}" ]]; then
  echo "Error: HEAD must match origin/master before publishing a release." >&2
  exit 1
fi

git tag --annotate "${TAG}" --message "NixOS Lab ${TAG}"
git push origin "${TAG}"

echo "Published ${TAG}. GitHub Actions will create the GitHub Release."
