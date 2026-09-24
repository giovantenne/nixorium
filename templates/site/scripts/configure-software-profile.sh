#!/usr/bin/env bash
set -euo pipefail

configure_site_software_profile() {
  if [[ $# -ne 3 ]]; then
    echo "Usage: configure_site_software_profile <catalog.json> <lab-software.json> <input-fd>" >&2
    return 1
  fi

  local CATALOG_FILE="$1"
  local SOFTWARE_FILE="$2"
  local INPUT_FD="$3"
  local CHOICE
  local CHOICE_NUMBER
  local CONFIRMATION
  local DEFAULT_PRESET
  local INDEX
  local PRESET_COUNT
  local PRESET_DESCRIPTION
  local PRESET_ID
  local PRESET_LABEL
  local SELECTED_ID
  local SOFTWARE_DIRECTORY
  local TEMPORARY_FILE

  if ! command -v jq >/dev/null 2>&1; then
    echo "Error: software profile selection requires jq from the official NixOS Minimal ISO." >&2
    return 1
  fi
  if [[ ! "$INPUT_FD" =~ ^[0-9]+$ ]]; then
    echo "Error: software profile input descriptor is invalid." >&2
    return 1
  fi
  if [[ ! -f "$CATALOG_FILE" || ! -f "$SOFTWARE_FILE" ]]; then
    echo "Error: the selected Nixorium revision does not provide a complete software profile template." >&2
    return 1
  fi
  if ! jq -e '
    type == "object" and
    .schemaVersion == 1 and
    (.defaultPreset | type == "string" and length > 0) and
    (.presets | type == "array" and length > 0) and
    ([.presets[].id] | unique | length) == (.presets | length) and
    (.defaultPreset as $default | any(.presets[]; .id == $default)) and
    all(.presets[];
      (type == "object") and
      (.id | type == "string" and test("^[a-z0-9][a-z0-9-]{0,39}$")) and
      (.label | type == "string" and length > 0) and
      (.description | type == "string" and length > 0) and
      (.packages | type == "array" and length > 0) and
      all(.packages[]; type == "string" and length > 0) and
      ((.packages | unique | length) == (.packages | length)))
  ' "$CATALOG_FILE" >/dev/null; then
    echo "Error: software-presets.json from the selected revision is invalid." >&2
    return 1
  fi

  DEFAULT_PRESET="$(jq -r '.defaultPreset' "$CATALOG_FILE")"
  PRESET_COUNT="$(jq -r '.presets | length' "$CATALOG_FILE")"
  echo "  Choose the initial applications before the controller is built."
  echo "  You can add or remove applications later from Nixorium."
  echo
  for ((INDEX = 0; INDEX < PRESET_COUNT; INDEX++)); do
    PRESET_ID="$(jq -r ".presets[$INDEX].id" "$CATALOG_FILE")"
    PRESET_LABEL="$(jq -r ".presets[$INDEX].label" "$CATALOG_FILE")"
    PRESET_DESCRIPTION="$(jq -r ".presets[$INDEX].description" "$CATALOG_FILE")"
    if [[ "$PRESET_ID" == "$DEFAULT_PRESET" ]]; then
      printf '  %d) %s [%s, default]\n' "$((INDEX + 1))" "$PRESET_LABEL" "$PRESET_ID"
    else
      printf '  %d) %s [%s]\n' "$((INDEX + 1))" "$PRESET_LABEL" "$PRESET_ID"
    fi
    printf '     %s\n' "$PRESET_DESCRIPTION"
    echo
  done

  while true; do
    printf '  > Profile [%s]: ' "$DEFAULT_PRESET"
    if ! IFS= read -r -u "$INPUT_FD" CHOICE; then
      echo >&2
      echo "Error: controller setup input ended during software profile selection." >&2
      return 1
    fi
    if [[ -z "$CHOICE" ]]; then
      SELECTED_ID="$DEFAULT_PRESET"
      break
    fi
    if [[ "$CHOICE" =~ ^[0-9]+$ ]]; then
      CHOICE_NUMBER=$((10#$CHOICE))
      if (( CHOICE_NUMBER >= 1 && CHOICE_NUMBER <= PRESET_COUNT )); then
        SELECTED_ID="$(jq -r ".presets[$((CHOICE_NUMBER - 1))].id" "$CATALOG_FILE")"
        break
      fi
    elif jq -e --arg id "$CHOICE" 'any(.presets[]; .id == $id)' "$CATALOG_FILE" >/dev/null; then
      SELECTED_ID="$CHOICE"
      break
    fi
    echo "  ! Choose a listed number or profile ID."
  done

  PRESET_LABEL="$(jq -r --arg id "$SELECTED_ID" '.presets[] | select(.id == $id) | .label' "$CATALOG_FILE")"
  echo
  echo "  Software profile review"
  echo
  echo "    Profile:    ${PRESET_LABEL}"
  echo "    Applies to: controller and all current or future clients"
  echo "    Later:      add or remove individual applications from Nixorium"
  echo
  while true; do
    printf '  > Use this software profile? [Y/n]: '
    if ! IFS= read -r -u "$INPUT_FD" CONFIRMATION; then
      echo >&2
      echo "Error: controller setup input ended before software confirmation." >&2
      return 1
    fi
    case "${CONFIRMATION,,}" in
      ""|y|yes) break ;;
      n|no)
        echo "Controller installation cancelled; the disk was not changed." >&2
        return 1
        ;;
      *) echo "  ! Enter y or n." ;;
    esac
  done

  SOFTWARE_DIRECTORY="$(dirname "$SOFTWARE_FILE")"
  TEMPORARY_FILE="$(mktemp "${SOFTWARE_DIRECTORY}/.lab-software.json.tmp.XXXXXX")"
  if ! jq --arg id "$SELECTED_ID" '
    {
      schemaVersion: 1,
      packages: (
        [.presets[] | select(.id == $id) | .packages[]]
        | sort
        | map({package: ., scope: {kind: "shared"}})
      )
    }
  ' "$CATALOG_FILE" > "$TEMPORARY_FILE"; then
    rm -f "$TEMPORARY_FILE"
    echo "Error: could not create the selected software declarations." >&2
    return 1
  fi
  chmod 0644 "$TEMPORARY_FILE"
  mv -- "$TEMPORARY_FILE" "$SOFTWARE_FILE"
  echo
  echo "  [ OK ] Selected ${PRESET_LABEL}; applications are ready for the first controller build."
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  if [[ $# -ne 2 ]]; then
    echo "Usage: configure-software-profile.sh <catalog.json> <lab-software.json>" >&2
    exit 1
  fi
  configure_site_software_profile "$1" "$2" 0
fi
