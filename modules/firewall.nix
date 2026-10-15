{ hostName, labSettings, lib, ... }:
let
  isController = hostName == labSettings.masterHostName;
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  isClient = laboratoryEnabled && !isController;
  # Clients without an explicit interface name are configured by type (ADR 0017).
  ifaceExplicit = labSettings.ifaceExplicit or true;
  transition = labSettings.networkTransition or null;
  clientRules = import ../lib/client-firewall-rules.nix {
    interface = if ifaceExplicit then labSettings.ifaceName else null;
    masterIp = labSettings.masterIp;
    # During a change of laboratory network SSH also comes from the
    # controller's previous address, for example from the deployment that
    # moves this computer to the new network.
    previousMasterIp = if transition == null then null else transition.masterIp;
  };
in
{
  networking.nftables.enable = true;
  # Reload only owned tables, preserving independently managed runtime rules.
  networking.nftables.flushRuleset = false;
  networking.firewall = {
    enable = true;
    interfaces = lib.mkIf (laboratoryEnabled && ifaceExplicit) {
      ${labSettings.ifaceName} = {
        allowedTCPPorts = lib.optionals isController [
          22 labSettings.cachePort labSettings.pxeHttpPort
        ];
        allowedUDPPorts = lib.optionals isController [ 5353 67 69 4011 ];
      };
    };
    extraInputRules = lib.optionalString isClient clientRules.allow;
  };

  # Enforce the master source even for existing connections or additional
  # site openings. No IPv6 management address is configured.
  networking.nftables.tables.nixorium-client-access = lib.mkIf isClient {
    family = "inet";
    content = clientRules.guard;
  };

  services.openssh.openFirewall = false;
  services.avahi.openFirewall = false;
}
