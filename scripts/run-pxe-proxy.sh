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
if [[ -n "${LAB_REPO_ROOT:-}" ]]; then
  REPO_ROOT="${LAB_REPO_ROOT}"
else
  REPO_ROOT=$(cd "${SCRIPT_DIR}/.." && pwd)
fi

# shellcheck source=/home/admin/nixorium/scripts/lib/lab-meta.sh
source "${SCRIPT_DIR}/lib/lab-meta.sh"
load_lab_meta "${REPO_ROOT}"
require_deployment_ready "${REPO_ROOT}"

IFACE="${LAB_IFACE_NAME}"
MASTER_IP="${LAB_CONTROLLER_DHCP_IP}"
HTTP_PORT="${LAB_PXE_HTTP_PORT}"
PREPARATION_FILE="/var/lib/nixorium/prepared/prepared.json"

if [[ -z "${IFACE}" ]]; then
  echo "Error: interface name missing from labMeta." >&2
  exit 1
fi

if [[ -z "${MASTER_IP}" ]]; then
  echo "Error: controller DHCP IP missing from labMeta." >&2
  exit 1
fi

if [[ -e "${PREPARATION_FILE}" ]]; then
  [[ -f "${PREPARATION_FILE}" && ! -L "${PREPARATION_FILE}" ]] \
    || { echo "Error: managed PXE preparation is not a regular file." >&2; exit 1; }
  [[ "$(stat -c '%U:%G:%a' "${PREPARATION_FILE}")" == admin:users:644 ]] \
    || { echo "Error: managed PXE preparation has unsafe ownership or permissions." >&2; exit 1; }
  CURRENT_REVISION=$(git -c safe.directory="${REPO_ROOT}" -C "${REPO_ROOT}" rev-parse HEAD)
  jq -e \
    --arg revision "${CURRENT_REVISION}" \
    --arg iface "${IFACE}" \
    --arg dhcpIp "${MASTER_IP}" \
    --argjson cachePort "${LAB_CACHE_PORT}" \
    --argjson pxeHttpPort "${HTTP_PORT}" \
    --argjson clientCount "${LAB_CLIENT_COUNT}" \
    '.schemaVersion == 1 and .revision == $revision and
     .controller.dhcpIp == $dhcpIp and .network.ifaceName == $iface and
     .network.cachePort == $cachePort and .network.pxeHttpPort == $pxeHttpPort and
     (.clients | length) == $clientCount and
     .artifacts.kernel.relativePath == "bzImage" and
     .artifacts.initrd.relativePath == "initrd" and
     .artifacts.ipxeScript.relativePath == "netboot.ipxe" and
     .artifacts.firmware.relativePath == "snponly.efi"' \
    "${PREPARATION_FILE}" >/dev/null \
    || { echo "Error: managed PXE preparation is stale or invalid; run nixorium pxe prepare." >&2; exit 1; }
  KERNEL_ROOT=$(jq -er '.artifacts.kernel.storePath' "${PREPARATION_FILE}")
  INITRD_ROOT=$(jq -er '.artifacts.initrd.storePath' "${PREPARATION_FILE}")
  IPXE_SCRIPT_ROOT=$(jq -er '.artifacts.ipxeScript.storePath' "${PREPARATION_FILE}")
  FIRMWARE_ROOT=$(jq -er '.artifacts.firmware.storePath' "${PREPARATION_FILE}")
  for store_path in "${KERNEL_ROOT}" "${INITRD_ROOT}" "${IPXE_SCRIPT_ROOT}" "${FIRMWARE_ROOT}"; do
    [[ "${store_path}" =~ ^/nix/store/[0-9a-z]{32}-[^/[:space:]]+$ ]] \
      || { echo "Error: managed PXE preparation contains an invalid store path." >&2; exit 1; }
  done
  KERNEL_FILE="${KERNEL_ROOT}/bzImage"
  INITRD_FILE="${INITRD_ROOT}/initrd"
  IPXE_SCRIPT_FILE="${IPXE_SCRIPT_ROOT}/netboot.ipxe"
  FIRMWARE_FILE="${FIRMWARE_ROOT}/snponly.efi"
else
  # Advanced compatibility path for deployments prepared with result symlinks.
  KERNEL_FILE="${REPO_ROOT}/result-kernel/bzImage"
  INITRD_FILE="${REPO_ROOT}/result-initrd/initrd"
  IPXE_SCRIPT_FILE="${REPO_ROOT}/result-ipxe/netboot.ipxe"
  FIRMWARE_FILE="${REPO_ROOT}/assets/ipxe/snponly.efi"
fi

for artifact in "${KERNEL_FILE}" "${INITRD_FILE}" "${IPXE_SCRIPT_FILE}" "${FIRMWARE_FILE}"; do
  [[ -f "${artifact}" ]] \
    || { echo "Error: missing prepared PXE artifact ${artifact}." >&2; exit 1; }
done

CMDLINE=$(grep '^kernel ' "${IPXE_SCRIPT_FILE}" | sed 's/^kernel [^ ]* //')
if [[ -z "${CMDLINE}" ]]; then
  echo "Error: could not extract kernel cmdline from result-ipxe/netboot.ipxe." >&2
  exit 1
fi

WORK_DIR=$(mktemp -d)
trap 'kill "${HTTP_PID:-0}" >/dev/null 2>&1 || true; rm -rf "${WORK_DIR}"' EXIT

chmod 0755 "${WORK_DIR}"
install -d -m 0755 "${WORK_DIR}/http"
install -d -m 0755 "${WORK_DIR}/tftp"

cp "${KERNEL_FILE}" "${WORK_DIR}/http/bzImage"
cp "${INITRD_FILE}" "${WORK_DIR}/http/initrd"
cp "${FIRMWARE_FILE}" "${WORK_DIR}/tftp/snponly.efi"

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
