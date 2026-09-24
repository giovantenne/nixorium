#!/usr/bin/env bash
set -euo pipefail

DEFAULT_RELEASE="v2.0.0-beta.5"
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
UI_RULE="----------------------------------------------------------------------"

# Keep bootstrap downloads independent from any cache configured in the live environment.
export NIX_CONFIG=$'experimental-features = nix-command flakes\nsubstituters = https://cache.nixos.org/\ntrusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY='

usage() {
  echo "Usage: install.sh [--release <tag|master>] [--disk <device>]" >&2
  echo "Example: install.sh --release master --disk /dev/sda" >&2
}

ui_banner() {
  cat <<'EOF'

 _   _ _                 _
| \ | (_)_  _____  _ __(_)_   _ _ __ ___
|  \| | \ \/ / _ \| '__| | | | | '_ ` _ \
| |\  | |>  < (_) | |  | | |_| | | | | | |
|_| \_|_/_/\_\___/|_|  |_|\__,_|_| |_| |_|

             NixOS lab controller bootstrap
EOF
}

ui_section() {
  local TITLE="$1"
  printf '\n%s\n  %s\n%s\n\n' "$UI_RULE" "$TITLE" "$UI_RULE"
}

ui_log() {
  printf '  [....] %s\n' "$1"
}

ui_success() {
  printf '  [ OK ] %s\n' "$1"
}

ui_note() {
  printf '  %s\n' "$1"
}

ui_feedback() {
  printf '  ! %s\n' "$1"
}

prompt_bootstrap_value() {
  local LABEL="$1"
  local DEFAULT_VALUE="$2"
  local TARGET_VAR="$3"
  local READ_VALUE

  printf '  > %s [%s]: ' "$LABEL" "$DEFAULT_VALUE"
  if ! IFS= read -r -u "$BOOTSTRAP_INPUT_FD" READ_VALUE; then
    echo >&2
    echo "Error: controller setup input ended before configuration was complete." >&2
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
      ui_feedback "Use a lowercase Unix username (letters, numbers, '_' or '-')."
      continue
    fi
    if [[ "$VALUE" == "root" || "$VALUE" == "admin" ]]; then
      ui_feedback "That username is reserved by the controller."
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
    echo "Error: the live environment provides neither mkpasswd nor openssl." >&2
    echo "Use the official NixOS Minimal ISO and retry." >&2
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
    printf '  > %s: ' "$LABEL"
    if ! IFS= read -r -s -u "$BOOTSTRAP_INPUT_FD" PASSWORD; then
      echo >&2
      echo "Error: controller setup input ended before configuration was complete." >&2
      return 1
    fi
    echo
    if [[ "$PASSWORD" == "nixos" ]]; then
      ui_feedback "Password must not use the public default."
      continue
    fi
    PASSWORD_LENGTH=${#PASSWORD}
    if (( PASSWORD_LENGTH < 8 )); then
      ui_feedback "Password must contain at least 8 bytes."
      continue
    fi
    printf '  > Confirm %s: ' "${LABEL,,}"
    if ! IFS= read -r -s -u "$BOOTSTRAP_INPUT_FD" CONFIRMATION; then
      echo >&2
      echo "Error: controller setup input ended before configuration was complete." >&2
      return 1
    fi
    echo
    if [[ "$PASSWORD" != "$CONFIRMATION" ]]; then
      ui_feedback "Password confirmation does not match. Try again."
      continue
    fi
    HASH="$(hash_bootstrap_password "$PASSWORD")"
    PASSWORD=""
    CONFIRMATION=""
    if [[ ! "$HASH" =~ ^\$6\$[^$]+\$[^$]+$ ]]; then
      echo "Error: the password tool returned an invalid SHA-512 crypt hash." >&2
      return 1
    fi
    printf -v "$TARGET_VAR" '%s' "$HASH"
    return
  done
}

activate_bootstrap_keyboard() {
  if [[ -n "${DISPLAY:-}" || -n "${WAYLAND_DISPLAY:-}" ]]; then
    echo "Error: the selected keyboard cannot be verified safely from a graphical terminal." >&2
    echo "Boot the official NixOS Minimal ISO, or switch to a Linux text console, then retry." >&2
    return 1
  fi
  if ! command -v loadkeys >/dev/null 2>&1; then
    echo "Error: the live environment does not provide loadkeys." >&2
    echo "Use the official NixOS Minimal ISO and retry." >&2
    return 1
  fi
  if ! sudo loadkeys "$BOOTSTRAP_CONSOLE_KEYMAP"; then
    echo "Error: could not activate console keymap '$BOOTSTRAP_CONSOLE_KEYMAP'." >&2
    echo "No account password has been requested; correct the live console and retry." >&2
    return 1
  fi
  ui_success "Active console keyboard: $BOOTSTRAP_KEYBOARD ($BOOTSTRAP_CONSOLE_KEYMAP)."
  ui_note "All remaining input uses this layout now and after reboot."
}

collect_bootstrap_configuration() {
  local CONFIRMATION

  if [[ ! -r "$BOOTSTRAP_TTY" ]]; then
    echo "Error: controller account and regional setup requires an interactive terminal." >&2
    return 1
  fi
  exec {BOOTSTRAP_INPUT_FD}< "$BOOTSTRAP_TTY"

  ui_section "CONTROLLER SETUP"
  ui_note "Recommended environment: official NixOS Minimal ISO in UEFI mode."
  ui_note "Configure regional settings first, then accounts and passwords."
  ui_note "The administrator account name is fixed as 'admin'."

  ui_section "1 / 4  REGIONAL SETTINGS"
  while true; do
    ui_note "Keyboard choices: us, it, gb, fr, de, es"
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
      *) ui_feedback "Choose one of the listed keyboard layouts." ;;
    esac
  done

  activate_bootstrap_keyboard
  echo
  while true; do
    if ! prompt_bootstrap_value "Time zone" "$BOOTSTRAP_TIME_ZONE" BOOTSTRAP_TIME_ZONE; then
      return 1
    fi
    if [[ "$BOOTSTRAP_TIME_ZONE" =~ ^[A-Za-z0-9_+.-]+(/[A-Za-z0-9_+.-]+)+$ ]] && \
      { [[ -e "/etc/zoneinfo/${BOOTSTRAP_TIME_ZONE}" ]] || \
        [[ -e "/usr/share/zoneinfo/${BOOTSTRAP_TIME_ZONE}" ]]; }; then
      break
    fi
    ui_feedback "Choose an installed IANA time zone such as America/New_York or Europe/Rome."
  done

  ui_section "2 / 4  ACCOUNTS"
  prompt_bootstrap_user "Teacher username" "$BOOTSTRAP_TEACHER_USER" "" BOOTSTRAP_TEACHER_USER
  echo
  prompt_bootstrap_user "Student username" "$BOOTSTRAP_STUDENT_USER" "$BOOTSTRAP_TEACHER_USER" BOOTSTRAP_STUDENT_USER

  ui_section "3 / 4  PASSWORDS"
  ui_note "Each password must contain at least 8 bytes. Input remains hidden."
  prompt_bootstrap_password "Administrator password" BOOTSTRAP_ADMIN_HASH
  echo
  prompt_bootstrap_password "Teacher password" BOOTSTRAP_TEACHER_HASH
  echo
  prompt_bootstrap_password "Student password" BOOTSTRAP_STUDENT_HASH

  ui_section "4 / 4  REVIEW"
  echo "  Administrator: admin"
  echo "  Teacher:       $BOOTSTRAP_TEACHER_USER"
  echo "  Student:       $BOOTSTRAP_STUDENT_USER"
  echo "  Time zone:     $BOOTSTRAP_TIME_ZONE"
  echo "  Keyboard:      $BOOTSTRAP_KEYBOARD"
  echo "  Passwords:     set locally and hidden"
  echo "  Client setup:  available later from Nixorium"
  echo
  while true; do
    printf '  > Continue with these settings? [Y/n]: '
    if ! IFS= read -r -u "$BOOTSTRAP_INPUT_FD" CONFIRMATION; then
      echo >&2
      echo "Error: controller setup input ended before configuration was complete." >&2
      return 1
    fi
    case "${CONFIRMATION,,}" in
      ""|y|yes)
        if [[ "$BOOTSTRAP_VERSION" == "1" ]]; then
          exec {BOOTSTRAP_INPUT_FD}<&-
          BOOTSTRAP_INPUT_FD=""
        fi
        return
        ;;
      n|no)
        exec {BOOTSTRAP_INPUT_FD}<&-
        BOOTSTRAP_INPUT_FD=""
        echo "Controller installation cancelled; no settings were changed." >&2
        return 1
        ;;
      *) ui_feedback "Enter y or n." ;;
    esac
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
      '    "consoleKeyMap": '*) printf '    "consoleKeyMap": "%s",\n' "$BOOTSTRAP_CONSOLE_KEYMAP" >> "$OUTPUT_FILE" ;;
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

  printf '\n%s\n  VERSION\n%s\n\n' "$UI_RULE" "$UI_RULE" >&3
  printf '  [....] Fetching published Nixorium releases...\n' >&3
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
    echo "Warning: could not retrieve the GitHub release list." >&3
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
  echo "  Select a Nixorium version:" >&3
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
    printf '  %d) %s (%s)\n' "$((INDEX + 1))" "$TAG" "$LABEL" >&3
  done

  while true; do
    printf '  > Version [%s]: ' "$MENU_DEFAULT" >&3
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

    echo "  ! Invalid selection. Enter a number from 1 to ${#AVAILABLE_RELEASES[@]}, or press Enter for ${MENU_DEFAULT}." >&3
  done

  echo "  [ OK ] Selected ${RELEASE}." >&3
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
ui_section "BOOTSTRAP LOG"
ui_log "Resolving ${RELEASE} to one immutable revision..."
if ! COMMIT_RESPONSE="$(curl -fsSL "$COMMIT_API_URL")"; then
  echo "Error: could not resolve ${RELEASE} to an immutable GitHub revision." >&2
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

ui_log "Checking installer capabilities..."
if ! CAPABILITY_SOURCE="$(curl -fsSL "$CAPABILITY_URL")"; then
  echo "Error: could not inspect the installer at ${UPSTREAM_REV}." >&2
  exit 1
fi
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

ui_section "PREPARATION LOG"
ui_success "Configuration collected."
ui_log "Downloading the pinned installer and deployment sources..."
ui_log "Preparing Nixorium ${RELEASE} at ${UPSTREAM_REV}..."
curl -fsSL "$INSTALLER_URL" -o "$TEMP_INSTALLER"
curl -fsSL "$DISKO_LAYOUT_URL" -o "$TEMP_DISKO_LAYOUT"

(
  cd "$TEMP_DEPLOYMENT"
  nix --extra-experimental-features "nix-command flakes" \
    flake init -t "${UPSTREAM_REF}#site"
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
    ui_section "SOFTWARE PROFILE"
    configure_site_software_profile software-presets.json lab-software.json "$BOOTSTRAP_INPUT_FD"
  fi
  "${GIT_COMMAND[@]}" init -b master
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

ui_section "INSTALLATION LOG"
ui_log "Installing the controller from the generated private deployment..."
ui_note "Wait for the final bootstrap completion message before rebooting."
FLAKE_REF="path:${TEMP_DEPLOYMENT}" \
  NIXORIUM_DEPLOYMENT_PATH="$TEMP_DEPLOYMENT" \
  NIXORIUM_UPSTREAM_REF="$UPSTREAM_REF" \
  NIXORIUM_TARGET_ROOT="$TARGET_ROOT" \
  DISKO_LAYOUT_FILE="$TEMP_DISKO_LAYOUT" \
  DISKO_LAYOUT_URL="$DISKO_LAYOUT_URL" \
  MASTER_HOST_NUMBER="$MASTER_HOST_NUMBER" \
  STUDENT_USER="$STUDENT_USER" \
  bash "$TEMP_INSTALLER" "${INSTALLER_ARGS[@]}"

(
  cd "$TEMP_DEPLOYMENT"
  "${GIT_COMMAND[@]}" add .
  "${GIT_COMMAND[@]}" \
    -c user.name="Nixorium Installer" \
    -c user.email="installer@nixorium.local" \
    commit -m "chore: initialize lab deployment"
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

ui_section "COMPLETE"
ui_success "Installation complete."
ui_note "After reboot, the deployment repository will be available at:"
ui_note "~/${DEPLOYMENT_NAME}"
ui_note "Reboot with: reboot"
