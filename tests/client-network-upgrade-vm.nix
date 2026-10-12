# Live upgrade of an existing client from the old name-based scripted network
# configuration to the name-independent NetworkManager profile (and back),
# activated from the controller over SSH exactly as a deployment does it.
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

    client = { config, lib, ... }: {
      imports = [ networkingModule ];
      options.test.explicit = lib.mkOption { type = lib.types.bool; default = true; };
      config = {
        # Same inputs mkLab gives modules/networking.nix. `explicit = true` is
        # the previous release's behaviour (name-based scripted networking).
        _module.args = {
          hostName = "pc01";
          hostIp = "10.0.0.1";
          labSettings = {
            deploymentMode = "laboratory";
            masterHostName = "pc99";
            ifaceName = "eth1";
            ifaceExplicit = config.test.explicit;
            networkPrefixLength = 24;
          };
        };
        virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
        networking.networkmanager.enable = true;
        # Model a PC with a single wired card: the test driver's NAT card is
        # not part of the laboratory.
        networking.networkmanager.unmanaged = [ "interface-name:eth0" ];
        networking.firewall.enable = false;
        services.openssh = { enable = true; settings.PermitRootLogin = "prohibit-password"; };
        users.users.root.openssh.authorizedKeys.keys = [ keys.snakeOilPublicKey ];
        specialisation.bytype.configuration.test.explicit = lib.mkForce false;
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

    # Previous release: explicit name, scripted networking.
    lab_ip_present()
    client.succeed("systemctl cat network-addresses-eth1.service")
    client.succeed("test ! -e /run/NetworkManager/system-connections/nixorium-lab.nmconnection")

    # Upgrade to name-independent while the controller is connected.
    controller.execute("ping -i 0.2 10.0.0.1 > /tmp/ping.log 2>&1 &")
    deploy(base + "/specialisation/bytype")
    client.wait_until_succeeds("nmcli -t -f NAME,DEVICE connection show --active | grep -Fx 'nixorium-lab:eth1'", timeout=90)
    client.fail("systemctl cat network-addresses-eth1.service")
    lab_ip_present()
    # A second, no-op deployment succeeds as well.
    deploy(base + "/specialisation/bytype")
    client.succeed("nmcli -t -f NAME,DEVICE connection show --active | grep -Fx 'nixorium-lab:eth1'")
    client.succeed("test -z \"$(systemctl --failed --no-legend)\"")

    # Reverse: back to an explicit interface name.
    deploy(base)
    client.succeed("systemctl cat network-addresses-eth1.service")
    client.succeed("test -z \"$(systemctl --failed --no-legend)\"")
    deploy(base)
    print(controller.succeed("pkill ping; tail -3 /tmp/ping.log"))
  '';
}
