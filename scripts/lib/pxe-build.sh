#!/usr/bin/env bash
# Included by the fixed PXE preparation service. The caller owns cleanup and
# keeps BUILD_DIRECTORY until permanent GC roots have been installed.
set -euo pipefail

build_pxe_outputs() {
  local FLAKE_REFERENCE="$1"
  shift
  local CLIENT_NAMES=("$@")
  # shellcheck disable=SC2016
  local EXPRESSION='
    let
      deployment = builtins.getFlake (builtins.getEnv "NIXORIUM_PXE_BUILD_FLAKE");
      describe = output: {
        drvPath = if builtins.isAttrs output && output ? drvPath then output.drvPath else null;
        storePath = toString output;
      };
      names = builtins.fromJSON (builtins.getEnv "NIXORIUM_PXE_BUILD_CLIENTS");
      boot = deployment.nixosConfigurations.netboot.config.system.build;
    in {
      artifacts = [
        (describe boot.kernel)
        (describe boot.netbootRamdisk)
        (describe boot.netbootIpxeScript)
        (describe deployment.packages.x86_64-linux.pxeFirmware)
      ];
      clients = map (name: { inherit name; } //
        describe deployment.nixosConfigurations.${name}.config.system.build.toplevel) names;
    }
  '
  local NAME DESCRIPTION DECLARED_NAMES STORE_PATH
  local BUILD_TARGETS=() OUTPUTS=()

  ((${#CLIENT_NAMES[@]} > 0)) || fail "labMeta contains no client hosts"
  for NAME in "${CLIENT_NAMES[@]}"; do
    [[ "$NAME" =~ ^pc[0-9]+$ ]] || fail "labMeta contains invalid client name"
  done
  DECLARED_NAMES="$(printf '%s\n' "${CLIENT_NAMES[@]}" | jq -Rn '[inputs]')"
  DESCRIPTION="$(NIXORIUM_PXE_BUILD_FLAKE="$FLAKE_REFERENCE" \
    NIXORIUM_PXE_BUILD_CLIENTS="$DECLARED_NAMES" \
    nix eval --impure --json --expr "$EXPRESSION" --no-write-lock-file --no-update-lock-file)" \
    || fail "could not evaluate the required PXE artifacts and client systems"
  jq -e --argjson names "$DECLARED_NAMES" '
    (.artifacts | length) == 4 and (.clients | map(.name)) == $names and
    ([.artifacts[], .clients[]] | all(
      (.drvPath == null or (.drvPath | type == "string" and test("^/nix/store/[0-9a-z]{32}-[^/[:space:]]+\\.drv$"))) and
      (.storePath | type == "string" and test("^/nix/store/[0-9a-z]{32}-[^/[:space:]]+$"))
    ))
  ' <<<"$DESCRIPTION" >/dev/null || fail "Nix returned an invalid PXE build description"
  mapfile -t BUILD_TARGETS < <(jq -r '[.artifacts[], .clients[]] | map(if .drvPath == null then .storePath else .drvPath + "^out" end) | unique[]' <<<"$DESCRIPTION")
  mapfile -t OUTPUTS < <(jq -r '.artifacts[].storePath, .clients[].storePath' <<<"$DESCRIPTION")

  BUILD_DIRECTORY="$(mktemp -d "$STATE_DIRECTORY/.build.XXXXXX")"
  # Build already evaluated derivations, so this second process never evaluates
  # the deployment again. Legacy literal store paths need no Flake evaluation
  # either. Temporary result roots retain all unique outputs.
  nix build "${BUILD_TARGETS[@]}" --out-link "$BUILD_DIRECTORY/result" \
    || fail "Nix build failed for the required PXE artifacts and client systems"
  for STORE_PATH in "${OUTPUTS[@]}"; do
    [[ -e "$STORE_PATH" ]] || fail "Nix build did not produce $STORE_PATH"
  done
  # Verify retention without relying on completion order or duplicate indices.
  local EXPECTED_ROOTS ACTUAL_ROOTS
  EXPECTED_ROOTS="$(printf '%s\n' "${OUTPUTS[@]}" | sort -u)"
  ACTUAL_ROOTS="$(find "$BUILD_DIRECTORY" -mindepth 1 -maxdepth 1 -type l -exec readlink {} \; | sort -u)"
  [[ "$ACTUAL_ROOTS" == "$EXPECTED_ROOTS" ]] || fail "Nix build returned unexpected output roots"

  KERNEL_PATH="${OUTPUTS[0]}"
  INITRD_PATH="${OUTPUTS[1]}"
  IPXE_SCRIPT_PATH="${OUTPUTS[2]}"
  FIRMWARE_PATH="${OUTPUTS[3]}"
  CLIENT_PATHS=("${OUTPUTS[@]:4}")
}
