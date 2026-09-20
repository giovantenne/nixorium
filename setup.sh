#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
DISKO_INSTALL_SCRIPT="${REPO_ROOT}/disko-install"
PUBLIC_KEY_FILE="${REPO_ROOT}/public-key"
MUTATION_STARTED=0
CURRENT_STEP="preflight"
MINIMUM_DISK_BYTES=8589934592
INSTALL_HEADROOM_BYTES=2147483648

# shellcheck source=/home/admin/nixorium/scripts/lib/lab-meta.sh
source "${REPO_ROOT}/scripts/lib/lab-meta.sh"

usage() {
  echo "Usage: ./setup.sh [pc-number-or-name] [disk]" >&2
  echo "Run without arguments for guided client enrollment." >&2
  echo "Example: ./setup.sh pc05 /dev/sdb" >&2
}

prompt_input() {
  local PROMPT_TEXT="$1"
  local TARGET_VAR="$2"
  if [[ -r /dev/tty ]]; then
    read -r -p "$PROMPT_TEXT" "$TARGET_VAR" < /dev/tty
  else
    read -r -p "$PROMPT_TEXT" "$TARGET_VAR"
  fi
}

read_optional_file() {
  local PATH_NAME="$1"
  if [[ -r "$PATH_NAME" ]]; then
    tr -d '\n' < "$PATH_NAME"
  else
    printf '%s' "unknown"
  fi
}

require_uefi() {
  if [[ ! -d /sys/firmware/efi ]]; then
    echo "Error: BIOS/Legacy boot is not supported. Enable UEFI in firmware settings." >&2
    return 1
  fi
}

valid_ipv4() {
  local ADDRESS="$1"
  local OCTET
  local -a OCTETS
  [[ "$ADDRESS" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || return 1
  IFS=. read -r -a OCTETS <<< "$ADDRESS"
  for OCTET in "${OCTETS[@]}"; do
    ((10#$OCTET <= 255)) || return 1
  done
}

apply_runtime_controller_address() {
  local CMDLINE_FILE="${1:-/proc/cmdline}"
  local TOKEN CANDIDATE=""
  [[ -r "$CMDLINE_FILE" ]] || return 0
  while IFS= read -r TOKEN; do
    case "$TOKEN" in
      nixorium.controller-dhcp-ip=*)
        TOKEN="${TOKEN#nixorium.controller-dhcp-ip=}"
        valid_ipv4 "$TOKEN" || {
          echo "Error: PXE boot supplied an invalid controller address." >&2
          return 1
        }
        if [[ -n "$CANDIDATE" && "$CANDIDATE" != "$TOKEN" ]]; then
          echo "Error: PXE boot supplied conflicting controller addresses." >&2
          return 1
        fi
        CANDIDATE="$TOKEN"
        ;;
    esac
  done < <(tr ' ' '\n' < "$CMDLINE_FILE")
  if [[ -n "$CANDIDATE" ]]; then
    LAB_CONTROLLER_DHCP_IP="$CANDIDATE"
    export LAB_CONTROLLER_DHCP_IP
  fi
}

display_hardware() {
  local CPU_MODEL MEMORY_KIB MEMORY_MIB INTERFACE MAC STATE
  CPU_MODEL=$(awk -F: '/^model name[[:space:]]*:/ { sub(/^[[:space:]]+/, "", $2); print $2; exit }' /proc/cpuinfo)
  MEMORY_KIB=$(awk '/^MemTotal:/ { print $2; exit }' /proc/meminfo)
  MEMORY_MIB=$((MEMORY_KIB / 1024))

  echo "Detected client hardware"
  echo "  Firmware: UEFI"
  echo "  System:   $(read_optional_file /sys/class/dmi/id/sys_vendor) $(read_optional_file /sys/class/dmi/id/product_name)"
  echo "  CPU:      ${CPU_MODEL:-unknown}"
  echo "  Memory:   ${MEMORY_MIB} MiB"
  echo "  Network interfaces:"
  for INTERFACE in /sys/class/net/*; do
    INTERFACE=${INTERFACE##*/}
    [[ "$INTERFACE" == "lo" ]] && continue
    MAC=$(read_optional_file "/sys/class/net/${INTERFACE}/address")
    STATE=$(read_optional_file "/sys/class/net/${INTERFACE}/operstate")
    echo "    ${INTERFACE}  ${MAC}  ${STATE}"
  done
}

collect_available_disks() {
  mapfile -t AVAILABLE_DISKS < <(
    lsblk -bdnpo PATH,TYPE,RO,SIZE | awk -v minimum="$MINIMUM_DISK_BYTES" '
      $2 == "disk" && $3 == "0" && $4 >= minimum { print $1 }
    '
  )
}

list_disks() {
  local INDEX=1 DISK DETAILS
  for DISK in "${AVAILABLE_DISKS[@]}"; do
    DETAILS=$(lsblk -dnpo PATH,SIZE,MODEL,SERIAL,TRAN -- "$DISK")
    echo "  ${INDEX}) ${DETAILS}"
    INDEX=$((INDEX + 1))
  done
}

canonicalize_disk() {
  local RAW_DISK="$1"
  if [[ "$RAW_DISK" != /dev/* ]]; then
    RAW_DISK="/dev/${RAW_DISK}"
  fi
  readlink -m -- "$RAW_DISK"
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

disk_identity() {
  lsblk -dnro MAJ:MIN -- "$1"
}

disk_size() {
  lsblk -bdnro SIZE -- "$1"
}

format_bytes() {
  numfmt --to=iec-i --suffix=B "$1"
}

list_hosts() {
  printf '%s' "$LAB_CLIENT_HOSTS_JSON" | jq -r '
    to_entries[] | "  \(.key + 1)) \(.value.name)  \(.value.ip)"
  '
}

canonicalize_host_choice() {
  local RAW_CHOICE="$1"
  if [[ "$RAW_CHOICE" =~ ^[0-9]+$ ]]; then
    printf 'pc%02d' "$((10#$RAW_CHOICE))"
  else
    printf '%s' "$RAW_CHOICE"
  fi
}

lookup_host_ip() {
  local HOST_NAME="$1"
  printf '%s' "$LAB_CLIENT_HOSTS_JSON" | jq -er --arg name "$HOST_NAME" '
    [.[] | select(.name == $name)]
    | if length == 1 then .[0].ip else empty end
  '
}

select_host() {
  local REQUESTED_HOST="$1"
  local CHOICE CANDIDATE HOST_IP

  echo "Configured client identities:"
  list_hosts
  while true; do
    if [[ -n "$REQUESTED_HOST" ]]; then
      CHOICE="$REQUESTED_HOST"
      REQUESTED_HOST=""
    else
      prompt_input "Choose client number or hostname: " CHOICE || return 1
    fi
    CANDIDATE=$(canonicalize_host_choice "$CHOICE")
    if HOST_IP=$(lookup_host_ip "$CANDIDATE"); then
      SELECTED_HOST="$CANDIDATE"
      SELECTED_HOST_IP="$HOST_IP"
      return 0
    fi
    echo "Error: '${CHOICE}' is not a configured client identity." >&2
  done
}

probe_host_identity() {
  local HOST_IP="$1"
  if ! command -v ip >/dev/null 2>&1 || ! command -v ping >/dev/null 2>&1; then
    return 2
  fi
  if ! ip route get "$HOST_IP" >/dev/null 2>&1; then
    return 2
  fi
  ping -c 1 -W 1 "$HOST_IP" >/dev/null 2>&1
}

check_duplicate_identity() {
  local RESULT=0
  probe_host_identity "$SELECTED_HOST_IP" || RESULT=$?
  case "$RESULT" in
    0)
      echo "Error: ${SELECTED_HOST} (${SELECTED_HOST_IP}) is already responding on the network." >&2
      echo "Refusing this identity to avoid an accidental duplicate installation." >&2
      return 1
      ;;
    1)
      echo "No response from ${SELECTED_HOST_IP}. This is a best-effort check, not a reservation."
      echo "Coordinate simultaneous installers before continuing."
      ;;
    *)
      echo "The route to ${SELECTED_HOST_IP} is unavailable; duplicate assignment cannot be checked."
      echo "This identity is not reserved. Coordinate simultaneous installers before continuing."
      ;;
  esac
}

select_disk() {
  local REQUESTED_DISK="$1"
  local CHOICE INDEX CANDIDATE

  collect_available_disks
  if [[ ${#AVAILABLE_DISKS[@]} -eq 0 ]]; then
    echo "Error: no writable install disks detected." >&2
    return 1
  fi

  echo "Writable installation disks:"
  list_disks
  if [[ -n "$REQUESTED_DISK" ]]; then
    CHOICE="$REQUESTED_DISK"
  elif [[ ${#AVAILABLE_DISKS[@]} -eq 1 ]]; then
    CHOICE="${AVAILABLE_DISKS[0]}"
    echo "Only one writable disk detected; selected ${CHOICE}."
  else
    prompt_input "Choose disk number or path: " CHOICE || return 1
    if [[ "$CHOICE" =~ ^[0-9]+$ ]]; then
      INDEX=$((10#$CHOICE - 1))
      if (( INDEX < 0 || INDEX >= ${#AVAILABLE_DISKS[@]} )); then
        echo "Error: disk number '${CHOICE}' is out of range." >&2
        return 1
      fi
      CHOICE="${AVAILABLE_DISKS[$INDEX]}"
    fi
  fi

  CANDIDATE=$(canonicalize_disk "$CHOICE")
  if ! [[ "$CANDIDATE" =~ ^/dev/[[:alnum:]_.+-]+$ ]] || ! is_available_disk "$CANDIDATE"; then
    echo "Error: disk '${CANDIDATE}' is not an available writable disk." >&2
    return 1
  fi
  SELECTED_DISK="$CANDIDATE"
  SELECTED_DISK_ID=$(disk_identity "$SELECTED_DISK")
  if [[ -z "$SELECTED_DISK_ID" ]]; then
    echo "Error: could not identify disk '${SELECTED_DISK}'." >&2
    return 1
  fi
  SELECTED_DISK_BYTES=$(disk_size "$SELECTED_DISK")
  if ! [[ "$SELECTED_DISK_BYTES" =~ ^[0-9]+$ ]]; then
    echo "Error: could not determine the capacity of disk '${SELECTED_DISK}'." >&2
    return 1
  fi
  if (( SELECTED_DISK_BYTES < REQUIRED_DISK_BYTES )); then
    echo "Error: disk '${SELECTED_DISK}' is too small for ${SELECTED_HOST}." >&2
    echo "Required: $(format_bytes "$REQUIRED_DISK_BYTES"); available: $(format_bytes "$SELECTED_DISK_BYTES")." >&2
    return 1
  fi
}

revalidate_disk() {
  local CURRENT_ID
  collect_available_disks
  if ! is_available_disk "$SELECTED_DISK"; then
    echo "Error: selected disk '${SELECTED_DISK}' disappeared or is no longer writable." >&2
    return 1
  fi
  CURRENT_ID=$(disk_identity "$SELECTED_DISK")
  if [[ "$CURRENT_ID" != "$SELECTED_DISK_ID" ]]; then
    echo "Error: selected disk '${SELECTED_DISK}' changed after review; refusing to erase it." >&2
    return 1
  fi
}

partition_disk() {
  sudo env NIXORIUM_INSTALL_DISK="${SELECTED_DISK#/dev/}" \
    "$DISKO_INSTALL_SCRIPT" --yes-wipe-all-disks
}

resolve_system() {
  local PATH_INFO REPORTED_PATH
  SELECTED_SYSTEM_PATH=$(
    nix --extra-experimental-features "nix-command flakes" \
      eval "${REPO_ROOT}#nixosConfigurations.${SELECTED_HOST}.config.system.build.toplevel.outPath" \
      --raw \
      --offline \
      --no-write-lock-file
  )
  if ! [[ "$SELECTED_SYSTEM_PATH" =~ ^/nix/store/[0-9a-z]{32}-nixos-system-${SELECTED_HOST}-[^/[:space:]]+$ ]]; then
    echo "Error: evaluation returned an invalid system path for ${SELECTED_HOST}." >&2
    return 1
  fi
  PATH_INFO=$(
    nix --extra-experimental-features "nix-command flakes" \
      path-info --closure-size "$SELECTED_SYSTEM_PATH" \
      --offline
  )
  read -r REPORTED_PATH SYSTEM_CLOSURE_BYTES <<< "$PATH_INFO"
  if [[ "$REPORTED_PATH" != "$SELECTED_SYSTEM_PATH" ]] ||
    ! [[ "$SYSTEM_CLOSURE_BYTES" =~ ^[0-9]+$ ]] ||
    (( SYSTEM_CLOSURE_BYTES == 0 )); then
    echo "Error: the prepared closure for ${SELECTED_HOST} is unavailable or invalid." >&2
    return 1
  fi
  REQUIRED_DISK_BYTES=$((SYSTEM_CLOSURE_BYTES + INSTALL_HEADROOM_BYTES))
}

install_system() {
  sudo nixos-install --system "$SELECTED_SYSTEM_PATH" \
    --option substituters "http://${LAB_CONTROLLER_DHCP_IP}:${LAB_CACHE_PORT}" \
    --option trusted-public-keys "$CACHE_KEY" \
    --option fallback false \
    --no-channel-copy \
    --no-root-passwd
}

verify_installation() {
  sudo test -e /mnt/nix/var/nix/profiles/system
  sync
}

reboot_system() {
  sudo systemctl reboot
}

cleanup() {
  local STATUS=$?
  if [[ "$STATUS" -ne 0 && "$MUTATION_STARTED" -eq 1 ]]; then
    echo "Installation FAILED during ${CURRENT_STEP}." >&2
    echo "The disk may have been modified; review the error above before retrying." >&2
  fi
  exit "$STATUS"
}

main() {
  local REQUESTED_HOST="${1:-}"
  local REQUESTED_DISK="${2:-}"
  local CONFIRMATION EXPECTED_CONFIRMATION REBOOT_CONFIRMATION

  if [[ $# -gt 2 ]]; then
    usage
    return 1
  fi
  if [[ "$REQUESTED_HOST" == "-h" || "$REQUESTED_HOST" == "--help" ]]; then
    usage
    return 0
  fi

  load_lab_meta "$REPO_ROOT" || return 1
  apply_runtime_controller_address || return 1
  require_deployment_ready "$REPO_ROOT" || return 1
  require_uefi || return 1

  if [[ -z "$LAB_CONTROLLER_DHCP_IP" ]]; then
    echo "Error: controller DHCP IP missing from installer metadata." >&2
    return 1
  fi
  if [[ ! -x "$DISKO_INSTALL_SCRIPT" ]]; then
    echo "Error: precompiled Disko installer missing from the installer bundle." >&2
    return 1
  fi
  if [[ ! -f "$PUBLIC_KEY_FILE" ]]; then
    echo "Error: binary-cache public key missing from the installer bundle." >&2
    return 1
  fi
  CACHE_KEY=$(tr -d '\n' < "$PUBLIC_KEY_FILE")
  if [[ -z "$CACHE_KEY" ]]; then
    echo "Error: binary-cache public key is empty." >&2
    return 1
  fi

  echo "Nixorium guided client enrollment"
  echo
  display_hardware
  echo
  collect_available_disks
  if [[ ${#AVAILABLE_DISKS[@]} -eq 0 ]]; then
    echo "Error: no writable install disks detected." >&2
    return 1
  fi
  echo "Detected writable disks:"
  list_disks
  echo

  select_host "$REQUESTED_HOST" || return 1
  echo "Selected identity: ${SELECTED_HOST} (${SELECTED_HOST_IP})"
  check_duplicate_identity || return 1
  resolve_system || return 1
  echo "Prepared system closure: $(format_bytes "$SYSTEM_CLOSURE_BYTES")"
  echo "Minimum target capacity: $(format_bytes "$REQUIRED_DISK_BYTES")"
  echo
  select_disk "$REQUESTED_DISK" || return 1
  echo "Selected target disk: ${SELECTED_DISK} (${SELECTED_DISK_ID})"
  echo
  echo "DESTRUCTIVE REVIEW"
  echo "  Install identity: ${SELECTED_HOST} (${SELECTED_HOST_IP})"
  echo "  Erase disk:       ${SELECTED_DISK}"
  echo "  All data on this disk will be permanently destroyed."
  echo "  No host reservation has been made on the controller."
  EXPECTED_CONFIRMATION="ERASE"
  prompt_input "Type ERASE to continue: " CONFIRMATION || return 1
  if [[ "$CONFIRMATION" != "$EXPECTED_CONFIRMATION" ]]; then
    echo "Installation cancelled; no disk operation was started."
    return 1
  fi

  revalidate_disk || return 1
  trap cleanup EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  MUTATION_STARTED=1
  CURRENT_STEP="disk partitioning"
  echo "[1/3] Partitioning ${SELECTED_DISK}..."
  partition_disk || return 1

  CURRENT_STEP="NixOS installation"
  echo "[2/3] Installing ${SELECTED_HOST} from the controller cache..."
  install_system || return 1

  CURRENT_STEP="installation verification"
  echo "[3/3] Verifying the installed system profile..."
  verify_installation || return 1

  MUTATION_STARTED=0
  trap - EXIT INT TERM

  echo
  echo "SUCCESS: ${SELECTED_HOST} is installed on ${SELECTED_DISK}."
  echo "Remove or deprioritize network boot so the machine starts from disk."
  prompt_input "Type REBOOT to reboot now, or press Enter to remain in the installer: " REBOOT_CONFIRMATION || return 1
  if [[ "$REBOOT_CONFIRMATION" == "REBOOT" ]]; then
    echo "Rebooting into ${SELECTED_HOST}..."
    reboot_system
  else
    echo "Reboot not requested. Run 'systemctl reboot' when ready."
  fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
