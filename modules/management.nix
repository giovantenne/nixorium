{ config, hostName, labSettings, nixoriumPackage, lib, pkgs, ... }:
let
  isController = hostName == labSettings.masterHostName;
  cfg = config.services.nixorium;
  installSecrets = pkgs.writeShellApplication {
    name = "nixorium-install-secrets";
    runtimeInputs = [ pkgs.coreutils pkgs.diffutils pkgs.git pkgs.nix nixoriumPackage ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}

      fail() {
        echo "Error: $*" >&2
        exit 1
      }

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
      pkgs.git
      pkgs.jq
      pkgs.nix
      pkgs.util-linux
      nixoriumPackage
    ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}
      EXPECTED_REVISION="''${1:-}"

      fail() {
        echo "Error: $*" >&2
        exit 1
      }

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
      as_admin nixorium setup keys --verify-only --repo "$REPOSITORY" --json \
        || fail "deployment key correspondence verification failed"
      [[ "$(as_admin nix eval "$FLAKE_URL#deploymentStatus.ready" --json --no-write-lock-file)" == true ]] \
        || fail "deploymentStatus.ready must be true before controller apply"

      cmp -s "$REPOSITORY/admin-ssh" /home/admin/.ssh/id_ed25519 \
        || fail "installed admin SSH key is absent or differs"
      cmp -s "$REPOSITORY/veyon-private-key.pem" /etc/veyon/keys/private/teacher/key \
        || fail "installed Veyon private key is absent or differs"
      cmp -s "$REPOSITORY/secret-key" /var/lib/nixorium/keys/harmonia-secret-key \
        || fail "installed Harmonia signing key is absent or differs"

      CONTROLLER_NAME="$(as_admin nix eval "$FLAKE_URL#labMeta.controller.name" --raw --no-write-lock-file)"
      [[ "$CONTROLLER_NAME" =~ ^pc[0-9]+$ ]] \
        || fail "evaluated controller name is invalid"

      SYSTEM_PATH="$(as_admin nix build "$FLAKE_URL#nixosConfigurations.$CONTROLLER_NAME.config.system.build.toplevel" \
        --no-write-lock-file --no-link --print-out-paths)"
      [[ "$SYSTEM_PATH" == /nix/store/* && "$SYSTEM_PATH" != *[[:space:]]* \
          && -x "$SYSTEM_PATH/bin/switch-to-configuration" ]] \
        || fail "controller build did not return one valid NixOS system closure"
      [[ -z "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" status --porcelain=v1 --untracked-files=normal)" \
          && "$(git -c safe.directory="$REPOSITORY" -C "$REPOSITORY" rev-parse HEAD)" == "$REVISION" ]] \
        || fail "deployment changed while the controller was building; activation refused"

      STATE_DIRECTORY=/var/lib/nixorium/controller
      ACTIVATION_RECORD="$STATE_DIRECTORY/applied.json"
      [[ -d "$STATE_DIRECTORY" && ! -L "$STATE_DIRECTORY" \
          && "$(stat -c '%U:%G:%a' "$STATE_DIRECTORY")" == root:root:755 ]] \
        || fail "controller state directory has unsafe ownership or permissions"
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

      TEMPORARY_RECORD="$(mktemp --tmpdir="$STATE_DIRECTORY" .applied.json.XXXXXX)"
      keep_temporary=true
      cleanup_record() {
        if [[ "$keep_temporary" == true ]]; then
          rm -f -- "$TEMPORARY_RECORD"
        fi
      }
      trap cleanup_record EXIT
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
      nixoriumPackage
    ];
    text = ''
      REPOSITORY=${lib.escapeShellArg cfg.deploymentPath}

      fail() {
        echo "Error: $*" >&2
        exit 1
      }

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

      META="$(nix eval "$FLAKE_URL#labMeta" --json --no-write-lock-file)" \
        || fail "could not evaluate labMeta"
      IFACE="$(jq -er '.network.ifaceName' <<<"$META")" \
        || fail "labMeta does not contain a valid interface"
      DHCP_IP="$(jq -er '.controller.dhcpIp' <<<"$META")" \
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
      DHCP_PRESENT=false
      OBSERVED_NON_STATIC=()
      for address in "''${ADDRESSES[@]}"; do
        [[ "$address" == "$DHCP_IP" ]] && DHCP_PRESENT=true
        [[ "$address" == "$STATIC_IP" ]] || OBSERVED_NON_STATIC+=("$address")
      done
      if [[ "$DHCP_PRESENT" != true ]]; then
        OBSERVED="''${OBSERVED_NON_STATIC[*]:-none}"
        fail "configured DHCP address $DHCP_IP is not assigned to $IFACE (observed non-static addresses: $OBSERVED); review configuration before rebuilding address-bound artifacts"
      fi

      systemctl is-active --quiet nixorium-harmonia.service \
        || fail "Harmonia cache service is not active"
      curl --fail --silent --show-error --max-time 5 \
        "http://$DHCP_IP:$CACHE_PORT/nix-cache-info" | grep -q '^StoreDir:' \
        || fail "Harmonia cache health check failed at $DHCP_IP:$CACHE_PORT"

      build_one() {
        local reference="$1"
        local output
        output="$(nix build "$reference" --no-write-lock-file --no-link --print-out-paths)" \
          || fail "Nix build failed for $reference"
        [[ "$output" =~ ^/nix/store/[0-9a-z]{32}-[^/[:space:]]+$ && -e "$output" ]] \
          || fail "Nix build returned an invalid store path for $reference"
        printf '%s' "$output"
      }

      KERNEL_PATH="$(build_one "$FLAKE_URL#nixosConfigurations.netboot.config.system.build.kernel")"
      INITRD_PATH="$(build_one "$FLAKE_URL#nixosConfigurations.netboot.config.system.build.netbootRamdisk")"
      IPXE_SCRIPT_PATH="$(build_one "$FLAKE_URL#nixosConfigurations.netboot.config.system.build.netbootIpxeScript")"
      FIRMWARE_PATH="$(build_one "$FLAKE_URL#packages.x86_64-linux.pxeFirmware")"
      [[ -f "$KERNEL_PATH/bzImage" ]] || fail "prepared kernel output lacks bzImage"
      [[ -f "$INITRD_PATH/initrd" ]] || fail "prepared initrd output lacks initrd"
      [[ -f "$IPXE_SCRIPT_PATH/netboot.ipxe" ]] || fail "prepared iPXE output lacks netboot.ipxe"
      [[ -f "$FIRMWARE_PATH/snponly.efi" ]] || fail "prepared firmware output lacks snponly.efi"

      mapfile -t CLIENT_NAMES < <(jq -er '.clients.hosts[].name' <<<"$META")
      ((''${#CLIENT_NAMES[@]} > 0)) || fail "labMeta contains no client hosts"
      CLIENTS_FILE="$(mktemp "$STATE_DIRECTORY/.clients.XXXXXX")"
      MANIFEST_TEMP="$(mktemp "$STATE_DIRECTORY/.prepared.XXXXXX")"
      cleanup() {
        rm -f "$CLIENTS_FILE" "$MANIFEST_TEMP"
      }
      trap cleanup EXIT
      for name in "''${CLIENT_NAMES[@]}"; do
        [[ "$name" =~ ^pc[0-9]+$ ]] || fail "labMeta contains invalid client name"
        client_path="$(build_one "$FLAKE_URL#nixosConfigurations.$name.config.system.build.toplevel")"
        printf '%s\t%s\n' "$name" "$client_path" >>"$CLIENTS_FILE"
      done
      CLIENTS="$(jq -Rn '[inputs | split("\t") | {name: .[0], storePath: .[1]}]' <"$CLIENTS_FILE")"

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
      trap - EXIT
      rm -f "$CLIENTS_FILE"
      find "$ROOTS_DIRECTORY" -mindepth 1 -maxdepth 1 -type d \
        ! -name "$REVISION" -exec rm -rf -- {} +
      echo "Prepared PXE artifacts for ''${#CLIENT_NAMES[@]} clients at revision $REVISION"
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
    environment.systemPackages = [
      nixoriumPackage
      pkgs.colmena
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
      "d /etc/veyon/keys/private/teacher 0750 root veyon-master -"
      "d /var/lib/nixorium/keys 0700 root root -"
      "d /var/cache/nixorium/admin 0700 admin users -"
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
        ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = [ "CAP_CHOWN" "CAP_DAC_OVERRIDE" "CAP_FOWNER" ];
      };
    };

    systemd.services.nixorium-apply-controller = {
      description = "Build and activate the reviewed Nixorium controller configuration";
      after = [ "nixorium-install-secrets.service" ];
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
        ReadWritePaths = [ "-/var/cache/nixorium" ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        TimeoutStartSec = "2h";
      };
    };

    systemd.services."nixorium-apply-controller@" = {
      description = "Build and activate reviewed Nixorium controller revision %i";
      after = [ "nixorium-install-secrets.service" ];
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
        ReadWritePaths = [ "-/var/cache/nixorium" ];
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
        ExecStart = "${pkgs.systemd}/bin/systemctl restart harmonia.service";
        User = "root";
        Group = "root";
        CapabilityBoundingSet = "";
        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectHome = true;
        ProtectSystem = "strict";
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
        ];
        Nice = 10;
        IOSchedulingClass = "best-effort";
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
        TimeoutStartSec = "4h";
      };
    };
  };
}
