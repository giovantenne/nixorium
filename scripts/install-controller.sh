#!/usr/bin/env bash
set -euo pipefail

if [[ $# -gt 1 ]]; then
  echo "Usage: ./install-controller.sh [disk]" >&2
  echo "Example: ./install-controller.sh /dev/sdb" >&2
  exit 1
fi

INSTALL_DISK="${1:-}"
FLAKE_REF="${FLAKE_REF:-github:giovantenne/nixorium}"
UPSTREAM_REF="${NIXORIUM_UPSTREAM_REF:-}"
DEPLOYMENT_PATH="${NIXORIUM_DEPLOYMENT_PATH:-}"
TARGET_ROOT="${NIXORIUM_TARGET_ROOT:-/mnt}"
DISKO_LAYOUT_FILE="${DISKO_LAYOUT_FILE:-}"
DISKO_LAYOUT_URL="${DISKO_LAYOUT_URL:-}"
MASTER_HOST_NUMBER="${MASTER_HOST_NUMBER:-99}"
STUDENT_USER="${STUDENT_USER:-student}"
INSTALLER_TTY="${NIXORIUM_INSTALLER_TTY:-/dev/tty}"
AVAILABLE_DISKS=()
BOOTSTRAP_SWAP=""
DISK_STEP_TITLE="${NIXORIUM_DISK_STEP_TITLE:-Choose the disk}"
DISK_CHANGE_STARTED=0
CURRENT_STEP=""
UI_RULE="--------------------------------------------------------"
UI_RESET=""
UI_BOLD=""
UI_TITLE=""
UI_FOCUS=""
UI_SUCCESS=""
UI_WARNING=""
UI_ERROR=""
UI_MUTED=""

# Same presentation as install.sh; plain text when redirected.
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
  printf '  %s!%s %s\n' "$UI_WARNING" "$UI_RESET" "$1" >&2
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

# Force the bootstrap install to use the official NixOS cache only.
# This avoids inheriting substituters from a preconfigured live/netboot
# environment, which may point at an unavailable or unsigned local cache.
export NIX_CONFIG=$'experimental-features = nix-command flakes\nsubstituters = https://cache.nixos.org/\ntrusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=\nmax-jobs = 1\ncores = 1'

INSTALLER_INPUT_FD=""

# Open the terminal once, so consecutive answers are read in order.
prompt_input() {
  local PROMPT_TEXT="$1"
  local TARGET_VAR="$2"
  if [[ -z "$INSTALLER_INPUT_FD" ]]; then
    if [[ -r "$INSTALLER_TTY" ]]; then
      exec {INSTALLER_INPUT_FD}< "$INSTALLER_TTY"
    else
      INSTALLER_INPUT_FD=0
    fi
  fi
  # shellcheck disable=SC2229 # The answer goes to the variable named by TARGET_VAR.
  read -r -u "$INSTALLER_INPUT_FD" -p "$PROMPT_TEXT" "$TARGET_VAR"
}

disk_description() {
  { lsblk -dn -o SIZE,MODEL "$1" 2>/dev/null || true; } | sed 's/[[:space:]]\{1,\}/ /g; s/^ //; s/ $//'
}

list_disks() {
  local INDEX
  for INDEX in "${!AVAILABLE_DISKS[@]}"; do
    printf '  %s%d)%s %-16s %s\n' "$UI_FOCUS" "$((INDEX + 1))" "$UI_RESET" \
      "${AVAILABLE_DISKS[$INDEX]}" "$(disk_description "${AVAILABLE_DISKS[$INDEX]}")"
  done
}

canonicalize_disk() {
  local RAW_DISK="$1"
  if [[ -z "$RAW_DISK" ]]; then
    echo ""
  elif [[ "$RAW_DISK" == /dev/* ]]; then
    echo "$RAW_DISK"
  else
    echo "/dev/$RAW_DISK"
  fi
}

is_available_disk() {
  local CANDIDATE="$1"
  local DISK
  for DISK in "${AVAILABLE_DISKS[@]}"; do
    if [[ "$DISK" == "$CANDIDATE" ]]; then
      return 0
    fi
  done
  return 1
}

disk_has_mounted_filesystem() {
  local CANDIDATE="$1"
  lsblk -nrpo MOUNTPOINT "$CANDIDATE" | awk 'NF { found = 1 } END { exit(found ? 0 : 1) }'
}

TEMP_DISKO_LAYOUT=$(mktemp)
TEMP_DISKO_FILE=$(mktemp)
cleanup() {
  local STATUS=$?
  if (( STATUS != 0 && DISK_CHANGE_STARTED == 1 )); then
    ui_error "Installation stopped while ${CURRENT_STEP}." \
      "The disk was partly written, so this computer cannot start from it yet." \
      "Read the message above (often an Internet problem), then run the setup" \
      "command again: it starts over and erases the disk again."
  fi
  if [[ -n "$BOOTSTRAP_SWAP" ]]; then
    sudo swapoff "$BOOTSTRAP_SWAP" >/dev/null 2>&1 || true
    sudo rm -f -- "$BOOTSTRAP_SWAP" >/dev/null 2>&1 || true
  fi
  rm -f "$TEMP_DISKO_LAYOUT" "$TEMP_DISKO_FILE"
}
trap cleanup EXIT

if [[ -n "$DISKO_LAYOUT_FILE" ]]; then
  if [[ ! -f "$DISKO_LAYOUT_FILE" || ! -r "$DISKO_LAYOUT_FILE" ]]; then
    echo "Error: DISKO_LAYOUT_FILE must name a readable regular file." >&2
    exit 1
  fi
  cp -- "$DISKO_LAYOUT_FILE" "$TEMP_DISKO_LAYOUT"
elif [[ -n "$DISKO_LAYOUT_URL" ]]; then
  echo "Downloading explicitly configured Disko layout..."
  curl -fsSL "$DISKO_LAYOUT_URL" -o "$TEMP_DISKO_LAYOUT"
elif [[ "$FLAKE_REF" =~ ^github:([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/([0-9a-f]{40})$ ]]; then
  DISKO_LAYOUT_URL="https://raw.githubusercontent.com/${BASH_REMATCH[1]}/${BASH_REMATCH[2]}/${BASH_REMATCH[3]}/lib/disko-layout.nix"
  echo "Downloading Disko layout from the pinned flake revision..."
  curl -fsSL "$DISKO_LAYOUT_URL" -o "$TEMP_DISKO_LAYOUT"
else
  echo "Error: provide DISKO_LAYOUT_FILE or use a full revision-pinned GitHub FLAKE_REF." >&2
  echo "An independently moving disk layout is not safe for controller installation." >&2
  exit 1
fi

# Detect UEFI
if [ ! -d "${NIXORIUM_INSTALLER_EFI_DIRECTORY:-/sys/firmware/efi}" ]; then
  ui_error "BIOS/Legacy boot is not supported; Nixorium needs UEFI." \
    "Enable UEFI boot in the firmware settings and start again from the USB stick."
  exit 1
fi

while IFS= read -r CANDIDATE_DISK; do
  if ! disk_has_mounted_filesystem "$CANDIDATE_DISK"; then
    AVAILABLE_DISKS+=("$CANDIDATE_DISK")
  fi
done < <(lsblk -dn -o PATH,TYPE -P | sed -n 's/^PATH="\([^"]*\)" TYPE="disk"$/\1/p')

if [[ ${#AVAILABLE_DISKS[@]} -eq 0 ]]; then
  ui_error "No disk is available for the installation." \
    "Disks in use are not offered. Connect a disk, then run the setup command again."
  exit 1
fi

ui_section "$DISK_STEP_TITLE"
if [[ -n "$INSTALL_DISK" ]]; then
  INSTALL_DISK=$(canonicalize_disk "$INSTALL_DISK")
  if ! is_available_disk "$INSTALL_DISK"; then
    ui_error "Disk '$INSTALL_DISK' is not available on this computer." "Available disks:"
    list_disks >&2
    exit 1
  fi
elif [[ ${#AVAILABLE_DISKS[@]} -eq 1 ]]; then
  INSTALL_DISK="${AVAILABLE_DISKS[0]}"
  ui_note "This computer has one available disk, so it will be used."
else
  ui_note "Disks on this computer (disks in use, such as the USB stick you"
  ui_note "started from, are not listed):"
  echo
  list_disks
  echo
  # A mistyped choice is asked again; only end of input stops here.
  while true; do
    if ! prompt_input "  > Disk number: " CHOSEN_DISK; then
      ui_error "No disk was chosen." "Nothing on this computer was changed."
      exit 1
    fi
    if [[ "$CHOSEN_DISK" =~ ^[0-9]+$ ]] && (( 10#$CHOSEN_DISK >= 1 && 10#$CHOSEN_DISK <= ${#AVAILABLE_DISKS[@]} )); then
      INSTALL_DISK="${AVAILABLE_DISKS[$((10#$CHOSEN_DISK - 1))]}"
      break
    fi
    INSTALL_DISK=$(canonicalize_disk "$CHOSEN_DISK")
    if [[ -n "$INSTALL_DISK" ]] && is_available_disk "$INSTALL_DISK"; then
      break
    fi
    ui_feedback "'${CHOSEN_DISK}' is not in the list; type a number from 1 to ${#AVAILABLE_DISKS[@]}."
  done
fi

# Generate a standalone Disko config with concrete arguments for the selected
# disk and student user. This avoids patching the text of the NixOS module.
cat > "$TEMP_DISKO_FILE" <<EOF
{
  disko.devices = import ${TEMP_DISKO_LAYOUT} {
    device = "${INSTALL_DISK}";
    studentUser = "${STUDENT_USER}";
  };
}
EOF

if [[ -z "$UPSTREAM_REF" || -z "$DEPLOYMENT_PATH" || ! -d "$DEPLOYMENT_PATH" ]]; then
  echo "Error: the bootstrap did not provide its pinned source and private deployment." >&2
  exit 1
fi

echo
ui_note "${UI_BOLD}Everything on this disk will be permanently deleted:${UI_RESET}"
ui_note "  ${INSTALL_DISK}  $(disk_description "$INSTALL_DISK")"
echo
# The same word as the client installer. A mistyped word is asked again; an
# empty answer or end of input cancels.
while true; do
  if ! prompt_input "  > Type ERASE to install, or press Enter to cancel: " CONFIRMATION || [[ -z "$CONFIRMATION" ]]; then
    ui_note "Installation cancelled; the disk was not changed."
    exit 1
  fi
  [[ "$CONFIRMATION" == "ERASE" ]] && break
  ui_feedback "That was not ERASE (capital letters). Type it again, or press Enter to cancel."
done

ui_section "Installing"
ui_note "Do not switch off this computer until the end. Downloading and"
ui_note "installing can take a while, depending on the Internet connection."
echo
DISK_CHANGE_STARTED=1
CURRENT_STEP="preparing the disk"
ui_log "[1/3] Preparing the disk (the partitioning tool is downloaded first)..."
sudo env NIX_CONFIG="$NIX_CONFIG" \
  nix --extra-experimental-features "nix-command flakes" \
  run "${UPSTREAM_REF}#disko" -- --mode disko "$TEMP_DISKO_FILE"

BOOTSTRAP_CACHE="${TARGET_ROOT}/var/cache/nixorium-bootstrap"
sudo install -d -o "$(id -u)" -g "$(id -g)" "$BOOTSTRAP_CACHE"
BOOTSTRAP_SWAP="${TARGET_ROOT}/.nixorium-bootstrap.swap"
if command -v btrfs >/dev/null 2>&1 && \
  sudo btrfs filesystem mkswapfile --size 4G "$BOOTSTRAP_SWAP" && \
  sudo swapon "$BOOTSTRAP_SWAP"; then
  ui_note "${UI_MUTED}Using temporary swap space on the new disk.${UI_RESET}"
else
  sudo rm -f -- "$BOOTSTRAP_SWAP" >/dev/null 2>&1 || true
  BOOTSTRAP_SWAP=""
  ui_feedback "Temporary swap space is unavailable; the installation continues more slowly."
fi
export XDG_CACHE_HOME="$BOOTSTRAP_CACHE"
CURRENT_STEP="fixing the exact versions of the lab configuration"
ui_log "[2/3] Fixing the exact versions of the lab configuration..."
# Start from the nixpkgs revision this release was validated with, not the
# newest commit of its channel. The lock still declares the channel, so later
# package-base updates move it forward through the reviewed workflow.
PACKAGE_BASE_REVISION="$(nix --extra-experimental-features "nix-command flakes" \
  eval --raw "${UPSTREAM_REF}#lib.packageBase.referenceRevision")"
if [[ ! "$PACKAGE_BASE_REVISION" =~ ^[0-9a-f]{40}$ ]]; then
  echo "Error: the release does not name a validated nixpkgs revision." >&2
  exit 1
fi
(
  cd "$DEPLOYMENT_PATH"
  nix --extra-experimental-features "nix-command flakes" \
    flake lock --override-input nixorium "$UPSTREAM_REF" \
    --override-input nixpkgs "github:NixOS/nixpkgs/${PACKAGE_BASE_REVISION}"
)
if [[ ! -f "${DEPLOYMENT_PATH}/flake.lock" ]]; then
  echo "Error: the private deployment lock was not created." >&2
  exit 1
fi

CURRENT_STEP="downloading and installing the controller system"
ui_log "[3/3] Downloading and installing the controller system. This is the longest step."
ui_note "${UI_MUTED}Files go to the new disk, not into the memory of the USB system.${UI_RESET}"
sudo env \
  NIX_CONFIG="$NIX_CONFIG" \
  XDG_CACHE_HOME="$BOOTSTRAP_CACHE" \
  nixos-install \
  --root "$TARGET_ROOT" \
  --flake "${FLAKE_REF}#pc${MASTER_HOST_NUMBER}" \
  --no-write-lock-file \
  --no-root-passwd

DISK_CHANGE_STARTED=0
ui_success "Controller system installed."
