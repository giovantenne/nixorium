#!/usr/bin/env bash
set -euo pipefail

DEFAULT_RELEASE="v2.0.0"
RELEASE="${NIXORIUM_RELEASE:-}"
INSTALL_DISK=""
REPOSITORY="giovantenne/nixorium"
RELEASES_API_URL="https://api.github.com/repos/${REPOSITORY}/releases?per_page=100"
TARGET_ROOT="${NIXORIUM_TARGET_ROOT:-/mnt}"
ADMIN_USER="admin"
DEPLOYMENT_NAME="nixorium-deployment"
INSTALLER_REF="${NIXORIUM_INSTALLER_REF:-}"
BOOTSTRAP_TTY="${NIXORIUM_BOOTSTRAP_TTY:-/dev/tty}"
INSTALLER_ARGS=()
BOOTSTRAP_VERSION=0
BOOTSTRAP_TEACHER_USER="teacher"
BOOTSTRAP_STUDENT_USER="student"
BOOTSTRAP_TIME_ZONE="America/New_York"
BOOTSTRAP_KEYBOARD="us"
BOOTSTRAP_CONSOLE_KEYMAP="us"
BOOTSTRAP_ADMIN_HASH=""
BOOTSTRAP_TEACHER_HASH=""
BOOTSTRAP_STUDENT_HASH=""
BOOTSTRAP_INPUT_FD=""
UI_RULE="--------------------------------------------------------"
UI_RESET=""
UI_BOLD=""
UI_TITLE=""
UI_FOCUS=""
UI_SUCCESS=""
UI_WARNING=""
UI_ERROR=""
UI_MUTED=""
SETUP_STAGE="start"
TIME_ZONE_SUGGESTED=true

# Keep redirected output and limited terminals readable without ANSI escapes.
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

# Keep bootstrap downloads independent from any cache configured in the live environment.
export NIX_CONFIG=$'experimental-features = nix-command flakes\nsubstituters = https://cache.nixos.org/\ntrusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY='

usage() {
  echo "Usage: install.sh [--release <tag|master>] [--disk <device>]" >&2
  echo "Example: install.sh --release master --disk /dev/sda" >&2
}

ui_banner() {
  printf '\n%s' "$UI_TITLE"
  cat <<'EOF'
   _   _ _                 _
  | \ | (_)_  _____  _ __(_)_   _ _ __ ___
  |  \| | \ \/ / _ \| '__| | | | | '_ ` _ \
  | |\  | |>  < (_) | |  | | |_| | | | | | |
  |_| \_|_/_/\_\___/|_|  |_|\__,_|_| |_| |_|
EOF
  printf '%s\n' "$UI_RESET"
  ui_note "${UI_BOLD}Controller setup for a NixOS computer lab${UI_RESET}"
  echo
  ui_note "This computer becomes the Nixorium controller: the PC that"
  ui_note "installs, updates and manages every student computer."
  echo
  ui_note "In five short steps you choose:"
  ui_note "  1. keyboard and time zone      4. the initial applications"
  ui_note "  2. account names               5. the disk to install on"
  ui_note "  3. passwords"
  echo
  ui_note "Nothing on this computer changes until you type ERASE."
  ui_note "${UI_MUTED}Press Ctrl+C at any time before that to stop.${UI_RESET}"
}

ui_step() {
  local NUMBER="$1"
  local TITLE="$2"
  ui_section "Step ${NUMBER} of 5 · ${TITLE}"
}

ui_section() {
  local TITLE="$1"
  printf '\n  %s%s%s\n  %s\n\n' "$UI_TITLE" "$TITLE" "$UI_RESET" "$UI_RULE"
}

ui_log() {
  printf '  %s[....]%s %s\n' "$UI_FOCUS" "$UI_RESET" "$1"
}

ui_success() {
  printf '  %s[ OK ]%s %s\n' "$UI_SUCCESS" "$UI_RESET" "$1"
}

ui_note() {
  printf '  %s\n' "$1"
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
  local LABEL="$1"
  local VALUE="$2"
  printf '    %-14s %s%s%s\n' "${LABEL}:" "$UI_BOLD" "$VALUE" "$UI_RESET"
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

prompt_bootstrap_value() {
  local LABEL="$1"
  local DEFAULT_VALUE="$2"
  local TARGET_VAR="$3"
  local READ_VALUE

  ui_prompt "$LABEL" "$DEFAULT_VALUE"
  if ! IFS= read -r -u "$BOOTSTRAP_INPUT_FD" READ_VALUE; then
    echo >&2
    ui_error "Setup input ended before all questions were answered."
    return 1
  fi
  if [[ -z "$READ_VALUE" ]]; then
    READ_VALUE="$DEFAULT_VALUE"
  fi
  printf -v "$TARGET_VAR" '%s' "$READ_VALUE"
}

prompt_bootstrap_user() {
  local LABEL="$1"
  local DEFAULT_VALUE="$2"
  local DIFFERENT_FROM="$3"
  local TARGET_VAR="$4"
  local VALUE

  while true; do
    if ! prompt_bootstrap_value "$LABEL" "$DEFAULT_VALUE" VALUE; then
      return 1
    fi
    if [[ ! "$VALUE" =~ ^[a-z_][a-z0-9_-]{0,30}$ ]]; then
      ui_feedback "Use lowercase letters and numbers only (also '_' or '-'), starting with a letter."
      continue
    fi
    if [[ "$VALUE" == "root" || "$VALUE" == "admin" ]]; then
      ui_feedback "'${VALUE}' is reserved; choose another name."
      continue
    fi
    if [[ -n "$DIFFERENT_FROM" && "$VALUE" == "$DIFFERENT_FROM" ]]; then
      ui_feedback "Teacher and student usernames must be different."
      continue
    fi
    printf -v "$TARGET_VAR" '%s' "$VALUE"
    return
  done
}

hash_bootstrap_password() {
  local PASSWORD="$1"
  if command -v mkpasswd >/dev/null 2>&1; then
    printf '%s\n' "$PASSWORD" | mkpasswd -m sha-512 --stdin
  elif command -v openssl >/dev/null 2>&1; then
    printf '%s\n' "$PASSWORD" | openssl passwd -6 -stdin
  else
    ui_error "This live system cannot encrypt passwords (no mkpasswd or openssl)." \
      "Start from the official NixOS Minimal ISO and run the command again."
    return 1
  fi
}

prompt_bootstrap_password() {
  local LABEL="$1"
  local TARGET_VAR="$2"
  local PASSWORD
  local CONFIRMATION
  local PASSWORD_LENGTH
  local HASH
  local LC_ALL=C

  while true; do
    ui_prompt "$LABEL"
    if ! IFS= read -r -s -u "$BOOTSTRAP_INPUT_FD" PASSWORD; then
      echo >&2
      ui_error "Setup input ended before all questions were answered."
      return 1
    fi
    echo
    if [[ "$PASSWORD" == "nixos" ]]; then
      ui_feedback "'nixos' is a publicly known default; choose another password."
      continue
    fi
    PASSWORD_LENGTH=${#PASSWORD}
    if (( PASSWORD_LENGTH < 8 )); then
      ui_feedback "Too short: use at least 8 characters."
      continue
    fi
    ui_prompt "Confirm ${LABEL,,}"
    if ! IFS= read -r -s -u "$BOOTSTRAP_INPUT_FD" CONFIRMATION; then
      echo >&2
      ui_error "Setup input ended before all questions were answered."
      return 1
    fi
    echo
    if [[ "$PASSWORD" != "$CONFIRMATION" ]]; then
      ui_feedback "The two entries differ; type the password again."
      continue
    fi
    HASH="$(hash_bootstrap_password "$PASSWORD")"
    PASSWORD=""
    CONFIRMATION=""
    if [[ ! "$HASH" =~ ^\$6\$[^$]+\$[^$]+$ ]]; then
      ui_error "The password could not be encrypted correctly." \
      "Start from the official NixOS Minimal ISO and run the command again."
      return 1
    fi
    printf -v "$TARGET_VAR" '%s' "$HASH"
    ui_success "${LABEL} set."
    return
  done
}

activate_bootstrap_keyboard() {
  if [[ -n "${DISPLAY:-}" || -n "${WAYLAND_DISPLAY:-}" ]]; then
    ui_error "The selected keyboard cannot be verified safely from a graphical terminal." \
      "Boot the official NixOS Minimal ISO, or switch to a Linux text console, then run the command again."
    return 1
  fi
  if ! command -v loadkeys >/dev/null 2>&1; then
    ui_error "This live system cannot change the keyboard layout (no loadkeys)." \
      "Start from the official NixOS Minimal ISO and run the command again."
    return 1
  fi
  if ! sudo loadkeys "$BOOTSTRAP_CONSOLE_KEYMAP"; then
    ui_error "Could not activate console keymap '$BOOTSTRAP_CONSOLE_KEYMAP'." \
      "No password has been asked yet. Check the live console and run the command again."
    return 1
  fi
  ui_success "Keyboard set to $BOOTSTRAP_KEYBOARD. It is used from now on and after installation."
}

suggest_time_zone() {
  # Propose the usual time zone of the chosen keyboard until one is typed.
  [[ "$TIME_ZONE_SUGGESTED" == "true" ]] || return 0
  case "$BOOTSTRAP_KEYBOARD" in
    us) BOOTSTRAP_TIME_ZONE="America/New_York" ;;
    it) BOOTSTRAP_TIME_ZONE="Europe/Rome" ;;
    gb) BOOTSTRAP_TIME_ZONE="Europe/London" ;;
    fr) BOOTSTRAP_TIME_ZONE="Europe/Paris" ;;
    de) BOOTSTRAP_TIME_ZONE="Europe/Berlin" ;;
    es) BOOTSTRAP_TIME_ZONE="Europe/Madrid" ;;
  esac
}

collect_regional_settings() {
  local SUGGESTED_TIME_ZONE

  ui_step 1 "Keyboard and time zone"
  ui_note "Keyboard layouts:"
  ui_note "  us  English (US)     gb  English (UK)     it  Italian"
  ui_note "  fr  French           de  German           es  Spanish"
  echo
  while true; do
    if ! prompt_bootstrap_value "Keyboard layout" "$BOOTSTRAP_KEYBOARD" BOOTSTRAP_KEYBOARD; then
      return 1
    fi
    case "$BOOTSTRAP_KEYBOARD" in
      us) BOOTSTRAP_CONSOLE_KEYMAP="us"; break ;;
      it) BOOTSTRAP_CONSOLE_KEYMAP="it2"; break ;;
      gb) BOOTSTRAP_CONSOLE_KEYMAP="uk"; break ;;
      fr) BOOTSTRAP_CONSOLE_KEYMAP="fr"; break ;;
      de) BOOTSTRAP_CONSOLE_KEYMAP="de"; break ;;
      es) BOOTSTRAP_CONSOLE_KEYMAP="es"; break ;;
      *) ui_feedback "Type one of the codes above, for example it." ;;
    esac
  done

  activate_bootstrap_keyboard || return 1
  suggest_time_zone
  echo
  ui_note "Time zone as Region/City, for example Europe/Rome or America/Chicago."
  while true; do
    SUGGESTED_TIME_ZONE="$BOOTSTRAP_TIME_ZONE"
    if ! prompt_bootstrap_value "Time zone" "$BOOTSTRAP_TIME_ZONE" BOOTSTRAP_TIME_ZONE; then
      return 1
    fi
    if [[ "$BOOTSTRAP_TIME_ZONE" =~ ^[A-Za-z0-9_+.-]+(/[A-Za-z0-9_+.-]+)+$ ]] && \
      { [[ -e "/etc/zoneinfo/${BOOTSTRAP_TIME_ZONE}" ]] || \
        [[ -e "/usr/share/zoneinfo/${BOOTSTRAP_TIME_ZONE}" ]]; }; then
      if [[ "$BOOTSTRAP_TIME_ZONE" != "$SUGGESTED_TIME_ZONE" ]]; then
        TIME_ZONE_SUGGESTED=false
      fi
      break
    fi
    ui_feedback "'${BOOTSTRAP_TIME_ZONE}' is not a known time zone. Use Region/City, for example Europe/Rome."
    BOOTSTRAP_TIME_ZONE="$SUGGESTED_TIME_ZONE"
  done
}

collect_accounts() {
  ui_step 2 "Account names"
  ui_note "Three local accounts are created:"
  ui_note "  admin     manages the lab from this controller (name fixed)"
  ui_note "  teacher   uses the classroom tools on this controller"
  ui_note "  student   opens automatically on the student computers"
  echo
  ui_note "Press Enter to keep the suggested name."
  echo
  prompt_bootstrap_user "Teacher username" "$BOOTSTRAP_TEACHER_USER" "" BOOTSTRAP_TEACHER_USER || return 1
  prompt_bootstrap_user "Student username" "$BOOTSTRAP_STUDENT_USER" "$BOOTSTRAP_TEACHER_USER" BOOTSTRAP_STUDENT_USER || return 1
}

collect_passwords() {
  ui_step 3 "Passwords"
  ui_note "At least 8 characters each. What you type stays hidden."
  ui_note "Keep the administrator password safe: you need it to manage the lab."
  echo
  prompt_bootstrap_password "Administrator password" BOOTSTRAP_ADMIN_HASH || return 1
  echo
  prompt_bootstrap_password "Teacher password" BOOTSTRAP_TEACHER_HASH || return 1
  echo
  prompt_bootstrap_password "Student password" BOOTSTRAP_STUDENT_HASH || return 1
}

collect_bootstrap_configuration() {
  local CONFIRMATION

  if [[ ! -r "$BOOTSTRAP_TTY" ]]; then
    ui_error "Setup needs a keyboard and screen." \
      "Run the command on the computer's own console, not through a pipe or script."
    return 1
  fi
  exec {BOOTSTRAP_INPUT_FD}< "$BOOTSTRAP_TTY"

  echo
  ui_note "${UI_MUTED}Recommended environment: official NixOS Minimal ISO in UEFI mode.${UI_RESET}"
  ui_note "Press Enter to keep the value shown in [brackets]."

  while true; do
    collect_regional_settings || return 1
    collect_accounts || return 1
    collect_passwords || return 1

    ui_section "Check your answers"
    ui_detail "Keyboard" "$BOOTSTRAP_KEYBOARD"
    ui_detail "Time zone" "$BOOTSTRAP_TIME_ZONE"
    echo
    ui_detail "Administrator" "admin"
    ui_detail "Teacher" "$BOOTSTRAP_TEACHER_USER"
    ui_detail "Student" "$BOOTSTRAP_STUDENT_USER"
    ui_detail "Passwords" "set (stored only as secure hashes)"
    echo
    ui_note "Student computers are added later, from Nixorium."
    echo
    while true; do
      ui_prompt "Are these answers correct?" "Y/n"
      if ! IFS= read -r -u "$BOOTSTRAP_INPUT_FD" CONFIRMATION; then
        echo >&2
        ui_error "Setup input ended before the answers were confirmed."
        return 1
      fi
      case "${CONFIRMATION,,}" in
        ""|y|yes)
          if [[ "$BOOTSTRAP_VERSION" == "1" ]]; then
            exec {BOOTSTRAP_INPUT_FD}<&-
            BOOTSTRAP_INPUT_FD=""
          fi
          ui_success "Answers saved for the installation."
          return
          ;;
        n|no)
          echo
          ui_note "Let's go through them again. Your previous answers are suggested;"
          ui_note "passwords must be typed again."
          break
          ;;
        *) ui_feedback "Type y for yes or n to change the answers." ;;
      esac
    done
  done
}

configure_bootstrap_settings() {
  local SETTINGS_FILE="$1"
  local OUTPUT_FILE
  local HAS_DEPLOYMENT_MODE=false
  local LINE

  if grep -q '"deploymentMode"' "$SETTINGS_FILE"; then
    HAS_DEPLOYMENT_MODE=true
  fi
  OUTPUT_FILE="$(mktemp)"
  while IFS= read -r LINE || [[ -n "$LINE" ]]; do
    case "$LINE" in
      '  "lab": {'*)
        printf '%s\n' "$LINE" >> "$OUTPUT_FILE"
        if [[ "$HAS_DEPLOYMENT_MODE" == "false" ]]; then
          printf '    "deploymentMode": "controller",\n' >> "$OUTPUT_FILE"
        fi
        ;;
      '    "deploymentMode": '*) printf '    "deploymentMode": "controller",\n' >> "$OUTPUT_FILE" ;;
      '    "pcCount": '*) printf '    "pcCount": 0,\n' >> "$OUTPUT_FILE" ;;
      '    "teacherUser": '*) printf '    "teacherUser": "%s",\n' "$BOOTSTRAP_TEACHER_USER" >> "$OUTPUT_FILE" ;;
      '    "studentUser": '*) printf '    "studentUser": "%s",\n' "$BOOTSTRAP_STUDENT_USER" >> "$OUTPUT_FILE" ;;
      '    "teacherPassword": '*) printf '    "teacherPassword": "%s",\n' "$BOOTSTRAP_TEACHER_HASH" >> "$OUTPUT_FILE" ;;
      '    "studentPassword": '*) printf '    "studentPassword": "%s",\n' "$BOOTSTRAP_STUDENT_HASH" >> "$OUTPUT_FILE" ;;
      '    "adminPassword": '*) printf '    "adminPassword": "%s",\n' "$BOOTSTRAP_ADMIN_HASH" >> "$OUTPUT_FILE" ;;
      '    "studentGitName": '*) printf '    "studentGitName": "%s",\n' "$BOOTSTRAP_STUDENT_USER" >> "$OUTPUT_FILE" ;;
      '    "timeZone": '*) printf '    "timeZone": "%s",\n' "$BOOTSTRAP_TIME_ZONE" >> "$OUTPUT_FILE" ;;
      '    "defaultLocale": '*) printf '    "defaultLocale": "en_US.UTF-8",\n' >> "$OUTPUT_FILE" ;;
      '    "extraLocale": '*) printf '    "extraLocale": "en_US.UTF-8",\n' >> "$OUTPUT_FILE" ;;
      '    "keyboardLayout": '*) printf '    "keyboardLayout": "%s",\n' "$BOOTSTRAP_KEYBOARD" >> "$OUTPUT_FILE" ;;
      '    "consoleKeyMap": '*)
        printf '    "consoleKeyMap": "%s"' "$BOOTSTRAP_CONSOLE_KEYMAP" >> "$OUTPUT_FILE"
        if [[ "$LINE" == *, ]]; then printf ',' >> "$OUTPUT_FILE"; fi
        printf '\n' >> "$OUTPUT_FILE"
        ;;
      *) printf '%s\n' "$LINE" >> "$OUTPUT_FILE" ;;
    esac
  done < "$SETTINGS_FILE"
  mv -- "$OUTPUT_FILE" "$SETTINGS_FILE"
}

choose_release() {
  local API_RESPONSE
  local API_SUCCEEDED=false
  local CHOICE
  local CHOICE_NUMBER
  local DEFAULT_AVAILABLE=false
  local INDEX
  local LABEL
  local LATEST_PRERELEASE=""
  local MENU_DEFAULT="$DEFAULT_RELEASE"
  local TAG
  local -a AVAILABLE_RELEASES=("master")
  local -a STABLE_RELEASES=()

  ui_section "Version" >&3
  ui_log "Fetching published Nixorium releases..." >&3
  if API_RESPONSE="$(curl -fsSL "$RELEASES_API_URL")"; then
    API_SUCCEEDED=true
    while IFS= read -r TAG; do
      if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
        continue
      fi

      if [[ "$TAG" == *-* ]]; then
        if [[ -z "$LATEST_PRERELEASE" ]]; then
          LATEST_PRERELEASE="$TAG"
        fi
      else
        STABLE_RELEASES+=("$TAG")
      fi
    done < <(
      printf '%s\n' "$API_RESPONSE" |
        sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\(v[^"]*\)",*$/\1/p'
    )
  else
    ui_feedback "Could not retrieve the GitHub release list." >&3
  fi

  if [[ -n "$LATEST_PRERELEASE" ]]; then
    AVAILABLE_RELEASES+=("$LATEST_PRERELEASE")
  fi
  AVAILABLE_RELEASES+=("${STABLE_RELEASES[@]}")

  if [[ "$API_SUCCEEDED" == "false" && "$DEFAULT_RELEASE" != "master" ]]; then
    AVAILABLE_RELEASES+=("$DEFAULT_RELEASE")
  fi

  if [[ "$DEFAULT_RELEASE" == *-* && -n "$LATEST_PRERELEASE" ]]; then
    MENU_DEFAULT="$LATEST_PRERELEASE"
  fi

  for TAG in "${AVAILABLE_RELEASES[@]}"; do
    if [[ "$TAG" == "$MENU_DEFAULT" ]]; then
      DEFAULT_AVAILABLE=true
      break
    fi
  done
  if [[ "$DEFAULT_AVAILABLE" == "false" ]]; then
    if [[ -n "$LATEST_PRERELEASE" ]]; then
      MENU_DEFAULT="$LATEST_PRERELEASE"
    elif (( ${#STABLE_RELEASES[@]} > 0 )); then
      MENU_DEFAULT="${STABLE_RELEASES[0]}"
    else
      MENU_DEFAULT="master"
    fi
  fi

  echo >&3
  ui_note "Select a Nixorium version:" >&3
  echo >&3
  for INDEX in "${!AVAILABLE_RELEASES[@]}"; do
    TAG="${AVAILABLE_RELEASES[$INDEX]}"
    if [[ "$TAG" == "master" ]]; then
      LABEL="development"
    elif [[ "$TAG" == *-* ]]; then
      LABEL="prerelease"
    else
      LABEL="stable"
    fi
    if [[ "$TAG" == "$MENU_DEFAULT" ]]; then
      LABEL="${LABEL}, default"
    fi
    printf '  %s%2d)%s %-22s %s\n' "$UI_FOCUS" "$((INDEX + 1))" "$UI_RESET" "$TAG" "$LABEL" >&3
  done

  echo >&3
  ui_note "Type a number, or press Enter to keep ${MENU_DEFAULT}." >&3
  echo >&3
  while true; do
    ui_prompt "Version" "$MENU_DEFAULT" >&3
    IFS= read -r CHOICE <&3

    if [[ -z "$CHOICE" ]]; then
      RELEASE="$MENU_DEFAULT"
      break
    fi
    if [[ "$CHOICE" =~ ^[0-9]+$ ]]; then
      CHOICE_NUMBER=$((10#$CHOICE))
      if (( CHOICE_NUMBER >= 1 && CHOICE_NUMBER <= ${#AVAILABLE_RELEASES[@]} )); then
        RELEASE="${AVAILABLE_RELEASES[$((CHOICE_NUMBER - 1))]}"
        break
      fi
    fi

    ui_feedback "Enter a number from 1 to ${#AVAILABLE_RELEASES[@]}, or press Enter for ${MENU_DEFAULT}." >&3
  done

  ui_success "Selected ${RELEASE}." >&3
  echo >&3
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --release)
      if [[ $# -lt 2 ]]; then
        echo "Error: --release requires a tag." >&2
        usage
        exit 1
      fi
      RELEASE="$2"
      shift 2
      ;;
    --disk)
      if [[ $# -lt 2 ]]; then
        echo "Error: --disk requires a device." >&2
        usage
        exit 1
      fi
      INSTALL_DISK="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: unknown argument '$1'." >&2
      usage
      exit 1
      ;;
  esac
done

ui_banner

if [[ ! -d "${NIXORIUM_INSTALLER_EFI_DIRECTORY:-/sys/firmware/efi}" ]]; then
  ui_error "This computer started in legacy BIOS mode; Nixorium needs UEFI." \
    "Enable UEFI boot in the firmware settings, start again from the NixOS USB stick" \
    "and run the command again. Nothing on this computer was changed."
  exit 1
fi

if [[ -z "$RELEASE" ]]; then
  if { exec 3<>/dev/tty; } 2>/dev/null; then
    choose_release
    exec 3>&-
  else
    RELEASE="$DEFAULT_RELEASE"
    echo "No interactive terminal detected; using ${RELEASE}." >&2
    echo "Pass --release <tag> or set NIXORIUM_RELEASE to choose explicitly." >&2
  fi
fi

if [[ "$RELEASE" != "master" && ! "$RELEASE" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "Error: '$RELEASE' is neither 'master' nor a valid release tag." >&2
  exit 1
fi

if [[ -z "$INSTALLER_REF" ]]; then
  INSTALLER_REF="$RELEASE"
fi

if [[ "$INSTALLER_REF" != "$RELEASE" ]]; then
  echo "Error: NIXORIUM_INSTALLER_REF must match the selected release." >&2
  echo "Installer, template, lock, and disk layout must come from one revision." >&2
  exit 1
fi

if [[ "$TARGET_ROOT" != /* || "$TARGET_ROOT" == "/" ]]; then
  echo "Error: NIXORIUM_TARGET_ROOT must be an absolute mount path other than /." >&2
  exit 1
fi

COMMIT_API_URL="https://api.github.com/repos/${REPOSITORY}/commits/${RELEASE}"
ui_section "Getting the installer"
ui_log "Finding the exact source of Nixorium ${RELEASE}..."
if ! COMMIT_RESPONSE="$(curl -fsSL "$COMMIT_API_URL")"; then
  ui_error "Could not reach GitHub to find Nixorium ${RELEASE}." \
    "Check that this computer is connected to the Internet, then run the command again." \
    "Nothing on this computer was changed."
  exit 1
fi
UPSTREAM_REV="$(
  printf '%s\n' "$COMMIT_RESPONSE" |
    sed -n 's/^[[:space:]]*"sha":[[:space:]]*"\([0-9a-f]\{40\}\)",\{0,1\}[[:space:]]*$/\1/p' |
    sed -n '1p'
)"
if [[ ! "$UPSTREAM_REV" =~ ^[0-9a-f]{40}$ ]]; then
  echo "Error: GitHub returned no valid immutable revision for ${RELEASE}." >&2
  exit 1
fi

UPSTREAM_REF="github:${REPOSITORY}/${UPSTREAM_REV}"
DECLARED_UPSTREAM_REF="github:${REPOSITORY}/${RELEASE}"
INSTALLER_URL="https://raw.githubusercontent.com/${REPOSITORY}/${UPSTREAM_REV}/scripts/install-controller.sh"
DISKO_LAYOUT_URL="https://raw.githubusercontent.com/${REPOSITORY}/${UPSTREAM_REV}/lib/disko-layout.nix"
CAPABILITY_URL="https://raw.githubusercontent.com/${REPOSITORY}/${UPSTREAM_REV}/flake.nix"
TEMP_INSTALLER="$(mktemp)"
TEMP_DISKO_LAYOUT="$(mktemp)"
TEMP_DEPLOYMENT="$(mktemp -d)"

cleanup() {
  local STATUS=$?
  if (( STATUS != 0 )) && [[ "$SETUP_STAGE" == "prepare" ]]; then
    ui_error "Setup stopped before installing. Nothing on this computer was changed." \
      "Read the message above, then run the command again."
  fi
  if [[ -n "$BOOTSTRAP_INPUT_FD" ]]; then
    exec {BOOTSTRAP_INPUT_FD}<&-
    BOOTSTRAP_INPUT_FD=""
  fi
  BOOTSTRAP_ADMIN_HASH=""
  BOOTSTRAP_TEACHER_HASH=""
  BOOTSTRAP_STUDENT_HASH=""
  rm -f "$TEMP_INSTALLER" "$TEMP_DISKO_LAYOUT"
  rm -rf "$TEMP_DEPLOYMENT"
}
trap cleanup EXIT

SETUP_STAGE="prepare"
ui_log "Checking what this version of the installer asks..."
if ! CAPABILITY_SOURCE="$(curl -fsSL "$CAPABILITY_URL")"; then
  ui_error "Could not download the installer of revision ${UPSTREAM_REV}." \
    "Check the Internet connection."
  exit 1
fi
ui_success "Using Nixorium ${RELEASE}, revision ${UPSTREAM_REV}."
if grep -Eq 'controllerBootstrapVersion[[:space:]]*=[[:space:]]*2;' <<< "$CAPABILITY_SOURCE"; then
  BOOTSTRAP_VERSION=2
elif grep -Eq 'controllerBootstrapVersion[[:space:]]*=[[:space:]]*1;' <<< "$CAPABILITY_SOURCE"; then
  BOOTSTRAP_VERSION=1
fi
CAPABILITY_SOURCE=""
if [[ "$BOOTSTRAP_VERSION" == "1" || "$BOOTSTRAP_VERSION" == "2" ]]; then
  collect_bootstrap_configuration
else
  echo "Warning: ${RELEASE} uses the legacy post-install setup flow." >&2
  echo "Choose master or a newer release for a controller that is ready at first boot." >&2
fi

if command -v git >/dev/null 2>&1; then
  GIT_COMMAND=(git)
else
  GIT_COMMAND=(
    nix
    --extra-experimental-features
    "nix-command flakes"
    shell
    nixpkgs#git
    --command
    git
  )
fi

ui_section "Preparing the lab configuration"
ui_log "Downloading the installer and the lab configuration template..."
curl -fsSL "$INSTALLER_URL" -o "$TEMP_INSTALLER"
curl -fsSL "$DISKO_LAYOUT_URL" -o "$TEMP_DISKO_LAYOUT"

(
  cd "$TEMP_DEPLOYMENT"
  # Show the template tool's file list only when it fails.
  if ! INIT_OUTPUT="$(nix --extra-experimental-features "nix-command flakes" \
    flake init -t "${UPSTREAM_REF}#site" 2>&1)"; then
    printf '%s\n' "$INIT_OUTPUT" >&2
    ui_error "Could not create the lab configuration from the template." \
      "Check the Internet connection."
    exit 1
  fi
  sed -i \
    's|nixorium\.url = "github:giovantenne/nixorium/[^"]*";|nixorium.url = "'"${DECLARED_UPSTREAM_REF}"'";|' \
    flake.nix
  if ! grep -Fxq "  inputs.nixorium.url = \"${DECLARED_UPSTREAM_REF}\";" flake.nix; then
    echo "Error: could not configure the generated deployment for ${DECLARED_UPSTREAM_REF}." >&2
    exit 1
  fi
  if [[ "$BOOTSTRAP_VERSION" == "1" || "$BOOTSTRAP_VERSION" == "2" ]]; then
    configure_bootstrap_settings lab-settings.json
  fi
  if [[ "$BOOTSTRAP_VERSION" == "2" ]]; then
    PROFILE_HELPER="scripts/configure-software-profile.sh"
    if [[ ! -r "$PROFILE_HELPER" ]]; then
      echo "Error: the selected revision advertises software profiles but its template helper is missing." >&2
      exit 1
    fi
    # shellcheck source=/dev/null
    source "$PROFILE_HELPER"
    ui_step 4 "Applications"
    configure_site_software_profile software-presets.json lab-software.json "$BOOTSTRAP_INPUT_FD"
  fi
  "${GIT_COMMAND[@]}" init -q -b master
  "${GIT_COMMAND[@]}" add .
)

if [[ -n "$BOOTSTRAP_INPUT_FD" ]]; then
  exec {BOOTSTRAP_INPUT_FD}<&-
  BOOTSTRAP_INPUT_FD=""
fi

MASTER_HOST_NUMBER="$(
  sed -n 's/^[[:space:]]*"masterHostNumber":[[:space:]]*\([0-9][0-9]*\),*$/\1/p' \
    "$TEMP_DEPLOYMENT/lab-settings.json"
)"
STUDENT_USER="$(
  sed -n 's/^[[:space:]]*"studentUser":[[:space:]]*"\([a-z_][a-z0-9_-]*\)",*$/\1/p' \
    "$TEMP_DEPLOYMENT/lab-settings.json"
)"
if [[ ! "$MASTER_HOST_NUMBER" =~ ^[0-9]+$ || ! "$STUDENT_USER" =~ ^[a-z_][a-z0-9_-]{0,30}$ ]]; then
  echo "Error: the generated deployment has invalid controller or student identity." >&2
  exit 1
fi

if [[ -n "$INSTALL_DISK" ]]; then
  INSTALLER_ARGS+=("$INSTALL_DISK")
fi

SETUP_STAGE="install"
FLAKE_REF="path:${TEMP_DEPLOYMENT}" \
  NIXORIUM_DEPLOYMENT_PATH="$TEMP_DEPLOYMENT" \
  NIXORIUM_UPSTREAM_REF="$UPSTREAM_REF" \
  NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  DISKO_LAYOUT_FILE="$TEMP_DISKO_LAYOUT" \
  DISKO_LAYOUT_URL="$DISKO_LAYOUT_URL" \
  MASTER_HOST_NUMBER="$MASTER_HOST_NUMBER" \
  STUDENT_USER="$STUDENT_USER" \
  NIXORIUM_DISK_STEP_TITLE="Step 5 of 5 · Disk" \
  bash "$TEMP_INSTALLER" "${INSTALLER_ARGS[@]}"
SETUP_STAGE="save"
ui_log "Saving the lab configuration on the new disk..."

(
  cd "$TEMP_DEPLOYMENT"
  "${GIT_COMMAND[@]}" add .
  "${GIT_COMMAND[@]}" \
    -c user.name="Nixorium Installer" \
    -c user.email="installer@nixorium.local" \
    commit -q -m "chore: initialize lab deployment"
)

ADMIN_HOME="${TARGET_ROOT}/home/${ADMIN_USER}"
DEPLOYMENT_TARGET="${ADMIN_HOME}/${DEPLOYMENT_NAME}"

if [[ ! -d "$ADMIN_HOME" ]]; then
  echo "Error: the installed admin home was not found at ${ADMIN_HOME}." >&2
  echo "The controller is installed, but the deployment repository must be recreated after reboot." >&2
  exit 1
fi

if sudo test -e "$DEPLOYMENT_TARGET"; then
  echo "Error: deployment target already exists: ${DEPLOYMENT_TARGET}" >&2
  exit 1
fi

ADMIN_OWNER="$(sudo stat -c '%u:%g' "$ADMIN_HOME")"
sudo install -d -m 0700 "$DEPLOYMENT_TARGET"
sudo cp -a "${TEMP_DEPLOYMENT}/." "${DEPLOYMENT_TARGET}/"
sudo chown -R "$ADMIN_OWNER" "$DEPLOYMENT_TARGET"

ui_section "All done"
ui_success "The controller is installed."
echo
ui_note "${UI_BOLD}Next${UI_RESET}"
ui_note "  1. Remove the USB stick, then type ${UI_BOLD}reboot${UI_RESET} and press Enter."
ui_note "  2. Sign in as ${UI_BOLD}admin${UI_RESET} with the administrator password."
ui_note "  3. Open ${UI_BOLD}Nixorium${UI_RESET} from the app grid, or type nixorium in a terminal."
ui_note "     To add the student computers, choose Installation > Network boot (PXE)."
echo
ui_note "${UI_MUTED}The lab configuration is saved in ~/${DEPLOYMENT_NAME}.${UI_RESET}"
echo
