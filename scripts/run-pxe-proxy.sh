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

# Resolve the repository root explicitly so the script does not depend on cwd.
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "${SCRIPT_DIR}/.." && pwd)

# shellcheck source=/home/admin/nixos-lab/scripts/lib/lab-meta.sh
source "${SCRIPT_DIR}/lib/lab-meta.sh"
load_lab_meta "${REPO_ROOT}"

IFACE="${LAB_IFACE_NAME}"
MASTER_IP="${LAB_CONTROLLER_DHCP_IP}"
HTTP_PORT="${LAB_PXE_HTTP_PORT}"

if [[ -z "${IFACE}" ]]; then
  echo "Error: interface name missing from labMeta." >&2
  exit 1
fi

if [[ -z "${MASTER_IP}" ]]; then
  echo "Error: controller DHCP IP missing from labMeta." >&2
  exit 1
fi

if [[ ! -f "${REPO_ROOT}/result-kernel/bzImage" ]]; then
  echo "Error: missing result-kernel/bzImage. Build netboot artifacts first." >&2
  exit 1
fi

if [[ ! -f "${REPO_ROOT}/result-initrd/initrd" ]]; then
  echo "Error: missing result-initrd/initrd. Build netboot artifacts first." >&2
  exit 1
fi

if [[ ! -f "${REPO_ROOT}/result-ipxe/netboot.ipxe" ]]; then
  echo "Error: missing result-ipxe/netboot.ipxe. Build netboot artifacts first." >&2
  exit 1
fi

if [[ ! -f "${REPO_ROOT}/assets/ipxe/snponly.efi" ]]; then
  echo "Error: missing assets/ipxe/snponly.efi." >&2
  echo "Copy it from your iPXE package/build into assets/ipxe/." >&2
  exit 1
fi

CMDLINE=$(grep '^kernel ' "${REPO_ROOT}/result-ipxe/netboot.ipxe" | sed 's/^kernel [^ ]* //')
if [[ -z "${CMDLINE}" ]]; then
  echo "Error: could not extract kernel cmdline from result-ipxe/netboot.ipxe." >&2
  exit 1
fi

WORK_DIR=$(mktemp -d)
trap 'kill "${HTTP_PID:-0}" >/dev/null 2>&1 || true; rm -rf "${WORK_DIR}"' EXIT

chmod 0755 "${WORK_DIR}"
install -d -m 0755 "${WORK_DIR}/http"
install -d -m 0755 "${WORK_DIR}/tftp"

cp "${REPO_ROOT}/result-kernel/bzImage" "${WORK_DIR}/http/bzImage"
cp "${REPO_ROOT}/result-initrd/initrd" "${WORK_DIR}/http/initrd"
cp "${REPO_ROOT}/assets/ipxe/snponly.efi" "${WORK_DIR}/tftp/snponly.efi"

# iPXE boot script: loads kernel and initrd over HTTP from the controller.
cat > "${WORK_DIR}/tftp/boot.ipxe" <<EOF
#!ipxe
dhcp
set base-url http://${MASTER_IP}:${HTTP_PORT}
kernel \${base-url}/bzImage ${CMDLINE}
initrd \${base-url}/initrd
boot
EOF

# Some iPXE builds look for autoexec.ipxe when no boot filename is provided.
# Keep an identical fallback script to avoid PXE boot loops.
cp "${WORK_DIR}/tftp/boot.ipxe" "${WORK_DIR}/tftp/autoexec.ipxe"

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

if ss -ltnp 2>/dev/null | awk -v port=":${HTTP_PORT}$" '$4 ~ port { found=1 } END { exit found ? 0 : 1 }'; then
  echo "Error: port ${HTTP_PORT} already in use. Stop the existing server and retry." >&2
  exit 1
fi

echo "Starting HTTP server on ${MASTER_IP}:${HTTP_PORT}"
python3 -m http.server "${HTTP_PORT}" --directory "${WORK_DIR}/http" --bind 0.0.0.0 &
HTTP_PID=$!
if ! kill -0 "${HTTP_PID}" >/dev/null 2>&1; then
  echo "Error: HTTP server failed to start on port ${HTTP_PORT}." >&2
  exit 1
fi

echo "Starting dnsmasq ProxyDHCP on interface ${IFACE}"
echo "DHCP leases remain handled by the institutional DHCP server."
exec dnsmasq --keep-in-foreground --conf-file="${WORK_DIR}/dnsmasq.conf"
