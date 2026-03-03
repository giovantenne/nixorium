#!/usr/bin/env bash
set -euo pipefail

if [[ $# -gt 0 ]]; then
  echo "Usage: ./scripts/run-pxe-proxy.sh" >&2
  exit 1
fi

if [[ "${EUID}" -ne 0 ]]; then
  echo "Error: run this script as root (use sudo)." >&2
  exit 1
fi

IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' flake.nix)
MASTER_IP=$(awk -F'"' '/masterDhcpIp =/ { print $2; exit }' flake.nix)
CMDLINE=$(grep '^kernel ' result-ipxe/netboot.ipxe | sed 's/^kernel [^ ]* //')

if [[ -z "${IFACE}" ]]; then
  echo "Error: ifaceName not found in flake.nix." >&2
  exit 1
fi

if [[ -z "${MASTER_IP}" || "${MASTER_IP}" == "MASTER_DHCP_IP" ]]; then
  echo "Error: masterDhcpIp not configured in flake.nix." >&2
  exit 1
fi

if [[ ! -f result-kernel/bzImage ]]; then
  echo "Error: missing result-kernel/bzImage. Build netboot artifacts first." >&2
  exit 1
fi

if [[ ! -f result-initrd/initrd ]]; then
  echo "Error: missing result-initrd/initrd. Build netboot artifacts first." >&2
  exit 1
fi

if [[ ! -f assets/ipxe/snponly.efi ]]; then
  echo "Error: missing assets/ipxe/snponly.efi." >&2
  echo "Copy it from your iPXE package/build into assets/ipxe/." >&2
  exit 1
fi

WORK_DIR=$(mktemp -d)
trap 'kill "${HTTP_PID:-0}" >/dev/null 2>&1 || true; rm -rf "${WORK_DIR}"' EXIT

chmod 0755 "${WORK_DIR}"
install -d -m 0755 "${WORK_DIR}/http"
install -d -m 0755 "${WORK_DIR}/tftp"

cp result-kernel/bzImage "${WORK_DIR}/http/bzImage"
cp result-initrd/initrd "${WORK_DIR}/http/initrd"
cp assets/ipxe/snponly.efi "${WORK_DIR}/tftp/snponly.efi"

# iPXE boot script: loads kernel and initrd over HTTP from pc99.
cat > "${WORK_DIR}/tftp/boot.ipxe" <<EOF
#!ipxe
dhcp
set base-url http://${MASTER_IP}:8080
kernel \${base-url}/bzImage ${CMDLINE}
initrd \${base-url}/initrd
boot
EOF

# Replicate the DRBL/Clonezilla ProxyDHCP dnsmasq configuration.
# In proxy mode, dnsmasq uses PXE Boot Server Discovery (port 4011) to tell
# clients which file to load via TFTP.  The pxe-service directive handles
# architecture-based routing automatically:
#   - UEFI firmware (arch 00007/00009) -> snponly.efi  (iPXE)
#   - iPXE re-does DHCP and identifies itself via user-class "iPXE"
#     so dnsmasq gives it boot.ipxe instead via dhcp-boot tag matching.
#
# Critical: dhcp-boot with tags works alongside pxe-service in dnsmasq
# because iPXE does a *standard DHCP request* (not PXE Boot Server Discovery),
# so it receives the dhcp-boot filename. Native UEFI firmware uses the
# pxe-service path instead.
cat > "${WORK_DIR}/dnsmasq.conf" <<EOF
port=0
log-dhcp
bind-interfaces
interface=${IFACE}
dhcp-no-override

enable-tftp
tftp-root=${WORK_DIR}/tftp

# ProxyDHCP: use the server's own IP (DRBL-style, not subnet).
dhcp-range=${MASTER_IP},proxy

# Tag iPXE clients by their user-class header.
dhcp-userclass=set:ipxe,iPXE

# Stage 1 - UEFI firmware PXE boot: serve snponly.efi (iPXE) via TFTP.
# These pxe-service lines respond to PXE Boot Server Discovery from native
# UEFI firmware.  The 3-arg form (no server IP) means "this server".
pxe-service=BC_EFI, "Boot iPXE UEFI BC", snponly.efi
pxe-service=X86-64_EFI, "Boot iPXE UEFI x64", snponly.efi
pxe-prompt="Network boot", 1

# Stage 2 - iPXE chainload: serve boot.ipxe via TFTP.
# iPXE issues a standard DHCP request (not PXE discovery), so dhcp-boot
# applies here.  boot.ipxe then fetches kernel + initrd over HTTP.
dhcp-boot=tag:ipxe,boot.ipxe
EOF

if ss -ltnp 2>/dev/null | awk '$4 ~ /:8080$/ { found=1 } END { exit found ? 0 : 1 }'; then
  echo "Error: port 8080 already in use. Stop the existing server and retry." >&2
  exit 1
fi

echo "Starting HTTP server on ${MASTER_IP}:8080"
python3 -m http.server 8080 --directory "${WORK_DIR}/http" --bind 0.0.0.0 &
HTTP_PID=$!
if ! kill -0 "${HTTP_PID}" >/dev/null 2>&1; then
  echo "Error: HTTP server failed to start on port 8080." >&2
  exit 1
fi

echo "Starting dnsmasq ProxyDHCP on interface ${IFACE}"
echo "DHCP leases remain handled by the institutional DHCP server."
exec dnsmasq --keep-in-foreground --conf-file="${WORK_DIR}/dnsmasq.conf"
