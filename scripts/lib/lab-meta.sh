#!/usr/bin/env bash
set -euo pipefail

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  echo "Error: source this file from another script." >&2
  exit 1
fi

load_lab_meta() {
  if [[ $# -ne 1 ]]; then
    echo "Usage: load_lab_meta <repo-root>" >&2
    return 1
  fi

  local REPO_ROOT="$1"
  local LAB_META_JSON

  if [[ ! -f "${REPO_ROOT}/flake.nix" ]]; then
    echo "Error: '${REPO_ROOT}' does not look like the repo root (missing flake.nix)." >&2
    return 1
  fi

  if ! command -v nix >/dev/null 2>&1; then
    echo "Error: nix is required to evaluate lab metadata." >&2
    return 1
  fi

  if ! command -v jq >/dev/null 2>&1; then
    echo "Error: jq is required to parse lab metadata." >&2
    return 1
  fi

  LAB_META_JSON=$(
    nix --extra-experimental-features "nix-command flakes" \
      eval "${REPO_ROOT}#labMeta" \
      --json \
      --no-write-lock-file
  )

  IFS=$'\t' read -r \
    LAB_META_SCHEMA_VERSION \
    LAB_CONTROLLER_NAME \
    LAB_CONTROLLER_NUMBER \
    LAB_CONTROLLER_STATIC_IP \
    LAB_CONTROLLER_DHCP_IP \
    LAB_CLIENT_COUNT \
    LAB_NETWORK_BASE \
    LAB_IFACE_NAME \
    LAB_CACHE_PORT \
    LAB_PXE_HTTP_PORT \
    LAB_STUDENT_USER \
    LAB_TEACHER_USER \
    <<< "$(printf '%s' "${LAB_META_JSON}" | jq -er '
      [
        .schemaVersion,
        .controller.name,
        (.controller.number | tostring),
        .controller.staticIp,
        .controller.dhcpIp,
        (.clients.count | tostring),
        .network.base,
        .network.ifaceName,
        (.network.cachePort | tostring),
        (.network.pxeHttpPort | tostring),
        .users.student,
        .users.teacher
      ] | @tsv
    ')"

  if [[ "${LAB_META_SCHEMA_VERSION}" != "1" ]]; then
    echo "Error: unsupported labMeta schema version '${LAB_META_SCHEMA_VERSION}'." >&2
    return 1
  fi

  export LAB_META_SCHEMA_VERSION
  export LAB_CONTROLLER_NAME
  export LAB_CONTROLLER_NUMBER
  export LAB_CONTROLLER_STATIC_IP
  export LAB_CONTROLLER_DHCP_IP
  export LAB_CLIENT_COUNT
  export LAB_NETWORK_BASE
  export LAB_IFACE_NAME
  export LAB_CACHE_PORT
  export LAB_PXE_HTTP_PORT
  export LAB_STUDENT_USER
  export LAB_TEACHER_USER
}
