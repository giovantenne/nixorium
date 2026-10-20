# Live upgrade of a computer from the scripted laboratory address of earlier
# releases (clients before 3.9, controllers and named clients before 3.10) to
# the NetworkManager profile, bound to the card by name, matched by card type,
# or as the controller; and back, as a rollback would. Activated from the
# controller over SSH exactly as a deployment does it.
{ networkingModule }:
{ pkgs, lib, ... }:
let
  keys = import "${pkgs.path}/nixos/tests/ssh-keys.nix" pkgs;
in
{
  name = "nixorium-client-network-upgrade";

  nodes = {
    controller = { ... }: {
      virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
      networking.interfaces.eth1.ipv4.addresses = [{ address = "10.0.0.99"; prefixLength = 24; }];
      networking.firewall.enable = false;
      environment.systemPackages = [ pkgs.openssh pkgs.iputils ];
      system.stateVersion = "25.11";
    };

    client = { config, lib, ... }:
      let
        mode = config.test.mode;
        scripted = mode == "scripted";
      in
      {
        imports = [ networkingModule ];
        options.test.mode = lib.mkOption {
          type = lib.types.enum [ "scripted" "named" "bytype" "controller" ];
          default = "scripted";
        };
        config = {
          # Same inputs mkLab gives modules/networking.nix. The scripted mode
          # is the earlier releases' configuration, written out here: the
          # module then runs in controller-only mode and adds nothing.
          _module.args = {
            hostName = "pc01";
            isController = mode == "controller";
            hostIp = "10.0.0.1";
            nixoriumClass = { id = "pc01"; shared = false; members = [ "pc01" ]; representative = "pc01"; };
            labSettings = {
              deploymentMode = if scripted then "controller" else "laboratory";
              masterHostName = "pc99";
              ifaceName = "eth1";
              ifaceExplicit = mode != "bytype";
              networkPrefixLength = 24;
            };
          };
          networking.interfaces = lib.mkIf scripted { eth1.ipv4.addresses = [{ address = "10.0.0.1"; prefixLength = 24; }]; };
          # mkIf on the whole set: an empty definition would still declare
          # the unit.
          systemd.services = lib.mkIf scripted { network-addresses-eth1.wantedBy = [ "network.target" ]; };
          virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
          networking.networkmanager.enable = true;
          # Model a PC with a single wired card: the test driver's NAT card is
          # not part of the laboratory.
          networking.networkmanager.unmanaged = [ "interface-name:eth0" ];
          networking.firewall.enable = false;
          services.openssh = { enable = true; settings.PermitRootLogin = "prohibit-password"; };
          users.users.root.openssh.authorizedKeys.keys = [ keys.snakeOilPublicKey ];
          specialisation = lib.genAttrs [ "named" "bytype" "controller" ] (name: {
            configuration.test.mode = lib.mkForce name;
          });
          system.stateVersion = "25.11";
        };
      };
  };

  testScript = ''
    start_all()
    client.wait_for_unit("multi-user.target")
    controller.wait_for_unit("multi-user.target")
    controller.succeed("install -m 0600 ${keys.snakeOilPrivateKey} /root/key")
    ssh = "ssh -i /root/key -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=5 root@10.0.0.1 "

    base = client.succeed("readlink -f /run/current-system").strip()
    controller.wait_until_succeeds(ssh + "true")

    def lab_ip_present(name="eth1"):
      client.wait_until_succeeds(f"ip -4 -o address show dev {name} | grep -F ' 10.0.0.1/24 '", timeout=60)

    def deploy(target):
      # Like a Colmena activation: switch-to-configuration inside an SSH session.
      status, output = controller.execute("timeout 120 " + ssh + "-o ServerAliveInterval=5 -o ServerAliveCountMax=3 " + f"{target}/bin/switch-to-configuration test")
      print(output)
      if status != 0:
        print(client.execute("ip addr; ip route; nmcli device; nmcli connection; systemctl status NetworkManager NetworkManager-ensure-profiles --no-pager; journalctl -u NetworkManager -u NetworkManager-ensure-profiles -b --no-pager | tail -50")[1])
        raise Exception("deployment lost the client")
      controller.wait_until_succeeds(ssh + "true")
      lab_ip_present()
      controller.succeed("ping -c 2 -W 2 10.0.0.1")

    # Earlier release: scripted laboratory address.
    lab_ip_present()
    client.succeed("systemctl cat network-addresses-eth1.service")
    client.succeed("test ! -e /run/NetworkManager/system-connections/nixorium-lab.nmconnection")

    def profile_active():
      client.wait_until_succeeds("nmcli -t -f NAME,DEVICE connection show --active | grep -Fx 'nixorium-lab:eth1'", timeout=90)
      client.fail("systemctl cat network-addresses-eth1.service")
      client.fail("systemctl cat dhcpcd.service")
      lab_ip_present()

    controller.execute("ping -i 0.2 10.0.0.1 > /tmp/ping.log 2>&1 &")
    # Upgrade to the profile bound to the card by name (a named client in a
    # class of its own), then a no-op deployment.
    deploy(base + "/specialisation/named")
    profile_active()
    client.succeed("grep -Fx 'interface-name=eth1' /run/NetworkManager/system-connections/nixorium-lab.nmconnection")
    deploy(base + "/specialisation/named")
    profile_active()
    client.succeed("test -z \"$(systemctl --failed --no-legend)\"")

    # Matched by card type, then as the controller (bound by name; automatic
    # DHCP stays allowed on its other cards).
    deploy(base + "/specialisation/bytype")
    profile_active()
    deploy(base + "/specialisation/controller")
    profile_active()
    client.succeed("grep -Fx 'no-auto-default=interface-name:eth1' /etc/NetworkManager/NetworkManager.conf")
    client.succeed("test -z \"$(systemctl --failed --no-legend)\"")

    # Network installation (modules/pxe.nix) takes the laboratory address
    # away from the connection applied to the card only, and gives it back;
    # the profile itself keeps it, so a restart restores it too.
    client.succeed("nmcli device modify eth1 -ipv4.addresses 10.0.0.1/24")
    client.wait_until_fails("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.1/24 '", timeout=30)
    client.succeed("nmcli -t -f NAME,DEVICE connection show --active | grep -Fx 'nixorium-lab:eth1'")
    client.succeed("nmcli -g ipv4.addresses connection show nixorium-lab | grep -F 10.0.0.1/24")
    client.succeed("nmcli device modify eth1 +ipv4.addresses 10.0.0.1/24")
    lab_ip_present()
    client.succeed("nmcli device modify eth1 -ipv4.addresses 10.0.0.1/24")
    client.wait_until_fails("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.1/24 '", timeout=30)
    client.succeed("systemctl restart NetworkManager")
    lab_ip_present()
    controller.wait_until_succeeds(ssh + "true")

    # Straight from the scripted address to the controller profile, as an
    # existing controller upgrades.
    deploy(base)
    client.succeed("systemctl cat network-addresses-eth1.service")
    deploy(base + "/specialisation/controller")
    profile_active()

    # Rollback to the earlier release keeps the computer reachable.
    deploy(base)
    client.succeed("systemctl cat network-addresses-eth1.service")
    client.succeed("test -z \"$(systemctl --failed --no-legend)\"")
    deploy(base)
    print(controller.succeed("pkill ping; tail -3 /tmp/ping.log"))
  '';
}
