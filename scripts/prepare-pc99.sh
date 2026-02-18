#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
FLAKE_FILE="${REPO_ROOT}/flake.nix"
TMUX_SESSION="lab-netboot"

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

detect_master_dhcp_ip() {
  local IFACE="$1"
  local DETECTED

  DETECTED=$(ip -4 -o addr show dev "$IFACE" | awk '/ dynamic / { split($4, a, "/"); print a[1]; exit }')
  if [[ -n "$DETECTED" ]]; then
    echo "$DETECTED"
    return 0
  fi

  DETECTED=$(ip -4 -o addr show dev "$IFACE" | awk '{ split($4, a, "/"); print a[1]; exit }')
  echo "$DETECTED"
}

log_step "Step 1/8: Checking prerequisites"
require_cmd nix
require_cmd ip
require_cmd awk
require_cmd sed
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

log_step "Step 2/8: Reading settings from flake.nix"
IFACE_NAME=$(extract_flake_string "ifaceName")
NETWORK_BASE=$(extract_flake_string "networkBase")
MASTER_DHCP_IP=$(extract_flake_string "masterDhcpIp")
MASTER_STATIC_IP="${NETWORK_BASE}.99"

if [[ -z "$IFACE_NAME" ]]; then
  log_err "ifaceName not found in flake.nix."
  exit 1
fi

if [[ "$MASTER_DHCP_IP" == "MASTER_DHCP_IP" || -z "$MASTER_DHCP_IP" ]]; then
  log_warn "masterDhcpIp is not configured; attempting auto-detection on ${IFACE_NAME}"
  DETECTED_DHCP_IP=$(detect_master_dhcp_ip "$IFACE_NAME")
  if [[ -z "$DETECTED_DHCP_IP" ]]; then
    log_err "Could not detect DHCP IP on interface ${IFACE_NAME}."
    exit 1
  fi
  log_step "Updating masterDhcpIp to ${DETECTED_DHCP_IP}"
  sed -i -E "s#masterDhcpIp = \".*\";#masterDhcpIp = \"${DETECTED_DHCP_IP}\";#" "$FLAKE_FILE"
  MASTER_DHCP_IP="$DETECTED_DHCP_IP"
  log_ok "flake.nix updated"
else
  log_ok "masterDhcpIp already configured: ${MASTER_DHCP_IP}"
fi

log_step "Step 3/8: Rebuilding pc99"
sudo nixos-rebuild switch --flake .#pc99 --no-write-lock-file
log_ok "pc99 rebuild completed"

log_step "Step 4/8: Building netboot artifacts"
nix build .#nixosConfigurations.netboot.config.system.build.kernel --out-link result-kernel
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --out-link result-initrd
nix build .#nixosConfigurations.netboot.config.system.build.netbootIpxeScript --out-link result-ipxe
log_ok "Netboot artifacts ready"

log_step "Step 5/8: Ensuring iPXE bootstrap binary (assets/ipxe/snponly.efi)"
nix build nixpkgs#ipxe --out-link result-ipxe-bin
install -D -m 0644 result-ipxe-bin/snp.efi assets/ipxe/snponly.efi
log_ok "snponly.efi installed"

log_step "Step 6/8: Pre-building all client closures"
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

log_step "Step 7/8: Preparing service startup (no static IP changes performed)"
log_warn "If needed, remove/add static IP manually as documented in README."
log_step "Using MASTER_DHCP_IP=${MASTER_DHCP_IP}, MASTER_STATIC_IP=${MASTER_STATIC_IP}, IFACE=${IFACE_NAME}"

log_step "Step 8/8: Starting Harmonia + PXE Proxy in tmux session '${TMUX_SESSION}'"
require_cmd tmux
if tmux has-session -t "${TMUX_SESSION}" 2>/dev/null; then
  log_warn "Existing tmux session '${TMUX_SESSION}' found. Killing it."
  tmux kill-session -t "${TMUX_SESSION}"
fi

sudo -v
tmux new-session -d -s "${TMUX_SESSION}" -n services "cd ${REPO_ROOT} && ./scripts/run-harmonia.sh 2>&1 | tee /tmp/harmonia.log"
tmux split-window -h -t "${TMUX_SESSION}:services" "cd ${REPO_ROOT} && sudo ./scripts/run-pxe-proxy.sh 2>&1 | tee /tmp/pxe-proxy.log"
tmux select-layout -t "${TMUX_SESSION}:services" even-horizontal
log_ok "Services started in tmux."
echo -e "${GREEN}Attach with:${RESET} tmux attach -t ${TMUX_SESSION}"
echo -e "${GREEN}Logs:${RESET} /tmp/harmonia.log and /tmp/pxe-proxy.log"
