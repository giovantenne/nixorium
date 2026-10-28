#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
DISKO_INSTALL_SCRIPT="${REPO_ROOT}/disko-install"
PUBLIC_KEY_FILE="${REPO_ROOT}/public-key"
MUTATION_STARTED=0
CURRENT_STEP="preflight"
MINIMUM_DISK_BYTES=8589934592
INSTALL_HEADROOM_BYTES=2147483648
NETWORK_INTERFACES_DIR="/sys/class/net"

INSTALL_LOG="${NIXORIUM_INSTALL_LOG:-/tmp/nixorium-install.log}"

# shellcheck source=/home/admin/nixorium/scripts/lib/ui.sh
source "${REPO_ROOT}/scripts/lib/ui.sh"
# shellcheck source=/home/admin/nixorium/scripts/lib/lab-meta.sh
source "${REPO_ROOT}/scripts/lib/lab-meta.sh"
# shellcheck source=/home/admin/nixorium/scripts/lib/client-installer.sh
source "${REPO_ROOT}/scripts/lib/client-installer.sh"

usage() {
  echo "Usage: ./setup.sh [pc-number-or-name] [disk]" >&2
  echo "Run without arguments for guided client enrollment." >&2
  echo "Example: ./setup.sh pc05 /dev/sdb" >&2
}

# Reads one answer. The visible label comes from ui_prompt (see ask).
prompt_input() {
  local PROMPT_TEXT="$1"
  local TARGET_VAR="$2"
  # TARGET_VAR names the variable to fill; "${TARGET_VAR?}" marks that on purpose.
  if [[ -r /dev/tty ]]; then
    read -r -p "$PROMPT_TEXT" "${TARGET_VAR?}" < /dev/tty
  else
    read -r -p "$PROMPT_TEXT" "${TARGET_VAR?}"
  fi
}

ask() {
  local LABEL="$1" DEFAULT_VALUE="$2" TARGET_VAR="$3"
  ui_prompt "$LABEL" "$DEFAULT_VALUE"
  prompt_input "" "$TARGET_VAR"
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
    ui_error "BIOS/Legacy boot is not supported. Enable UEFI in firmware settings."
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
          ui_error "PXE boot supplied an invalid controller address."
          return 1
        }
        if [[ -n "$CANDIDATE" && "$CANDIDATE" != "$TOKEN" ]]; then
          ui_error "PXE boot supplied conflicting controller addresses."
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

  ui_detail "Firmware" "UEFI"
  ui_detail "System" "$(read_optional_file /sys/class/dmi/id/sys_vendor) $(read_optional_file /sys/class/dmi/id/product_name)"
  ui_detail "CPU" "${CPU_MODEL:-unknown}"
  ui_detail "Memory" "${MEMORY_MIB} MiB"
  for INTERFACE in /sys/class/net/*; do
    INTERFACE=${INTERFACE##*/}
    [[ "$INTERFACE" == "lo" ]] && continue
    MAC=$(read_optional_file "/sys/class/net/${INTERFACE}/address")
    STATE=$(read_optional_file "/sys/class/net/${INTERFACE}/operstate")
    ui_detail "Network" "${INTERFACE}  ${MAC}  ${STATE}"
  done
}

collect_available_disks() {
  mapfile -t AVAILABLE_DISKS < <(nixorium_collect_available_disks "$MINIMUM_DISK_BYTES")
}

list_disks() {
  local INDEX=1 DISK DETAILS
  for DISK in "${AVAILABLE_DISKS[@]}"; do
    DETAILS=$(lsblk -dnpo PATH,MODEL,SERIAL,TRAN -- "$DISK" | sed 's/[[:space:]]\{1,\}/ /g')
    printf '    %s%d)%s %s, %s\n' "$UI_FOCUS" "$INDEX" "$UI_RESET" "$(format_bytes "$(disk_size "$DISK")")" "$DETAILS"
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
  nixorium_disk_identity_json "$1"
}

disk_size() {
  lsblk -bdnro SIZE -- "$1"
}

format_bytes() {
  # Decimal units (GB), the way disks are sold and shown on screen.
  numfmt --to=si --suffix=B "$1" | sed -E 's/([0-9.])([KMGTPE]?B)$/\1 \2/'

}

# One readable line for the review; the JSON stays the exact identity that
# revalidate_disk compares.
describe_disk_identity() {
  local size details
  if ! size=$(printf '%s' "$1" | jq -er '.sizeBytes | numbers' 2>/dev/null) \
      || ! details=$(printf '%s' "$1" | jq -r '
        def clean: gsub("\\\\x20"; " ") | gsub("^ +| +$"; "");
        [ (if .model != "" then (.model | clean) else empty end),
          (if .serial != "" then "serial \(.serial)" else empty end),
          (if .transport != "" then .transport else empty end)
        ] | join(", ")' 2>/dev/null); then
    printf '%s\n' "$1"
    return 0
  fi
  # The same size unit as the disk list above it.
  printf '%s, %s%s\n' "$(printf '%s' "$1" | jq -r '.path')" "$(format_bytes "$size")" "${details:+, $details}"
}

list_hosts() {
  printf '%s' "$LAB_CLIENT_HOSTS_JSON" | jq -r '
    to_entries[] | "    \(.key + 1)) \(.value.name)  \(.value.ip)"
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

  ui_note "Configured computers:"
  list_hosts
  echo
  while true; do
    if [[ -n "$REQUESTED_HOST" ]]; then
      CHOICE="$REQUESTED_HOST"
      REQUESTED_HOST=""
    else
      ask "Choose client number or hostname" "" CHOICE || return 1
    fi
    CANDIDATE=$(canonicalize_host_choice "$CHOICE")
    if HOST_IP=$(lookup_host_ip "$CANDIDATE"); then
      SELECTED_HOST="$CANDIDATE"
      SELECTED_HOST_IP="$HOST_IP"
      return 0
    fi
    ui_feedback "'${CHOICE}' is not a configured client identity."
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
      ui_error "${SELECTED_HOST} (${SELECTED_HOST_IP}) is already responding on the network." \
        "Refusing this identity to avoid an accidental duplicate installation."
      return 1
      ;;
    1)
      ui_success "No other computer answers as ${SELECTED_HOST} (${SELECTED_HOST_IP})."
      ui_note "       If someone is installing another computer right now, make sure it is not also ${SELECTED_HOST}."
      ;;
    *)
      ui_feedback "Cannot check whether ${SELECTED_HOST} (${SELECTED_HOST_IP}) is already in use: the network route is unavailable."
      ui_note "  If someone is installing another computer right now, make sure it is not also ${SELECTED_HOST}."
      ;;
  esac
}

# The interface that carries this boot's IPv4 address, when exactly one does.
connected_interface() {
  local NAMES
  NAMES=$(ip -o -4 address show scope global 2>/dev/null | awk '{ print $2 }' | sort -u)
  [[ -n "$NAMES" && $(wc -l <<< "$NAMES") -eq 1 ]] || return 1
  printf '%s\n' "$NAMES"
}

require_client_interface() {
  local CONFIGURED_INTERFACE INTERFACE EXPLICIT
  # Without an explicit client interface name the installed system configures
  # its wired NIC by type, so there is no name to verify (ADR 0017). Metadata
  # that predates the flag is treated as explicit.
  EXPLICIT=$(jq -er --arg name "$SELECTED_HOST" '
    [.[] | select(.name == $name)]
    | if length == 1 then (.[0] | if has("ifaceExplicit") then .ifaceExplicit else true end | tostring) else empty end
  ' <<< "$LAB_CLIENT_HOSTS_JSON") || {
    ui_error "Installer metadata has no valid network interface for ${SELECTED_HOST}."
    return 1
  }
  if [[ "$EXPLICIT" == false ]]; then
    ui_success "Client network interface: detected automatically (no interface name is configured for ${SELECTED_HOST})"
    return 0
  fi
  CONFIGURED_INTERFACE=$(jq -er --arg name "$SELECTED_HOST" --arg fallback "$LAB_IFACE_NAME" '
    [.[] | select(.name == $name)]
    | if length == 1 then (.[0].ifaceName // $fallback) else empty end
    | select(type == "string" and test("^[A-Za-z0-9_.:-]{1,64}$"))
  ' <<< "$LAB_CLIENT_HOSTS_JSON") || {
    ui_error "Installer metadata has no valid network interface for ${SELECTED_HOST}."
    return 1
  }
  if [[ -e "${NETWORK_INTERFACES_DIR}/${CONFIGURED_INTERFACE}" ]]; then
    ui_success "Configured client network interface: ${CONFIGURED_INTERFACE} (present)"
    return 0
  fi
  ui_error "${SELECTED_HOST} requires network interface '${CONFIGURED_INTERFACE}', which is absent on this computer."
  echo "    Detected wired interfaces:" >&2
  for INTERFACE in "${NETWORK_INTERFACES_DIR}"/*; do
    [[ -e "$INTERFACE" && "${INTERFACE##*/}" != lo && ! -d "$INTERFACE/wireless" ]] || continue
    echo "      ${INTERFACE##*/}" >&2
  done
  CONNECTED_INTERFACE=$(connected_interface || true)
  if [[ -n "$CONNECTED_INTERFACE" ]]; then
    echo "    This computer started from the network through '${CONNECTED_INTERFACE}': that is the name to use." >&2
  fi
  echo "    On the controller, correct this client's interface in Settings > Network (Client computers' network interface, or its hostIfaceNames override), save and prepare installation files again." >&2
  echo "    Installation stopped before erase confirmation; no disk operation was started." >&2
  return 1
}

select_disk() {
  local REQUESTED_DISK="$1"
  local CHOICE INDEX CANDIDATE

  collect_available_disks
  if [[ ${#AVAILABLE_DISKS[@]} -eq 0 ]]; then
    ui_error "No writable install disks detected."
    return 1
  fi

  ui_note "Disks this computer can install on:"
  list_disks
  echo
  if [[ -n "$REQUESTED_DISK" ]]; then
    CHOICE="$REQUESTED_DISK"
  elif [[ ${#AVAILABLE_DISKS[@]} -eq 1 ]]; then
    CHOICE="${AVAILABLE_DISKS[0]}"
    ui_note "Only one disk can be used, so ${CHOICE} is selected."
  else
    # A mistyped choice is asked again; only end of input stops here.
    while true; do
      ask "Choose disk number or path" "" CHOICE || return 1
      if [[ "$CHOICE" =~ ^[0-9]+$ ]]; then
        INDEX=$((10#$CHOICE - 1))
        if (( INDEX < 0 || INDEX >= ${#AVAILABLE_DISKS[@]} )); then
          ui_feedback "Disk number '${CHOICE}' is not in the list; choose again."
          continue
        fi
        CHOICE="${AVAILABLE_DISKS[$INDEX]}"
      fi
      CANDIDATE=$(canonicalize_disk "$CHOICE")
      if [[ "$CANDIDATE" =~ ^/dev/[[:alnum:]_.+-]+$ ]] && is_available_disk "$CANDIDATE"; then
        break
      fi
      ui_feedback "'${CHOICE}' is not an available writable disk; choose again."
    done
  fi

  CANDIDATE=$(canonicalize_disk "$CHOICE")
  if ! [[ "$CANDIDATE" =~ ^/dev/[[:alnum:]_.+-]+$ ]] || ! is_available_disk "$CANDIDATE"; then
    ui_error "Disk '${CANDIDATE}' is not an available writable disk."
    return 1
  fi
  SELECTED_DISK="$CANDIDATE"
  SELECTED_DISK_ID=$(disk_identity "$SELECTED_DISK")
  if [[ -z "$SELECTED_DISK_ID" ]]; then
    ui_error "Could not identify disk '${SELECTED_DISK}'."
    return 1
  fi
  SELECTED_DISK_BYTES=$(disk_size "$SELECTED_DISK")
  if ! [[ "$SELECTED_DISK_BYTES" =~ ^[0-9]+$ ]]; then
    ui_error "Could not determine the capacity of disk '${SELECTED_DISK}'."
    return 1
  fi
  if (( SELECTED_DISK_BYTES < REQUIRED_DISK_BYTES )); then
    ui_error "Disk '${SELECTED_DISK}' is too small for ${SELECTED_HOST}." \
      "Required: $(format_bytes "$REQUIRED_DISK_BYTES"); available: $(format_bytes "$SELECTED_DISK_BYTES")."
    return 1
  fi
}

revalidate_disk() {
  local CURRENT_ID
  collect_available_disks
  if ! is_available_disk "$SELECTED_DISK"; then
    ui_error "Selected disk '${SELECTED_DISK}' disappeared or is no longer writable."
    return 1
  fi
  CURRENT_ID=$(disk_identity "$SELECTED_DISK")
  if [[ "$CURRENT_ID" != "$SELECTED_DISK_ID" ]]; then
    ui_error "Selected disk '${SELECTED_DISK}' changed after review; refusing to erase it."
    return 1
  fi
}

# The tools behind the installation are chatty (shell traces, boot loader
# warnings). The complete output goes to the log; the screen shows the last
# lines of it only when something fails.
show_install_log_tail() {
  local LINE
  printf '\n  %sx%s %s%s%s\n' "$UI_ERROR" "$UI_RESET" "$UI_BOLD" "$1" "$UI_RESET" >&2
  printf '    %sLast lines of the log:%s\n' "$UI_MUTED" "$UI_RESET" >&2
  while IFS= read -r LINE; do
    printf '      %s\n' "$LINE" >&2
  done < <(tail -n 15 -- "$INSTALL_LOG" 2>/dev/null || true)
  printf '    Full log: %s\n' "$INSTALL_LOG" >&2
}

partition_disk() {
  if ! nixorium_run_disko "$DISKO_INSTALL_SCRIPT" "$SELECTED_DISK" >> "$INSTALL_LOG" 2>&1; then
    show_install_log_tail "Could not prepare the disk ${SELECTED_DISK}."
    return 1
  fi
}

resolve_system() {
  local PATH_INFO REPORTED_PATH CACHE_URL CLASS SYSTEM_NAME
  SELECTED_SYSTEM_PATH=$(
    nixorium_flake_eval "${REPO_ROOT}#nixosConfigurations.${SELECTED_HOST}.config.system.build.toplevel.outPath" \
      --raw \
      --offline \
      --no-write-lock-file
  )
  CLASS=$(jq -r --arg name "$SELECTED_HOST" '
    [.[] | select(.name == $name)] | if length == 1 then (.[0].class // "") else error("unknown host") end
  ' <<< "$LAB_CLIENT_HOSTS_JSON") || {
    ui_error "Installer metadata does not describe ${SELECTED_HOST}."
    return 1
  }
  SYSTEM_NAME=$(nixorium_expected_system_name "$SELECTED_HOST" "$CLASS") || return 1
  if ! [[ "$SELECTED_SYSTEM_PATH" =~ ^/nix/store/[0-9a-z]{32}-nixos-system-${SYSTEM_NAME}-[^/[:space:]]+$ ]]; then
    ui_error "Evaluation returned an invalid system path for ${SELECTED_HOST}."
    return 1
  fi
  # Client systems are never in this netboot store: they live in the
  # controller's signed binary cache, so the closure is queried there.
  CACHE_URL="http://${LAB_CONTROLLER_DHCP_IP}:${LAB_CACHE_PORT}"
  if ! nix --extra-experimental-features "nix-command flakes" \
    store info --store "$CACHE_URL" \
    --option connect-timeout 10 >/dev/null 2>&1; then
    ui_error "The controller's binary cache at ${LAB_CONTROLLER_DHCP_IP}:${LAB_CACHE_PORT} is not reachable." \
      "Check the network and that client preparation finished on the controller."
    return 1
  fi
  if ! PATH_INFO=$(
    nix --extra-experimental-features "nix-command flakes" \
      path-info --store "$CACHE_URL" --closure-size "$SELECTED_SYSTEM_PATH" \
      --option connect-timeout 10 \
      --option trusted-public-keys "$CACHE_KEY" \
      --option require-sigs true \
      2>/dev/null
  ); then
    ui_error "The controller has not prepared ${SELECTED_HOST} for this configuration." \
      "On the controller run Installation -> Prepare clients again, then restart this installer."
    return 1
  fi
  read -r REPORTED_PATH SYSTEM_CLOSURE_BYTES <<< "$PATH_INFO"
  if [[ "$REPORTED_PATH" != "$SELECTED_SYSTEM_PATH" ]] ||
    ! [[ "$SYSTEM_CLOSURE_BYTES" =~ ^[0-9]+$ ]] ||
    (( SYSTEM_CLOSURE_BYTES == 0 )); then
    ui_error "The prepared closure for ${SELECTED_HOST} is unavailable or invalid."
    return 1
  fi
  REQUIRED_DISK_BYTES=$((SYSTEM_CLOSURE_BYTES + INSTALL_HEADROOM_BYTES))
}

install_system() {
  # The installation activates the system once; a shared class system reads
  # the computer's number from this record then (ADR 0033).
  nixorium_write_identity_record "$SELECTED_HOST" || return 1
  ui_log "Copying ${SELECTED_HOST}'s system from the controller and installing the boot loader (several minutes)..."
  if ! nixorium_with_udev_in_target /mnt sudo nixos-install --system "$SELECTED_SYSTEM_PATH" \
    --option substituters "http://${LAB_CONTROLLER_DHCP_IP}:${LAB_CACHE_PORT}" \
    --option trusted-public-keys "$CACHE_KEY" \
    --option fallback false \
    --no-channel-copy \
    --no-root-passwd 2>&1 | tee -a "$INSTALL_LOG" | nixorium_copy_progress; then
    show_install_log_tail "The system could not be copied or the boot loader could not be installed."
    return 1
  fi
  ui_success "System copied"
  ui_success "Boot loader installed"
}

verify_installation() {
  nixorium_verify_installed_profile "$SELECTED_SYSTEM_PATH" "$SELECTED_DISK" &&
    nixorium_write_identity_record "$SELECTED_HOST" &&
    HOST_KEY_FINGERPRINT=$(nixorium_prepare_host_key)
}

reboot_system() {
  sudo systemctl reboot
}

cleanup() {
  local STATUS=$?
  if [[ "$STATUS" -ne 0 && "$MUTATION_STARTED" -eq 1 ]]; then
    ui_error "Installation FAILED during ${CURRENT_STEP}." \
      "The disk may have been modified; review the error above before retrying."
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
    ui_error "Controller DHCP IP missing from installer metadata."
    return 1
  fi
  if [[ ! -x "$DISKO_INSTALL_SCRIPT" ]]; then
    ui_error "Precompiled Disko installer missing from the installer bundle."
    return 1
  fi
  if [[ ! -f "$PUBLIC_KEY_FILE" ]]; then
    ui_error "Binary-cache public key missing from the installer bundle."
    return 1
  fi
  CACHE_KEY=$(tr -d '\n' < "$PUBLIC_KEY_FILE")
  if [[ -z "$CACHE_KEY" ]]; then
    ui_error "Binary-cache public key is empty."
    return 1
  fi

  ui_section "Nixorium guided client enrollment"
  ui_note "This installs a student computer from the controller, in three steps:"
  ui_note "  1. the computer and its name      3. the installation"
  ui_note "  2. the disk to install on"
  echo
  ui_note "Nothing on this computer changes until you type ERASE."
  collect_available_disks
  if [[ ${#AVAILABLE_DISKS[@]} -eq 0 ]]; then
    ui_error "No writable install disks detected."
    return 1
  fi

  ui_step 1 "Computer"
  display_hardware
  echo

  select_host "$REQUESTED_HOST" || return 1
  ui_success "This computer will be ${SELECTED_HOST} (${SELECTED_HOST_IP})."
  require_client_interface || return 1
  check_duplicate_identity || return 1
  ui_log "Checking the size of ${SELECTED_HOST}'s system..."
  resolve_system || return 1
  ui_success "System size: $(format_bytes "$SYSTEM_CLOSURE_BYTES") · disk needed: at least $(format_bytes "$REQUIRED_DISK_BYTES")"

  ui_step 2 "Disk"
  select_disk "$REQUESTED_DISK" || return 1
  ui_success "Selected disk: $(describe_disk_identity "$SELECTED_DISK_ID")"
  ui_section "DESTRUCTIVE REVIEW"
  ui_detail "Install" "${SELECTED_HOST} (${SELECTED_HOST_IP})"
  ui_detail "Erase disk" "${SELECTED_DISK}"
  echo
  ui_feedback "All data on this disk will be permanently destroyed."
  ui_note "  Nothing is changed until you type ERASE."
  echo
  EXPECTED_CONFIRMATION="ERASE"
  # A mistyped word is asked again; an empty answer or end of input cancels.
  while true; do
    ui_prompt "Type ERASE to continue, or press Enter to cancel"
    if ! prompt_input "" CONFIRMATION || [[ -z "$CONFIRMATION" ]]; then
      ui_note "Installation cancelled; no disk operation was started."
      return 1
    fi
    [[ "$CONFIRMATION" == "$EXPECTED_CONFIRMATION" ]] && break
    ui_feedback "That was not ERASE (capital letters). Type it again, or press Enter to cancel."
  done

  revalidate_disk || return 1
  trap cleanup EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  ui_step 3 "Install"
  ui_note "Do not switch off this computer until the end."
  ui_note "${UI_MUTED}Technical details are saved in ${INSTALL_LOG}.${UI_RESET}"
  echo
  if ! : > "$INSTALL_LOG" 2>/dev/null; then
    ui_error "Cannot write the installation log ${INSTALL_LOG}." \
      "Run the installer as it was started (sudo /installer/setup.sh) or set NIXORIUM_INSTALL_LOG to a writable file."
    return 1
  fi
  MUTATION_STARTED=1
  CURRENT_STEP="disk partitioning"
  ui_log "Partitioning ${SELECTED_DISK}..."
  partition_disk || return 1
  ui_success "Disk prepared"

  CURRENT_STEP="NixOS installation"
  install_system || return 1

  CURRENT_STEP="installation verification"
  ui_log "Verifying the installed system..."
  verify_installation || return 1
  ui_success "Installed system verified"

  MUTATION_STARTED=0
  trap - EXIT INT TERM

  ui_section "All done"
  ui_success "${SELECTED_HOST} is installed on ${SELECTED_DISK}."
  if [[ -n "${HOST_KEY_FINGERPRINT:-}" ]]; then
    ui_detail "SSH key" "${HOST_KEY_FINGERPRINT}"
  fi
  echo
  ui_note "${UI_BOLD}Next${UI_RESET}"
  ui_note "  1. Remove or deprioritize network boot so the computer starts from its disk."
  if [[ -n "${HOST_KEY_FINGERPRINT:-}" ]]; then
    ui_note "  2. If the controller asks to review a changed SSH key for ${SELECTED_HOST},"
    ui_note "     it must show the fingerprint above."
  fi
  echo
  ask "Reboot now? (Enter stays in the installer)" "y/N" REBOOT_CONFIRMATION || return 1
  if [[ "${REBOOT_CONFIRMATION,,}" == "y" || "${REBOOT_CONFIRMATION,,}" == "yes" ]]; then
    ui_log "Rebooting into ${SELECTED_HOST}..."
    reboot_system
  else
    ui_note "Reboot not requested. Run 'systemctl reboot' when ready."
  fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
