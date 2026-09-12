{ config, hostName, labSettings, lib, pkgs, ... }:
let
  isController = hostName == labSettings.masterHostName;
  preparationFile = "/var/lib/nixorium/prepared/prepared.json";
  sessionFile = "/var/lib/nixorium/pxe/session.json";
  lastSessionFile = "/var/lib/nixorium/pxe/last-session.json";
  networkAction = pkgs.writeShellApplication {
    name = "nixorium-pxe-network-action";
    runtimeInputs = [ pkgs.coreutils pkgs.gawk pkgs.iproute2 pkgs.jq ];
    text = ''
      ACTION="''${1:-}"
      [[ $# -eq 1 && "$ACTION" =~ ^(start|stop|recover)$ ]] \
        || { echo "Error: internal PXE network action requires start, stop, or recover." >&2; exit 2; }

      PREPARATION_FILE=${lib.escapeShellArg preparationFile}
      SESSION_FILE=${lib.escapeShellArg sessionFile}
      LAST_SESSION_FILE=${lib.escapeShellArg lastSessionFile}
      CONFIGURED_IFACE=${lib.escapeShellArg labSettings.ifaceName}
      CONFIGURED_DHCP_IP=${lib.escapeShellArg labSettings.masterDhcpIp}
      CONFIGURED_STATIC_IP=${lib.escapeShellArg labSettings.masterIp}
      CONFIGURED_PREFIX=${lib.escapeShellArg (toString labSettings.networkPrefixLength)}
      CONFIGURED_STATIC_CIDR="$CONFIGURED_STATIC_IP/$CONFIGURED_PREFIX"

      fail() {
        echo "Error: $*" >&2
        exit 1
      }

      address_present() {
        local iface="$1"
        local address="$2"
        ip -4 -o addr show dev "$iface" scope global \
          | awk '{ print $4 }' \
          | grep -Fxq "$address"
      }

      ip_present() {
        local iface="$1"
        local address="$2"
        ip -4 -o addr show dev "$iface" scope global \
          | awk '{ split($4, value, "/"); print value[1] }' \
          | grep -Fxq "$address"
      }

      validate_session_file() {
        [[ -f "$SESSION_FILE" && ! -L "$SESSION_FILE" ]] \
          || fail "PXE session record is not a regular file"
        [[ "$(stat -c '%U:%G:%a' "$SESSION_FILE")" == root:root:600 ]] \
          || fail "PXE session record has unsafe ownership or permissions"
        [[ "$(stat -c '%s' "$SESSION_FILE")" -le 1048576 ]] \
          || fail "PXE session record exceeds the size limit"
        jq -e '
          .schemaVersion == 1 and
          (.state == "transitioning" or .state == "network-active") and
          (.interface | type == "string" and test("^[A-Za-z0-9_.:-]+$")) and
          (.dhcpAddress | type == "string" and test("^[0-9.]+$")) and
          (.staticAddress | type == "string" and test("^[0-9.]+$")) and
          (.prefixLength | type == "number" and . >= 1 and . <= 32) and
          .removedStatic == true and
          (.originalAddresses | type == "array") and
          (.artifacts | type == "object")
        ' "$SESSION_FILE" >/dev/null \
          || fail "PXE session record is invalid"
      }

      archive_session() {
        local final_state="$1"
        local reason="$2"
        local temporary
        temporary="$(mktemp "$STATE_DIRECTORY/.last-session.XXXXXX")"
        jq \
          --arg state "$final_state" \
          --arg reason "$reason" \
          --arg stoppedAt "$(date --utc --iso-8601=seconds)" \
          '.state = $state | .stopReason = $reason | .stoppedAt = $stoppedAt' \
          "$SESSION_FILE" >"$temporary"
        chmod 0600 "$temporary"
        sync -f "$temporary"
        mv -T "$temporary" "$LAST_SESSION_FILE"
        sync -f "$STATE_DIRECTORY"
        rm -f "$SESSION_FILE"
        sync -f "$STATE_DIRECTORY"
      }

      restore_session() {
        local final_state="$1"
        local reason="$2"
        local iface static_ip prefix static_cidr
        [[ -e "$SESSION_FILE" ]] || return 0
        validate_session_file
        iface="$(jq -er '.interface' "$SESSION_FILE")"
        static_ip="$(jq -er '.staticAddress' "$SESSION_FILE")"
        prefix="$(jq -er '.prefixLength' "$SESSION_FILE")"
        static_cidr="$static_ip/$prefix"
        jq -e --arg staticCidr "$static_cidr" \
          '.originalAddresses | index($staticCidr) != null' \
          "$SESSION_FILE" >/dev/null \
          || fail "PXE session does not prove that the static address was originally present"
        ip link show dev "$iface" >/dev/null \
          || fail "recorded PXE interface $iface does not exist"
        if ! address_present "$iface" "$static_cidr"; then
          ip addr add "$static_cidr" dev "$iface" \
            || fail "could not restore $static_cidr on $iface"
        fi
        address_present "$iface" "$static_cidr" \
          || fail "restored static address $static_cidr is not observable on $iface"
        archive_session "$final_state" "$reason"
        echo "Restored $static_cidr on $iface ($reason)"
      }

      validate_preparation() {
        [[ -f "$PREPARATION_FILE" && ! -L "$PREPARATION_FILE" ]] \
          || fail "managed PXE preparation is missing or not a regular file"
        [[ "$(stat -c '%U:%G:%a' "$PREPARATION_FILE")" == admin:users:644 ]] \
          || fail "managed PXE preparation has unsafe ownership or permissions"
        [[ "$(stat -c '%s' "$PREPARATION_FILE")" -le 1048576 ]] \
          || fail "managed PXE preparation exceeds the size limit"
        local revision
        revision="$(jq -er '.revision' "$PREPARATION_FILE")"
        [[ "$revision" =~ ^[0-9a-f]{40}$ ]] \
          || fail "deployment revision is invalid"
        jq -e \
          --arg revision "$revision" \
          --arg iface "$CONFIGURED_IFACE" \
          --arg dhcpIp "$CONFIGURED_DHCP_IP" \
          --arg staticIp "$CONFIGURED_STATIC_IP" \
          '.schemaVersion == 1 and .revision == $revision and
           .controller.dhcpIp == $dhcpIp and .controller.staticIp == $staticIp and
           .network.ifaceName == $iface and
           .artifacts.kernel.relativePath == "bzImage" and
           .artifacts.initrd.relativePath == "initrd" and
           .artifacts.ipxeScript.relativePath == "netboot.ipxe" and
           .artifacts.firmware.relativePath == "snponly.efi" and
           (.clients | type == "array" and length > 0)' \
          "$PREPARATION_FILE" >/dev/null \
          || fail "managed PXE preparation is stale or invalid; run nixorium pxe prepare"

        mapfile -t artifact_rows < <(jq -er '.artifacts | [.kernel, .initrd, .ipxeScript, .firmware][] | [.storePath, .relativePath] | @tsv' "$PREPARATION_FILE")
        [[ "''${#artifact_rows[@]}" -eq 4 ]] \
          || fail "managed PXE preparation does not contain exactly four artifacts"
        for row in "''${artifact_rows[@]}"; do
          IFS=$'\t' read -r store_path relative_path <<<"$row"
          [[ "$store_path" =~ ^/nix/store/[0-9a-z]{32}-[^/[:space:]]+$ ]] \
            || fail "managed PXE preparation contains an invalid artifact store path"
          [[ "$relative_path" =~ ^(bzImage|initrd|netboot\.ipxe|snponly\.efi)$ ]] \
            || fail "managed PXE preparation contains an invalid artifact name"
          [[ -f "$store_path/$relative_path" ]] \
            || fail "prepared artifact is unavailable: $relative_path"
        done
        mapfile -t client_rows < <(jq -er '.clients[] | [.name, .storePath] | @tsv' "$PREPARATION_FILE")
        [[ "''${#client_rows[@]}" -gt 0 ]] \
          || fail "managed PXE preparation contains no client closures"
        for row in "''${client_rows[@]}"; do
          IFS=$'\t' read -r name store_path <<<"$row"
          [[ "$name" =~ ^pc[0-9]+$ && "$store_path" =~ ^/nix/store/[0-9a-z]{32}-[^/[:space:]]+$ && -d "$store_path" ]] \
            || fail "managed PXE preparation contains an invalid client closure"
        done
        PREPARATION_REVISION="$revision"
      }

      start_network() {
        if [[ -e "$SESSION_FILE" ]]; then
          restore_session recovered pre-start-recovery
        fi
        validate_preparation
        ip link show dev "$CONFIGURED_IFACE" >/dev/null \
          || fail "configured interface $CONFIGURED_IFACE does not exist"
        ip_present "$CONFIGURED_IFACE" "$CONFIGURED_DHCP_IP" \
          || fail "configured DHCP address $CONFIGURED_DHCP_IP is not assigned to $CONFIGURED_IFACE"
        address_present "$CONFIGURED_IFACE" "$CONFIGURED_STATIC_CIDR" \
          || fail "expected static address $CONFIGURED_STATIC_CIDR is not assigned; refusing an untracked transition"

        mapfile -t original_addresses < <(ip -4 -o addr show dev "$CONFIGURED_IFACE" scope global \
          | awk '{ print $4 }' | sort -u)
        local original_json artifacts_json temporary active_temporary
        original_json="$(printf '%s\n' "''${original_addresses[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')"
        artifacts_json="$(jq -c '.artifacts' "$PREPARATION_FILE")"
        temporary="$(mktemp "$STATE_DIRECTORY/.session.XXXXXX")"
        jq -n \
          --arg revision "$PREPARATION_REVISION" \
          --arg startedAt "$(date --utc --iso-8601=seconds)" \
          --arg iface "$CONFIGURED_IFACE" \
          --arg dhcpAddress "$CONFIGURED_DHCP_IP" \
          --arg staticAddress "$CONFIGURED_STATIC_IP" \
          --argjson prefixLength "$CONFIGURED_PREFIX" \
          --argjson originalAddresses "$original_json" \
          --argjson artifacts "$artifacts_json" \
          '{
            schemaVersion: 1,
            state: "transitioning",
            preparationRevision: $revision,
            startedAt: $startedAt,
            interface: $iface,
            dhcpAddress: $dhcpAddress,
            staticAddress: $staticAddress,
            prefixLength: $prefixLength,
            removedStatic: true,
            originalAddresses: $originalAddresses,
            artifacts: $artifacts
          }' >"$temporary"
        chmod 0600 "$temporary"
        sync -f "$temporary"
        mv -T "$temporary" "$SESSION_FILE"
        sync -f "$STATE_DIRECTORY"

        if ! ip addr del "$CONFIGURED_STATIC_CIDR" dev "$CONFIGURED_IFACE"; then
          restore_session start-failed remove-static-failed
          fail "could not remove $CONFIGURED_STATIC_CIDR from $CONFIGURED_IFACE"
        fi
        if address_present "$CONFIGURED_IFACE" "$CONFIGURED_STATIC_CIDR" \
            || ! ip_present "$CONFIGURED_IFACE" "$CONFIGURED_DHCP_IP"; then
          restore_session start-failed transition-verification-failed
          fail "PXE address transition verification failed and was rolled back"
        fi

        active_temporary="$(mktemp "$STATE_DIRECTORY/.session-active.XXXXXX")"
        if ! jq '.state = "network-active"' "$SESSION_FILE" >"$active_temporary" \
            || ! chmod 0600 "$active_temporary" \
            || ! sync -f "$active_temporary" \
            || ! mv -T "$active_temporary" "$SESSION_FILE"; then
          rm -f "$active_temporary"
          restore_session start-failed state-publication-failed
          fail "could not publish active PXE network state; transition was rolled back"
        fi
        sync -f "$STATE_DIRECTORY"
        echo "PXE networking active on $CONFIGURED_IFACE using $CONFIGURED_DHCP_IP"
      }

      case "$ACTION" in
        start) start_network ;;
        stop) restore_session stopped normal-stop ;;
        recover) restore_session recovered boot-or-explicit-recovery ;;
      esac
    '';
  };
in
{
  config = lib.mkIf isController {
    systemd.services.nixorium-pxe-network = {
      description = "Apply and restore the Nixorium PXE address transition";
      wants = [ "network-online.target" ];
      after = [ "network-online.target" "nixorium-pxe-recover.service" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        ExecStart = "${networkAction}/bin/nixorium-pxe-network-action start";
        ExecStop = "${networkAction}/bin/nixorium-pxe-network-action stop";
        ExecStopPost = "${networkAction}/bin/nixorium-pxe-network-action recover";
        User = "root";
        Group = "root";
        UMask = "0077";
        StateDirectory = "nixorium/pxe";
        StateDirectoryMode = "0700";
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = "read-only";
        ReadOnlyPaths = [ preparationFile ];
        ReadWritePaths = [ "-/var/lib/nixorium/pxe" ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = [ "CAP_NET_ADMIN" ];
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
      };
    };

    systemd.services.nixorium-pxe-recover = {
      description = "Recover an unfinished Nixorium PXE network session";
      wantedBy = [ "multi-user.target" ];
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];
      before = [ "nixorium-pxe-network.service" ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${networkAction}/bin/nixorium-pxe-network-action recover";
        User = "root";
        Group = "root";
        UMask = "0077";
        StateDirectory = "nixorium/pxe";
        StateDirectoryMode = "0700";
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ "-/var/lib/nixorium/pxe" ];
        NoNewPrivileges = true;
        CapabilityBoundingSet = [ "CAP_NET_ADMIN" ];
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_NETLINK" "AF_UNIX" ];
      };
    };
  };
}
