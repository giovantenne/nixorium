{ remoteInstallerBundle, clientSystem, deploymentRevision }:
let
  # Fixed test-only signing material. It authenticates only closures served by
  # this isolated VM test and is never included by a product module or output.
  cacheSecretKey = "nixorium-remote-vm-1:LyFjyE1IpGYQenMUqGQKq0fZSXr5uaoqIldXReT6v1wsQKHQIF/ewV58htcET9bsFyS9r9jMjxsRZVnv3sSXfA==";
  cachePublicKey = "nixorium-remote-vm-1:LECh0CBf3sFefIbXBE/W7Bckva/YzI8bEWVZ797El3w=";
  adminPublicKey = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ./fixtures/remote-vm-admin.pub);
in
{
  name = "nixorium-remote-client-installer";

  nodes = {
    cache = { pkgs, ... }: {
      virtualisation.additionalPaths = [ remoteInstallerBundle clientSystem ];
      environment.systemPackages = [ pkgs.iproute2 pkgs.jq pkgs.nix ];
      networking.firewall.allowedTCPPorts = [ 5000 ];
      services.harmonia.cache = {
        enable = true;
        signKeyPaths = [ (pkgs.writeText "remote-vm-cache-secret" cacheSecretKey) ];
        settings.bind = "[::]:5000";
      };
      system.stateVersion = "25.11";
    };

    installer = { pkgs, modulesPath, ... }: {
      imports = [ (modulesPath + "/testing/test-instrumentation.nix") ];
      virtualisation = {
        memorySize = 4096;
        emptyDiskImages = [ 20480 ];
        useBootLoader = true;
        useEFIBoot = true;
      };
      boot.loader.systemd-boot.enable = true;
      boot.loader.efi.canTouchEfiVariables = true;
      environment.systemPackages = [ pkgs.curl pkgs.jq pkgs.nix pkgs.openssh pkgs.sudo ];
      nix.settings.experimental-features = [ "nix-command" "flakes" ];
      nix.settings.substituters = [ ];
      security.sudo.wheelNeedsPassword = false;
      services.openssh.enable = true;
      system.stateVersion = "25.11";
    };

    target = {
      virtualisation = {
        diskImage = "./empty0.qcow2";
        memorySize = 4096;
        useBootLoader = true;
        useEFIBoot = true;
      };
      system.stateVersion = "25.11";
    };
  };

  testScript = ''
    import base64
    import json
    import os

    cache.start()
    cache.wait_for_unit("harmonia.socket")
    installer.start()
    installer.wait_for_unit("multi-user.target")
    installer.succeed("test -d /sys/firmware/efi; ip link set eth0 down; test -z \"$(ip -4 route show default)\"")
    installer.fail("test -e ${remoteInstallerBundle}")
    installer.fail("test -e ${clientSystem}")
    cache_ip = cache.succeed("ip -j -4 address show dev eth1 | jq -r '.[0].addr_info[0].local'").strip()
    live = json.loads(installer.succeed("ip -j -4 address show dev eth1"))[0]
    live_iface = live["ifname"]
    live_ip = live["addr_info"][0]["local"]
    installer.succeed(
      "nix --extra-experimental-features 'nix-command' copy "
      f"--from http://{cache_ip}:5000 ${remoteInstallerBundle} "
      f"--option substituters http://{cache_ip}:5000 --option trusted-public-keys '${cachePublicKey}' "
      "--option require-sigs true --option fallback false"
    )
    installer.succeed("test -e ${remoteInstallerBundle}; test ! -e ${clientSystem}")
    facts = json.loads(installer.succeed("${remoteInstallerBundle}/bin/nixorium-remote-client-installer probe"))
    disk = next(item for item in facts["disks"] if item["path"] == "/dev/vdb")
    host_key = installer.succeed("tr -d '\\n' < /etc/ssh/ssh_host_ed25519_key.pub").strip()
    plan = {
      "schemaVersion": 1,
      "operationId": "0123456789abcdef0123456789abcdef",
      "bootId": facts["bootId"],
      "deploymentRevision": "${deploymentRevision}",
      "systemPath": "${clientSystem}",
      "host": {"name": "pc01", "interface": live_iface, "liveIp": live_ip, "staticIp": "10.0.0.1"},
      "cache": {"url": f"http://{cache_ip}:5000", "publicKey": "${cachePublicKey}"},
      "disk": {key: disk.get(key, "") for key in ["path", "kname", "majorMinor", "sizeBytes", "serial", "wwn", "model", "transport", "diskSeq"]},
      "adminPublicKey": "${adminPublicKey}",
      "hostKeyPublic": host_key,
      "hostKeyRotation": False,
    }
    encoded_plan = base64.b64encode(json.dumps(plan).encode()).decode()
    installer.succeed(f"printf '%s' '{encoded_plan}' | base64 -d > /tmp/remote-plan.json")
    receipt = json.loads(installer.succeed("${remoteInstallerBundle}/bin/nixorium-remote-client-installer apply < /tmp/remote-plan.json"))
    assert receipt["state"] == "accepted" and not receipt["mutationStarted"]
    installer.wait_until_fails("systemctl is-active --quiet nixorium-remote-install-0123456789abcdef0123456789abcdef.service", timeout=900)
    receipt = json.loads(installer.succeed("${remoteInstallerBundle}/bin/nixorium-remote-client-installer status 0123456789abcdef0123456789abcdef"))
    if receipt["state"] != "ready-to-reboot" or not receipt["installed"] or not receipt["diskMayBeModified"]:
      operation_log = installer.succeed("${remoteInstallerBundle}/bin/nixorium-remote-client-installer log 0123456789abcdef0123456789abcdef")
      journal = installer.succeed("journalctl -u nixorium-remote-install-0123456789abcdef0123456789abcdef.service --no-pager")
      raise Exception(f"remote installer failed: {receipt!r}\n{operation_log}\n{journal}")
    installer.succeed("${remoteInstallerBundle}/bin/nixorium-remote-client-installer log 0123456789abcdef0123456789abcdef > /tmp/remote-operation.log; test $(stat -c %s /tmp/remote-operation.log) -le 1048576")
    installer.succeed("test -L /mnt/nix/var/nix/profiles/system; test \"$(readlink -f /mnt/nix/var/nix/profiles/system)\" = ${clientSystem}")
    installer.succeed("umount -R /mnt; sync")
    installer.shutdown()

    target.state_dir = installer.state_dir
    os.environ["NIX_EFI_VARS"] = str(installer.state_dir / "installer-efi-vars.fd")
    target.start()
    target.wait_for_unit("multi-user.target")
    target.succeed("test \"$(hostname)\" = pc01")
    target.succeed("test \"$(readlink -f /run/current-system)\" = ${clientSystem}")
    target.succeed("test \"$(nixos-version --configuration-revision)\" = ${deploymentRevision}")
  '';
}
