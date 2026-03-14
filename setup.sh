#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "Usage: ./setup.sh <pc-number> [disk]" >&2
  echo "Example: ./setup.sh 5 /dev/sdb" >&2
  exit 1
fi

PC_NUMBER="$1"
INSTALL_DISK="${2:-}"
REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
DISKO_FILE="${REPO_ROOT}/disko-uefi.nix"
PUBLIC_KEY_FILE="${REPO_ROOT}/public-key"

# shellcheck source=/home/admin/nixos-lab/scripts/lib/lab-meta.sh
source "${REPO_ROOT}/scripts/lib/lab-meta.sh"
load_lab_meta "${REPO_ROOT}"

PC_COUNT="${LAB_CLIENT_COUNT}"
STUDENT_USER="${LAB_STUDENT_USER}"

if ! [[ "$PC_NUMBER" =~ ^[0-9]+$ ]]; then
  echo "Error: PC number must be numeric." >&2
  exit 1
fi

if [[ "$PC_NUMBER" -lt 1 || "$PC_NUMBER" -gt "$PC_COUNT" ]]; then
  echo "Error: PC number must be between 1 and ${PC_COUNT}." >&2
  exit 1
fi

PC_ID=$(printf "%02d" "$PC_NUMBER")
PC_NAME="pc${PC_ID}"

AVAILABLE_DISKS=()

prompt_input() {
  local PROMPT_TEXT="$1"
  local TARGET_VAR="$2"
  if [[ -r /dev/tty ]]; then
    read -r -p "$PROMPT_TEXT" "$TARGET_VAR" < /dev/tty
  else
    read -r -p "$PROMPT_TEXT" "$TARGET_VAR"
  fi
}

list_disks() {
  lsblk -dn -o PATH,SIZE,TYPE,MODEL -P | awk '
    /TYPE="disk"/ {
      match($0, /PATH="([^"]*)"/, path)
      match($0, /SIZE="([^"]*)"/, size)
      match($0, /MODEL="([^"]*)"/, model)
      modelValue = model[1]
      if (modelValue == "") {
        modelValue = "-"
      }
      printf "  %s  %s  %s\n", path[1], size[1], modelValue
    }
  '
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

mapfile -t AVAILABLE_DISKS < <(
  lsblk -dn -o PATH,TYPE -P | sed -n 's/^PATH="\([^"]*\)" TYPE="disk"$/\1/p'
)

if [[ ${#AVAILABLE_DISKS[@]} -eq 0 ]]; then
  echo "Error: no installable disks detected." >&2
  exit 1
fi

if [[ -n "$INSTALL_DISK" ]]; then
  INSTALL_DISK=$(canonicalize_disk "$INSTALL_DISK")
  if ! is_available_disk "$INSTALL_DISK"; then
    echo "Error: disk '$INSTALL_DISK' is not available on this machine." >&2
    echo "Available disks:"
    list_disks
    exit 1
  fi
elif [[ ${#AVAILABLE_DISKS[@]} -eq 1 ]]; then
  INSTALL_DISK="${AVAILABLE_DISKS[0]}"
  echo "Only one disk detected, selecting: $INSTALL_DISK"
else
  echo "Available disks:"
  list_disks
  prompt_input "Choose install disk: " CHOSEN_DISK
  INSTALL_DISK=$(canonicalize_disk "$CHOSEN_DISK")
  if [[ -z "$INSTALL_DISK" ]]; then
    echo "Error: no disk selected." >&2
    exit 1
  fi
  if ! is_available_disk "$INSTALL_DISK"; then
    echo "Error: disk '$INSTALL_DISK' is not available on this machine." >&2
    exit 1
  fi
fi

echo "Selected disk: $INSTALL_DISK"
prompt_input "This will erase all data on $INSTALL_DISK. Type YES to continue: " CONFIRMATION
if [[ "$CONFIRMATION" != "YES" ]]; then
  echo "Installation cancelled."
  exit 1
fi

# Extract settings from lab metadata and the generated public key file
MASTER_IP="${LAB_CONTROLLER_DHCP_IP}"
CACHE_PORT="${LAB_CACHE_PORT}"
CACHE_KEY=""

if [[ -f "$PUBLIC_KEY_FILE" ]]; then
  CACHE_KEY=$(tr -d '\n' < "$PUBLIC_KEY_FILE")
fi

if [[ -z "$MASTER_IP" ]]; then
  echo "Error: controller DHCP IP missing from labMeta." >&2
  exit 1
fi

if [[ -z "$CACHE_KEY" ]]; then
  echo "Error: public-key missing in ${PUBLIC_KEY_FILE}." >&2
  echo "Generate it on the controller with: nix key convert-secret-to-public < secret-key > public-key" >&2
  echo "Then run: git add public-key" >&2
  echo "Then rebuild the netboot artifacts before booting clients again." >&2
  exit 1
fi

if [[ -z "$CACHE_PORT" ]]; then
  CACHE_PORT=5000
fi

echo "Using binary cache at ${MASTER_IP}:${CACHE_PORT}"

# Detect UEFI
if [ -d /sys/firmware/efi ]; then
  echo "Detected UEFI boot"
else
  echo "Error: BIOS/Legacy boot is not supported. Enable UEFI in firmware settings." >&2
  exit 1
fi

TEMP_DISKO_FILE=$(mktemp)
trap 'rm -f "$TEMP_DISKO_FILE"' EXIT
# Replace disk device and resolve labSettings.studentUser for standalone disko use.
# disko-uefi.nix is a NixOS module that normally receives labSettings via specialArgs,
# but disko CLI evaluates it standalone without that context.
sed -E \
  -e "s#device = \"[^\"]+\";#device = \"${INSTALL_DISK}\";#" \
  -e 's/\{ labSettings, \.\.\. \}:/{ ... }:/' \
  -e "s/\\$\\{labSettings\\.studentUser\\}/${STUDENT_USER}/g" \
  "$DISKO_FILE" > "$TEMP_DISKO_FILE"

echo "Partitioning disk..."
sudo disko --mode disko "$TEMP_DISKO_FILE"

echo "Installing NixOS for ${PC_NAME}..."
sudo nixos-install --flake "${REPO_ROOT}#${PC_NAME}" \
  --option substituters "http://${MASTER_IP}:${CACHE_PORT}" \
  --option trusted-public-keys "${CACHE_KEY}" \
  --no-channel-copy \
  --no-root-passwd
