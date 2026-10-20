#!/usr/bin/env bash

# Shared identity, disk and installation primitives. Callers own UI,
# authorization and operation state; this file owns the safety checks used by
# the live installers.

# shellcheck disable=SC2034 # Version marker of this library, read by the sourcing installers.
NIXORIUM_INSTALLER_SCHEMA_VERSION=1
NIXORIUM_MAX_DISKS=128

nixorium_canonical_ed25519_public_key() {
  local value=$1 key_type key_data
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || return 1
  read -r key_type key_data _ <<< "$value"
  [[ "$key_type" == ssh-ed25519 && "$key_data" =~ ^[A-Za-z0-9+/=]+$ ]] || return 1
  printf '%s %s\n' "$key_type" "$key_data"
}

nixorium_valid_unicast_ipv4() {
  local address=$1 octet normalized first second
  local -a octets
  [[ "$address" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || return 1
  IFS=. read -r -a octets <<< "$address"
  normalized=""
  for octet in "${octets[@]}"; do
    (( 10#$octet <= 255 )) || return 1
    normalized+="${normalized:+.}$((10#$octet))"
  done
  [[ "$normalized" == "$address" ]] || return 1
  first=$((10#${octets[0]}))
  second=$((10#${octets[1]}))
  (( first > 0 && first < 224 && first != 127 )) || return 1
  (( first != 169 || second != 254 )) || return 1
  [[ "$address" != "255.255.255.255" ]]
}

nixorium_canonical_disk() {
  local raw=$1
  [[ "$raw" == /dev/* ]] || raw="/dev/$raw"
  readlink -m -- "$raw"
}

# nixos-install forwards nix's "copying path ..." line for every store path
# (thousands per installation) and has no quiet option. This keeps everything
# else, including errors, and replaces those lines with a count every 500.
nixorium_summarize_copy_output() {
  awk '
    /^copying path / { n++; if (n % 500 == 0) { print "  " n " store paths copied..."; fflush() }; next }
    { print; fflush() }
    END { if (n > 0) print "  " n " store paths copied." }
  '
}

nixorium_disk_kname() {
  lsblk -dnro KNAME -- "$1"
}

nixorium_disk_identity_json() {
  local disk=$1 kname size major_minor serial wwn model transport diskseq
  kname=$(nixorium_disk_kname "$disk") || return 1
  size=$(lsblk -bdnro SIZE -- "$disk") || return 1
  major_minor=$(lsblk -dnro MAJ:MIN -- "$disk") || return 1
  serial=$(lsblk -dnro SERIAL -- "$disk" | sed 's/[[:space:]]*$//')
  wwn=$(lsblk -dnro WWN -- "$disk" | sed 's/[[:space:]]*$//')
  model=$(lsblk -dnro MODEL -- "$disk" | sed 's/[[:space:]]*$//')
  transport=$(lsblk -dnro TRAN -- "$disk" | sed 's/[[:space:]]*$//')
  diskseq=""
  if [[ -r "/sys/class/block/$kname/diskseq" ]]; then
    read -r diskseq < "/sys/class/block/$kname/diskseq"
  fi
  [[ "$size" =~ ^[0-9]+$ && "$major_minor" =~ ^[0-9]+:[0-9]+$ ]]
  jq -cn \
    --arg path "$disk" --arg kname "$kname" --arg majorMinor "$major_minor" \
    --arg serial "$serial" --arg wwn "$wwn" --arg model "$model" \
    --arg transport "$transport" --arg diskSeq "$diskseq" \
    --argjson sizeBytes "$size" \
    '{path:$path,kname:$kname,majorMinor:$majorMinor,sizeBytes:$sizeBytes,serial:$serial,wwn:$wwn,model:$model,transport:$transport,diskSeq:$diskSeq}'
}

nixorium_disk_matches_identity() {
  local disk=$1 expected=$2 actual
  actual=$(nixorium_disk_identity_json "$disk") || return 1
  jq -e --argjson actual "$actual" --argjson expected "$expected" '
    $actual.path == $expected.path and
    $actual.kname == $expected.kname and
    $actual.majorMinor == $expected.majorMinor and
    $actual.sizeBytes == $expected.sizeBytes and
    $actual.serial == $expected.serial and
    $actual.wwn == $expected.wwn and
    $actual.model == $expected.model and
    $actual.transport == $expected.transport and
    $actual.diskSeq == $expected.diskSeq
  ' <<< '{}' >/dev/null
}

nixorium_backing_disks_for_mount() {
  local target source
  for target in / /iso /nix/.ro-store /run/live/medium; do
    source=$(findmnt -rn -o SOURCE --target "$target" 2>/dev/null || true)
    [[ "$source" == /dev/* ]] || continue
    readlink -f -- "$source" 2>/dev/null || printf '%s\n' "$source"
  done | while IFS= read -r source; do
    lsblk -snpo PATH,TYPE -- "$source" 2>/dev/null |
      awk '$2 == "disk" { print $1 }'
  done | sort -u
}

nixorium_disk_has_mounts() {
  lsblk -nrpo MOUNTPOINTS -- "$1" |
    awk 'NF { found=1 } END { exit !found }'
}

nixorium_disk_has_swap() {
  local disk=$1 swap
  while IFS= read -r swap; do
    [[ -n "$swap" ]] || continue
    if lsblk -snpo PATH -- "$swap" 2>/dev/null | grep -Fx -- "$disk" >/dev/null; then
      return 0
    fi
  done < <(swapon --noheadings --raw --show=NAME 2>/dev/null || true)
  return 1
}

nixorium_disk_has_holders() {
  local kname child holder
  while IFS= read -r child; do
    [[ -n "$child" ]] || continue
    for holder in "/sys/class/block/$child/holders/"*; do
      [[ -e "$holder" ]] && return 0
    done
  done < <(lsblk -nro KNAME -- "$1")
  kname=$(nixorium_disk_kname "$1") || return 0
  [[ -n "$kname" ]] || return 0
  return 1
}

nixorium_disk_exclusion_reasons_json() {
  local disk=$1 minimum=$2 ro size removable type reason_json='[]' backing
  type=$(lsblk -dnro TYPE -- "$disk") || return 1
  ro=$(lsblk -dnro RO -- "$disk") || return 1
  size=$(lsblk -bdnro SIZE -- "$disk") || return 1
  removable=$(lsblk -dnro RM -- "$disk") || return 1

  add_reason() {
    reason_json=$(jq -cn --argjson current "$reason_json" --arg reason "$1" '$current + [$reason]')
  }

  [[ "$type" == disk ]] || add_reason "not-disk"
  [[ "$ro" == 0 ]] || add_reason "read-only"
  [[ "$removable" == 0 ]] || add_reason "removable"
  [[ "$size" =~ ^[0-9]+$ && "$size" -ge "$minimum" ]] || add_reason "too-small"
  if nixorium_disk_has_mounts "$disk"; then add_reason "mounted"; fi
  if nixorium_disk_has_swap "$disk"; then add_reason "swap-active"; fi
  if nixorium_disk_has_holders "$disk"; then add_reason "holders-active"; fi
  while IFS= read -r backing; do
    if [[ "$backing" == "$disk" ]]; then
      add_reason "live-media"
      break
    fi
  done < <(nixorium_backing_disks_for_mount)
  jq -c 'sort' <<< "$reason_json"
}

nixorium_collect_disks_json() {
  local minimum=$1 disk reasons identity result='[]' count=0
  while IFS= read -r disk; do
    [[ -n "$disk" ]] || continue
    count=$((count + 1))
    (( count <= NIXORIUM_MAX_DISKS )) || {
      echo "more than $NIXORIUM_MAX_DISKS disks detected" >&2
      return 1
    }
    disk=$(nixorium_canonical_disk "$disk")
    reasons=$(nixorium_disk_exclusion_reasons_json "$disk" "$minimum") || return 1
    identity=$(nixorium_disk_identity_json "$disk") || return 1
    result=$(jq -cn --argjson current "$result" --argjson identity "$identity" \
      --argjson reasons "$reasons" '$current + [$identity + {eligible:($reasons|length == 0),exclusionReasons:$reasons}]')
  done < <(lsblk -dnpo PATH,TYPE | awk '$2 == "disk" { print $1 }')
  printf '%s\n' "$result"
}

nixorium_collect_available_disks() {
  local minimum=$1
  nixorium_collect_disks_json "$minimum" | jq -r '.[] | select(.eligible) | .path'
}

nixorium_run_disko() {
  local script=$1 disk=$2 basename
  [[ -x "$script" ]] || { echo "Disko script is not executable" >&2; return 1; }
  disk=$(nixorium_canonical_disk "$disk")
  [[ "$disk" =~ ^/dev/[[:alnum:]_.+-]+$ ]] || {
    echo "invalid installation disk" >&2
    return 1
  }
  basename=${disk#/dev/}
  if (( EUID == 0 )); then
    env NIXORIUM_INSTALL_DISK="$basename" "$script" --yes-wipe-all-disks
  else
    sudo env NIXORIUM_INSTALL_DISK="$basename" "$script" --yes-wipe-all-disks
  fi
}

nixorium_require_clean_install_mount() {
  if findmnt -rn --mountpoint /mnt >/dev/null 2>&1; then
    echo "/mnt is already occupied" >&2
    return 1
  fi
}

nixorium_require_unique_target_labels() {
  local selected_disk=$1 path label parent
  while read -r path label; do
    [[ "$label" == disk-main-esp || "$label" == disk-main-root ]] || continue
    parent=$(lsblk -snpo PATH,TYPE -- "$path" | awk '$2 == "disk" { print $1; exit }')
    if [[ -z "$parent" || "$parent" != "$selected_disk" ]]; then
      echo "target partition label $label already exists on another or unknown disk" >&2
      return 1
    fi
  done < <(lsblk -nrpo PATH,PARTLABEL)
}

# The name inside a client's system store path: its own name, or the class
# name when the computer shares one system with its class (ADR 0033).
nixorium_expected_system_name() {
  local host_name=$1 class=${2:-}
  [[ "$host_name" =~ ^pc[0-9]{2,4}$ ]] || { echo "invalid computer name" >&2; return 1; }
  if [[ -z "$class" || "$class" == "$host_name" ]]; then
    printf '%s\n' "$host_name"
  elif [[ "$class" =~ ^shared-[0-9]{1,4}$ ]]; then
    printf 'nixorium-client-%s\n' "$class"
  else
    echo "invalid class of $host_name" >&2
    return 1
  fi
}

# The identity record of the installed computer: a shared system takes the
# computer's name and address from it; a per-host system writes the same
# record itself. Written before the first boot and read back.
nixorium_write_identity_record() {
  local host_name=$1 install_root=${2:-/mnt} number expected record
  [[ "$host_name" =~ ^pc0*([1-9][0-9]{0,3})$ ]] || { echo "invalid computer name" >&2; return 1; }
  number=${BASH_REMATCH[1]}
  expected="{\"schemaVersion\":1,\"number\":${number}}"
  record="$install_root/var/lib/nixorium/identity.json"
  local -a privileged=()
  (( EUID == 0 )) || privileged=(sudo)
  "${privileged[@]}" mkdir -p "$install_root/var/lib/nixorium"
  printf '%s\n' "$expected" | "${privileged[@]}" tee "$record.tmp" >/dev/null
  "${privileged[@]}" chmod 0644 "$record.tmp"
  "${privileged[@]}" mv -f "$record.tmp" "$record"
  sync
  [[ "$("${privileged[@]}" cat "$record")" == "$expected" ]] || {
    echo "the identity record was not written" >&2
    return 1
  }
}

# nixorium_prepare_host_key creates the installed computer's SSH host key
# (ed25519, the type the controller pins) when the installation has none and
# prints its fingerprint, so it can be shown at the end of the installation and
# compared with the one the controller asks to review for a reinstalled
# computer.
nixorium_prepare_host_key() {
  local install_root=${1:-/mnt} key
  key="$install_root/etc/ssh/ssh_host_ed25519_key"
  local -a privileged=()
  (( EUID == 0 )) || privileged=(sudo)
  if ! "${privileged[@]}" test -s "$key"; then
    "${privileged[@]}" install -d -m 0755 "$install_root/etc/ssh"
    "${privileged[@]}" ssh-keygen -q -t ed25519 -N "" -C "" -f "$key" </dev/null >/dev/null
  fi
  "${privileged[@]}" ssh-keygen -l -E sha256 -f "$key.pub" | awk '{ print $2 }'
}

nixorium_verify_installed_profile() {
  local expected_system=$1 selected_disk=$2 install_root=${3:-/mnt} profile root_source root_device root_parent
  profile=$(readlink -f "$install_root/nix/var/nix/profiles/system" 2>/dev/null || true)
  [[ "$profile" == "$expected_system" ]] || {
    echo "installed profile does not match the reviewed system" >&2
    return 1
  }
  root_source=$(findmnt -rn -o SOURCE --target "$install_root" 2>/dev/null || true)
  [[ "$root_source" == /dev/* ]] || {
    echo "installed root mount is unavailable" >&2
    return 1
  }
  root_device=${root_source%%\[*}
  root_parent=$(lsblk -snpo PATH,TYPE -- "$root_device" | awk '$2 == "disk" { print $1; exit }')
  [[ "$root_parent" == "$selected_disk" ]] || {
    echo "installed root is not backed by the reviewed disk" >&2
    return 1
  }
  [[ -e "$install_root/boot/EFI/NixOS-boot/grubx64.efi" || -e "$install_root/boot/EFI/systemd/systemd-bootx64.efi" ]] || {
    echo "installed EFI bootloader is missing" >&2
    return 1
  }
  sync
}
