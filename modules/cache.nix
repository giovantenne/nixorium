{ config, labSettings, lib, ... }:
let
  isMaster = config.networking.hostName == labSettings.masterHostName;
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  signingKeyPath = "/var/lib/nixorium/keys/harmonia-secret-key";
  transition = labSettings.networkTransition or null;
in
{
  nix.settings = {
    # The master does not need itself as a substituter. During a change of
    # laboratory network clients try the new controller address first and the
    # previous one second. Clients use only the controller: updates never need
    # the Internet, and filtered school networks reject cache.nixos.org. This
    # replaces the NixOS default; a deployment adding client caches uses mkForce.
    substituters = if isMaster then [] else lib.mkOverride 90 (map
      (address: "http://${address}:${toString labSettings.cachePort}")
      ([ labSettings.masterIp ] ++ lib.optional (transition != null) transition.masterIp));
    trusted-public-keys = if labSettings.cachePublicKey == null then [] else [ labSettings.cachePublicKey ];
  };

  services.harmonia.cache = lib.mkIf (isMaster && laboratoryEnabled) {
    enable = true;
    signKeyPaths = [ signingKeyPath ];
    # Every address of the controller, so the cache answers on both laboratory
    # addresses during a change of laboratory network.
    settings.bind = "[::]:${toString labSettings.cachePort}";
  };

  # Keep the native Harmonia unit while exposing a stable product-level name
  # to the management application and operators.
  systemd.services.harmonia = lib.mkIf (isMaster && laboratoryEnabled) {
    aliases = [ "nixorium-harmonia.service" ];
    wantedBy = [ "multi-user.target" ];
  };
}
