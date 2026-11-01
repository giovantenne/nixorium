#!/usr/bin/env bash
# shellcheck disable=SC2034 # The variables are read by the scripts that source this file.
set -euo pipefail

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  echo "Error: source this file from another script." >&2
  exit 1
fi

# Terminal presentation for setup.sh. install.sh and scripts/install-controller.sh
# are fetched with curl and carry their own copy; tests/ux-shell-regressions.sh
# fails when the shared helpers drift apart.
UI_RULE="--------------------------------------------------------"
UI_RESET=""
UI_BOLD=""
UI_TITLE=""
UI_FOCUS=""
UI_SUCCESS=""
UI_WARNING=""
UI_ERROR=""
UI_MUTED=""

# Same presentation as the controller bootstrap (install.sh); plain text when
# redirected or on a limited terminal.
if [[ -t 1 && "${TERM:-dumb}" != "dumb" && -z "${NO_COLOR:-}" ]]; then
  UI_RESET=$'\033[0m'
  UI_BOLD=$'\033[1m'
  UI_TITLE=$'\033[1;35m'
  UI_FOCUS=$'\033[36m'
  UI_SUCCESS=$'\033[32m'
  UI_WARNING=$'\033[33m'
  UI_ERROR=$'\033[31m'
  UI_MUTED=$'\033[2m'
fi

ui_section() {
  printf '\n  %s%s%s\n  %s\n\n' "$UI_TITLE" "$1" "$UI_RESET" "$UI_RULE"
}

ui_step() {
  ui_section "Step $1 of 3 · $2"
}

ui_note() {
  printf '  %s\n' "$1"
}

ui_log() {
  printf '  %s[....]%s %s\n' "$UI_FOCUS" "$UI_RESET" "$1"
}

ui_success() {
  printf '  %s[ OK ]%s %s\n' "$UI_SUCCESS" "$UI_RESET" "$1"
}

ui_feedback() {
  printf '  %s!%s %s\n' "$UI_WARNING" "$UI_RESET" "$1"
}

# An error line followed by indented lines that say what to do next.
ui_error() {
  local LINE
  printf '\n  %sx%s %s%s%s\n' "$UI_ERROR" "$UI_RESET" "$UI_BOLD" "$1" "$UI_RESET" >&2
  shift
  for LINE in "$@"; do
    printf '    %s\n' "$LINE" >&2
  done
}

ui_detail() {
  printf '    %-14s %s%s%s\n' "${1}:" "$UI_BOLD" "$2" "$UI_RESET"
}

ui_prompt() {
  local LABEL="$1"
  local DEFAULT_VALUE="${2:-}"
  printf '  %s>%s %s%s%s' "$UI_FOCUS" "$UI_RESET" "$UI_BOLD" "$LABEL" "$UI_RESET"
  if [[ -n "$DEFAULT_VALUE" ]]; then
    printf ' [%s]' "$DEFAULT_VALUE"
  fi
  printf ': '
}
