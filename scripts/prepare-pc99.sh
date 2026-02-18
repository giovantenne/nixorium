#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
FLAKE_FILE="${REPO_ROOT}/flake.nix"

BLUE='\033[1;34m'
GREEN='\033[1;32m'
YELLOW='\033[1;33m'
RED='\033[1;31m'
RESET='\033[0m'

log_step() {
  echo -e "${BLUE}==>${RESET} $1"
}

log_ok() {
  echo -e "${GREEN}[OK]${RESET} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${RESET} $1"
}

log_err() {
  echo -e "${RED}[ERROR]${RESET} $1" >&2
}

require_cmd() {
  local CMD="$1"
  if ! command -v "$CMD" >/dev/null 2>&1; then
    log_err "Missing required command: $CMD"
    exit 1
  fi
}

extract_flake_string() {
  local KEY="$1"
  awk -F'"' -v key="$KEY" '$0 ~ key" =" { print $2; exit }' "$FLAKE_FILE"
}

extract_flake_number() {
  local KEY="$1"
  awk -v key="$KEY" '$0 ~ key" =" { gsub(/[^0-9]/, ""); print; exit }' "$FLAKE_FILE"
}

# ── Step 1/7: Prerequisites ──────────────────────────────────────────
log_step "Step 1/7: Checking prerequisites"
require_cmd nix
require_cmd ip
require_cmd awk
require_cmd install
require_cmd seq
require_cmd sudo

if [[ ! -f "$FLAKE_FILE" ]]; then
  log_err "flake.nix not found in repo root."
  exit 1
fi

if [[ ! -f "${REPO_ROOT}/secret-key" ]]; then
  log_warn "secret-key is missing: ${REPO_ROOT}/secret-key"
  log_warn "Harmonia will fail until you copy it."
fi
log_ok "Prerequisites check complete"

# ── Step 2/7: Read and validate settings from flake.nix ──────────────
log_step "Step 2/7: Reading settings from flake.nix"
IFACE_NAME=$(extract_flake_string "ifaceName")
NETWORK_BASE=$(extract_flake_string "networkBase")
MASTER_DHCP_IP=$(extract_flake_string "masterDhcpIp")
MASTER_HOST_NUMBER=$(extract_flake_number "masterHostNumber")

if [[ -z "$IFACE_NAME" ]]; then
  log_err "ifaceName not found in flake.nix."
  exit 1
fi

if [[ -z "$NETWORK_BASE" ]]; then
  log_err "networkBase not found in flake.nix."
  exit 1
fi

if [[ -z "$MASTER_HOST_NUMBER" ]]; then
  log_err "masterHostNumber not found in flake.nix."
  exit 1
fi

if [[ "$MASTER_DHCP_IP" == "MASTER_DHCP_IP" || -z "$MASTER_DHCP_IP" ]]; then
  log_err "masterDhcpIp is not configured in flake.nix."
  log_err "Edit flake.nix first: set masterDhcpIp to the DHCP address of this machine."
  log_err "Run 'ip -4 addr show dev ${IFACE_NAME}' to find it."
  exit 1
fi

MASTER_STATIC_IP="${NETWORK_BASE}.${MASTER_HOST_NUMBER}"
log_ok "Interface: ${IFACE_NAME}"
log_ok "DHCP IP: ${MASTER_DHCP_IP}"
log_ok "Static IP: ${MASTER_STATIC_IP}"

# ── Step 3/7: Rebuild pc99 ───────────────────────────────────────────
log_step "Step 3/7: Rebuilding pc99"
sudo nixos-rebuild switch --flake .#pc99 --no-write-lock-file
log_ok "pc99 rebuild completed"

# ── Step 4/7: Build netboot artifacts ────────────────────────────────
log_step "Step 4/7: Building netboot artifacts"
nix build .#nixosConfigurations.netboot.config.system.build.kernel --out-link result-kernel
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --out-link result-initrd
nix build .#nixosConfigurations.netboot.config.system.build.netbootIpxeScript --out-link result-ipxe
log_ok "Netboot artifacts ready"

# ── Step 5/7: iPXE bootstrap binary ─────────────────────────────────
log_step "Step 5/7: Ensuring iPXE bootstrap binary (assets/ipxe/snponly.efi)"
nix build nixpkgs#ipxe --out-link result-ipxe-bin
install -D -m 0644 result-ipxe-bin/snp.efi assets/ipxe/snponly.efi
log_ok "snponly.efi installed"

# ── Step 6/7: Pre-build all client closures ──────────────────────────
log_step "Step 6/7: Pre-building all client closures"
PC_COUNT=$(extract_flake_number "pcCount")
if [[ -z "$PC_COUNT" ]]; then
  log_err "pcCount not found in flake.nix."
  exit 1
fi

CLIENT_TARGETS=()
for PC_NUMBER in $(seq 1 "$PC_COUNT"); do
  CLIENT_TARGETS+=(".#nixosConfigurations.pc$(printf "%02d" "$PC_NUMBER").config.system.build.toplevel")
done

nix build --max-jobs 1 "${CLIENT_TARGETS[@]}"
log_ok "Client closures pre-built"

# ── Step 7/7: Remove static IP for netboot ───────────────────────────
log_step "Step 7/7: Removing static IP for netboot"
if ip -4 addr show dev "${IFACE_NAME}" 2>/dev/null | grep -q "${MASTER_STATIC_IP}/"; then
  sudo ip addr del "${MASTER_STATIC_IP}/24" dev "${IFACE_NAME}"
  log_ok "Removed ${MASTER_STATIC_IP}/24 from ${IFACE_NAME}"
else
  log_ok "Static IP ${MASTER_STATIC_IP} not present on ${IFACE_NAME}, nothing to remove"
fi

# ── Done ─────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}========================================${RESET}"
echo -e "${GREEN}  Preparation complete!${RESET}"
echo -e "${GREEN}========================================${RESET}"
echo ""
echo -e "Start the netboot services in ${YELLOW}two separate terminals${RESET}:"
echo ""
echo -e "  ${BLUE}Terminal 1 (binary cache):${RESET}"
echo -e "    ./scripts/run-harmonia.sh"
echo ""
echo -e "  ${BLUE}Terminal 2 (PXE/netboot server):${RESET}"
echo -e "    sudo ./scripts/run-pxe-proxy.sh"
echo ""
echo -e "When all clients are installed, restore the static IP (or reboot pc99):"
echo -e "    sudo ip addr add ${MASTER_STATIC_IP}/24 dev ${IFACE_NAME}"
echo ""
