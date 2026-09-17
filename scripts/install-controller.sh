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

# Force the bootstrap install to use the official NixOS cache only.
# This avoids inheriting substituters from a preconfigured live/netboot
# environment, which may point at an unavailable or unsigned local cache.
export NIX_CONFIG=$'experimental-features = nix-command flakes\nsubstituters = https://cache.nixos.org/\ntrusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQX27P3FJrRo=\nmax-jobs = 1\ncores = 1'

prompt_input() {
  local PROMPT_TEXT="$1"
  local TARGET_VAR="$2"
  if [[ -r "$INSTALLER_TTY" ]]; then
    read -r -p "$PROMPT_TEXT" "$TARGET_VAR" < "$INSTALLER_TTY"
  else
    read -r -p "$PROMPT_TEXT" "$TARGET_VAR"
  fi
}

list_disks() {
  local DISK
  for DISK in "${AVAILABLE_DISKS[@]}"; do
    lsblk -dn -o PATH,SIZE,MODEL "$DISK" | sed 's/^/  /'
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
if [ -d /sys/firmware/efi ]; then
  echo "Detected UEFI boot"
else
  echo "Error: BIOS/Legacy boot is not supported. Enable UEFI in firmware settings." >&2
  exit 1
fi

while IFS= read -r CANDIDATE_DISK; do
  if ! disk_has_mounted_filesystem "$CANDIDATE_DISK"; then
    AVAILABLE_DISKS+=("$CANDIDATE_DISK")
  fi
done < <(lsblk -dn -o PATH,TYPE -P | sed -n 's/^PATH="\([^"]*\)" TYPE="disk"$/\1/p')

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

echo "Selected disk: $INSTALL_DISK"
prompt_input "This will erase all data on $INSTALL_DISK. Type YES to continue: " CONFIRMATION
if [[ "$CONFIRMATION" != "YES" ]]; then
  echo "Installation cancelled; the disk was not changed."
  exit 1
fi

echo "Downloading the pinned partitioning tool. The disk remains unchanged until it is ready..."
echo "Partitioning disk with the Disko revision pinned by the deployment..."
sudo env NIX_CONFIG="$NIX_CONFIG" \
  nix --extra-experimental-features "nix-command flakes" \
  run "${UPSTREAM_REF}#disko" -- --mode disko "$TEMP_DISKO_FILE"

BOOTSTRAP_CACHE="${TARGET_ROOT}/var/cache/nixorium-bootstrap"
sudo install -d -o "$(id -u)" -g "$(id -g)" "$BOOTSTRAP_CACHE"
BOOTSTRAP_SWAP="${TARGET_ROOT}/.nixorium-bootstrap.swap"
if command -v btrfs >/dev/null 2>&1 && \
  sudo btrfs filesystem mkswapfile --size 4G "$BOOTSTRAP_SWAP" && \
  sudo swapon "$BOOTSTRAP_SWAP"; then
  echo "Enabled temporary target-disk swap for the installation."
else
  sudo rm -f -- "$BOOTSTRAP_SWAP" >/dev/null 2>&1 || true
  BOOTSTRAP_SWAP=""
  echo "Warning: temporary installation swap is unavailable; continuing with bounded Nix jobs." >&2
fi
export XDG_CACHE_HOME="$BOOTSTRAP_CACHE"
echo "Locking the private deployment on the installed disk..."
(
  cd "$DEPLOYMENT_PATH"
  nix --extra-experimental-features "nix-command flakes" \
    flake lock --override-input nixorium "$UPSTREAM_REF"
)
if [[ ! -f "${DEPLOYMENT_PATH}/flake.lock" ]]; then
  echo "Error: the private deployment lock was not created." >&2
  exit 1
fi

echo "Installing NixOS for the controller..."
echo "The controller system is downloaded into the installed disk, not the live ISO memory."
sudo env \
  NIX_CONFIG="$NIX_CONFIG" \
  XDG_CACHE_HOME="$BOOTSTRAP_CACHE" \
  nixos-install \
  --root "$TARGET_ROOT" \
  --flake "${FLAKE_REF}#pc${MASTER_HOST_NUMBER}" \
  --no-write-lock-file \
  --no-root-passwd

echo "Controller system installed. Returning to bootstrap to save the deployment."
