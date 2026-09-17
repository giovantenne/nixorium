{ hostName, labSettings, lib, ... }:
let
  isController = hostName == labSettings.masterHostName;
  useNativeVeyon = builtins.elem hostName labSettings.veyonNativeHosts;
  interfaceRules = {
    allowedTCPPorts = [
      22
      11100
    ]
    ++ lib.optional (!useNativeVeyon) 5900
    ++ lib.optionals isController [
      labSettings.cachePort
      labSettings.pxeHttpPort
    ];
    allowedUDPPorts = [ 5353 ] ++ lib.optionals isController [
      67
      69
      4011
    ];
  };
in
{
  networking.firewall = {
    enable = true;
    interfaces = lib.mkIf ((labSettings.deploymentMode or "laboratory") == "laboratory") {
      ${labSettings.ifaceName} = interfaceRules;
    };
  };

  # Avoid the OpenSSH module adding port 22 on every controller interface.
  services.openssh.openFirewall = false;

  # Preserve desktop mDNS discovery without opening it on other interfaces.
  services.avahi.openFirewall = false;
}
