#!/usr/bin/env bash
set -euo pipefail

if [[ $# -gt 0 ]]; then
  echo "Usage: ./scripts/validate.sh" >&2
  exit 1
fi

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
SITE_DIR="${TEMP_DIR}/site"
CACHE_DIR="${TEMP_DIR}/cache"

cleanup() {
  rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

mkdir -p "$CACHE_DIR" "$SITE_DIR"
export XDG_CACHE_HOME="$CACHE_DIR"
export NIX_CONFIG="${NIX_CONFIG:-}"$'\nexperimental-features = nix-command flakes'

cd "$REPO_ROOT"

git diff --check
bash -n install.sh setup.sh scripts/*.sh scripts/lib/*.sh
diff -qr skills/nixorium-maintainer templates/site/skills/nixorium-maintainer
test -e .agents/skills/nixorium-developer/SKILL.md
test -e .claude/skills/nixorium-developer/SKILL.md
test -e .pi/skills/nixorium-developer/SKILL.md
nix flake check "path:${REPO_ROOT}" --no-write-lock-file
nix eval "path:${REPO_ROOT}#labMeta" --json --no-write-lock-file >/dev/null

CONTROLLER_NAME=$(nix eval "path:${REPO_ROOT}#labMeta.controller.name" --raw --no-write-lock-file)

nix build "path:${REPO_ROOT}#nixosConfigurations.pc01.config.system.build.toplevel" --no-write-lock-file --no-link
nix build "path:${REPO_ROOT}#nixosConfigurations.${CONTROLLER_NAME}.config.system.build.toplevel" --no-write-lock-file --no-link
nix build "path:${REPO_ROOT}#nixosConfigurations.netboot.config.system.build.netbootRamdisk" --no-write-lock-file --no-link
nix build "path:${REPO_ROOT}#disko" --no-write-lock-file --no-link
nix build "path:${REPO_ROOT}#installerBundle" --no-write-lock-file --no-link

(
  cd "$SITE_DIR"
  nix flake init -t "path:${REPO_ROOT}#site"
  git init -q -b master
  git add .
  nix flake lock --override-input nixorium "path:${REPO_ROOT}"
  git add flake.lock

  test -e .agents/skills/nixorium-maintainer/SKILL.md
  test -e .claude/skills/nixorium-maintainer/SKILL.md
  test -e .pi/skills/nixorium-maintainer/SKILL.md
  test ! -e skills/nixorium-developer
)

if [[ "$(nix eval "path:${SITE_DIR}#deploymentStatus.ready" --json --no-write-lock-file)" != "false" ]]; then
  echo "Error: a fresh template unexpectedly reports that it is deployment-ready." >&2
  exit 1
fi

DIRECT_CLIENT_DRV=$(
  nix eval "path:${SITE_DIR}#nixosConfigurations.pc01.config.system.build.toplevel.drvPath" \
    --raw \
    --no-write-lock-file
)

nix build "path:${SITE_DIR}#installerBundle" \
  --no-write-lock-file \
  --out-link "${TEMP_DIR}/installer-result"

INSTALLER_STORE_PATH=$(readlink -f "${TEMP_DIR}/installer-result")
OFFLINE_CLIENT_DRV=$(
  XDG_CACHE_HOME="${TEMP_DIR}/offline-cache" \
    nix eval "path:${INSTALLER_STORE_PATH}#nixosConfigurations.pc01.config.system.build.toplevel.drvPath" \
      --raw \
      --offline \
      --no-write-lock-file
)

if [[ "$DIRECT_CLIENT_DRV" != "$OFFLINE_CLIENT_DRV" ]]; then
  echo "Error: deployment and offline installer produce different pc01 derivations." >&2
  echo "Deployment: ${DIRECT_CLIENT_DRV}" >&2
  echo "Offline:    ${OFFLINE_CLIENT_DRV}" >&2
  exit 1
fi

echo "Validation completed successfully."
