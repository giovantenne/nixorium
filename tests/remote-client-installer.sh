#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=${NIXORIUM_TEST_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}
HELPER="$REPO_ROOT/scripts/remote-client-installer.sh"
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
    hostKeyPublic:"ssh-ed25519 YWJjZA== root@test"
  }'
}

test_valid_plan() {
  test "$(valid_plan | "$HELPER" validate-plan)" = valid
}

test_unknown_and_trailing_fields_are_rejected() {
  ! valid_plan | jq '.unexpected = true' | "$HELPER" validate-plan >/dev/null 2>&1
  ! { valid_plan; printf '%s\n' '{}'; } | "$HELPER" validate-plan >/dev/null 2>&1
}

test_noncanonical_or_unsafe_addresses_are_rejected() {
  ! valid_plan | jq '.host.liveIp = "192.0.2.020"' | "$HELPER" validate-plan >/dev/null 2>&1
  ! valid_plan | jq '.host.liveIp = "169.254.1.2"' | "$HELPER" validate-plan >/dev/null 2>&1
  ! valid_plan | jq '.cache.url = "http://999.0.2.10:5000"' | "$HELPER" validate-plan >/dev/null 2>&1
  ! valid_plan | jq '.cache.url = "http://192.0.2.10:99999"' | "$HELPER" validate-plan >/dev/null 2>&1
}

test_cross_field_mismatch_is_rejected() {
  ! valid_plan | jq '.host.name = "pc02"' | "$HELPER" validate-plan >/dev/null 2>&1
  ! valid_plan | jq '.host.staticIp = .host.liveIp' | "$HELPER" validate-plan >/dev/null 2>&1
}

test_size_limit_precedes_parsing() {
  ! head -c 65537 /dev/zero | tr '\0' x | "$HELPER" validate-plan >/dev/null 2>&1
}

test_status_id_is_not_a_path() {
  ! "$HELPER" status ../escape >/dev/null 2>&1
}

test_valid_plan
test_unknown_and_trailing_fields_are_rejected
test_noncanonical_or_unsafe_addresses_are_rejected
test_cross_field_mismatch_is_rejected
test_size_limit_precedes_parsing
test_status_id_is_not_a_path

echo "Remote client installer tests passed."
