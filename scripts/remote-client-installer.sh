#!/usr/bin/env bash
set -euo pipefail

REMOTE_SCHEMA_VERSION=1
MAX_PLAN_BYTES=65536
MINIMUM_DISK_BYTES=8589934592
OPERATION_ROOT=${NIXORIUM_OPERATION_ROOT:-/run/nixorium-remote-install}
SYSTEMD_RUN=${NIXORIUM_SYSTEMD_RUN:-/run/current-system/sw/bin/systemd-run}
OPERATION_OWNER=${NIXORIUM_OPERATION_OWNER:-0:0}

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
  receipt="$OPERATION_ROOT/$operation_id/receipt.json"
  [[ -f "$receipt" && ! -L "$receipt" ]] || {
    jq -cn --argjson schemaVersion "$REMOTE_SCHEMA_VERSION" --arg id "$operation_id" \
      '{schemaVersion:$schemaVersion,operationId:$id,state:"unknown",phase:"preflight",mutationStarted:false,diskMayBeModified:false,installed:false,message:"operation receipt is not present"}'
    return 0
  }
  size=$(stat -c %s -- "$receipt")
  (( size <= MAX_PLAN_BYTES )) || { echo "receipt exceeds size limit" >&2; return 1; }
  cat -- "$receipt"
}

write_receipt() {
  local directory=$1 operation_id=$2 state=$3 phase=$4 mutation_started=$5 installed=$6 message=$7
  local temporary="$directory/receipt.json.tmp" sequence=1
  if [[ -f "$directory/receipt.json" && ! -L "$directory/receipt.json" ]]; then
    sequence=$(jq -er '(.sequence // 0) + 1' "$directory/receipt.json") || sequence=1
  fi
  jq -cn \
    --argjson schemaVersion "$REMOTE_SCHEMA_VERSION" --arg operationId "$operation_id" \
    --arg state "$state" --arg phase "$phase" --argjson mutationStarted "$mutation_started" \
    --argjson diskMayBeModified "$mutation_started" --argjson installed "$installed" \
    --argjson sequence "$sequence" --arg message "$message" \
    '{schemaVersion:$schemaVersion,operationId:$operationId,state:$state,phase:$phase,sequence:$sequence,mutationStarted:$mutationStarted,diskMayBeModified:$diskMayBeModified,installed:$installed,message:$message}' \
    > "$temporary"
  chmod 0600 "$temporary"
  mv -fT -- "$temporary" "$directory/receipt.json"
  sync -f "$directory/receipt.json" 2>/dev/null || true
}

run_failure_receipt() {
  local status_code=$?
  if (( status_code != 0 )) && [[ "${RUN_RECEIPT_ARMED:-false}" == true ]]; then
    write_receipt "$RUN_OPERATION_DIR" "$RUN_OPERATION_ID" failed "$RUN_PHASE" \
      "$RUN_MUTATION_STARTED" "$RUN_INSTALLED" \
      "remote installer failed; inspect the private operation log"
  fi
  return "$status_code"
}

validate_plan() {
  local plan_file=$1
  jq -e '
    type == "object" and
    (keys | sort) == (["adminPublicKey","bootId","cache","deploymentRevision","disk","host","hostKeyPublic","hostKeyRotation","operationId","schemaVersion","systemPath"] | sort) and
    .schemaVersion == 1 and
    (.operationId | type == "string" and test("^[0-9a-f]{32}$")) and
    (.bootId | type == "string" and test("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")) and
    (.deploymentRevision | type == "string" and test("^[0-9a-f]{40,64}$")) and
    (.systemPath | type == "string" and test("^/nix/store/[0-9a-z]{32}-nixos-system-pc[0-9]{2}-[^/[:space:]]+$")) and
    (.adminPublicKey | type == "string" and length <= 1024 and test("^ssh-ed25519 [A-Za-z0-9+/=]+( .*)?$")) and
    (.hostKeyPublic | type == "string" and length <= 1024 and test("^ssh-ed25519 [A-Za-z0-9+/=]+( .*)?$")) and
    (.hostKeyRotation | type == "boolean") and
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

dispatch_plan() {
  local plan_file operation_id operation_dir operation_root remote_program log_file

  umask 077
  plan_file=$(mktemp)
  trap 'rm -f -- "$plan_file"' RETURN
  read_and_validate_plan "$plan_file"
  operation_id=$(jq -r .operationId "$plan_file")
  operation_root=$OPERATION_ROOT
  operation_dir="$operation_root/$operation_id"
  remote_program=${NIXORIUM_REMOTE_PROGRAM:-}
  if [[ "${NIXORIUM_TESTING:-}" == 1 ]]; then
    [[ -x "$remote_program" ]] || { echo "test remote installer entrypoint is unavailable" >&2; return 1; }
  elif [[ ! "$remote_program" =~ ^/nix/store/[0-9a-z]{32}-[^/[:space:]]+/bin/nixorium-remote-client-installer$ || ! -x "$remote_program" ]]; then
    echo "immutable remote installer entrypoint is unavailable" >&2
    return 1
  fi
  if [[ -e "$operation_root" || -L "$operation_root" ]]; then
    [[ -d "$operation_root" && ! -L "$operation_root" && "$(stat -c '%u:%g:%a' "$operation_root")" == "$OPERATION_OWNER:700" ]] || {
      echo "remote operation root is unsafe" >&2
      return 1
    }
  else
    if [[ "${NIXORIUM_TESTING:-}" == 1 ]]; then
      install -d -m 0700 "$operation_root"
    else
      install -d -o root -g root -m 0700 "$operation_root"
    fi
  fi
  if ! mkdir -m 0700 "$operation_dir" 2>/dev/null; then
    [[ -d "$operation_dir" && ! -L "$operation_dir" && "$(stat -c '%u:%g:%a' "$operation_dir")" == "$OPERATION_OWNER:700" ]] || {
      echo "existing remote operation directory is unsafe" >&2
      return 1
    }
    [[ -f "$operation_dir/plan.json" && ! -L "$operation_dir/plan.json" ]] || {
      echo "existing remote operation has no safe plan" >&2
      return 1
    }
    cmp -s -- "$plan_file" "$operation_dir/plan.json" || {
      echo "operation ID already belongs to a different plan" >&2
      return 1
    }
    status "$operation_id"
    return 0
  fi
  install -m 0600 "$plan_file" "$operation_dir/plan.json"
  log_file="$operation_dir/operation.log"
  install -m 0600 /dev/null "$log_file"
  write_receipt "$operation_dir" "$operation_id" accepted preflight false false \
    "reviewed operation accepted by the independent live installer job"
  if ! "$SYSTEMD_RUN" \
    --unit="nixorium-remote-install-$operation_id" --collect --no-block --quiet \
    --property=Type=exec --property=Restart=no --property=TimeoutStartSec=infinity \
    --property="StandardOutput=append:$log_file" --property="StandardError=append:$log_file" \
    "$remote_program" run "$operation_id"; then
    write_receipt "$operation_dir" "$operation_id" failed preflight false false \
      "could not start the independent live installer job"
    return 1
  fi
  status "$operation_id"
}

run_plan() {
  local plan_file operation_id operation_dir boot_id selected_disk expected_identity
  local cache_url cache_key system_path host_name host_interface static_ip live_ip
  local admin_key host_key deployment_revision current_boot current_host_key planned_host_key bundle_admin bundle_cache
  local admin_key_found authorized_keys_path
  local reasons

  umask 077
  operation_id=${1:-}
  [[ "$operation_id" =~ ^[0-9a-f]{32}$ ]] || { echo "invalid operation id" >&2; return 2; }
  operation_dir="$OPERATION_ROOT/$operation_id"
  [[ -d "$operation_dir" && ! -L "$operation_dir" && "$(stat -c '%u:%g:%a' "$operation_dir")" == "$OPERATION_OWNER:700" ]] || {
    echo "remote operation directory is unsafe" >&2
    return 1
  }
  RUN_RECEIPT_ARMED=true
  RUN_OPERATION_DIR=$operation_dir
  RUN_OPERATION_ID=$operation_id
  RUN_PHASE=preflight
  RUN_MUTATION_STARTED=false
  RUN_INSTALLED=false
  trap run_failure_receipt EXIT
  plan_file="$operation_dir/plan.json"
  [[ -f "$plan_file" && ! -L "$plan_file" && "$(stat -c '%u:%g:%a' "$plan_file")" == "$OPERATION_OWNER:600" ]] || {
    echo "remote operation plan is unsafe" >&2
    return 1
  }
  if ! mkdir -m 000 "$operation_dir/execution.claim" 2>/dev/null; then
    status "$operation_id"
    return 0
  fi
  validate_plan "$plan_file"
  validate_plan_semantics "$plan_file"
  [[ "$(jq -r .operationId "$plan_file")" == "$operation_id" ]] || {
    echo "remote operation plan identity differs" >&2
    return 1
  }
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
  write_receipt "$operation_dir" "$operation_id" running "$RUN_PHASE" false false "preflight started"

  current_boot=$(tr -d '\n' < /proc/sys/kernel/random/boot_id)
  [[ "$current_boot" == "$boot_id" ]] || { echo "live boot ID changed" >&2; return 1; }
  [[ "$live_ip" != "$static_ip" ]] || { echo "live and static client addresses conflict" >&2; return 1; }
  ip link show dev "$host_interface" >/dev/null
  ip -j -4 address show dev "$host_interface" | jq -e --arg address "$live_ip" \
    'any(.[]?.addr_info[]?; .local == $address)' >/dev/null

  current_host_key=$(nixorium_canonical_ed25519_public_key "$(< /etc/ssh/ssh_host_ed25519_key.pub)") || {
    echo "live host key is invalid" >&2
    return 1
  }
  planned_host_key=$(nixorium_canonical_ed25519_public_key "$host_key") || {
    echo "reviewed live host key is invalid" >&2
    return 1
  }
  [[ "$current_host_key" == "$planned_host_key" ]] || { echo "live host key changed" >&2; return 1; }
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

  RUN_PHASE=revalidate
  nix --extra-experimental-features "nix-command flakes" path-info \
    --store "$cache_url" --recursive "$system_path" \
    --option trusted-public-keys "$cache_key" --option require-sigs true \
    --option fallback false >/dev/null

  RUN_PHASE=partition
  RUN_MUTATION_STARTED=true
  write_receipt "$operation_dir" "$operation_id" running "$RUN_PHASE" true false "disk mutation started"
  (umask 022; nixorium_run_disko "$NIXORIUM_DISKO_SCRIPT" "$selected_disk")

  RUN_PHASE=install
  nixos-install --system "$system_path" \
    --option substituters "$cache_url" \
    --option trusted-public-keys "$cache_key" \
    --option require-sigs true \
    --option fallback false \
    --option builders "" \
    --no-channel-copy --no-root-passwd

  RUN_PHASE=verify
  install -D -o root -g root -m 0600 /etc/ssh/ssh_host_ed25519_key /mnt/etc/ssh/ssh_host_ed25519_key
  install -D -o root -g root -m 0644 /etc/ssh/ssh_host_ed25519_key.pub /mnt/etc/ssh/ssh_host_ed25519_key.pub

  nixorium_verify_installed_profile "$system_path" "$selected_disk"
  admin_key_found=false
  for authorized_keys_path in /mnt/etc/ssh/authorized_keys.d /mnt/root/.ssh/authorized_keys; do
    [[ -e "$authorized_keys_path" ]] || continue
    if grep -R -F -x -- "$admin_key" "$authorized_keys_path" >/dev/null 2>&1; then
      admin_key_found=true
      break
    fi
  done
  [[ "$admin_key_found" == true ]] || {
    echo "declared admin SSH key is absent from target" >&2
    return 1
  }
  grep -R -F -- "$deployment_revision" /mnt/etc /mnt/nix/var/nix/profiles/system 2>/dev/null >/dev/null || true

  RUN_INSTALLED=true
  RUN_PHASE=ready-to-reboot
  write_receipt "$operation_dir" "$operation_id" ready-to-reboot "$RUN_PHASE" true true \
    "installation completed; remove or deprioritize the USB before reboot"
  trap - EXIT
}

reboot_operation() {
  local operation_id=$1 operation_dir receipt
  [[ "$operation_id" =~ ^[0-9a-f]{32}$ ]] || { echo "invalid operation id" >&2; return 2; }
  operation_dir="$OPERATION_ROOT/$operation_id"
  receipt="$operation_dir/receipt.json"
  [[ -f "$receipt" && ! -L "$receipt" ]] || { echo "operation receipt is unavailable" >&2; return 1; }
  jq -e '.operationId == $id and .state == "ready-to-reboot" and .phase == "ready-to-reboot" and .installed == true and .mutationStarted == true' \
    --arg id "$operation_id" "$receipt" >/dev/null || {
      echo "operation is not ready for an explicit reboot" >&2
      return 1
    }
  write_receipt "$operation_dir" "$operation_id" reboot-requested reboot true true \
    "reboot requested; waiting for post-boot verification at the reviewed static address"
  /run/current-system/sw/bin/systemctl reboot --no-block
  status "$operation_id"
}

operation_log() {
  local operation_id=$1 log_file size
  [[ "$operation_id" =~ ^[0-9a-f]{32}$ ]] || { echo "invalid operation id" >&2; return 2; }
  log_file="$OPERATION_ROOT/$operation_id/operation.log"
  [[ -f "$log_file" && ! -L "$log_file" ]] || { echo "operation log is unavailable" >&2; return 1; }
  size=$(stat -c %s -- "$log_file")
  (( size <= 1048576 )) || { echo "operation log exceeds the transfer limit" >&2; return 1; }
  cat -- "$log_file"
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
    dispatch_plan
    ;;
  run)
    [[ $# -eq 2 ]] || exit 2
    run_plan "$2"
    ;;
  reboot)
    [[ $# -eq 2 ]] || exit 2
    reboot_operation "$2"
    ;;
  log)
    [[ $# -eq 2 ]] || exit 2
    operation_log "$2"
    ;;
  validate-plan)
    [[ $# -eq 1 ]] || exit 2
    validate_plan_command
    ;;
  *)
    echo "usage: nixorium-remote-client-installer {probe|validate-plan|apply|run OPERATION_ID|status OPERATION_ID|reboot OPERATION_ID|log OPERATION_ID}" >&2
    exit 2
    ;;
esac
