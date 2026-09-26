#!/usr/bin/env bash
set -euo pipefail

# Installed with a fixed runtime PATH on clients only. Never accept rule text,
# unit names, addresses, or filesystem paths from the caller.
[[ "$EUID" == 0 ]] || { echo "Administrator access required" >&2; exit 77; }
ACTION="${1:-}"
case "$ACTION" in
  status) [[ $# == 1 ]] || exit 64 ;;
  block|unblock) [[ $# == 2 ]] || exit 64 ;;
  *) exit 64 ;;
esac
umask 077
exec 9>/run/nixorium-internet.lock
flock -w 5 9
BOOT_ID="$(cat /proc/sys/kernel/random/boot_id)"
if [[ "$ACTION" != status ]]; then
  [[ "$2" == "$BOOT_ID" ]] || { echo "Client rebooted; review again" >&2; exit 75; }
  if [[ "$ACTION" == block ]]; then
    systemctl start nixorium-internet-block.service
  else
    systemctl stop nixorium-internet-block.service
    # Also recover a leftover owned table after an interrupted service stop.
    nft destroy table inet nixorium_internet
  fi
fi
TABLES="$(nft list tables)"
UNIT_STATE="$(systemctl show --property=ActiveState --value nixorium-internet-block.service)"
STATE=unknown
if grep -Fxq 'table inet nixorium_internet' <<< "$TABLES"; then
  [[ "$UNIT_STATE" != active ]] || STATE=blocked
elif [[ "$UNIT_STATE" == inactive ]]; then
  STATE=enabled
fi
printf '{"schemaVersion":1,"bootId":"%s","state":"%s"}\n' "$BOOT_ID" "$STATE"
