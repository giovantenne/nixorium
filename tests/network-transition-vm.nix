# A change of laboratory network in progress, from 10.0.0.0/24 to 10.1.0.0/24.
# The controller and the client use the laboratory networking, firewall,
# cache and Internet-block modules with the settings mkLab derives from a
# transition record. The client is already on the new network: it accepts SSH
# from both controller addresses, from no other address, and reaches the cache
# through the new controller address.
{ transitionLab }:
{ pkgs, lib, ... }:
let
  keys = import "${pkgs.path}/nixos/tests/ssh-keys.nix" pkgs;
  # Fixed test-only signing material shared with the installer VM tests; its
  # public half is the lab's cache key.
  cacheSecretKey = pkgs.writeText "network-transition-cache-secret"
    "nixorium-remote-vm-1:LyFjyE1IpGYQenMUqGQKq0fZSXr5uaoqIldXReT6v1wsQKHQIF/ewV58htcET9bsFyS9r9jMjxsRZVnv3sSXfA==";
  hostArgs = name: {
    inherit (transitionLab.nixosConfigurations.${name}._module.specialArgs) hostName hostIp labSettings;
  };
  laboratoryModules = [
    ../modules/networking.nix
    ../modules/firewall.nix
    ../modules/cache.nix
  ];
in
{
  name = "nixorium-network-transition";

  nodes = {
    controller = { ... }: {
      imports = laboratoryModules;
      _module.args = hostArgs "pc99";
      virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
      systemd.tmpfiles.rules = [
        "d /var/lib/nixorium/keys 0700 root root -"
        "C /var/lib/nixorium/keys/harmonia-secret-key 0600 root root - ${cacheSecretKey}"
      ];
      environment.systemPackages = [ pkgs.openssh pkgs.iproute2 ];
      system.stateVersion = "26.05";
    };

    client = { ... }: {
      imports = laboratoryModules ++ [ ../modules/client-internet.nix ];
      _module.args = hostArgs "pc01";
      virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
      networking.networkmanager.enable = true;
      # A PC with a single wired card: the test driver's own card is not part
      # of the laboratory.
      networking.networkmanager.unmanaged = [ "interface-name:eth0" ];
      services.openssh = { enable = true; settings.PermitRootLogin = "prohibit-password"; };
      users.users.root.openssh.authorizedKeys.keys = [ keys.snakeOilPublicKey ];
      environment.systemPackages = [ pkgs.curl pkgs.iproute2 ];
      system.stateVersion = "26.05";
    };
  };

  testScript = ''
    start_all()
    controller.wait_for_unit("multi-user.target")
    client.wait_for_unit("multi-user.target")

    with subtest("the controller carries both laboratory addresses"):
      controller.wait_until_succeeds("ip -4 -o address show dev eth1 | grep -F ' 10.1.0.99/24 '")
      controller.succeed("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.99/24 '")
      controller.wait_for_unit("harmonia.service")
      controller.wait_for_open_port(5000)

    with subtest("the client has only the new address"):
      client.wait_until_succeeds("ip -4 -o address show dev eth1 | grep -F ' 10.1.0.1/24 '", timeout=90)
      client.fail("ip -4 -o address show | grep -F ' 10.0.0.'")
      client.succeed("grep -E '^substituters = http://10\\.1\\.0\\.99:5000 http://10\\.0\\.0\\.99:5000( |$)' /etc/nix/nix.conf")

    with subtest("the client reaches the cache through the new controller address"):
      client.succeed("curl -fsS --max-time 10 http://10.1.0.99:5000/nix-cache-info | grep -F StoreDir")
      # Without a route to the previous network only the new address works.
      client.fail("curl -fsS --max-time 5 http://10.0.0.99:5000/nix-cache-info")
      path = controller.succeed("echo laboratory-network-transition > /tmp/payload; nix-store --add /tmp/payload").strip()
      client.fail(f"test -e {path}")
      client.succeed(f"nix-store --realise {path}")
      client.succeed(f"grep -Fx laboratory-network-transition {path}")

    controller.succeed("install -m 0600 ${keys.snakeOilPrivateKey} /root/key")
    ssh = "ssh -i /root/key -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=5 "

    with subtest("the client accepts SSH from the new controller address"):
      controller.wait_until_succeeds(ssh + "-b 10.1.0.99 root@10.1.0.1 true", timeout=60)

    # Test aid: computers still on the previous network reach the client
    # directly; here a route lets the client answer the previous address.
    client.succeed("ip route add 10.0.0.0/24 dev eth1")

    with subtest("the client accepts SSH from the previous controller address"):
      controller.succeed(ssh + "-b 10.0.0.99 root@10.1.0.1 true")
      # The cache answers on the previous address as well.
      client.succeed("curl -fsS --max-time 10 http://10.0.0.99:5000/nix-cache-info | grep -F StoreDir")

    with subtest("the client refuses SSH from any other address"):
      controller.succeed("ip address add 10.1.0.50/24 dev eth1")
      controller.fail(ssh + "-b 10.1.0.50 root@10.1.0.1 true")
      controller.succeed("ip address del 10.1.0.50/24 dev eth1")

    with subtest("the classroom Internet block keeps both laboratory networks reachable"):
      client.succeed("systemctl start nixorium-internet-block")
      controller.succeed(ssh + "-b 10.1.0.99 root@10.1.0.1 true")
      controller.succeed(ssh + "-b 10.0.0.99 root@10.1.0.1 true")
      client.succeed("curl -fsS --max-time 10 http://10.1.0.99:5000/nix-cache-info")
      client.succeed("systemctl stop nixorium-internet-block")
  '';
}
