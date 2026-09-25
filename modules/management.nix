{ config, hostName, labSettings, nixoriumPackage, lib, pkgs, ... }:
let
  isController = hostName == labSettings.masterHostName;
  cfg = config.services.nixorium;
  operationGate = ''
    COORDINATION_DIRECTORY=/var/lib/nixorium/coordination
    COORDINATION_LOCK="$COORDINATION_DIRECTORY/operation.lock"
    USB_RESERVATION="$COORDINATION_DIRECTORY/usb-reservation.json"
    [[ -d "$COORDINATION_DIRECTORY" && ! -L "$COORDINATION_DIRECTORY" \
        && "$(stat -c '%U:%G:%a' "$COORDINATION_DIRECTORY")" == root:nixorium-operations:770 ]] \
      || fail "managed operation coordination directory is unsafe"
    [[ -f "$COORDINATION_LOCK" && ! -L "$COORDINATION_LOCK" \
        && "$(stat -c '%U:%G:%a' "$COORDINATION_LOCK")" == root:nixorium-operations:660 ]] \
      || fail "managed operation lock is unsafe"
    [[ ! -e "$USB_RESERVATION" && ! -L "$USB_RESERVATION" ]] \
      || fail "a USB installation remains reserved; reconcile it before starting another operation"
    exec 9<>"$COORDINATION_LOCK"
    flock -n 9 \
      || fail "another Nixorium controller or client operation is already running"
    [[ ! -e "$USB_RESERVATION" && ! -L "$USB_RESERVATION" ]] \
      || fail "a USB installation became reserved while acquiring the operation lock"
    LEGACY_LOCK=/home/admin/.local/state/nixorium/operations/deploy.lock
    if [[ -e "$LEGACY_LOCK" || -L "$LEGACY_LOCK" ]]; then
      [[ -f "$LEGACY_LOCK" && ! -L "$LEGACY_LOCK" \
          && "$(stat -c '%U:%G:%a' "$LEGACY_LOCK")" == admin:users:600 ]] \
        || fail "legacy deployment lock is unsafe; close old Nixorium processes before migration"
      exec 8<>"$LEGACY_LOCK"
      flock -n 8 \
        || fail "a legacy Nixorium deployment is still running"
    fi
  '';
  installSecrets = pkgs.writeShellApplication {
    name = "nixorium-install-secrets";
    runtimeInputs = [ pkgs.coreutils pkgs.diffutils pkgs.git pkgs.nix pkgs.util-linux nixoriumPackage ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}

      fail() {
        echo "Error: $*" >&2
        exit 1
      }

      ${operationGate}

      [[ -d "$REPOSITORY" && ! -L "$REPOSITORY" ]] \
        || fail "configured deployment path is not a real directory"
      [[ -f "$REPOSITORY/flake.nix" && -d "$REPOSITORY/.git" ]] \
        || fail "configured deployment path is not a Git Flake"
      [[ "$(stat -c '%U' "$REPOSITORY")" == admin ]] \
        || fail "deployment repository must be owned by admin"

      nixorium setup keys --verify-only --repo "$REPOSITORY" --json >/dev/null \
        || fail "deployment key correspondence verification failed"

      install_checked() {
        local source_path="$1"
        local target_path="$2"
        local mode="$3"
        local owner="$4"
        local group="$5"

        [[ -f "$source_path" && ! -L "$source_path" ]] \
          || fail "key source is not a regular non-symlink file: $source_path"
        if [[ -L "$target_path" ]]; then
          fail "refusing symlink destination: $target_path"
        fi
        if [[ -e "$target_path" ]]; then
          [[ -f "$target_path" ]] || fail "key destination is not a regular file: $target_path"
          cmp -s "$source_path" "$target_path" \
            || fail "refusing to replace different key material at $target_path"
        fi
        install -m "$mode" -o "$owner" -g "$group" "$source_path" "$target_path"
      }

      install -d -m 0700 -o admin -g users /home/admin/.ssh
      install -d -m 0750 -o root -g veyon-master /etc/veyon/keys/private/teacher
      install -d -m 0700 -o root -g root /var/lib/nixorium/keys

      install_checked "$REPOSITORY/admin-ssh" /home/admin/.ssh/id_ed25519 0600 admin users
      install_checked "$REPOSITORY/keys/admin-ssh.pub" /home/admin/.ssh/id_ed25519.pub 0644 admin users
      install_checked "$REPOSITORY/veyon-private-key.pem" /etc/veyon/keys/private/teacher/key 0640 root veyon-master
      install_checked "$REPOSITORY/secret-key" /var/lib/nixorium/keys/harmonia-secret-key 0600 root root
    '';
  };
  applyController = pkgs.writeShellApplication {
    name = "nixorium-apply-controller";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.diffutils
      pkgs.gawk
      pkgs.git
      pkgs.iproute2
      pkgs.jq
      pkgs.nix
      pkgs.util-linux
      nixoriumPackage
    ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}
      EXPECTED_REVISION="''${1:-}"

      STATE_DIRECTORY=/var/lib/nixorium/controller
      [[ -d "$STATE_DIRECTORY" && ! -L "$STATE_DIRECTORY" \
          && "$(stat -c '%U:%G:%a' "$STATE_DIRECTORY")" == root:root:755 ]] || {
        echo "Error: controller state directory has unsafe ownership or permissions" >&2
        exit 1
      }
      PROGRESS_FILE="$STATE_DIRECTORY/progress.json"
      PROGRESS_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
      PROGRESS_STATE=running
      PROGRESS_PHASE=starting
      PROGRESS_CURRENT=0
      PROGRESS_TOTAL=4
      PROGRESS_RECENT='[]'
      TEMPORARY_RECORD=""
      keep_temporary=false

      publish_progress() {
        local state="$1"
        local phase="$2"
        local activity="$3"
        local current="$4"
        local temporary

        PROGRESS_STATE="$state"
        PROGRESS_PHASE="$phase"
        PROGRESS_CURRENT="$current"
        PROGRESS_RECENT="$(jq -c --arg activity "$activity" \
          '. + [$activity] | if length > 5 then .[-5:] else . end' \
          <<<"$PROGRESS_RECENT")"
        temporary="$(mktemp "$STATE_DIRECTORY/.progress.XXXXXX")"
        jq -n \
          --arg operation controller-apply \
          --arg state "$PROGRESS_STATE" \
          --arg phase "$PROGRESS_PHASE" \
          --arg startedAt "$PROGRESS_STARTED_AT" \
          --arg updatedAt "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" \
          --argjson current "$PROGRESS_CURRENT" \
          --argjson total "$PROGRESS_TOTAL" \
          --argjson recent "$PROGRESS_RECENT" \
          '{schemaVersion: 1, operation: $operation, state: $state, phase: $phase,
            startedAt: $startedAt, updatedAt: $updatedAt, current: $current,
            total: $total, recent: $recent}' >"$temporary"
        chown admin:users "$temporary"
        chmod 0600 "$temporary"
        mv -fT -- "$temporary" "$PROGRESS_FILE"
        echo "$activity"
      }

      cleanup_controller() {
        local status="$?"
        if [[ "$keep_temporary" == true && -n "$TEMPORARY_RECORD" ]]; then
          rm -f -- "$TEMPORARY_RECORD"
        fi
        if [[ "$status" -ne 0 && "$PROGRESS_STATE" == running ]]; then
          publish_progress failed "$PROGRESS_PHASE" \
            "Controller apply stopped unexpectedly" "$PROGRESS_CURRENT" || true
        fi
      }
      trap cleanup_controller EXIT

      fail() {
        local message="$*"
        if [[ "$PROGRESS_STATE" == running ]]; then
          publish_progress failed "$PROGRESS_PHASE" \
            "Failed: $message" "$PROGRESS_CURRENT" || true
        fi
        echo "Error: $message" >&2
        exit 1
      }

      ${operationGate}

      publish_progress running starting "Starting controller apply" 0
      publish_progress running validate "Validating the reviewed controller configuration" 0

      [[ -d "$REPOSITORY" && ! -L "$REPOSITORY" ]] \
        || fail "configured deployment path is not a real directory"
      [[ -f "$REPOSITORY/flake.nix" && -d "$REPOSITORY/.git" ]] \
        || fail "configured deployment path is not a Git Flake"
      [[ "$(stat -c '%U' "$REPOSITORY")" == admin ]] \
        || fail "deployment repository must be owned by admin"
      [[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" status --porcelain=v1 --untracked-files=normal)" ]] \
        || fail "deployment worktree must be clean before controller apply"
      [[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" ls-files -- \
          secret-key admin-ssh veyon-private-key.pem)" ]] \
        || fail "private key files must not be tracked by Git"

      REVISION="$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" rev-parse HEAD)"
      [[ "$REVISION" =~ ^[0-9a-f]{40}$ ]] \
        || fail "deployment HEAD is not a full Git object ID"
      if [[ -n "$EXPECTED_REVISION" ]]; then
        [[ "$EXPECTED_REVISION" =~ ^[0-9a-f]{40}$ ]] \
          || fail "expected controller revision is invalid"
        [[ "$REVISION" == "$EXPECTED_REVISION" ]] \
          || fail "deployment revision differs from the reviewed controller plan"
      fi

      # Pin the Git fetcher to the reviewed commit so ignored private keys are
      # excluded and a concurrent clean commit cannot change the build input.
      FLAKE_URL="git+file://$REPOSITORY?rev=$REVISION"
      [[ -d /var/cache/nixorium/admin && ! -L /var/cache/nixorium/admin \
          && "$(stat -c '%U:%G:%a' /var/cache/nixorium/admin)" == admin:users:700 ]] \
        || fail "administrator Nix cache directory has unsafe ownership or permissions"
      export XDG_CACHE_HOME=/var/cache/nixorium/admin
      export NIX_CONFIG="experimental-features = nix-command flakes"

      as_admin() {
        runuser -u admin -- "$@"
      }

      as_admin nixorium config validate --repo "$REPOSITORY" --json \
        || fail "deployment configuration validation failed"
      CONTROLLER_READINESS="$(as_admin nix eval "$FLAKE_URL#deploymentStatus" --json --no-write-lock-file \
        | jq -ce '.controller // {ready: .ready, issues: .issues, requiresKeys: true}')"
      jq -e '.ready == true' <<<"$CONTROLLER_READINESS" >/dev/null \
        || fail "controller readiness must be true before controller apply"

      # Only an explicit capability from the pinned configuration can omit lab
      # keys. Old deployments continue to require all three installed pairs.
      if ! jq -e '.requiresKeys == false' <<<"$CONTROLLER_READINESS" >/dev/null; then
        as_admin nixorium setup keys --verify-only --repo "$REPOSITORY" --json \
          || fail "deployment key correspondence verification failed"
        cmp -s "$REPOSITORY/admin-ssh" /home/admin/.ssh/id_ed25519 \
          || fail "installed admin SSH key is absent or differs"
        cmp -s "$REPOSITORY/veyon-private-key.pem" /etc/veyon/keys/private/teacher/key \
          || fail "installed Veyon private key is absent or differs"
        cmp -s "$REPOSITORY/secret-key" /var/lib/nixorium/keys/harmonia-secret-key \
          || fail "installed Harmonia signing key is absent or differs"
      fi

      publish_progress running build "Validated controller prerequisites" 1
      publish_progress running build "Building the reviewed controller system" 1

      LAB_META="$(as_admin nix eval "$FLAKE_URL#labMeta" --json --no-write-lock-file)" \
        || fail "could not evaluate controller network metadata"
      CONTROLLER_NAME="$(jq -er '.controller.name' <<<"$LAB_META")" \
        || fail "evaluated controller name is missing"
      [[ "$CONTROLLER_NAME" =~ ^pc[0-9]+$ ]] \
        || fail "evaluated controller name is invalid"
      DEPLOYMENT_MODE="$(jq -er '.deploymentMode // "laboratory"' <<<"$LAB_META")" \
        || fail "evaluated deployment mode is missing"
      [[ "$DEPLOYMENT_MODE" == controller || "$DEPLOYMENT_MODE" == laboratory ]] \
        || fail "evaluated deployment mode is invalid"
      if [[ "$DEPLOYMENT_MODE" == laboratory ]]; then
        [[ ! -e /var/lib/nixorium/pxe/session.json \
            && ! -L /var/lib/nixorium/pxe/session.json ]] \
          || fail "PXE networking is transitioning or active; stop or recover installation mode before controller apply"
        CONTROLLER_IFACE="$(jq -er '.controller.ifaceName // .network.ifaceName' <<<"$LAB_META")" \
          || fail "evaluated controller interface is missing"
        CONTROLLER_STATIC_IP="$(jq -er '.controller.staticIp' <<<"$LAB_META")" \
          || fail "evaluated controller static address is missing"
        CONTROLLER_PREFIX_LENGTH="$(jq -er '.network.prefixLength' <<<"$LAB_META")" \
          || fail "evaluated network prefix is missing"
        [[ "$CONTROLLER_IFACE" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,14}$ ]] \
          || fail "evaluated controller interface is invalid"
        [[ "$CONTROLLER_STATIC_IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] \
          || fail "evaluated controller static address is invalid"
        [[ "$CONTROLLER_PREFIX_LENGTH" =~ ^[0-9]+$ \
            && "$CONTROLLER_PREFIX_LENGTH" -ge 1 \
            && "$CONTROLLER_PREFIX_LENGTH" -le 30 ]] \
          || fail "evaluated network prefix is invalid"
      fi

      SYSTEM_PATH="$(as_admin nix build "$FLAKE_URL#nixosConfigurations.$CONTROLLER_NAME.config.system.build.toplevel" \
        --no-write-lock-file --no-link --print-out-paths)"
      [[ "$SYSTEM_PATH" == /nix/store/* && "$SYSTEM_PATH" != *[[:space:]]* \
          && -x "$SYSTEM_PATH/bin/switch-to-configuration" ]] \
        || fail "controller build did not return one valid NixOS system closure"
      [[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" status --porcelain=v1 --untracked-files=normal)" \
          && "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" rev-parse HEAD)" == "$REVISION" ]] \
        || fail "deployment changed while the controller was building; activation refused"

      publish_progress running activate "Built controller system; beginning activation" 2
      ACTIVATION_RECORD="$STATE_DIRECTORY/applied.json"
      if [[ -L "$ACTIVATION_RECORD" ]]; then
        fail "refusing symlink controller activation record"
      fi
      if [[ -e "$ACTIVATION_RECORD" && ! -f "$ACTIVATION_RECORD" ]]; then
        fail "controller activation record is not a regular file"
      fi

      # Invalidate any earlier success before activation. A failed switch can
      # update /run/current-system before a later activation snippet fails, so
      # the active symlink alone is not durable evidence of completion.
      rm -f -- "$ACTIVATION_RECORD"
      "$SYSTEM_PATH/bin/switch-to-configuration" switch
      ACTIVE_SYSTEM="$(readlink -f /run/current-system)"
      [[ "$ACTIVE_SYSTEM" == "$SYSTEM_PATH" ]] \
        || fail "active system differs after controller activation"

      # A controller-only installation already has a live interface when the
      # first laboratory configuration is activated. The generated NixOS
      # address unit is persistent across reboot, but it is not guaranteed to
      # receive a fresh device event during a live switch. Reconcile the one
      # reviewed static address before PXE preparation is allowed to continue.
      if [[ "$DEPLOYMENT_MODE" == laboratory ]]; then
        CONTROLLER_STATIC_CIDR="$CONTROLLER_STATIC_IP/$CONTROLLER_PREFIX_LENGTH"
        ip link show dev "$CONTROLLER_IFACE" >/dev/null \
          || fail "configured controller interface is not present after activation"
        if ! ip -4 -o address show dev "$CONTROLLER_IFACE" scope global \
            | awk -v cidr="$CONTROLLER_STATIC_CIDR" \
              '$3 == "inet" && $4 == cidr { found = 1 } END { exit !found }'; then
          ip address replace "$CONTROLLER_STATIC_CIDR" dev "$CONTROLLER_IFACE" \
            || fail "could not apply the controller static address after activation"
        fi
        ip -4 -o address show dev "$CONTROLLER_IFACE" scope global \
          | awk -v cidr="$CONTROLLER_STATIC_CIDR" \
            '$3 == "inet" && $4 == cidr { found = 1 } END { exit !found }' \
          || fail "controller static address is absent after activation"
      fi

      publish_progress running verify "Activated controller system and verified networking" 3

      TEMPORARY_RECORD="$(mktemp --tmpdir="$STATE_DIRECTORY" .applied.json.XXXXXX)"
      keep_temporary=true
      jq -n \
        --arg revision "$REVISION" \
        --arg systemPath "$SYSTEM_PATH" \
        --arg activatedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{schemaVersion: 1, revision: $revision, systemPath: $systemPath, activatedAt: $activatedAt}' \
        > "$TEMPORARY_RECORD"
      chmod 0644 "$TEMPORARY_RECORD"
      sync "$TEMPORARY_RECORD"
      mv -fT -- "$TEMPORARY_RECORD" "$ACTIVATION_RECORD"
      keep_temporary=false
      sync "$STATE_DIRECTORY"
      publish_progress completed complete "Controller revision activated and verified" 4
      trap - EXIT
    '';
  };
  preparePxe = pkgs.writeShellApplication {
    name = "nixorium-prepare-pxe";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.curl
      pkgs.gawk
      pkgs.git
      pkgs.gnugrep
      pkgs.iproute2
      pkgs.jq
      pkgs.nix
      pkgs.util-linux
      nixoriumPackage
    ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}

      PROGRESS_FILE="$STATE_DIRECTORY/progress.json"
      PROGRESS_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
      PROGRESS_STATE=running
      PROGRESS_PHASE=starting
      PROGRESS_CURRENT=0
      PROGRESS_TOTAL=0
      PROGRESS_RECENT='[]'
      CLIENTS_FILE=""
      MANIFEST_TEMP=""

      publish_progress() {
        local state="$1"
        local phase="$2"
        local activity="$3"
        local current="$4"
        local total="$5"
        local temporary

        PROGRESS_STATE="$state"
        PROGRESS_PHASE="$phase"
        PROGRESS_CURRENT="$current"
        PROGRESS_TOTAL="$total"
        if [[ -n "$activity" ]]; then
          PROGRESS_RECENT="$(jq -c --arg activity "$activity" \
            '. + [$activity] | if length > 5 then .[-5:] else . end' \
            <<<"$PROGRESS_RECENT")"
        fi
        temporary="$(mktemp "$STATE_DIRECTORY/.progress.XXXXXX")"
        jq -n \
          --arg operation pxe-prepare \
          --arg state "$PROGRESS_STATE" \
          --arg phase "$PROGRESS_PHASE" \
          --arg startedAt "$PROGRESS_STARTED_AT" \
          --arg updatedAt "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" \
          --argjson current "$PROGRESS_CURRENT" \
          --argjson total "$PROGRESS_TOTAL" \
          --argjson recent "$PROGRESS_RECENT" \
          '{
            schemaVersion: 1,
            operation: $operation,
            state: $state,
            phase: $phase,
            startedAt: $startedAt,
            updatedAt: $updatedAt,
            current: $current,
            total: $total,
            recent: $recent
          }' >"$temporary"
        chmod 0600 "$temporary"
        mv -fT -- "$temporary" "$PROGRESS_FILE"
        [[ -z "$activity" ]] || echo "$activity"
      }

      cleanup() {
        local status="$?"
        [[ -z "$CLIENTS_FILE" ]] || rm -f -- "$CLIENTS_FILE"
        [[ -z "$MANIFEST_TEMP" ]] || rm -f -- "$MANIFEST_TEMP"
        if [[ "$status" -ne 0 && "$PROGRESS_STATE" == running ]]; then
          publish_progress failed "$PROGRESS_PHASE" \
            "Preparation stopped unexpectedly" "$PROGRESS_CURRENT" "$PROGRESS_TOTAL" || true
        fi
      }
      trap cleanup EXIT

      fail() {
        local message="$*"
        if [[ "$PROGRESS_STATE" == running ]]; then
          publish_progress failed "$PROGRESS_PHASE" \
            "Failed: $message" "$PROGRESS_CURRENT" "$PROGRESS_TOTAL" || true
        fi
        echo "Error: $message" >&2
        exit 1
      }

      ${operationGate}

      publish_progress running starting "Starting PXE preparation" 0 0
      publish_progress running validate "Validating the reviewed deployment" 0 0

      [[ -d "$REPOSITORY" && ! -L "$REPOSITORY" ]] \
        || fail "configured deployment path is not a real directory"
      [[ -f "$REPOSITORY/flake.nix" && -d "$REPOSITORY/.git" ]] \
        || fail "configured deployment path is not a Git Flake"
      [[ "$(stat -c '%U' "$REPOSITORY")" == admin ]] \
        || fail "deployment repository must be owned by admin"
      [[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" status --porcelain=v1 --untracked-files=normal)" ]] \
        || fail "deployment worktree must be clean before PXE preparation"
      [[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" ls-files -- \
          secret-key admin-ssh veyon-private-key.pem)" ]] \
        || fail "private key files must not be tracked by Git"

      FLAKE_URL="git+file://$REPOSITORY"
      [[ -d /var/cache/nixorium/admin && ! -L /var/cache/nixorium/admin \
          && "$(stat -c '%U:%G:%a' /var/cache/nixorium/admin)" == admin:users:700 ]] \
        || fail "administrator Nix cache directory has unsafe ownership or permissions"
      export XDG_CACHE_HOME=/var/cache/nixorium/admin
      export NIX_CONFIG="experimental-features = nix-command flakes"

      nixorium config validate --repo "$REPOSITORY" --json >/dev/null \
        || fail "deployment configuration validation failed"
      [[ "$(nix eval "$FLAKE_URL#deploymentStatus.ready" --json --no-write-lock-file)" == true ]] \
        || fail "deploymentStatus.ready must be true before PXE preparation"
      REVISION="$(git -C "$REPOSITORY" rev-parse HEAD)" \
        || fail "could not resolve deployment revision"

      publish_progress running validate "Validated deployment configuration" 0 0
      publish_progress running network "Checking controller network and binary cache" 0 0

      META="$(nix eval "$FLAKE_URL#labMeta" --json --no-write-lock-file)" \
        || fail "could not evaluate labMeta"
      IFACE="$(jq -er '.network.ifaceName' <<<"$META")" \
        || fail "labMeta does not contain a valid interface"
      CONFIGURED_DHCP_IP="$(jq -er '.controller.dhcpIp' <<<"$META")" \
        || fail "labMeta does not contain a valid controller DHCP address"
      STATIC_IP="$(jq -er '.controller.staticIp' <<<"$META")" \
        || fail "labMeta does not contain a valid controller static address"
      CACHE_PORT="$(jq -er '.network.cachePort' <<<"$META")" \
        || fail "labMeta does not contain a valid cache port"
      PXE_HTTP_PORT="$(jq -er '.network.pxeHttpPort' <<<"$META")" \
        || fail "labMeta does not contain a valid PXE HTTP port"

      ip link show dev "$IFACE" >/dev/null \
        || fail "configured interface $IFACE does not exist"
      mapfile -t ADDRESSES < <(ip -4 -o addr show dev "$IFACE" scope global \
        | awk '{ split($4, address, "/"); print address[1] }')
      ((''${#ADDRESSES[@]} > 0)) \
        || fail "configured interface $IFACE has no global IPv4 address"
      DHCP_IP=""
      OBSERVED_NON_STATIC=()
      for address in "''${ADDRESSES[@]}"; do
        if [[ "$address" != "$STATIC_IP" && "$address" != 169.254.* ]]; then
          OBSERVED_NON_STATIC+=("$address")
          [[ "$address" == "$CONFIGURED_DHCP_IP" ]] && DHCP_IP="$address"
        fi
      done
      if [[ -z "$DHCP_IP" ]]; then
        if [[ "''${#OBSERVED_NON_STATIC[@]}" -eq 1 ]]; then
          DHCP_IP="''${OBSERVED_NON_STATIC[0]}"
          echo "Configured DHCP address $CONFIGURED_DHCP_IP is no longer assigned; preparing PXE for the unambiguous live address $DHCP_IP"
        else
          OBSERVED="''${OBSERVED_NON_STATIC[*]:-none}"
          fail "configured DHCP address $CONFIGURED_DHCP_IP is not assigned to $IFACE and the live controller address is ambiguous (observed non-static addresses: $OBSERVED)"
        fi
      fi

      systemctl is-active --quiet nixorium-harmonia.service \
        || fail "Harmonia cache service is not active"
      curl --fail --silent --show-error --max-time 5 \
        "http://$DHCP_IP:$CACHE_PORT/nix-cache-info" | grep -q '^StoreDir:' \
        || fail "Harmonia cache health check failed at $DHCP_IP:$CACHE_PORT"

      publish_progress running network "Controller network and binary cache are ready" 0 0

      build_one() {
        local reference="$1"
        local output
        output="$(nix build "$reference" --no-write-lock-file --no-link --print-out-paths)" \
          || fail "Nix build failed for $reference"
        [[ "$output" =~ ^/nix/store/[0-9a-z]{32}-[^/[:space:]]+$ && -e "$output" ]] \
          || fail "Nix build returned an invalid store path for $reference"
        printf '%s' "$output"
      }

      publish_progress running artifacts "Building shared netboot artifacts" 0 4
      KERNEL_PATH="$(build_one "$FLAKE_URL#nixosConfigurations.netboot.config.system.build.kernel")"
      publish_progress running artifacts "Built netboot kernel (1/4)" 1 4
      INITRD_PATH="$(build_one "$FLAKE_URL#nixosConfigurations.netboot.config.system.build.netbootRamdisk")"
      publish_progress running artifacts "Built netboot initrd (2/4)" 2 4
      IPXE_SCRIPT_PATH="$(build_one "$FLAKE_URL#nixosConfigurations.netboot.config.system.build.netbootIpxeScript")"
      publish_progress running artifacts "Built iPXE boot script (3/4)" 3 4
      FIRMWARE_PATH="$(build_one "$FLAKE_URL#packages.x86_64-linux.pxeFirmware")"
      publish_progress running artifacts "Built iPXE firmware (4/4)" 4 4
      [[ -f "$KERNEL_PATH/bzImage" ]] || fail "prepared kernel output lacks bzImage"
      [[ -f "$INITRD_PATH/initrd" ]] || fail "prepared initrd output lacks initrd"
      [[ -f "$IPXE_SCRIPT_PATH/netboot.ipxe" ]] || fail "prepared iPXE output lacks netboot.ipxe"
      [[ -f "$FIRMWARE_PATH/snponly.efi" ]] || fail "prepared firmware output lacks snponly.efi"

      mapfile -t CLIENT_NAMES < <(jq -er '.clients.hosts[].name' <<<"$META")
      ((''${#CLIENT_NAMES[@]} > 0)) || fail "labMeta contains no client hosts"
      CLIENTS_FILE="$(mktemp "$STATE_DIRECTORY/.clients.XXXXXX")"
      MANIFEST_TEMP="$(mktemp "$STATE_DIRECTORY/.prepared.XXXXXX")"
      publish_progress running clients "Building client system closures" 0 "''${#CLIENT_NAMES[@]}"
      client_index=0
      for name in "''${CLIENT_NAMES[@]}"; do
        [[ "$name" =~ ^pc[0-9]+$ ]] || fail "labMeta contains invalid client name"
        client_path="$(build_one "$FLAKE_URL#nixosConfigurations.$name.config.system.build.toplevel")"
        printf '%s\t%s\n' "$name" "$client_path" >>"$CLIENTS_FILE"
        client_index=$((client_index + 1))
        publish_progress running clients \
          "Built client $name ($client_index/''${#CLIENT_NAMES[@]})" \
          "$client_index" "''${#CLIENT_NAMES[@]}"
      done
      CLIENTS="$(jq -Rn '[inputs | split("\t") | {name: .[0], storePath: .[1]}]' <"$CLIENTS_FILE")"

      publish_progress running publish "Retaining artifacts and publishing the manifest" 0 0
      ROOTS_DIRECTORY="$STATE_DIRECTORY/roots"
      ROOT_GENERATION="$ROOTS_DIRECTORY/$REVISION"
      install -d -m 0755 "$ROOT_GENERATION"
      root_path() {
        local name="$1"
        local store_path="$2"
        local root="$ROOT_GENERATION/$name"
        if [[ -L "$root" ]]; then
          [[ "$(readlink -f "$root")" == "$store_path" ]] \
            || fail "existing PXE GC root $name differs at revision $REVISION"
          return
        fi
        [[ ! -e "$root" ]] || fail "PXE GC root $name is not a symlink"
        nix-store --realise "$store_path" --add-root "$root" --indirect >/dev/null \
          || fail "could not retain prepared store path for $name"
      }
      root_path kernel "$KERNEL_PATH"
      root_path initrd "$INITRD_PATH"
      root_path ipxe-script "$IPXE_SCRIPT_PATH"
      root_path firmware "$FIRMWARE_PATH"
      while IFS=$'\t' read -r name client_path; do
        root_path "client-$name" "$client_path"
      done <"$CLIENTS_FILE"

      jq -n \
        --arg revision "$REVISION" \
        --arg preparedAt "$(date --utc --iso-8601=seconds)" \
        --arg iface "$IFACE" \
        --arg dhcpIp "$DHCP_IP" \
        --arg staticIp "$STATIC_IP" \
        --argjson cachePort "$CACHE_PORT" \
        --argjson pxeHttpPort "$PXE_HTTP_PORT" \
        --arg kernel "$KERNEL_PATH" \
        --arg initrd "$INITRD_PATH" \
        --arg ipxeScript "$IPXE_SCRIPT_PATH" \
        --arg firmware "$FIRMWARE_PATH" \
        --argjson clients "$CLIENTS" \
        '{
          schemaVersion: 1,
          revision: $revision,
          preparedAt: $preparedAt,
          controller: { dhcpIp: $dhcpIp, staticIp: $staticIp },
          network: { ifaceName: $iface, cachePort: $cachePort, pxeHttpPort: $pxeHttpPort },
          artifacts: {
            kernel: { storePath: $kernel, relativePath: "bzImage" },
            initrd: { storePath: $initrd, relativePath: "initrd" },
            ipxeScript: { storePath: $ipxeScript, relativePath: "netboot.ipxe" },
            firmware: { storePath: $firmware, relativePath: "snponly.efi" }
          },
          clients: $clients
        }' >"$MANIFEST_TEMP"
      chmod 0644 "$MANIFEST_TEMP"
      sync -f "$MANIFEST_TEMP"
      mv -T "$MANIFEST_TEMP" "$STATE_DIRECTORY/prepared.json"
      sync -f "$STATE_DIRECTORY"
      MANIFEST_TEMP=""
      rm -f -- "$CLIENTS_FILE"
      CLIENTS_FILE=""
      find "$ROOTS_DIRECTORY" -mindepth 1 -maxdepth 1 -type d \
        ! -name "$REVISION" -exec rm -rf -- {} +
      publish_progress completed complete \
        "Prepared PXE artifacts for ''${#CLIENT_NAMES[@]} clients" \
        "''${#CLIENT_NAMES[@]}" "''${#CLIENT_NAMES[@]}"
      trap - EXIT
      echo "Prepared PXE artifacts at revision $REVISION"
    '';
  };
  restartCache = pkgs.writeShellApplication {
    name = "nixorium-restart-cache";
    runtimeInputs = [ pkgs.coreutils pkgs.systemd pkgs.util-linux ];
    text = ''
      fail() {
        echo "Error: $*" >&2
        exit 1
      }

      ${operationGate}
      systemctl restart harmonia.service
    '';
  };
in
{
  options.services.nixorium.deploymentPath = lib.mkOption {
    type = lib.types.strMatching "/.*";
    default = "/home/admin/nixorium-deployment";
    description = "Fixed administrator-owned private deployment used by privileged Nixorium actions";
  };

  config = lib.mkIf isController {
    users.groups.nixorium-operations = { };
    users.users.admin.extraGroups = [ "nixorium-operations" ];

    environment.etc."nixorium/deployment-path" = {
      text = "${cfg.deploymentPath}\n";
      mode = "0444";
      user = "root";
      group = "root";
    };

    environment.systemPackages = [
      nixoriumPackage
      pkgs.colmena
      pkgs.git
    ];

    security.polkit.enable = true;
    security.polkit.extraConfig = ''
      polkit.addRule(function(action, subject) {
        var unit = action.lookup("unit");
        var verb = action.lookup("verb");
        if (action.id == "org.freedesktop.systemd1.manage-units" &&
            ((verb == "start" &&
              (unit == "nixorium-install-secrets.service" ||
               unit == "nixorium-apply-controller.service" ||
               /^nixorium-apply-controller@[0-9a-f]{40}\.service$/.test(unit) ||
               unit == "nixorium-prepare-pxe.service" ||
               unit == "nixorium-remote-install.service" ||
               unit == "nixorium-restart-cache.service" ||
               unit == "nixorium-pxe-recover.service")) ||
             (unit == "nixorium-pxe.service" &&
              (verb == "start" || verb == "stop")) ||
             (unit == "nixorium-pxe-network.service" && verb == "stop")) &&
            subject.isInGroup("wheel")) {
          return polkit.Result.YES;
        }
      });
    '';

    systemd.tmpfiles.rules = [
      "d /home/admin/.ssh 0700 admin users -"
      "f /home/admin/.ssh/known_hosts 0600 admin users -"
      "f /home/admin/.ssh/.nixorium-known-hosts.lock 0600 admin users -"
      "d /etc/veyon/keys/private/teacher 0750 root veyon-master -"
      "d /var/lib/nixorium/keys 0700 root root -"
      "d /var/cache/nixorium/admin 0700 admin users -"
      "d /var/lib/nixorium/coordination 0770 root nixorium-operations -"
      "f /var/lib/nixorium/coordination/operation.lock 0660 root nixorium-operations -"
      "d /var/lib/nixorium/remote-install 0700 admin users -"
      "d /var/lib/nixorium/remote-install/logs 0700 admin users -"
      "d /home/admin/.local 0700 admin users -"
      "d /home/admin/.local/state 0700 admin users -"
      "d /home/admin/.local/state/nixorium 0700 admin users -"
      "d /home/admin/.local/state/nixorium/operations 0700 admin users -"
    ];

    systemd.services.nixorium-install-secrets = {
      description = "Install verified Nixorium controller key material";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${installSecrets}/bin/nixorium-install-secrets";
        User = "root";
        Group = "root";
        UMask = "0077";
        CacheDirectory = "nixorium";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium";
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [
          "-/home/admin/.ssh"
          "-/etc/veyon/keys/private/teacher"
          "-/var/lib/nixorium/keys"
          "-/var/cache/nixorium"
          "/var/lib/nixorium/coordination"
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = [ "CAP_CHOWN" "CAP_DAC_OVERRIDE" "CAP_FOWNER" ];
      };
    };

    systemd.services.nixorium-apply-controller = {
      description = "Build and activate the reviewed Nixorium controller configuration";
      after = [ "nixorium-install-secrets.service" ];
      # This unit executes switch-to-configuration itself. Never let that
      # switch terminate the running job before it can verify and record the
      # newly active controller system.
      restartIfChanged = false;
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${applyController}/bin/nixorium-apply-controller";
        User = "root";
        Group = "root";
        UMask = "0077";
        CacheDirectory = "nixorium";
        StateDirectory = "nixorium/controller";
        StateDirectoryMode = "0755";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium";
        PrivateTmp = true;
        # NixOS activation legitimately updates declared user homes and
        # /run/user. Keep the reviewed deployment itself read-only instead of
        # placing the entire switch-to-configuration process behind
        # ProtectHome, which also makes /run/user read-only.
        ProtectHome = false;
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [ "-/var/cache/nixorium" "/var/lib/nixorium/coordination" ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        TimeoutStartSec = "2h";
      };
    };

    systemd.services."nixorium-apply-controller@" = {
      description = "Build and activate reviewed Nixorium controller revision %i";
      after = [ "nixorium-install-secrets.service" ];
      # The target configuration may contain a newer management command and
      # therefore a different ExecStart store path. The active reviewed job
      # must survive that unit-file change and finish its receipt.
      restartIfChanged = false;
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${applyController}/bin/nixorium-apply-controller %i";
        User = "root";
        Group = "root";
        UMask = "0077";
        CacheDirectory = "nixorium";
        StateDirectory = "nixorium/controller";
        StateDirectoryMode = "0755";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium";
        PrivateTmp = true;
        # See the parameterless first-run unit above. Revision binding and the
        # explicit read-only deployment mount remain the security boundary.
        ProtectHome = false;
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [ "-/var/cache/nixorium" "/var/lib/nixorium/coordination" ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        TimeoutStartSec = "2h";
      };
    };

    systemd.services.nixorium-restart-cache = {
      description = "Restart the Nixorium binary cache";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${restartCache}/bin/nixorium-restart-cache";
        User = "root";
        Group = "root";
        CapabilityBoundingSet = "";
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectHome = true;
        ProtectSystem = "strict";
        ReadWritePaths = [ "/var/lib/nixorium/coordination" ];
      };
    };

    systemd.services.nixorium-prepare-pxe = {
      description = "Build and record Nixorium PXE artifacts and client closures";
      wants = [ "harmonia.service" "network-online.target" ];
      after = [ "harmonia.service" "network-online.target" ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${preparePxe}/bin/nixorium-prepare-pxe";
        User = "admin";
        Group = "users";
        UMask = "0022";
        StateDirectory = "nixorium/prepared";
        StateDirectoryMode = "0755";
        Environment = "XDG_CACHE_HOME=/var/cache/nixorium/admin";
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath ];
        ReadWritePaths = [
          "-/var/cache/nixorium"
          "-/var/lib/nixorium/prepared"
          "/var/lib/nixorium/coordination"
        ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
        TimeoutStartSec = "4h";
      };
    };

    systemd.services.nixorium-remote-install = {
      description = "Coordinate reviewed Nixorium USB SSH client installations";
      path = [ pkgs.git pkgs.nix pkgs.openssh ];
      serviceConfig = {
        Type = "simple";
        ExecStart = "${nixoriumPackage}/bin/nixorium-remote-worker";
        User = "admin";
        Group = "users";
        SupplementaryGroups = [ "nixorium-operations" ];
        UMask = "0077";
        RuntimeDirectory = "nixorium/remote-install";
        RuntimeDirectoryMode = "0700";
        StateDirectory = "nixorium/remote-install";
        StateDirectoryMode = "0700";
        CacheDirectory = "nixorium/admin";
        CacheDirectoryMode = "0700";
        Environment = [
          "XDG_CACHE_HOME=/var/cache/nixorium/admin"
          "XDG_STATE_HOME=/home/admin/.local/state"
        ];
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ cfg.deploymentPath "/etc/nixorium/deployment-path" "/home/admin/.ssh/id_ed25519" "/home/admin/.ssh/id_ed25519.pub" ];
        ReadWritePaths = [
          "/run/nixorium/remote-install"
          "/var/lib/nixorium/remote-install"
          "/var/lib/nixorium/coordination"
          "-/home/admin/.ssh/known_hosts"
          "-/home/admin/.ssh/.nixorium-known-hosts.lock"
          "-/home/admin/.local/state/nixorium/operations"
          "-/var/cache/nixorium/admin"
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
        LimitCORE = 0;
        Restart = "on-failure";
        RestartSec = "2s";
      };
    };
  };
}
