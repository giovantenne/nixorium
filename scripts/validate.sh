#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./scripts/validate.sh [MODE]

Modes:
  --quick                Fast local checks (default)
  --eval                 Quick checks plus the complete mkLab evaluation
  --management-vm        Quick checks plus the management VM test
  --client-installer-vm  Quick checks plus the client installer VM test
  --full                 Complete release and milestone validation
  --ci                   Evaluation-only CI validation
EOF
}

if [[ $# -gt 1 ]]; then
  usage >&2
  exit 1
fi

MODE="${1:---quick}"
case "$MODE" in
  --quick | --eval | --management-vm | --client-installer-vm | --full | --ci) ;;
  --help | -h)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 1
    ;;
esac

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEMP_DIR=$(mktemp -d)
SITE_DIR="${TEMP_DIR}/site"

if [[ -n "${NIXORIUM_VALIDATION_CACHE_HOME:-}" ]]; then
  CACHE_DIR="${NIXORIUM_VALIDATION_CACHE_HOME}"
elif [[ -n "${XDG_CACHE_HOME:-}" ]]; then
  CACHE_DIR="${XDG_CACHE_HOME}/nixorium-validation"
elif [[ -n "${HOME:-}" ]]; then
  CACHE_DIR="${HOME}/.cache/nixorium-validation"
else
  CACHE_DIR="${TEMP_DIR}/cache"
fi

cleanup() {
  rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

mkdir -p "$CACHE_DIR" "$SITE_DIR"
export XDG_CACHE_HOME="$CACHE_DIR"
export NIX_CONFIG="${NIX_CONFIG:-}"$'\nexperimental-features = nix-command flakes'

if [[ "${MODE}" == "--ci" ]]; then
  export NIX_CONFIG="${NIX_CONFIG}"$'\nallow-import-from-derivation = false'
fi

cd "$REPO_ROOT"

echo "Validation mode: ${MODE}"
echo "Reusable Nix evaluation cache: ${CACHE_DIR}"

git diff --check
bash -n install.sh setup.sh scripts/*.sh scripts/lib/*.sh
bash tests/client-installer.sh
bash tests/controller-bootstrap.sh
bash tests/controller-installer.sh
diff -qr skills/nixorium-maintainer templates/site/skills/nixorium-maintainer
diff -u docs/troubleshooting.md templates/site/TROUBLESHOOTING.md
test -e .agents/skills/nixorium-developer/SKILL.md
test -e .claude/skills/nixorium-developer/SKILL.md
test -e .pi/skills/nixorium-developer/SKILL.md

run_quick_checks() {
  nix build \
    --file "${REPO_ROOT}/tests/source-checks.nix" \
    config-schema \
    settings-schema \
    software-schema \
    nixorium \
    --no-write-lock-file \
    --no-link
}

run_mk_lab_check() {
  nix build \
    "path:${REPO_ROOT}#checks.x86_64-linux.mk-lab" \
    --no-write-lock-file \
    --no-link
}

run_full_checks() {
  nix build \
    "path:${REPO_ROOT}#checks.x86_64-linux.config-schema" \
    "path:${REPO_ROOT}#checks.x86_64-linux.settings-schema" \
    "path:${REPO_ROOT}#checks.x86_64-linux.software-schema" \
    "path:${REPO_ROOT}#checks.x86_64-linux.mk-lab" \
    "path:${REPO_ROOT}#checks.x86_64-linux.client-installer" \
    "path:${REPO_ROOT}#checks.x86_64-linux.client-installer-vm" \
    "path:${REPO_ROOT}#checks.x86_64-linux.management-vm" \
    --no-write-lock-file \
    --no-link
}

case "$MODE" in
  --quick)
    run_quick_checks
    echo "Quick validation completed successfully."
    exit 0
    ;;
  --eval)
    run_quick_checks
    run_mk_lab_check
    echo "Complete mkLab evaluation completed successfully."
    exit 0
    ;;
  --management-vm)
    run_quick_checks
    nix build "path:${REPO_ROOT}#checks.x86_64-linux.management-vm" \
      --no-write-lock-file \
      --no-link
    echo "Management VM validation completed successfully."
    exit 0
    ;;
  --client-installer-vm)
    run_quick_checks
    nix build "path:${REPO_ROOT}#checks.x86_64-linux.client-installer-vm" \
      --no-write-lock-file \
      --no-link
    echo "Client installer VM validation completed successfully."
    exit 0
    ;;
esac

if [[ "${MODE}" == "--ci" ]]; then
  nix eval "path:${REPO_ROOT}#checks.x86_64-linux.config-schema.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#checks.x86_64-linux.settings-schema.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#checks.x86_64-linux.software-schema.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#checks.x86_64-linux.mk-lab.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#checks.x86_64-linux.client-installer.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#checks.x86_64-linux.client-installer-vm.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#checks.x86_64-linux.management-vm.drvPath" --raw --no-write-lock-file >/dev/null
else
  run_full_checks
fi

LAB_META=$(nix eval "path:${REPO_ROOT}#labMeta" --json --no-write-lock-file)
CONTROLLER_NAME=$(jq -r .controller.name <<<"$LAB_META")

if [[ "${MODE}" == "--ci" ]]; then
  nix eval "path:${REPO_ROOT}#nixosConfigurations.pc01.config.system.build.toplevel.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#nixosConfigurations.${CONTROLLER_NAME}.config.system.build.toplevel.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#nixosConfigurations.netboot.config.system.build.netbootRamdisk.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#packages.x86_64-linux.disko.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#packages.x86_64-linux.installerBundle.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#packages.x86_64-linux.pxeFirmware.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#apps.x86_64-linux.run-harmonia.program" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#apps.x86_64-linux.run-pxe-proxy.program" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#apps.x86_64-linux.nixorium.program" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#packages.x86_64-linux.nixorium.drvPath" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#colmena.pc01.deployment.targetHost" --raw --no-write-lock-file >/dev/null
  nix eval "path:${REPO_ROOT}#deploymentStatus" --json --no-write-lock-file >/dev/null
else
  nix build \
    "path:${REPO_ROOT}#nixosConfigurations.pc01.config.system.build.toplevel" \
    "path:${REPO_ROOT}#nixosConfigurations.${CONTROLLER_NAME}.config.system.build.toplevel" \
    "path:${REPO_ROOT}#nixosConfigurations.netboot.config.system.build.netbootRamdisk" \
    "path:${REPO_ROOT}#disko" \
    "path:${REPO_ROOT}#installerBundle" \
    "path:${REPO_ROOT}#pxeFirmware" \
    "path:${REPO_ROOT}#nixorium" \
    --no-write-lock-file \
    --no-link
fi

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
  test -e lab-settings.json
  test -e lab-software.json
  test ! -e lab-config.nix
)

if [[ "$(nix eval "path:${SITE_DIR}#deploymentStatus.ready" --json --no-write-lock-file)" != "false" ]]; then
  echo "Error: a fresh template unexpectedly reports that it is deployment-ready." >&2
  exit 1
fi

if [[ "${MODE}" == "--ci" ]]; then
  nix eval "path:${SITE_DIR}#apps.x86_64-linux.nixorium.program" \
    --raw \
    --no-write-lock-file >/dev/null
  nix eval "path:${SITE_DIR}#packages.x86_64-linux.pxeFirmware.drvPath" \
    --raw \
    --no-write-lock-file >/dev/null
  nix eval "path:${SITE_DIR}#nixosConfigurations.pc01.config.system.build.toplevel.drvPath" \
    --raw \
    --no-write-lock-file >/dev/null
  nix eval "path:${SITE_DIR}#installerBundle.drvPath" \
    --raw \
    --no-write-lock-file >/dev/null
  echo "CI evaluation completed successfully."
  exit 0
fi

SITE_NIXORIUM_STORE=$(nix build "path:${SITE_DIR}#nixorium" \
  --print-out-paths \
  --no-write-lock-file \
  --no-link)
SITE_NIXORIUM="${SITE_NIXORIUM_STORE}/bin/nixorium"

"$SITE_NIXORIUM" config validate --repo "$SITE_DIR" --json >/dev/null
"$SITE_NIXORIUM" software catalog --repo "$SITE_DIR" --json >/dev/null
"$SITE_NIXORIUM" software plan --repo "$SITE_DIR" --package vlc --scope all-clients | \
  grep -q 'Software proposal: READY'
cp "$SITE_DIR/lab-settings.json" "$TEMP_DIR/candidate.json"
"$SITE_NIXORIUM" config plan --repo "$SITE_DIR" --file "$TEMP_DIR/candidate.json" | \
  grep -q 'Configuration plan: UNCHANGED'
"$SITE_NIXORIUM" setup status --repo "$SITE_DIR" --json >/dev/null

# Exercise the actual save/validation boundary and carry shared declarations
# through the offline-equivalence check below. This temporary site has no keys.
"$SITE_NIXORIUM" software plan --repo "$SITE_DIR" --package hello --scope shared --json \
  >"${TEMP_DIR}/shared-software-plan.json"
jq -e '.state == "ready" and .affectedController != null and (.affectedClients | length) > 0' \
  "${TEMP_DIR}/shared-software-plan.json" >/dev/null
SOFTWARE_REVIEW_TOKEN=$(jq -r .reviewToken "${TEMP_DIR}/shared-software-plan.json")
"$SITE_NIXORIUM" software apply --repo "$SITE_DIR" --package hello --scope shared \
  --expect "$SOFTWARE_REVIEW_TOKEN" --yes --json \
  >"${TEMP_DIR}/shared-software-apply.json"
jq -e '.state == "applied" and .affectedController != null' \
  "${TEMP_DIR}/shared-software-apply.json" >/dev/null
git -C "$SITE_DIR" add lab-software.json

nix eval "path:${SITE_DIR}#apps.x86_64-linux.nixorium.program" \
  --raw \
  --no-write-lock-file >/dev/null
nix eval "path:${SITE_DIR}#packages.x86_64-linux.pxeFirmware.drvPath" \
  --raw \
  --no-write-lock-file >/dev/null

DIRECT_CLIENT_DRV=$(
  nix eval "path:${SITE_DIR}#nixosConfigurations.pc01.config.system.build.toplevel.drvPath" \
    --raw \
    --no-write-lock-file
)

nix build "path:${SITE_DIR}#installerBundle" \
  --no-write-lock-file \
  --out-link "${TEMP_DIR}/installer-result"

INSTALLER_STORE_PATH=$(readlink -f "${TEMP_DIR}/installer-result")
test -x "${INSTALLER_STORE_PATH}/disko-install"
jq -e '
  .schemaVersion == 2 and
  .clients.count > 0 and
  (.clients.hosts | length) == .clients.count
' "${INSTALLER_STORE_PATH}/lab-meta.json" >/dev/null
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

echo "Full validation completed successfully."
