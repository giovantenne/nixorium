{ hostIp, hostName, labSettings, lib, pkgs, ... }:
let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  isController = hostName == labSettings.masterHostName;
  # mkLab sets `ifaceExplicit` for every host. The controller and any client
  # with a clientIfaceName or hostIfaceNames entry use their configured name.
  # Other clients ignore the template fallback name and configure the wired
  # NIC by type, because real PCs rarely share VirtualBox's enp0s3 (ADR 0017).
  # Callers that do not provide the flag keep the name-based behaviour.
  ifaceExplicit = labSettings.ifaceExplicit or true;
  byType = laboratoryEnabled && !isController && !ifaceExplicit;
  prefixLength = labSettings.networkPrefixLength;
  # During a change of laboratory network the controller also keeps its
  # previous address on the laboratory interface, so computers not yet moved
  # still reach it. Clients take only the new address.
  transition = labSettings.networkTransition or null;
  previousControllerAddress = lib.optional (isController && transition != null) {
    address = transition.masterIp;
    prefixLength = transition.prefixLength;
  };
in
{
  networking.hostName = hostName;
  # Controller-only bootstrap uses NetworkManager, without a speculative lab IP.
  networking.interfaces = lib.mkIf (laboratoryEnabled && !byType) {
    ${labSettings.ifaceName} = {
      useDHCP = lib.mkDefault true;
      ipv4.addresses = [
        {
          address = hostIp;
          prefixLength = prefixLength;
        }
      ] ++ previousControllerAddress;
    };
  };

  # Name-independent client network. NetworkManager already runs on every
  # host, so one declarative profile for any wired Ethernet device (wireless,
  # bridges, veth and other virtual devices are different NetworkManager
  # types) avoids a second network backend. It keeps DHCP for addresses, the
  # default route and DNS, and adds the static lab address, which is applied
  # at once and stays even when no DHCP server answers (infinite DHCP timeout).
  # A profile without an interface name is active on one device at a time, so
  # with several wired NICs the first one to come up carries it; automatic
  # DHCP-only profiles for the other NICs are disabled so that nothing else
  # competes. Such PCs should use hostIfaceNames.
  networking.networkmanager.settings.main."no-auto-default" = lib.mkIf byType "*";
  networking.networkmanager.ensureProfiles.profiles.nixorium-lab = lib.mkIf byType {
    connection = {
      id = "nixorium-lab";
      uuid = "5c0e2a3e-8f3b-4d57-9f0f-3b1e0d6a9c11";
      type = "ethernet";
      autoconnect = "true";
      autoconnect-retries = "0";
    };
    ethernet = { };
    ipv4 = {
      method = "auto";
      address1 = "${hostIp}/${toString prefixLength}";
      dhcp-timeout = "2147483647";
      may-fail = "false";
    };
    ipv6.method = "auto";
  };
  # Re-run the profile writer when its content changes during activation.
  systemd.services.NetworkManager-ensure-profiles = lib.mkIf byType {
    serviceConfig.RemainAfterExit = true;
    path = [ pkgs.networkmanager pkgs.gawk pkgs.gnugrep ];
    # Upgrading a running client (Colmena switch) replaces the scripted lab
    # address with this profile: the old address is removed and the restarted
    # NetworkManager adopts the card as "connected (externally)", which blocks
    # autoconnect. Activate the profile explicitly, without waiting, unless it
    # is already active or activating (normal boot).
    postStart = ''
      if ! nmcli -t -f NAME connection show --active | grep -Fxq nixorium-lab; then
        device=$(nmcli -t -f DEVICE,TYPE,STATE device \
          | awk -F: '$2 == "ethernet" && $3 != "unmanaged" { print $1; exit }')
        if [ -n "$device" ]; then
          nmcli -w 0 connection up nixorium-lab ifname "$device" || true
        fi
      fi
    '';
  };

  # The interface already exists when a controller-only installation switches
  # live to laboratory mode, so no new udev event starts this generated unit.
  # Linking it to the active network target also applies the static address
  # during switch-to-configuration; PXE later removes it transactionally.
  systemd.services."network-addresses-${labSettings.ifaceName}" = lib.mkIf (laboratoryEnabled && isController) {
    wantedBy = [ "network.target" ];
  };
}
