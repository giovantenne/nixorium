#!/usr/bin/env bash
# Included by the fixed PXE preparation service. The caller owns cleanup and
# keeps BUILD_DIRECTORY until permanent GC roots have been installed.
set -euo pipefail

# Every NixOS evaluation costs roughly 0.5-1.5 GB, so one evaluator covering a
# large laboratory can exhaust controller memory. Evaluate the shared artifacts
# and then the clients in separate, sequential `nix eval` processes of at most
# NIXORIUM_PXE_EVAL_BATCH names (default 4). Each process exits before the next
# starts, so peak evaluation memory is that of one batch, not of the laboratory.
# The later `nix build` receives only resolved derivations and does not evaluate
# NixOS configurations, so a single build process stays small and keeps the
# existing shared scheduling of common dependencies.
PXE_EVAL_BATCH_DEFAULT=4

# Describe the selected part of the deployment as JSON. Part "artifacts" needs
# no client names; part "clients" describes exactly the names in the JSON array.
pxe_eval_description() {
  local FLAKE_REFERENCE="$1" PART="$2" NAMES_JSON="$3"
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
    in
      if builtins.getEnv "NIXORIUM_PXE_BUILD_PART" == "artifacts" then {
        artifacts = [
          (describe boot.kernel)
          (describe boot.netbootRamdisk)
          (describe boot.netbootIpxeScript)
          (describe deployment.packages.x86_64-linux.pxeFirmware)
        ];
      } else {
        clients = map (name: { inherit name; } //
          describe deployment.nixosConfigurations.${name}.config.system.build.toplevel) names;
      }
  '
  NIXORIUM_PXE_BUILD_FLAKE="$FLAKE_REFERENCE" \
    NIXORIUM_PXE_BUILD_PART="$PART" \
    NIXORIUM_PXE_BUILD_CLIENTS="$NAMES_JSON" \
    nix eval --impure --json --expr "$EXPRESSION" --no-write-lock-file --no-update-lock-file
}

# Name a batch for messages: "pc05" or "pc05–pc08".
pxe_batch_label() {
  if (($# == 1)); then
    printf '%s' "$1"
  else
    printf '%s–%s' "$1" "${*: -1}"
  fi
}

build_pxe_outputs() {
  local FLAKE_REFERENCE="$1"
  shift
  local CLIENT_NAMES=("$@")
  local BATCH_SIZE="${NIXORIUM_PXE_EVAL_BATCH-$PXE_EVAL_BATCH_DEFAULT}"
  local NAME DESCRIPTION DECLARED_NAMES STORE_PATH BATCH_NAMES BATCH_JSON BATCH_DESCRIPTION
  local START END TOTAL ARTIFACTS CLIENTS INDEX KEY REPRESENTATIVES
  local BUILD_TARGETS=() OUTPUTS=() EVAL_NAMES=()
  local -A FIRST_OF=() REPRESENTATIVE_OF=()

  if ! [[ "$BATCH_SIZE" =~ ^[0-9]+$ ]]; then
    fail "NIXORIUM_PXE_EVAL_BATCH must be an integer from 1 to 64"
  elif ((10#$BATCH_SIZE < 1 || 10#$BATCH_SIZE > 64)); then
    fail "NIXORIUM_PXE_EVAL_BATCH must be an integer from 1 to 64"
  fi
  BATCH_SIZE=$((10#$BATCH_SIZE))
  ((${#CLIENT_NAMES[@]} > 0)) || fail "labMeta contains no client hosts"
  for NAME in "${CLIENT_NAMES[@]}"; do
    [[ "$NAME" =~ ^pc[0-9]+$ ]] || fail "labMeta contains invalid client name"
  done
  DECLARED_NAMES="$(printf '%s\n' "${CLIENT_NAMES[@]}" | jq -Rn '[inputs]')"
  # Computers of a class share one system (ADR 0033): the caller may set
  # CLIENT_CLASSES, parallel to the names; one member of each class is
  # evaluated and its system is given to the others.
  for INDEX in "${!CLIENT_NAMES[@]}"; do
    NAME=${CLIENT_NAMES[$INDEX]}
    KEY=${CLIENT_CLASSES[$INDEX]:-$NAME}
    [[ "$KEY" == "$NAME" || "$KEY" =~ ^shared-[0-9]+$ ]] || fail "labMeta contains an invalid class of $NAME"
    if [[ -z "${FIRST_OF[$KEY]:-}" ]]; then
      FIRST_OF[$KEY]=$NAME
      EVAL_NAMES+=("$NAME")
    fi
    REPRESENTATIVE_OF[$NAME]=${FIRST_OF[$KEY]}
  done
  REPRESENTATIVES="$(for NAME in "${CLIENT_NAMES[@]}"; do printf '%s\t%s\n' "$NAME" "${REPRESENTATIVE_OF[$NAME]}"; done \
    | jq -Rn '[inputs | split("\t") | {key: .[0], value: .[1]}] | from_entries')"
  TOTAL=${#EVAL_NAMES[@]}

  ARTIFACTS="$(pxe_eval_description "$FLAKE_REFERENCE" artifacts '[]')" \
    || fail "could not evaluate the required PXE artifacts"
  CLIENTS='[]'
  for ((START = 0; START < TOTAL; START += BATCH_SIZE)); do
    BATCH_NAMES=("${EVAL_NAMES[@]:START:BATCH_SIZE}")
    END=$((START + ${#BATCH_NAMES[@]}))
    if declare -F publish_progress >/dev/null; then
      publish_progress running clients \
        "Evaluating client configurations $((START + 1))–$END of $TOTAL" "$START" "$TOTAL"
    fi
    BATCH_JSON="$(printf '%s\n' "${BATCH_NAMES[@]}" | jq -Rn '[inputs]')"
    BATCH_DESCRIPTION="$(pxe_eval_description "$FLAKE_REFERENCE" clients "$BATCH_JSON")" \
      || fail "could not evaluate client systems $(pxe_batch_label "${BATCH_NAMES[@]}")"
    CLIENTS="$(jq -c --argjson more "$BATCH_DESCRIPTION" '. + ($more.clients // [])' <<<"$CLIENTS")" \
      || fail "Nix returned an invalid PXE build description"
  done
  DESCRIPTION="$(jq -n --argjson artifacts "$ARTIFACTS" --argjson clients "$CLIENTS" \
    --argjson names "$DECLARED_NAMES" --argjson representatives "$REPRESENTATIVES" '
      {artifacts: $artifacts.artifacts,
       clients: [$names[] as $name | ($clients[] | select(.name == $representatives[$name])) | .name = $name]}')" \
    || fail "Nix returned an invalid PXE build description"
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
  # Build already evaluated derivations, so this final process never evaluates
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

  # shellcheck disable=SC2034 # Results for the caller of this function.
  {
    KERNEL_PATH="${OUTPUTS[0]}"
    INITRD_PATH="${OUTPUTS[1]}"
    IPXE_SCRIPT_PATH="${OUTPUTS[2]}"
    FIRMWARE_PATH="${OUTPUTS[3]}"
    CLIENT_PATHS=("${OUTPUTS[@]:4}")
  }
}
