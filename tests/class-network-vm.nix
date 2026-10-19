# Clients that share one system take their name and laboratory address from
# the identity record (ADR 0033), with the real networking and identity
# modules: a shared system from the start, a live switch from a per-host
# system configured by card name, and a duplicated number.
{ networkingModule, identityModule }:
{ pkgs, lib, ... }:
let
  keys = import "${pkgs.path}/nixos/tests/ssh-keys.nix" pkgs;
  record = "/var/lib/nixorium/identity.json";
  env = "/run/nixorium/identity.env";
  labSettings = {
    deploymentMode = "laboratory";
    masterHostName = "pc99";
    ifaceName = "eth1";
    networkPrefixLength = 24;
    clientIdentities = [
      { number = 1; name = "pc01"; address = "10.0.0.1"; }
      { number = 2; name = "pc02"; address = "10.0.0.2"; }
      { number = 3; name = "pc03"; address = "10.0.0.3"; }
    ];
  };
  sharedClass = { id = "shared-1"; shared = true; members = [ "pc01" "pc02" "pc03" ]; representative = "pc01"; };
  unknown = name: throw "${name} is not known in a shared configuration";
  common = {
    imports = [ networkingModule identityModule ];
    virtualisation.memorySize = 512;
    virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
    networking.networkmanager.enable = true;
    networking.networkmanager.unmanaged = [ "interface-name:eth0" ];
    networking.firewall.enable = false;
    services.openssh = { enable = true; settings.PermitRootLogin = "prohibit-password"; };
    users.users.root.openssh.authorizedKeys.keys = [ keys.snakeOilPublicKey ];
    system.stateVersion = "25.11";
  };
in
{
  name = "nixorium-class-network";

  nodes = {
    controller = { ... }: {
      virtualisation.memorySize = 768;
      virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
      networking.interfaces.eth1.ipv4.addresses = [{ address = "10.0.0.99"; prefixLength = 24; }];
      networking.firewall.enable = false;
      environment.systemPackages = [ pkgs.openssh pkgs.iputils ];
      system.stateVersion = "25.11";
    };
    # A shared system configured by card type, without a record at first.
    a = { ... }: {
      imports = [ common ];
      _module.args = {
        hostName = unknown "hostName";
        hostIp = unknown "hostIp";
        isController = false;
        nixoriumClass = sharedClass;
        labSettings = labSettings // { ifaceExplicit = false; };
      };
    };
    # pc02 on its per-host system with a named card (scripted networking),
    # switched live to the shared system that binds the profile to the card.
    b = { lib, ... }: {
      imports = [ common ];
      _module.args = {
        hostName = "pc02";
        hostIp = "10.0.0.2";
        isController = false;
        nixoriumClass = { id = "pc02"; shared = false; members = [ "pc02" ]; representative = "pc02"; };
        labSettings = labSettings // { ifaceExplicit = true; };
      };
      specialisation.class.configuration._module.args = {
        hostName = lib.mkForce (unknown "hostName");
        hostIp = lib.mkForce (unknown "hostIp");
        nixoriumClass = lib.mkForce sharedClass;
      };
    };
  };

  testScript = ''
    start_all()
    controller.wait_for_unit("multi-user.target")
    controller.succeed("install -m 0600 ${keys.snakeOilPrivateKey} /root/key")
    ssh = "ssh -i /root/key -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=5 "

    def show(machine):
      print(machine.execute("hostname; cat ${env}; ip -4 -o address show dev eth1; nmcli -t connection show --active; systemctl --failed --no-legend")[1])

    def write_record(machine, number):
      machine.succeed(f"mkdir -p /var/lib/nixorium && printf '{{\"schemaVersion\":1,\"number\":{number}}}\\n' > ${record}")

    with subtest("A shared system without a record fails visibly and has no lab address"):
      a.wait_for_unit("multi-user.target")
      a.wait_until_fails("systemctl is-active nixorium-identity.service")
      a.succeed("systemctl is-failed nixorium-identity.service")
      a.fail("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.'")
      a.fail("test -e ${env}")

    with subtest("With a record the shared system takes its name and address"):
      write_record(a, 1)
      a.shutdown()
      a.start()
      a.wait_for_unit("multi-user.target")
      a.wait_until_succeeds("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.1/24 '", timeout=60)
      show(a)
      assert a.succeed("hostname").strip() == "pc01"
      assert a.succeed("hostnamectl hostname").strip() == "pc01"
      a.succeed("getent hosts pc01")
      a.succeed("grep -Fx NIXORIUM_HOST_NUMBER=1 ${env}")
      a.succeed("systemctl is-active nixorium-identity.service")
      print("wake-on-lan:", a.succeed("nmcli -g 802-3-ethernet.wake-on-lan connection show nixorium-lab"))
      a.succeed("nmcli -g 802-3-ethernet.wake-on-lan connection show nixorium-lab | grep -E 'magic|64'")
      controller.wait_until_succeeds(ssh + "root@10.0.0.1 true", timeout=60)
      assert controller.succeed(ssh + "root@10.0.0.1 hostname").strip() == "pc01"

    with subtest("A per-host system writes its record and keeps its scripted card"):
      b.wait_for_unit("multi-user.target")
      b.wait_until_succeeds("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.2/24 '", timeout=60)
      b.succeed("grep -Fx '{\"schemaVersion\":1,\"number\":2}' ${record}")
      b.succeed("systemctl cat network-addresses-eth1.service")
      controller.wait_until_succeeds(ssh + "root@10.0.0.2 true", timeout=60)

    with subtest("A live switch to the shared system keeps the computer reachable"):
      base = b.succeed("readlink -f /run/current-system").strip()
      status, output = controller.execute("timeout 180 " + ssh + "-o ServerAliveInterval=5 -o ServerAliveCountMax=6 root@10.0.0.2 " + base + "/specialisation/class/bin/switch-to-configuration test")
      print(output)
      show(b)
      controller.wait_until_succeeds(ssh + "root@10.0.0.2 true", timeout=90)
      assert controller.succeed(ssh + "root@10.0.0.2 hostname").strip() == "pc02"
      b.wait_until_succeeds("nmcli -t -f NAME,DEVICE connection show --active | grep -Fx 'nixorium-lab:eth1'", timeout=60)
      b.fail("systemctl cat network-addresses-eth1.service")
      b.succeed("test -z \"$(systemctl --failed --no-legend)\"")
      assert status == 0, "switch-to-configuration failed"

    with subtest("A duplicated number does not take the address and retries only a few times"):
      write_record(b, 1)
      b.succeed(base + "/specialisation/class/bin/switch-to-configuration test")
      b.succeed("grep -Fx NIXORIUM_LAB_ADDRESS=10.0.0.1 ${env}")
      b.succeed("systemctl restart NetworkManager-ensure-profiles.service")
      b.execute("nmcli -w 20 connection up nixorium-lab ifname eth1")
      b.sleep(20)
      b.fail("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.1/24 '")
      refused = int(b.succeed("journalctl -b -u NetworkManager --no-pager | grep -c 'already in use' || true").strip())
      print("activations refused by duplicate address detection:", refused)
      assert 1 <= refused <= 6, f"NetworkManager retried {refused} times"
      controller.succeed("ping -c 3 -W 2 10.0.0.1")
      assert controller.succeed(ssh + "root@10.0.0.1 hostname").strip() == "pc01"
  '';
}
