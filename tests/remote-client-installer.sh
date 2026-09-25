#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=${NIXORIUM_TEST_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}
HELPER="$REPO_ROOT/scripts/remote-client-installer.sh"
HELPER_COMMAND=(bash "$HELPER")
export NIXORIUM_INSTALLER_LIB="$REPO_ROOT/scripts/lib/client-installer.sh"

valid_plan() {
  jq -cn '{
    schemaVersion:1,
    operationId:"0123456789abcdef0123456789abcdef",
    bootId:"01234567-89ab-cdef-0123-456789abcdef",
    deploymentRevision:"0123456789abcdef0123456789abcdef01234567",
    systemPath:"/nix/store/00000000000000000000000000000000-nixos-system-pc01-test",
    host:{name:"pc01",interface:"enp0s2",liveIp:"192.0.2.20",staticIp:"10.0.0.1"},
    cache:{url:"http://192.0.2.10:5000",publicKey:"cache.example:YWJjZA=="},
    disk:{path:"/dev/sda",kname:"sda",majorMinor:"8:0",sizeBytes:17179869184,serial:"serial",wwn:"",model:"disk",transport:"sata",diskSeq:"1"},
    adminPublicKey:"ssh-ed25519 YWJjZA== admin@test",
    hostKeyPublic:"ssh-ed25519 YWJjZA== root@test",
    hostKeyRotation:false
  }'
}

test_valid_plan() {
  test "$(valid_plan | "${HELPER_COMMAND[@]}" validate-plan)" = valid
}

test_unknown_and_trailing_fields_are_rejected() {
  ! valid_plan | jq '.unexpected = true' | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
  ! { valid_plan; printf '%s\n' '{}'; } | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
}

test_duplicate_fields_are_rejected_by_compiled_validator() {
  [[ -n "${NIXORIUM_PLAN_VALIDATOR:-}" ]] || return 0
  ! valid_plan | sed 's/"schemaVersion":1/"schemaVersion":1,"schemaVersion":1/' | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
}

test_noncanonical_or_unsafe_addresses_are_rejected() {
  ! valid_plan | jq '.host.liveIp = "192.0.2.020"' | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
  ! valid_plan | jq '.host.liveIp = "169.254.1.2"' | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
  ! valid_plan | jq '.cache.url = "http://999.0.2.10:5000"' | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
  ! valid_plan | jq '.cache.url = "http://192.0.2.10:99999"' | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
}

test_cross_field_mismatch_is_rejected() {
  ! valid_plan | jq '.host.name = "pc02"' | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
  ! valid_plan | jq '.host.staticIp = .host.liveIp' | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
}

test_size_limit_precedes_parsing() {
  ! head -c 65537 /dev/zero | tr '\0' x | "${HELPER_COMMAND[@]}" validate-plan >/dev/null 2>&1
}

test_status_id_is_not_a_path() {
  ! "${HELPER_COMMAND[@]}" status ../escape >/dev/null 2>&1
}

test_apply_dispatch_is_idempotent_and_independent() {
  local fixture receipt
  fixture=$(mktemp -d)
  trap 'rm -rf -- "$fixture"' RETURN
  install -m 0700 "$HELPER" "$fixture/remote-program"
  cat > "$fixture/systemd-run" <<SCRIPT
#!$(command -v bash)
set -euo pipefail
printf '%s\n' "\$*" >> "\$NIXORIUM_TEST_SYSTEMD_CALLS"
SCRIPT
  chmod 0700 "$fixture/systemd-run"
  export NIXORIUM_TESTING=1
  export NIXORIUM_OPERATION_ROOT="$fixture/operations"
  export NIXORIUM_SYSTEMD_RUN="$fixture/systemd-run"
  export NIXORIUM_REMOTE_PROGRAM="$fixture/remote-program"
  export NIXORIUM_TEST_SYSTEMD_CALLS="$fixture/systemd-calls"
  export NIXORIUM_OPERATION_OWNER="$(id -u):$(id -g)"
  receipt=$(valid_plan | "${HELPER_COMMAND[@]}" apply)
  jq -e '.state == "accepted" and .phase == "preflight" and .mutationStarted == false and .sequence == 1' <<< "$receipt" >/dev/null
  grep -F "nixorium-remote-install-0123456789abcdef0123456789abcdef" "$fixture/systemd-calls" >/dev/null
  grep -F "$fixture/remote-program run 0123456789abcdef0123456789abcdef" "$fixture/systemd-calls" >/dev/null
  test "$(wc -l < "$fixture/systemd-calls")" -eq 1
  valid_plan | "${HELPER_COMMAND[@]}" apply | jq -e '.state == "accepted"' >/dev/null
  test "$(wc -l < "$fixture/systemd-calls")" -eq 1
  ! valid_plan | jq '.disk.serial = "replacement"' | "${HELPER_COMMAND[@]}" apply >/dev/null 2>&1
  test "$(wc -l < "$fixture/systemd-calls")" -eq 1
  ! "${HELPER_COMMAND[@]}" run 0123456789abcdef0123456789abcdef >/dev/null 2>&1
  "${HELPER_COMMAND[@]}" status 0123456789abcdef0123456789abcdef |
    jq -e '.state == "failed" and .phase == "preflight" and .mutationStarted == false and .sequence >= 2' >/dev/null
  unset NIXORIUM_TESTING NIXORIUM_OPERATION_ROOT NIXORIUM_SYSTEMD_RUN NIXORIUM_REMOTE_PROGRAM NIXORIUM_TEST_SYSTEMD_CALLS NIXORIUM_OPERATION_OWNER
}

test_valid_plan
test_unknown_and_trailing_fields_are_rejected
test_duplicate_fields_are_rejected_by_compiled_validator
test_noncanonical_or_unsafe_addresses_are_rejected
test_cross_field_mismatch_is_rejected
test_size_limit_precedes_parsing
test_status_id_is_not_a_path
test_apply_dispatch_is_idempotent_and_independent

echo "Remote client installer tests passed."
