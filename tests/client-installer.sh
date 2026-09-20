#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=${NIXORIUM_TEST_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}

new_fixture() {
  FIXTURE_DIR=$(mktemp -d)
  printf '%s\n' 'cache.example:test-key' > "${FIXTURE_DIR}/public-key"
  printf '%s\n' '#!/usr/bin/env bash' > "${FIXTURE_DIR}/disko-install"
  chmod +x "${FIXTURE_DIR}/disko-install"
  ACTION_LOG="${FIXTURE_DIR}/actions"
}

load_test_installer() {
  # shellcheck source=../setup.sh
  source "${REPO_ROOT}/setup.sh"
  PUBLIC_KEY_FILE="${FIXTURE_DIR}/public-key"
  DISKO_INSTALL_SCRIPT="${FIXTURE_DIR}/disko-install"

  load_lab_meta() {
    LAB_META_SCHEMA_VERSION=2
    LAB_CONTROLLER_DHCP_IP=192.0.2.10
    LAB_CACHE_PORT=5000
    LAB_STUDENT_USER=student
    LAB_CLIENT_HOSTS_JSON='[{"name":"pc01","ip":"10.0.0.1"},{"name":"pc02","ip":"10.0.0.2"}]'
    export LAB_META_SCHEMA_VERSION LAB_CONTROLLER_DHCP_IP LAB_CACHE_PORT
    export LAB_STUDENT_USER LAB_CLIENT_HOSTS_JSON
  }
  require_deployment_ready() { return 0; }
  require_uefi() { return 0; }
  display_hardware() { echo "Detected client hardware fixture"; }
  prompt_input() {
    local _PROMPT="$1"
    local TARGET_VAR="$2"
    IFS= read -r "$TARGET_VAR"
  }
  collect_available_disks() { AVAILABLE_DISKS=(/dev/vda /dev/vdb); }
  list_disks() {
    echo "  1) /dev/vda 20G Fixture-A"
    echo "  2) /dev/vdb 40G Fixture-B"
  }
  disk_identity() {
    case "$1" in
      /dev/vda) echo "252:0" ;;
      /dev/vdb) echo "252:16" ;;
      *) return 1 ;;
    esac
  }
  disk_size() {
    case "$1" in
      /dev/vda) echo "21474836480" ;;
      /dev/vdb) echo "42949672960" ;;
      *) return 1 ;;
    esac
  }
  resolve_system() {
    SELECTED_SYSTEM_PATH=/nix/store/00000000000000000000000000000000-nixos-system-${SELECTED_HOST}-test
    SYSTEM_CLOSURE_BYTES=10737418240
    REQUIRED_DISK_BYTES=$((SYSTEM_CLOSURE_BYTES + INSTALL_HEADROOM_BYTES))
  }
  probe_host_identity() { return 1; }
  partition_disk() {
    echo "partition:${SELECTED_DISK}" >> "$ACTION_LOG"
  }
  install_system() { echo "install:${SELECTED_HOST}" >> "$ACTION_LOG"; }
  verify_installation() { echo "verify" >> "$ACTION_LOG"; }
  reboot_system() { echo "reboot" >> "$ACTION_LOG"; }
}

test_guided_success_and_reboot() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  load_test_installer

  OUTPUT=$(main <<'EOF'
2
2
ERASE
REBOOT
EOF
  )

  grep -q 'Nixorium guided client enrollment' <<< "$OUTPUT"
  grep -q 'Selected identity: pc02 (10.0.0.2)' <<< "$OUTPUT"
  grep -q 'best-effort check, not a reservation' <<< "$OUTPUT"
  grep -q 'DESTRUCTIVE REVIEW' <<< "$OUTPUT"
  grep -q '\[1/3\] Partitioning /dev/vdb' <<< "$OUTPUT"
  grep -q '\[2/3\] Installing pc02' <<< "$OUTPUT"
  grep -q '\[3/3\] Verifying' <<< "$OUTPUT"
  grep -q 'SUCCESS: pc02 is installed on /dev/vdb' <<< "$OUTPUT"
  test "$(printf '%s\n' partition:/dev/vdb install:pc02 verify reboot)" = "$(cat "$ACTION_LOG")"
)

test_reachable_identity_is_refused() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  load_test_installer
  probe_host_identity() { return 0; }

  set +e
  OUTPUT=$(main pc01 /dev/vda 2>&1)
  STATUS=$?
  set -e

  test "$STATUS" -eq 1
  grep -q 'already responding on the network' <<< "$OUTPUT"
  grep -q 'Refusing this identity' <<< "$OUTPUT"
  test ! -e "$ACTION_LOG"
)

test_inexact_confirmation_changes_nothing() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  load_test_installer

  set +e
  OUTPUT=$(main pc01 /dev/vda <<'EOF' 2>&1
YES
EOF
  )
  STATUS=$?
  set -e

  test "$STATUS" -eq 1
  grep -q 'Installation cancelled; no disk operation was started' <<< "$OUTPUT"
  test ! -e "$ACTION_LOG"
)

test_disk_identity_change_is_refused() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  load_test_installer
  ID_CALLS="${FIXTURE_DIR}/identity-calls"
  disk_identity() {
    local COUNT=0
    if [[ -f "$ID_CALLS" ]]; then
      COUNT=$(cat "$ID_CALLS")
    fi
    COUNT=$((COUNT + 1))
    echo "$COUNT" > "$ID_CALLS"
    if [[ "$COUNT" -eq 1 ]]; then
      echo "252:0"
    else
      echo "252:99"
    fi
  }

  set +e
  OUTPUT=$(main pc01 /dev/vda <<'EOF' 2>&1
ERASE
EOF
  )
  STATUS=$?
  set -e

  test "$STATUS" -eq 1
  grep -q 'changed after review; refusing to erase it' <<< "$OUTPUT"
  test ! -e "$ACTION_LOG"
)

test_failure_reports_modified_disk() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  load_test_installer
  install_system() { return 42; }

  set +e
  (
    main pc02 /dev/vdb <<'EOF'
ERASE

EOF
  ) > "${FIXTURE_DIR}/failure-output" 2>&1
  STATUS=$?
  set -e

  test "$STATUS" -eq 1
  grep -q 'Installation FAILED during NixOS installation' "${FIXTURE_DIR}/failure-output"
  grep -q 'The disk may have been modified' "${FIXTURE_DIR}/failure-output"
  grep -q '^partition:/dev/vdb$' "$ACTION_LOG"
  ! grep -q '^verify$' "$ACTION_LOG"
)

test_embedded_metadata_avoids_nix_evaluation() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  printf '%s\n' '{}' > "${FIXTURE_DIR}/flake.nix"
  printf '%s\n' '{"schemaVersion":2,"controller":{"name":"pc99","number":99,"staticIp":"10.0.0.99","dhcpIp":"192.0.2.10"},"clients":{"count":2,"hosts":[{"name":"pc01","ip":"10.0.0.1"},{"name":"pc02","ip":"10.0.0.2"}]},"network":{"base":"10.0.0.0","prefixLength":24,"ifaceName":"enp1s0","cachePort":5000,"pxeHttpPort":8080},"users":{"student":"student","teacher":"teacher"}}' > "${FIXTURE_DIR}/lab-meta.json"
  # shellcheck source=../scripts/lib/lab-meta.sh
  source "${REPO_ROOT}/scripts/lib/lab-meta.sh"
  nix() { return 99; }

  load_lab_meta "$FIXTURE_DIR"
  test "$LAB_CONTROLLER_DHCP_IP" = "192.0.2.10"
  test "$LAB_CLIENT_HOSTS_JSON" = '[{"name":"pc01","ip":"10.0.0.1"},{"name":"pc02","ip":"10.0.0.2"}]'
)

test_runtime_controller_address_overrides_embedded_hint() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  load_test_installer
  printf '%s\n' 'init=/nix/store/test-init quiet nixorium.controller-dhcp-ip=192.0.2.11' > "${FIXTURE_DIR}/cmdline"

  apply_runtime_controller_address "${FIXTURE_DIR}/cmdline"
  test "$LAB_CONTROLLER_DHCP_IP" = "192.0.2.11"

  printf '%s\n' 'nixorium.controller-dhcp-ip=999.0.2.11' > "${FIXTURE_DIR}/cmdline"
  ! apply_runtime_controller_address "${FIXTURE_DIR}/cmdline" >/dev/null 2>&1
)

test_too_small_disk_is_refused_before_mutation() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  load_test_installer
  disk_size() { echo "10737418240"; }

  set +e
  OUTPUT=$(main pc01 /dev/vda 2>&1)
  STATUS=$?
  set -e

  test "$STATUS" -eq 1
  grep -q "is too small for pc01" <<< "$OUTPUT"
  grep -q "Required: 12GiB; available: 10GiB" <<< "$OUTPUT"
  test ! -e "$ACTION_LOG"
)

test_offline_system_resolution_precedes_system_install() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  load_test_installer
  # Reload the real command after the common fixture installs behavior mocks.
  source "${REPO_ROOT}/setup.sh"
  SELECTED_HOST=pc01
  LAB_CONTROLLER_DHCP_IP=192.0.2.10
  LAB_CACHE_PORT=5000
  CACHE_KEY=cache.example:test-key
  nix() {
    if [[ " $* " == *" eval "* ]]; then
      printf '%s' /nix/store/00000000000000000000000000000000-nixos-system-pc01-test
      printf '%s\n' "$@" > "${FIXTURE_DIR}/nix-eval-arguments"
    else
      printf '%s\t%s\n' /nix/store/00000000000000000000000000000000-nixos-system-pc01-test 10737418240
      printf '%s\n' "$@" > "${FIXTURE_DIR}/nix-path-info-arguments"
    fi
  }
  sudo() { printf '%s\n' "$@" > "$ACTION_LOG"; }

  resolve_system
  install_system
  grep -Fxq 'eval' "${FIXTURE_DIR}/nix-eval-arguments"
  grep -Fxq "${REPO_ROOT}#nixosConfigurations.pc01.config.system.build.toplevel.outPath" "${FIXTURE_DIR}/nix-eval-arguments"
  grep -Fxq -- '--offline' "${FIXTURE_DIR}/nix-eval-arguments"
  grep -Fxq 'path-info' "${FIXTURE_DIR}/nix-path-info-arguments"
  grep -Fxq -- '--closure-size' "${FIXTURE_DIR}/nix-path-info-arguments"
  grep -Fxq -- '--offline' "${FIXTURE_DIR}/nix-path-info-arguments"
  test "$SYSTEM_CLOSURE_BYTES" -eq 10737418240
  test "$REQUIRED_DISK_BYTES" -eq 12884901888
  test "$(printf '%s\n' nixos-install --system /nix/store/00000000000000000000000000000000-nixos-system-pc01-test --option substituters http://192.0.2.10:5000 --option trusted-public-keys "$CACHE_KEY" --option fallback false --no-channel-copy --no-root-passwd)" = "$(cat "$ACTION_LOG")"
  ! grep -q -- '--flake' "$ACTION_LOG"
)

test_precompiled_disko_receives_validated_basename() (
  new_fixture
  trap 'rm -rf "$FIXTURE_DIR"' EXIT
  # shellcheck source=../setup.sh
  source "${REPO_ROOT}/setup.sh"
  DISKO_INSTALL_SCRIPT="${FIXTURE_DIR}/disko-install"
  SELECTED_DISK=/dev/nvme0n1
  sudo() { printf '%s\n' "$@" > "$ACTION_LOG"; }

  partition_disk
  test "$(printf '%s\n' env NIXORIUM_INSTALL_DISK=nvme0n1 "${FIXTURE_DIR}/disko-install" --yes-wipe-all-disks)" = "$(cat "$ACTION_LOG")"
)

test_guided_success_and_reboot
test_reachable_identity_is_refused
test_inexact_confirmation_changes_nothing
test_disk_identity_change_is_refused
test_failure_reports_modified_disk
test_too_small_disk_is_refused_before_mutation
test_embedded_metadata_avoids_nix_evaluation
test_runtime_controller_address_overrides_embedded_hint
test_offline_system_resolution_precedes_system_install
test_precompiled_disko_receives_validated_basename

echo "Client installer tests passed."
