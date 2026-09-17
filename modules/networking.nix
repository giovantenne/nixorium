{ hostIp, hostName, labSettings, lib, ... }:
{
  networking.hostName = hostName;
  # Controller-only bootstrap uses NetworkManager, without a speculative lab IP.
  networking.interfaces = lib.mkIf ((labSettings.deploymentMode or "laboratory") == "laboratory") {
    ${labSettings.ifaceName} = {
      useDHCP = lib.mkDefault true;
      ipv4.addresses = [
        {
          address = hostIp;
          prefixLength = labSettings.networkPrefixLength;
        }
      ];
    };
  };
}
