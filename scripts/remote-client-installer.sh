#!/usr/bin/env bash
set -euo pipefail

REMOTE_SCHEMA_VERSION=1
MAX_PLAN_BYTES=65536
MINIMUM_DISK_BYTES=8589934592

if [[ -z "${NIXORIUM_INSTALLER_LIB:-}" || ! -r "$NIXORIUM_INSTALLER_LIB" ]]; then
  echo "remote installer library is unavailable" >&2
  exit 70
fi
# shellcheck disable=SC1090
source "$NIXORIUM_INSTALLER_LIB"

json_string_file() {
  local path=$1 maximum=$2 content
  [[ -r "$path" ]] || { printf 'null'; return; }
  content=$(head -c "$((maximum + 1))" -- "$path")
  (( ${#content} <= maximum )) || { echo "fact exceeds size limit" >&2; return 1; }
  jq -Rn --arg value "$content" '$value'
}

probe() {
  local os_release arch boot_id variant version build_id uefi sudo_ready
  local mem_available store_available disks interfaces
  os_release=/etc/os-release
  [[ -r "$os_release" ]] || { echo "missing os-release" >&2; return 1; }
  arch=$(/run/current-system/sw/bin/uname -m)
  boot_id=$(tr -d '\n' < /proc/sys/kernel/random/boot_id)
  variant=$(sed -n 's/^VARIANT_ID=//p' "$os_release" | tr -d '"' | head -n1)
  version=$(sed -n 's/^VERSION_ID=//p' "$os_release" | tr -d '"' | head -n1)
  build_id=$(sed -n 's/^BUILD_ID=//p' "$os_release" | tr -d '"' | head -n1)
  [[ -d /sys/firmware/efi ]] && uefi=true || uefi=false
  if sudo -n true >/dev/null 2>&1; then sudo_ready=true; else sudo_ready=false; fi
  mem_available=$(awk '/^MemAvailable:/ { print $2 * 1024; exit }' /proc/meminfo)
  store_available=$(df -B1 --output=avail /nix/store | awk 'NR == 2 { print $1 }')
  disks=$(nixorium_collect_disks_json "$MINIMUM_DISK_BYTES")
  interfaces=$(ip -j -4 address show scope global | jq -c '[.[] | {name:.ifname,addresses:[.addr_info[].local]}]')
  jq -cn \
    --argjson schemaVersion "$REMOTE_SCHEMA_VERSION" \
    --arg variantId "$variant" --arg versionId "$version" --arg buildId "$build_id" \
    --arg architecture "$arch" --arg bootId "$boot_id" \
    --argjson uefi "$uefi" --argjson sudoReady "$sudo_ready" \
    --argjson memoryAvailableBytes "${mem_available:-0}" \
    --argjson storeAvailableBytes "${store_available:-0}" \
    --argjson interfaces "$interfaces" --argjson disks "$disks" \
    '{schemaVersion:$schemaVersion,variantId:$variantId,versionId:$versionId,buildId:$buildId,architecture:$architecture,uefi:$uefi,sudoReady:$sudoReady,bootId:$bootId,memoryAvailableBytes:$memoryAvailableBytes,storeAvailableBytes:$storeAvailableBytes,interfaces:$interfaces,disks:$disks}'
}

status() {
  local operation_id=$1 receipt
  [[ "$operation_id" =~ ^[0-9a-f]{32}$ ]] || { echo "invalid operation id" >&2; return 2; }
  receipt="/run/nixorium-remote-install/$operation_id/receipt.json"
  [[ -f "$receipt" && ! -L "$receipt" ]] || {
    jq -cn --argjson schemaVersion "$REMOTE_SCHEMA_VERSION" --arg id "$operation_id" \
      '{schemaVersion:$schemaVersion,operationId:$id,state:"unknown"}'
    return 3
  }
  size=$(stat -c %s -- "$receipt")
  (( size <= MAX_PLAN_BYTES )) || { echo "receipt exceeds size limit" >&2; return 1; }
  cat -- "$receipt"
}

write_receipt() {
  local directory=$1 operation_id=$2 state=$3 phase=$4 mutation_started=$5 installed=$6 message=$7
  local temporary="$directory/receipt.json.tmp"
  jq -cn \
    --argjson schemaVersion "$REMOTE_SCHEMA_VERSION" --arg operationId "$operation_id" \
    --arg state "$state" --arg phase "$phase" --argjson mutationStarted "$mutation_started" \
    --argjson diskMayBeModified "$mutation_started" --argjson installed "$installed" \
    --arg message "$message" \
    '{schemaVersion:$schemaVersion,operationId:$operationId,state:$state,phase:$phase,mutationStarted:$mutationStarted,diskMayBeModified:$diskMayBeModified,installed:$installed,message:$message}' \
    > "$temporary"
  chmod 0600 "$temporary"
  mv -fT -- "$temporary" "$directory/receipt.json"
  sync -f "$directory/receipt.json" 2>/dev/null || true
}

validate_plan() {
  local plan_file=$1
  jq -e '
    type == "object" and
    (keys | sort) == (["adminPublicKey","bootId","cache","deploymentRevision","disk","host","hostKeyPublic","operationId","schemaVersion","systemPath"] | sort) and
    .schemaVersion == 1 and
    (.operationId | type == "string" and test("^[0-9a-f]{32}$")) and
    (.bootId | type == "string" and test("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")) and
    (.deploymentRevision | type == "string" and test("^[0-9a-f]{40,64}$")) and
    (.systemPath | type == "string" and test("^/nix/store/[0-9a-z]{32}-nixos-system-pc[0-9]{2}-[^/[:space:]]+$")) and
    (.adminPublicKey | type == "string" and length <= 1024 and test("^ssh-ed25519 [A-Za-z0-9+/=]+( .*)?$")) and
    (.hostKeyPublic | type == "string" and length <= 1024 and test("^ssh-ed25519 [A-Za-z0-9+/=]+( .*)?$")) and
    (.host | type == "object" and (keys | sort) == (["interface","liveIp","name","staticIp"] | sort)) and
    (.host.name | type == "string" and test("^pc[0-9]{2}$")) and
    (.host.interface | type == "string" and test("^[A-Za-z0-9_.:-]{1,64}$")) and
    (.host.liveIp | type == "string" and test("^([0-9]{1,3}\\.){3}[0-9]{1,3}$")) and
    (.host.staticIp | type == "string" and test("^([0-9]{1,3}\\.){3}[0-9]{1,3}$")) and
    (.cache | type == "object" and (keys | sort) == (["publicKey","url"] | sort)) and
    (.cache.url | type == "string" and test("^http://([0-9]{1,3}\\.){3}[0-9]{1,3}:[0-9]{1,5}$")) and
    (.cache.publicKey | type == "string" and length <= 256 and test("^[A-Za-z0-9._-]+:[A-Za-z0-9+/=]+$")) and
    (.disk | type == "object" and (keys | sort) == (["diskSeq","kname","majorMinor","model","path","serial","sizeBytes","transport","wwn"] | sort)) and
    (.disk.path | type == "string" and test("^/dev/[A-Za-z0-9_.+-]+$")) and
    (.disk.kname | type == "string" and test("^[A-Za-z0-9_.+-]+$")) and
    (.disk.majorMinor | type == "string" and test("^[0-9]+:[0-9]+$")) and
    (.disk.sizeBytes | type == "number" and floor == . and . >= 8589934592) and
    ([.disk.serial,.disk.wwn,.disk.model,.disk.transport,.disk.diskSeq] | all(type == "string" and length <= 256))
  ' "$plan_file" >/dev/null
}

validate_plan_semantics() {
  local plan_file=$1 live_ip static_ip cache_url cache_address cache_port host_name system_path
  live_ip=$(jq -r .host.liveIp "$plan_file")
  static_ip=$(jq -r .host.staticIp "$plan_file")
  cache_url=$(jq -r .cache.url "$plan_file")
  host_name=$(jq -r .host.name "$plan_file")
  system_path=$(jq -r .systemPath "$plan_file")
  nixorium_valid_unicast_ipv4 "$live_ip" || { echo "invalid live IPv4 address" >&2; return 2; }
  nixorium_valid_unicast_ipv4 "$static_ip" || { echo "invalid static IPv4 address" >&2; return 2; }
  [[ "$live_ip" != "$static_ip" ]] || { echo "live and static client addresses conflict" >&2; return 2; }
  cache_address=${cache_url#http://}
  cache_address=${cache_address%:*}
  cache_port=${cache_url##*:}
  nixorium_valid_unicast_ipv4 "$cache_address" || { echo "invalid cache IPv4 address" >&2; return 2; }
  (( cache_port >= 1 && cache_port <= 65535 )) || { echo "invalid cache port" >&2; return 2; }
  [[ "$system_path" == *"-nixos-system-$host_name-"* ]] || { echo "system path does not match host" >&2; return 2; }
}

read_and_validate_plan() {
  local destination=$1 canonical
  head -c "$((MAX_PLAN_BYTES + 1))" > "$destination"
  (( $(stat -c %s -- "$destination") <= MAX_PLAN_BYTES )) || {
    echo "plan exceeds size limit" >&2
    return 2
  }
  if [[ -n "${NIXORIUM_PLAN_VALIDATOR:-}" ]]; then
    [[ -x "$NIXORIUM_PLAN_VALIDATOR" ]] || { echo "remote plan validator is unavailable" >&2; return 2; }
    canonical="${destination}.validated"
    if ! "$NIXORIUM_PLAN_VALIDATOR" < "$destination" > "$canonical"; then
      rm -f -- "$canonical"
      return 2
    fi
    mv -fT -- "$canonical" "$destination"
  else
    validate_plan "$destination" || { echo "invalid remote installation plan" >&2; return 2; }
    validate_plan_semantics "$destination"
  fi
}

validate_plan_command() {
  local plan_file status_code
  plan_file=$(mktemp)
  if read_and_validate_plan "$plan_file"; then
    status_code=0
  else
    status_code=$?
    rm -f -- "$plan_file"
    return "$status_code"
  fi
  rm -f -- "$plan_file"
  printf '%s\n' valid
}

apply_plan() {
  local plan_file operation_id operation_dir boot_id selected_disk expected_identity
  local cache_url cache_key system_path host_name host_interface static_ip live_ip
  local admin_key host_key deployment_revision current_boot current_host_key bundle_admin bundle_cache
  local mutation_started=false installed=false phase=preflight reasons

  umask 077
  plan_file=$(mktemp)
  read_and_validate_plan "$plan_file"

  operation_id=$(jq -r .operationId "$plan_file")
  operation_dir="/run/nixorium-remote-install/$operation_id"
  boot_id=$(jq -r .bootId "$plan_file")
  selected_disk=$(jq -r .disk.path "$plan_file")
  expected_identity=$(jq -c .disk "$plan_file")
  cache_url=$(jq -r .cache.url "$plan_file")
  cache_key=$(jq -r .cache.publicKey "$plan_file")
  system_path=$(jq -r .systemPath "$plan_file")
  host_name=$(jq -r .host.name "$plan_file")
  host_interface=$(jq -r .host.interface "$plan_file")
  live_ip=$(jq -r .host.liveIp "$plan_file")
  static_ip=$(jq -r .host.staticIp "$plan_file")
  admin_key=$(jq -r .adminPublicKey "$plan_file")
  host_key=$(jq -r .hostKeyPublic "$plan_file")
  deployment_revision=$(jq -r .deploymentRevision "$plan_file")
  mkdir -p /run/nixorium-remote-install
  chmod 0700 /run/nixorium-remote-install
  if ! mkdir "$operation_dir" 2>/dev/null; then
    status "$operation_id" || true
    return 20
  fi
  chmod 0700 "$operation_dir"
  install -m 0600 "$plan_file" "$operation_dir/plan.json"

  failure_receipt() {
    local status_code=$?
    rm -f -- "$plan_file"
    if (( status_code != 0 )); then
      write_receipt "$operation_dir" "$operation_id" failed "$phase" "$mutation_started" "$installed" \
        "remote installer failed; inspect the private operation log"
    fi
    return "$status_code"
  }
  trap failure_receipt EXIT
  write_receipt "$operation_dir" "$operation_id" running "$phase" false false "preflight started"

  current_boot=$(tr -d '\n' < /proc/sys/kernel/random/boot_id)
  [[ "$current_boot" == "$boot_id" ]] || { echo "live boot ID changed" >&2; return 1; }
  [[ "$live_ip" != "$static_ip" ]] || { echo "live and static client addresses conflict" >&2; return 1; }
  ip link show dev "$host_interface" >/dev/null
  ip -j -4 address show dev "$host_interface" | jq -e --arg address "$live_ip" \
    'any(.[]?.addr_info[]?; .local == $address)' >/dev/null

  current_host_key=$(tr -d '\n' < /etc/ssh/ssh_host_ed25519_key.pub)
  [[ "$current_host_key" == "$host_key" ]] || { echo "live host key changed" >&2; return 1; }
  bundle_admin=$(tr -d '\n' < "${NIXORIUM_BUNDLE_SHARE:-/nonexistent}/admin-ssh.pub" 2>/dev/null || true)
  bundle_cache=$(tr -d '\n' < "${NIXORIUM_BUNDLE_SHARE:-/nonexistent}/cache-public-key" 2>/dev/null || true)
  [[ -n "$bundle_admin" && "$bundle_admin" == "$admin_key" ]] || { echo "admin key is absent from or differs from bundle" >&2; return 1; }
  [[ -n "$bundle_cache" && "$bundle_cache" == "$cache_key" ]] || { echo "cache key is absent from or differs from bundle" >&2; return 1; }

  reasons=$(nixorium_disk_exclusion_reasons_json "$selected_disk" "$MINIMUM_DISK_BYTES")
  [[ "$reasons" == "[]" ]] || { echo "selected disk is no longer eligible: $reasons" >&2; return 1; }
  nixorium_disk_matches_identity "$selected_disk" "$expected_identity" || {
    echo "selected disk identity changed" >&2
    return 1
  }
  nixorium_require_clean_install_mount
  nixorium_require_unique_target_labels "$selected_disk"

  phase=cache-revalidation
  nix --extra-experimental-features "nix-command flakes" path-info \
    --store "$cache_url" --recursive "$system_path" \
    --option trusted-public-keys "$cache_key" --option require-sigs true \
    --option fallback false >/dev/null

  phase=partition
  mutation_started=true
  write_receipt "$operation_dir" "$operation_id" running "$phase" true false "disk mutation started"
  nixorium_run_disko "$NIXORIUM_DISKO_SCRIPT" "$selected_disk"

  phase=install
  nixos-install --system "$system_path" \
    --option substituters "$cache_url" \
    --option trusted-public-keys "$cache_key" \
    --option require-sigs true \
    --option fallback false \
    --option builders "" \
    --no-channel-copy --no-root-passwd

  phase=host-key
  install -D -o root -g root -m 0600 /etc/ssh/ssh_host_ed25519_key /mnt/etc/ssh/ssh_host_ed25519_key
  install -D -o root -g root -m 0644 /etc/ssh/ssh_host_ed25519_key.pub /mnt/etc/ssh/ssh_host_ed25519_key.pub

  phase=verify
  nixorium_verify_installed_profile "$system_path" "$selected_disk"
  grep -R -F -x -- "$admin_key" /mnt/etc/ssh/authorized_keys.d /mnt/root/.ssh/authorized_keys >/dev/null 2>&1 || {
    echo "declared admin SSH key is absent from target" >&2
    return 1
  }
  grep -R -F -- "$deployment_revision" /mnt/etc /mnt/nix/var/nix/profiles/system 2>/dev/null >/dev/null || true

  installed=true
  phase=ready-to-reboot
  write_receipt "$operation_dir" "$operation_id" ready-to-reboot "$phase" true true \
    "installation completed; remove or deprioritize the USB before reboot"
  trap - EXIT
  rm -f -- "$plan_file"
}

case "${1:-}" in
  probe)
    [[ $# -eq 1 ]] || exit 2
    probe
    ;;
  status)
    [[ $# -eq 2 ]] || exit 2
    status "$2"
    ;;
  apply)
    [[ $# -eq 1 ]] || exit 2
    apply_plan
    ;;
  validate-plan)
    [[ $# -eq 1 ]] || exit 2
    validate_plan_command
    ;;
  *)
    echo "usage: nixorium-remote-client-installer {probe|validate-plan|apply|status OPERATION_ID}" >&2
    exit 2
    ;;
esac
