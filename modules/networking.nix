{ hostIp, hostName, isController, labSettings, lib, pkgs, nixoriumClass ? null, ... }:
let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  # A system shared by the computers of a class (ADR 0033) takes the name and
  # addresses from the identity record (modules/identity.nix writes them to
  # this file): one NetworkManager profile, by card type or by name.
  shared = laboratoryEnabled && !isController && nixoriumClass != null && nixoriumClass.shared;
  identityEnvironment = "/run/nixorium/identity.env";
  # mkLab sets `ifaceExplicit` for every host. The controller and any client
  # with a clientIfaceName or hostIfaceNames entry use their configured name.
  # Other clients ignore the template fallback name and configure the wired
  # NIC by type, because real PCs rarely share VirtualBox's enp0s3 (ADR 0017).
  # Callers that do not provide the flag keep the name-based behaviour.
  ifaceExplicit = labSettings.ifaceExplicit or true;
  byType = laboratoryEnabled && !isController && !ifaceExplicit;
  # Every laboratory host, the controller included, is configured only
  # through the NetworkManager profile below: NixOS scripted addresses and
  # dhcpcd on the same card as NetworkManager raced at boot and could drop
  # the static address (owner decision 2026-10-20).
  profile = laboratoryEnabled;
  # The controller and named clients bind the profile to their card.
  byName = profile && !byType;
  prefixLength = labSettings.networkPrefixLength;
  # Clients answer Wake-on-LAN magic packets on their lab card, so Switch on
  # can start them; their firmware must allow it too.
  wakeByName = laboratoryEnabled && !isController && !byType;
  ethtool = "${pkgs.ethtool}/bin/ethtool";
  # During a change of laboratory network every host keeps its previous
  # address next to the new one: the controller so that computers not yet
  # moved still reach it, and clients so that the update that moves them
  # keeps its connection and an undo finds them on the restored network.
  # The record's removal (finish or undo) drops the other address.
  transition = labSettings.networkTransition or null;
  previousHostIp =
    if transition == null then null
    else if isController then transition.masterIp
    else if shared then "$NIXORIUM_PREVIOUS_ADDRESS"
    else transition.hostIps.${hostName} or null;
  labAddress = if shared then "$NIXORIUM_LAB_ADDRESS" else hostIp;
in
{
  # A shared system has no single name. Services that write the configured
  # name into their own files (Apache's ServerName, for example) need a valid
  # one, so the configuration carries a placeholder; the computer's real name
  # is set at activation in /etc/hostname and the kernel (modules/identity.nix),
  # and mDNS announces that real name.
  networking.hostName = if shared then "nixorium-client" else hostName;
  environment.etc.hostname.enable = lib.mkIf shared (lib.mkForce false);
  services.avahi.hostName = lib.mkIf shared (lib.mkDefault "");
  system.name = lib.mkIf shared "nixorium-client-${nixoriumClass.id}";
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
  networking.networkmanager.settings.main = lib.mkIf profile ({
    # Clients: nothing else competes for a wired card. The controller keeps
    # NetworkManager's automatic DHCP on its other cards (an Internet uplink).
    "no-auto-default" = if isController then "interface-name:${labSettings.ifaceName}" else "*";
  } // lib.optionalAttrs shared {
    # DHCP must never rename a computer of a shared system.
    hostname-mode = "none";
  });
  networking.networkmanager.ensureProfiles.environmentFiles = lib.mkIf shared [ identityEnvironment ];
  networking.networkmanager.ensureProfiles.profiles.nixorium-lab = lib.mkIf profile {
    connection = {
      id = "nixorium-lab";
      uuid = "5c0e2a3e-8f3b-4d57-9f0f-3b1e0d6a9c11";
      type = "ethernet";
      autoconnect = "true";
      # A shared system detects a duplicated address (two computers with the
      # same number): NetworkManager then retries a few times and waits five
      # minutes, instead of retrying several times a second.
      autoconnect-retries = if shared then "3" else "0";
      # Preferred over any connection NetworkManager saved for the card
      # earlier (for example by a release that still raced).
      autoconnect-priority = "100";
    } // lib.optionalAttrs byName {
      interface-name = labSettings.ifaceName;
    };
    # Magic-packet wake (0x40 in NetworkManager's flags), set on the card
    # whenever this profile is activated.
    ethernet.wake-on-lan = "64";
    ipv4 = {
      method = "auto";
      address1 = "${labAddress}/${toString prefixLength}";
      dhcp-timeout = "2147483647";
      may-fail = "false";
    } // lib.optionalAttrs shared {
      dad-timeout = "3000";
    } // lib.optionalAttrs (previousHostIp != null) {
      address2 = "${previousHostIp}/${toString transition.prefixLength}";
    };
    ipv6.method = "auto";
  };
  # Re-run the profile writer when its content changes during activation.
  systemd.services.NetworkManager-ensure-profiles = lib.mkIf profile {
    serviceConfig.RemainAfterExit = true;
    path = [ pkgs.networkmanager pkgs.gawk pkgs.gnugrep ];
    # A deployment that changes NetworkManager's settings restarts it, and
    # the profile writer, although ordered after it, can then run before
    # NetworkManager answers ("NetworkManager is not running"). Wait for it,
    # at most a minute.
    preStart = ''
      for _ in $(seq 120); do
        nmcli -t general status >/dev/null 2>&1 && exit 0
        sleep 0.5
      done
    '';
    # Upgrading a running computer from the scripted lab address (clients
    # before 3.9, controllers and named clients before 3.10) replaces it with
    # this profile: the old address is removed and the restarted
    # NetworkManager adopts the card as "connected (externally)", which blocks
    # autoconnect. Activate the profile explicitly, without waiting, unless it
    # is already active or activating (normal boot).
    postStart = ''
      if ! nmcli -t -f NAME connection show --active | grep -Fxq nixorium-lab; then
        device=${if byName then lib.escapeShellArg labSettings.ifaceName else ''$(nmcli -t -f DEVICE,TYPE,STATE device \
          | awk -F: '$2 == "ethernet" && $3 != "unmanaged" { print $1; exit }')''}
        if [ -n "$device" ]; then
          nmcli -w 0 connection up nixorium-lab ifname "$device" || true
        fi
      fi
    '' + lib.optionalString (transition != null) ''
      # During a change of laboratory network the profile gains the new
      # address next to the previous one; apply it to the running card at
      # once. The address the activating SSH session uses stays. After the
      # change the address no longer configured disappears at the next start.
      device=$(nmcli -t -f NAME,DEVICE connection show --active \
        | awk -F: '$1 == "nixorium-lab" { print $2; exit }')
      if [ -n "$device" ]; then
        nmcli device reapply "$device" || true
      fi
    '';
  };

  # A named lab card gets magic-packet wake when it appears at boot. The
  # NixOS wakeOnLan option matches only the kernel's original name (a .link
  # OriginalName), which predictable names such as enp3s0 replace, so udev
  # matches the final name here, or the kernel name when nothing renames it.
  services.udev.extraRules = lib.mkIf wakeByName ''
    ACTION=="add", SUBSYSTEM=="net", NAME=="${labSettings.ifaceName}", RUN+="${ethtool} -s $name wol g"
    ACTION=="add", SUBSYSTEM=="net", NAME=="", KERNEL=="${labSettings.ifaceName}", RUN+="${ethtool} -s $name wol g"
  '';

}
