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
  --remote-client-installer-vm
                         Quick checks plus the remote installer VM test
  --full                 Complete release and milestone validation
                         (NIXORIUM_FULL_SHARD selects one CI shard)
  --ci                   Evaluation-only CI validation
EOF
}

if [[ $# -gt 1 ]]; then
  usage >&2
  exit 1
fi

MODE="${1:---quick}"
case "$MODE" in
  --quick | --eval | --management-vm | --client-installer-vm | --remote-client-installer-vm | --full | --ci) ;;
  --help | -h)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 1
    ;;
esac

# Release CI may split --full into parallel jobs with NIXORIUM_FULL_SHARD: one
# group of tests/validation-groups.nix, or "systems" for the system builds,
# template profiles and offline equivalence. Unset, --full runs everything.
FULL_SHARD="${NIXORIUM_FULL_SHARD:-}"
if [[ -n "$FULL_SHARD" && "$MODE" != "--full" ]]; then
  echo "Error: NIXORIUM_FULL_SHARD applies only to --full." >&2
  exit 1
fi

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
bash tests/client-installer-library.sh
bash tests/remote-client-installer.sh
bash tests/controller-bootstrap.sh
bash tests/controller-installer.sh
bash tests/software-profile-bootstrap.sh
bash tests/canonical-copy-sync.sh
bash tests/known-hosts-migration.sh
bash tests/pxe-build.sh
bash tests/ux-shell-regressions.sh
bash scripts/sync-canonical-copies.sh --check
test -e .agents/skills/nixorium-developer/SKILL.md
test -e .claude/skills/nixorium-developer/SKILL.md
test -e .pi/skills/nixorium-developer/SKILL.md
nix eval --file tests/validation-groups-test.nix --json | jq -e '. == true' >/dev/null

run_quick_checks() {
  nix build \
    --file "${REPO_ROOT}/tests/source-checks.nix" \
    config-schema \
    settings-schema \
    software-schema \
    software-preset-schema \
    workspace-schema \
    workspace-resolution \
    documentation-check \
    nixorium \
    nixorium-runtime \
    --no-write-lock-file \
    --no-link
  bash scripts/check-agent-guidance.sh
}

run_mk_lab_check() {
  local CHECK
  for CHECK in mk-lab workspace-template workspace-preparation workspace-runtime workspace-candidate workspace-rejection; do
    nix build "path:${REPO_ROOT}#checks.x86_64-linux.${CHECK}" \
      --no-write-lock-file --no-link
  done
}

eval_ci_group() {
  local SOURCE="$1"
  local GROUP="$2"
  echo "Evaluating ${GROUP} outputs..." >&2
  NIXORIUM_CI_SOURCE="$SOURCE" NIXORIUM_CI_GROUP="$GROUP" \
    NIXORIUM_CI_EVALUATOR="${REPO_ROOT}/tests/ci-eval.nix" \
    nix eval --impure --json --no-write-lock-file --expr '
      let
        flake = builtins.getFlake ("path:" + builtins.getEnv "NIXORIUM_CI_SOURCE");
        groups = import (builtins.toPath (builtins.getEnv "NIXORIUM_CI_EVALUATOR")) { inherit flake; };
      in groups.${builtins.getEnv "NIXORIUM_CI_GROUP"}
    '
}

# With a group name, build only that group; otherwise build every group.
run_full_checks() {
  local ONLY="${1:-}"
  local GROUP CHECK
  local -a CHECKS OUTPUTS
  while IFS= read -r GROUP; do
    if [[ -n "$ONLY" && "$GROUP" != "$ONLY" ]]; then
      continue
    fi
    mapfile -t CHECKS < <(jq -r --arg group "$GROUP" '.[$group][]' <<<"$VALIDATION_GROUPS")
    OUTPUTS=()
    for CHECK in "${CHECKS[@]}"; do
      OUTPUTS+=("path:${REPO_ROOT}#checks.x86_64-linux.${CHECK}")
    done
    echo "Building full ${GROUP} outputs..."
    nix build "${OUTPUTS[@]}" --no-write-lock-file --no-link
  done < <(jq -r 'keys[]' <<<"$VALIDATION_GROUPS")
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
  --remote-client-installer-vm)
    run_quick_checks
    nix build "path:${REPO_ROOT}#checks.x86_64-linux.remote-client-installer-vm" \
      --no-write-lock-file \
      --no-link
    echo "Remote client installer VM validation completed successfully."
    exit 0
    ;;
esac

# CI may split --ci into parallel shards with NIXORIUM_CI_SHARD. Unlisted
# groups fall into "lab", so a new group is never skipped.
CI_SHARD="${NIXORIUM_CI_SHARD:-}"
case "$CI_SHARD" in
  "" | lab | workspace-a | workspace-b | template | template-dev | template-minimal) ;;
  *)
    echo "Error: unknown NIXORIUM_CI_SHARD '${CI_SHARD}' (lab, workspace-a, workspace-b, template, template-dev, template-minimal)." >&2
    exit 1
    ;;
esac
ci_shard_of() {
  case "$1" in
    checks-workspace-candidate | checks-workspace-preparation) echo workspace-a ;;
    checks-workspace-rejection | checks-workspace-runtime | checks-workspace-systems) echo workspace-b ;;
    checks-workspace-template | systems | template | profile-default) echo template ;;
    profile-dev) echo template-dev ;;
    profile-minimal) echo template-minimal ;;
    *) echo lab ;;
  esac
}
in_ci_shard() {
  [[ -z "$CI_SHARD" || "$(ci_shard_of "$1")" == "$CI_SHARD" ]]
}

VALIDATION_GROUPS=$(eval_ci_group "$REPO_ROOT" checkGroups)
if [[ "${MODE}" == "--ci" ]]; then
  while IFS= read -r GROUP; do
    if in_ci_shard "$GROUP"; then
      eval_ci_group "$REPO_ROOT" "$GROUP" >/dev/null
    fi
  done < <(jq -r 'keys[]' <<<"$VALIDATION_GROUPS")
  if ! in_ci_shard template && ! in_ci_shard profile-dev && ! in_ci_shard profile-minimal; then
    echo "CI shard ${CI_SHARD} completed successfully."
    exit 0
  fi
else
  if [[ -n "$FULL_SHARD" && "$FULL_SHARD" != systems ]]; then
    if ! jq -e --arg group "$FULL_SHARD" 'has($group)' <<<"$VALIDATION_GROUPS" >/dev/null; then
      echo "Error: unknown NIXORIUM_FULL_SHARD '${FULL_SHARD}' (systems or a group of tests/validation-groups.nix)." >&2
      exit 1
    fi
    run_full_checks "$FULL_SHARD"
    echo "Full validation shard ${FULL_SHARD} completed successfully."
    exit 0
  fi
  bash scripts/check-agent-guidance.sh
  nix build --file "${REPO_ROOT}/tests/source-checks.nix" documentation-check --no-write-lock-file --no-link
  if [[ -z "$FULL_SHARD" ]]; then
    run_full_checks
  fi
fi

if [[ "${MODE}" == "--ci" ]]; then
  if in_ci_shard systems; then
    eval_ci_group "$REPO_ROOT" systems >/dev/null
  fi
else
  LAB_META=$(nix eval "path:${REPO_ROOT}#labMeta" --json --no-write-lock-file)
  CONTROLLER_NAME=$(jq -r .controller.name <<<"$LAB_META")
  nix build \
    "path:${REPO_ROOT}#nixosConfigurations.pc01.config.system.build.toplevel" \
    "path:${REPO_ROOT}#nixosConfigurations.${CONTROLLER_NAME}.config.system.build.toplevel" \
    "path:${REPO_ROOT}#nixosConfigurations.netboot.config.system.build.netbootRamdisk" \
    "path:${REPO_ROOT}#disko" \
    "path:${REPO_ROOT}#installerBundle" \
    "path:${REPO_ROOT}#remoteInstallerBundle" \
    "path:${REPO_ROOT}#pxeFirmware" \
    "path:${REPO_ROOT}#nixorium" \
    "path:${REPO_ROOT}#nixoriumOfflineCheck" \
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

profile_state() {
  NIXORIUM_PROFILE_SITE="${1:-$SITE_DIR}" nix eval --impure --json --expr '
    let
      deployment = builtins.getFlake ("path:" + builtins.getEnv "NIXORIUM_PROFILE_SITE");
      config = deployment.nixosConfigurations.pc01.config;
      pkgs = deployment.nixosConfigurations.pc01.pkgs;
    in {
      chromiumPolicy = config.environment.etc ? "chromium/policies/managed/homepage.json";
      desktopExtensions =
        builtins.elem pkgs.gnomeExtensions.dash-to-dock config.environment.systemPackages
        && builtins.elem pkgs.gnomeExtensions.desktop-icons-ng-ding config.environment.systemPackages;
      desktopExtensionDefaults =
        pkgs.lib.hasInfix "ding@rastersoft.com" config.services.desktopManager.gnome.extraGSettingsOverrides
        && pkgs.lib.hasInfix "dash-to-dock@micxgx.gmail.com" config.services.desktopManager.gnome.extraGSettingsOverrides
        && pkgs.lib.all (setting:
          pkgs.lib.hasInfix setting config.services.desktopManager.gnome.extraGSettingsOverrides
        ) [
          "dock-fixed=false"
          "autohide=true"
          "intellihide=true"
          "intellihide-mode='"'"'ALL_WINDOWS'"'"'"
        ];
      desktopExtensionRepair =
        pkgs.lib.hasInfix "gnome-extensions enable \"ding@rastersoft.com\""
          config.environment.etc."lab/gnome-user-setup.sh".text
        && pkgs.lib.hasInfix "gnome-extensions enable \"dash-to-dock@micxgx.gmail.com\""
          config.environment.etc."lab/gnome-user-setup.sh".text;
      docker = config.virtualisation.docker.rootless.enable;
      screensaver = config.systemd.user.services ? lab-screensaver;
      vscodeHome = builtins.match ".*vscjava[.]vscode-java-pack.*"
        config.system.activationScripts.siteHomeProfile.text != null;
      homeOwnershipOrdering =
        builtins.elem "siteHomeProfile" config.system.activationScripts.nixoriumUserHomeOwnership.deps;
      vscodeHomeOwnership =
        builtins.match ".*install -d -o admin.*[.]config/Code/User.*"
          config.system.activationScripts.siteHomeProfile.text != null
        && builtins.match ".*install -d -o teacher.*[.]vscode/extensions.*"
          config.system.activationScripts.siteHomeProfile.text != null;
    }
  '
}


profile_check_failed() {
  echo "Error: template profile check failed ($1): $PROFILE_STATE" >&2
  exit 1
}
PROFILE_STATE=""

# The three software scenarios are independent evaluations of copies of the
# template; run them in parallel and check each result.
DEV_SITE="${TEMP_DIR}/site-dev"
MINIMAL_SITE="${TEMP_DIR}/site-minimal"
cp -a "$SITE_DIR" "$DEV_SITE"
cp -a "$SITE_DIR" "$MINIMAL_SITE"
jq '.packages += [
  {"package": "docker", "scope": {"kind": "shared"}},
  {"package": "vscode", "scope": {"kind": "shared"}}
] | .packages |= sort_by(.package)' \
  "$SITE_DIR/lab-software.json" > "$DEV_SITE/lab-software.json"
# Removing the browser and terminal also requires a student workspace profile
# that no longer names them; the saved template profile does.
jq 'del(.browser) | .desktop.favorites |= map(select(. != "com.mitchellh.ghostty.desktop" and . != "chromium-browser.desktop"))' \
  "$SITE_DIR/workspace-profile.json" > "$MINIMAL_SITE/workspace-profile.json"
jq '.packages |= map(select(.package as $package | [
  "chromium",
  "docker",
  "ghostty",
  "nodejs",
  "python3Packages.terminaltexteffects",
  "vscode"
] | index($package) | not))' "$SITE_DIR/lab-software.json" > "$MINIMAL_SITE/lab-software.json"
# Each scenario is a full NixOS evaluation. Locally they run in parallel; a
# CI shard runs only its own, and a full-validation shard runs them one after
# another, keeping each runner within its memory.
PROFILE_PIDS=()
for SCENARIO in default dev minimal; do
  if ! in_ci_shard "profile-${SCENARIO}"; then
    continue
  fi
  case "$SCENARIO" in
    default) SCENARIO_SITE="$SITE_DIR" ;;
    dev) SCENARIO_SITE="$DEV_SITE" ;;
    minimal) SCENARIO_SITE="$MINIMAL_SITE" ;;
  esac
  if [[ -n "$CI_SHARD" || -n "$FULL_SHARD" ]]; then
    profile_state "$SCENARIO_SITE" > "$TEMP_DIR/profile-${SCENARIO}.json" || profile_check_failed "${SCENARIO} software evaluation"
  else
    profile_state "$SCENARIO_SITE" > "$TEMP_DIR/profile-${SCENARIO}.json" &
    PROFILE_PIDS+=("$!")
  fi
done
for PID in "${PROFILE_PIDS[@]}"; do
  wait "$PID" || profile_check_failed "software scenario evaluation"
done

if in_ci_shard profile-default; then
  PROFILE_STATE=$(cat "$TEMP_DIR/profile-default.json")
  jq -e '
    .chromiumPolicy and .screensaver and .desktopExtensions and
    .desktopExtensionDefaults and .desktopExtensionRepair and
    ((.docker or .vscodeHome or .vscodeHomeOwnership) | not) and
    .homeOwnershipOrdering
  ' <<<"$PROFILE_STATE" >/dev/null || profile_check_failed "default software"
fi
# The template's active student workspace profile owns the student's VS Code
# extensions, so adding the package prepares only staff homes.
if in_ci_shard profile-dev; then
  PROFILE_STATE=$(cat "$TEMP_DIR/profile-dev.json")
  jq -e '
    .chromiumPolicy and .docker and .screensaver and .desktopExtensions and
    .desktopExtensionDefaults and .desktopExtensionRepair and (.vscodeHome | not) and
    .vscodeHomeOwnership and .homeOwnershipOrdering
  ' <<<"$PROFILE_STATE" >/dev/null || profile_check_failed "Docker and VS Code added"
fi
if in_ci_shard profile-minimal; then
  PROFILE_STATE=$(cat "$TEMP_DIR/profile-minimal.json")
  jq -e '
    .desktopExtensions and .desktopExtensionDefaults and .desktopExtensionRepair and
    ((.chromiumPolicy or .docker or .screensaver or .vscodeHome or .vscodeHomeOwnership) | not) and
    .homeOwnershipOrdering
  ' <<<"$PROFILE_STATE" >/dev/null || profile_check_failed "minimal software"
fi

if [[ "${MODE}" == "--ci" ]]; then
  if in_ci_shard template; then
    eval_ci_group "$SITE_DIR" template >/dev/null
  fi
  echo "CI evaluation completed successfully."
  exit 0
fi

# Evaluating a deployment output passes through its whole host configuration
# (several GiB). A separate evaluation releases that memory before the build,
# which a single "nix build" would otherwise hold while compiling and testing.
build_evaluated() {
  local DRV
  DRV=$(nix eval "$1.drvPath" --raw --no-write-lock-file)
  shift
  nix build "${DRV}^out" "$@"
}

SITE_NIXORIUM_STORE=$(build_evaluated "path:${SITE_DIR}#nixorium" \
  --print-out-paths \
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
nix eval "path:${SITE_DIR}#packages.x86_64-linux.remoteInstallerBundle.drvPath" \
  --raw \
  --no-write-lock-file >/dev/null

DIRECT_CLIENT_DRV=$(
  nix eval "path:${SITE_DIR}#nixosConfigurations.pc01.config.system.build.toplevel.drvPath" \
    --raw \
    --no-write-lock-file
)

build_evaluated "path:${SITE_DIR}#installerBundle" \
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
