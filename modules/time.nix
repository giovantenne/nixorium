# Time on the laboratory network. The controller serves the time to the
# laboratory and every client takes it from the controller only, so a computer
# whose clock battery is drained shows the right time without Internet of its
# own (certificates, file dates and logs stay correct). The controller keeps
# the NixOS default Internet pools as its upstream. Without a laboratory
# network (controller-only mode) the NixOS defaults apply unchanged.
#
# Controller (chrony):
# - `allow` serves the laboratory network only; UDP 123 is also opened on the
#   laboratory interface only, in firewall.nix.
# - `local stratum 10` keeps answering when the Internet upstream is
#   unreachable. `waitunsynced 300` gives the upstream five minutes after start
#   first, so a clock that may still be wrong (drained battery) is not served
#   to computers that are already running while the upstream is being reached.
# - `bindaddress` limits the listener to the laboratory address (chrony sets
#   IP_FREEBIND, so the address need not be up at start). chrony binds only one
#   address, so during a change of laboratory network, when the interface has
#   two, the interface-scoped firewall and `allow` are the scope.
{ isController, labSettings, lib, ... }:
let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  transition = labSettings.networkTransition or null;
  # Like the cache, both controller addresses answer during a change of
  # laboratory network: clients list the new one first and the previous second,
  # and the controller serves both networks.
  controllerAddresses =
    [ labSettings.masterIp ] ++ lib.optional (transition != null) transition.masterIp;
  laboratoryNetworks =
    [ "${labSettings.networkBase}/${toString labSettings.networkPrefixLength}" ]
    ++ lib.optional (transition != null) "${transition.base}/${toString transition.prefixLength}";
in
lib.mkIf laboratoryEnabled (lib.mkMerge [
  (lib.mkIf isController {
    # systemd-timesyncd is only a client; chrony also serves time.
    services.chrony = {
      enable = true;
      extraConfig = lib.concatMapStrings (line: line + "\n") (
        map (network: "allow ${network}") laboratoryNetworks
        ++ [ "local stratum 10 waitunsynced 300" ]
        ++ lib.optional (transition == null) "bindaddress ${labSettings.masterIp}"
      );
    };
  })

  (lib.mkIf (!isController) {
    # The controller is the only time server: no Internet pool is listed, not
    # even as a fallback, so a client never waits for hosts it cannot reach and
    # a client that has Internet after sign-in still takes the time from the
    # controller. A site may override these defaults.
    networking.timeServers = lib.mkDefault controllerAddresses;
    services.timesyncd = {
      servers = lib.mkDefault controllerAddresses;
      fallbackServers = lib.mkDefault [ ];
    };
  })
])
