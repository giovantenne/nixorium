#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=${NIXORIUM_TEST_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}
# shellcheck source=../scripts/lib/client-installer.sh
source "$REPO_ROOT/scripts/lib/client-installer.sh"

mock_lsblk() {
  case "$*" in
    "-dnro TYPE -- /dev/sdz") echo disk ;;
    "-dnro RO -- /dev/sdz") echo "${MOCK_RO:-0}" ;;
    "-bdnro SIZE -- /dev/sdz") echo "${MOCK_SIZE:-21474836480}" ;;
    "-dnro RM -- /dev/sdz") echo "${MOCK_RM:-0}" ;;
    "-dnro KNAME -- /dev/sdz") echo sdz ;;
    "-dnro MAJ:MIN -- /dev/sdz") echo 65:144 ;;
    "-dnro SERIAL -- /dev/sdz") echo SERIAL-1 ;;
    "-dnro WWN -- /dev/sdz") echo WWN-1 ;;
    "-dnro MODEL -- /dev/sdz") echo "Fixture Disk" ;;
    "-dnro TRAN -- /dev/sdz") echo sata ;;
    "-nro KNAME -- /dev/sdz") echo sdz ;;
    "-snpo PATH,TYPE -- /dev/sdz2") printf '%s\n' "/dev/sdz2 part" "/dev/sdz disk" ;;
    *) echo "unexpected lsblk arguments: $*" >&2; return 99 ;;
  esac
}

test_identity_is_content_bound() (
  lsblk() { mock_lsblk "$@"; }
  first=$(nixorium_disk_identity_json /dev/sdz)
  jq -e '.path == "/dev/sdz" and .majorMinor == "65:144" and .sizeBytes == 21474836480 and .serial == "SERIAL-1" and .transport == "sata"' <<<"$first" >/dev/null
  nixorium_disk_matches_identity /dev/sdz "$first"
  MOCK_SIZE=21474836481
  export MOCK_SIZE
  ! nixorium_disk_matches_identity /dev/sdz "$first"
)

test_exclusions_are_additive() (
  lsblk() { mock_lsblk "$@"; }
  nixorium_disk_has_mounts() { return 0; }
  nixorium_disk_has_swap() { return 0; }
  nixorium_disk_has_holders() { return 0; }
  nixorium_backing_disks_for_mount() { echo /dev/sdz; }
  MOCK_RO=1
  MOCK_RM=1
  MOCK_SIZE=1024
  export MOCK_RO MOCK_RM MOCK_SIZE
  reasons=$(nixorium_disk_exclusion_reasons_json /dev/sdz 8589934592)
  jq -e 'sort == ["holders-active","live-media","mounted","read-only","removable","swap-active","too-small"]' <<<"$reasons" >/dev/null
)

test_safe_disk_has_no_exclusion() (
  lsblk() { mock_lsblk "$@"; }
  nixorium_disk_has_mounts() { return 1; }
  nixorium_disk_has_swap() { return 1; }
  nixorium_disk_has_holders() { return 1; }
  nixorium_backing_disks_for_mount() { :; }
  test "$(nixorium_disk_exclusion_reasons_json /dev/sdz 8589934592)" = "[]"
)

test_exact_profile_and_parent_are_required() (
  fixture=$(mktemp -d)
  trap 'rm -rf "$fixture"' EXIT
  expected="$fixture/system"
  mkdir -p "$expected" "$fixture/root/nix/var/nix/profiles" "$fixture/root/boot/EFI/NixOS-boot"
  ln -s "$expected" "$fixture/root/nix/var/nix/profiles/system"
  touch "$fixture/root/boot/EFI/NixOS-boot/grubx64.efi"
  findmnt() { echo '/dev/sdz2[/@root]'; }
  lsblk() { mock_lsblk "$@"; }
  sync() { :; }
  nixorium_verify_installed_profile "$expected" /dev/sdz "$fixture/root"
  ! nixorium_verify_installed_profile "$fixture/other-system" /dev/sdz "$fixture/root" >/dev/null 2>&1
  ! nixorium_verify_installed_profile "$expected" /dev/sdy "$fixture/root" >/dev/null 2>&1
)

test_identity_is_content_bound
test_exclusions_are_additive
test_safe_disk_has_no_exclusion
test_exact_profile_and_parent_are_required

echo "Client installer library tests passed."
