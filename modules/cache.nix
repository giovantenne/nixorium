{ config, labSettings, lib, ... }:
let
  isMaster = config.networking.hostName == labSettings.masterHostName;
  signingKeyPath = "/var/lib/nixorium/keys/harmonia-secret-key";
in
{
  nix.settings = {
    # The master does not need itself as a substituter
    substituters = if isMaster then [] else [ "http://${labSettings.masterIp}:${toString labSettings.cachePort}" ];
    trusted-public-keys = if labSettings.cachePublicKey == null then [] else [ labSettings.cachePublicKey ];
  };

  services.harmonia.cache = lib.mkIf isMaster {
    enable = true;
    signKeyPaths = [ signingKeyPath ];
    settings.bind = "[::]:${toString labSettings.cachePort}";
  };

  # Keep the native Harmonia unit while exposing a stable product-level name
  # to the management application and operators.
  systemd.services.harmonia = lib.mkIf isMaster {
    aliases = [ "nixorium-harmonia.service" ];
    wantedBy = [ "multi-user.target" ];
  };
}
