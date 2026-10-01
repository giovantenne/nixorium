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
  local DEFAULT_LABEL
  local DEFAULT_PRESET
  local INDEX
  local PRESET_COUNT
  local PRESET_DESCRIPTION
  local PRESET_ID
  local PRESET_LABEL
  local SELECTED_ID
  local SOFTWARE_DIRECTORY
  local TAILORED_PROFILE
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
  DEFAULT_LABEL="$(jq -r --arg id "$DEFAULT_PRESET" '.presets[] | select(.id == $id) | .label' "$CATALOG_FILE")"
  echo "  Choose the applications installed on the controller and on every"
  echo "  student computer. You can add or remove single applications later"
  echo "  from Nixorium, so this choice is only a starting point."
  echo

  while true; do
    for ((INDEX = 0; INDEX < PRESET_COUNT; INDEX++)); do
      PRESET_ID="$(jq -r ".presets[$INDEX].id" "$CATALOG_FILE")"
      PRESET_LABEL="$(jq -r ".presets[$INDEX].label" "$CATALOG_FILE")"
      PRESET_DESCRIPTION="$(jq -r ".presets[$INDEX].description" "$CATALOG_FILE")"
      if [[ "$PRESET_ID" == "$DEFAULT_PRESET" ]]; then
        printf '  %s%d)%s %s%s%s  (default)\n' "${UI_FOCUS:-}" "$((INDEX + 1))" "${UI_RESET:-}" "${UI_BOLD:-}" "$PRESET_LABEL" "${UI_RESET:-}"
      else
        printf '  %s%d)%s %s%s%s\n' "${UI_FOCUS:-}" "$((INDEX + 1))" "${UI_RESET:-}" "${UI_BOLD:-}" "$PRESET_LABEL" "${UI_RESET:-}"
      fi
      printf '     %s\n' "$PRESET_DESCRIPTION" | fold -s -w 72 | sed 's/[[:space:]]*$//; 2,$s/^/     /'
      echo
    done

    while true; do
      printf '  %s>%s %sApplications%s [%s]: ' "${UI_FOCUS:-}" "${UI_RESET:-}" "${UI_BOLD:-}" "${UI_RESET:-}" "$DEFAULT_LABEL"
      if ! IFS= read -r -u "$INPUT_FD" CHOICE; then
        echo >&2
        echo "  x Setup input ended while choosing the applications." >&2
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
      echo "  ${UI_WARNING:-}!${UI_RESET:-} Type a number from 1 to ${PRESET_COUNT}, or press Enter for ${DEFAULT_LABEL}."
    done

    PRESET_LABEL="$(jq -r --arg id "$SELECTED_ID" '.presets[] | select(.id == $id) | .label' "$CATALOG_FILE")"
    echo
    echo "  Check the applications"
    echo
    echo "    Choice:        ${PRESET_LABEL}"
    echo "    Installed on:  the controller and every current or future student computer"
    echo "    Later:         add or remove single applications from Nixorium"
    echo
    while true; do
      printf '  %s>%s %sUse these applications?%s [Y/n]: ' "${UI_FOCUS:-}" "${UI_RESET:-}" "${UI_BOLD:-}" "${UI_RESET:-}"
      if ! IFS= read -r -u "$INPUT_FD" CONFIRMATION; then
        echo >&2
        echo "  x Setup input ended before the applications were confirmed." >&2
        return 1
      fi
      case "${CONFIRMATION,,}" in
        ""|y|yes) break 2 ;;
        n|no)
          echo
          echo "  Choose again:"
          echo
          break
          ;;
        *) echo "  ${UI_WARNING:-}!${UI_RESET:-} Type y for yes or n to choose again." ;;
      esac
    done
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
  # A profile may ship its own starting student home, such as Programming's
  # editor extensions. Others keep the template's common profile.
  TAILORED_PROFILE="${SOFTWARE_DIRECTORY}/workspace-profile.${SELECTED_ID}.example.json"
  if [[ -f "$TAILORED_PROFILE" && -f "${SOFTWARE_DIRECTORY}/workspace-profile.json" ]]; then
    TEMPORARY_FILE="$(mktemp "${SOFTWARE_DIRECTORY}/.workspace-profile.json.tmp.XXXXXX")"
    if ! jq -e '.schemaVersion == 1' "$TAILORED_PROFILE" > /dev/null || ! cp -- "$TAILORED_PROFILE" "$TEMPORARY_FILE"; then
      rm -f "$TEMPORARY_FILE"
      echo "Error: could not prepare the student home for ${PRESET_LABEL}." >&2
      return 1
    fi
    chmod 0644 "$TEMPORARY_FILE"
    mv -- "$TEMPORARY_FILE" "${SOFTWARE_DIRECTORY}/workspace-profile.json"
  fi
  echo
  printf '  %s[ OK ]%s %s selected.\n' "${UI_SUCCESS:-}" "${UI_RESET:-}" "$PRESET_LABEL"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  if [[ $# -ne 2 ]]; then
    echo "Usage: configure-software-profile.sh <catalog.json> <lab-software.json>" >&2
    exit 1
  fi
  configure_site_software_profile "$1" "$2" 0
fi
