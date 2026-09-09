{ hostIp, hostName, labSettings, lib, ... }:
{
  networking.hostName = hostName;
  networking.interfaces = {
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
