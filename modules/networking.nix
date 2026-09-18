{ hostIp, hostName, labSettings, lib, ... }:
let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  isController = hostName == labSettings.masterHostName;
in
{
  networking.hostName = hostName;
  # Controller-only bootstrap uses NetworkManager, without a speculative lab IP.
  networking.interfaces = lib.mkIf laboratoryEnabled {
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

  # The interface already exists when a controller-only installation switches
  # live to laboratory mode, so no new udev event starts this generated unit.
  # Linking it to the active network target also applies the static address
  # during switch-to-configuration; PXE later removes it transactionally.
  systemd.services."network-addresses-${labSettings.ifaceName}" = lib.mkIf (laboratoryEnabled && isController) {
    wantedBy = [ "network.target" ];
  };
}
