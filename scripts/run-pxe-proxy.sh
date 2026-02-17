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

PROXY_CIDR=$(
  ip -4 route show dev "${IFACE}" |
    awk '$1 ~ /^[0-9]/ && $1 ~ /\// && $0 ~ /scope link/ { print $1; exit }'
)

if [[ -z "${PROXY_CIDR}" ]]; then
  echo "Error: could not detect IPv4 subnet for interface ${IFACE}." >&2
  exit 1
fi

PROXY_SUBNET=${PROXY_CIDR%/*}
PROXY_PREFIX=${PROXY_CIDR#*/}
PROXY_NETMASK=$(python3 - <<EOF
import ipaddress
print(ipaddress.IPv4Network("0.0.0.0/${PROXY_PREFIX}").netmask)
EOF
)

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

cat > "${WORK_DIR}/http/boot.ipxe" <<EOF
#!ipxe
dhcp
set base-url http://${MASTER_IP}:8080
kernel \${base-url}/bzImage ${CMDLINE}
initrd \${base-url}/initrd
boot
EOF

cat > "${WORK_DIR}/dnsmasq.conf" <<EOF
port=0
log-dhcp
bind-interfaces
interface=${IFACE}

enable-tftp
tftp-root=${WORK_DIR}/tftp

dhcp-range=${PROXY_SUBNET},proxy,${PROXY_NETMASK}

dhcp-match=set:efi64,option:client-arch,6
dhcp-match=set:efi64,option:client-arch,7
dhcp-match=set:efi64,option:client-arch,9
dhcp-userclass=set:ipxe,iPXE

dhcp-boot=tag:!ipxe,tag:efi64,snponly.efi,,${MASTER_IP}
dhcp-boot=tag:ipxe,http://${MASTER_IP}:8080/boot.ipxe,,${MASTER_IP}
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
