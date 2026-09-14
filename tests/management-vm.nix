{ nixoriumPackage }:
{
  name = "nixorium-management";

  nodes.controller = { pkgs, ... }:
  let
    fakeRuntime = pkgs.buildEnv {
      name = "nixorium-test-system-path";
      paths = [
        pkgs.bash
        pkgs.colmena
        pkgs.coreutils
        pkgs.git
        pkgs.gnugrep
        pkgs.jq
        pkgs.nix
        pkgs.shadow
        pkgs.systemd
        pkgs.util-linux
        fakeColmena
        fakeHostState
        nixoriumPackage
      ];
    };
    fakeControllerSystem = pkgs.runCommand "nixorium-test-controller-system" { } ''
      mkdir -p "$out/bin"
      ln -s ${fakeRuntime} "$out/sw"
      cat > "$out/bin/switch-to-configuration" <<'SCRIPT'
      #!${pkgs.runtimeShell}
      [[ "''${1:-}" == switch ]] || exit 2
      SYSTEM_PATH="$(${pkgs.coreutils}/bin/dirname "$(${pkgs.coreutils}/bin/dirname "$(${pkgs.coreutils}/bin/readlink -f "$0")")")"
      ${pkgs.coreutils}/bin/ln -sfn "$SYSTEM_PATH" /run/current-system
      ${pkgs.coreutils}/bin/touch /run/nixorium-controller-applied
      SCRIPT
      chmod +x "$out/bin/switch-to-configuration"
      touch "$out/bzImage" "$out/initrd" "$out/snponly.efi"
      cat > "$out/netboot.ipxe" <<'IPXE'
      #!ipxe
      kernel bzImage init=/nix/store/test-init
      IPXE
    '';
    fakeColmena = pkgs.writeShellScriptBin "nixorium-test-colmena" ''
      set -eu
      printf '%s\n' "$*" >> "''${NIXORIUM_TEST_COLMENA_INVOCATIONS}"
      printf 'fake colmena: %s\n' "$*"
      if [ "''${NIXORIUM_TEST_COLMENA_FAIL:-}" = "''${1:-}" ]; then
        exit 42
      fi
    '';
    fakeHostState = pkgs.writeShellScriptBin "nixorium-host-state" ''
      set -eu
      readlink -f /run/current-system
      git -c safe.directory=/home/admin/nixorium-deployment -C /home/admin/nixorium-deployment rev-parse HEAD
    '';
  in
  {
    imports = [
      ../modules/cache.nix
      ../modules/firewall.nix
      ../modules/management.nix
      ../modules/pxe.nix
    ];

    _module.args = {
      hostName = "pc99";
      labSettings = {
        masterHostName = "pc99";
        masterIp = "10.0.0.99";
        masterDhcpIp = "192.0.2.10";
        networkPrefixLength = 8;
        ifaceName = "lab0";
        cachePort = 5000;
        pxeHttpPort = 8080;
        cachePublicKey = null;
        veyonNativeHosts = [];
      };
      inherit nixoriumPackage;
    };

    networking.hostName = "pc99";
    environment.systemPackages = [ pkgs.curl pkgs.git pkgs.jq pkgs.python3 pkgs.util-linux fakeColmena fakeHostState ];
    users.groups.veyon-master = {};
    users.users.admin = {
      isNormalUser = true;
      extraGroups = [ "wheel" "veyon-master" ];
    };
    systemd.services.nixorium-test-network = {
      description = "Create the persistent dummy network used by the Nixorium VM test";
      wantedBy = [ "multi-user.target" ];
      before = [ "network-online.target" "nixorium-pxe-recover.service" ];
      path = [ pkgs.iproute2 ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
      };
      script = ''
        ip link show dev lab0 >/dev/null 2>&1 || ip link add lab0 type dummy
        ip addr replace 192.0.2.10/24 dev lab0
        ip addr replace 10.0.0.99/8 dev lab0
        ip link set lab0 up
      '';
    };
    services.openssh.enable = true;
    environment.etc."nixorium-test/flake.nix".text = ''
      {
        inputs.fakeSystem = {
          url = "path:${fakeControllerSystem}";
          flake = false;
        };
        outputs = { self, fakeSystem }: {
          labMeta = {
            schemaVersion = 2;
            version = "vm-test";
            controller = {
              name = "pc99";
              number = 99;
              staticIp = "10.0.0.99";
              dhcpIp = "192.0.2.10";
            };
            clients = {
              count = 1;
              hosts = [
                {
                  name = "pc01";
                  ip = "127.0.0.1";
                }
              ];
            };
            network = {
              base = "127.0.0.0";
              prefixLength = 8;
              ifaceName = "lab0";
              cachePort = 5000;
              pxeHttpPort = 8080;
            };
            users = {
              student = "student";
              teacher = "teacher";
            };
          };
          deploymentStatus = {
            ready = true;
            issues = [];
          };
          nixosConfigurations = {
            pc99.config.system.build.toplevel = fakeSystem.outPath;
            pc01.config.system.build.toplevel = fakeSystem.outPath;
            netboot.config.system.build = {
              kernel = fakeSystem.outPath;
              netbootRamdisk = fakeSystem.outPath;
              netbootIpxeScript = fakeSystem.outPath;
            };
          };
          packages.x86_64-linux.pxeFirmware = fakeSystem.outPath;
          nixoriumValidateCandidate = candidate: builtins.deepSeq candidate true;
        };
      }
    '';
    environment.etc."nixorium-test/lab-settings.json".source = ../templates/site/lab-settings.json;
    environment.etc."nixorium-test/.gitignore".source = ../templates/site/.gitignore;
  };

  testScript = ''
    start_all()
    controller.wait_for_unit("sshd.service")
    controller.wait_for_unit("nixorium-test-network.service")
    controller.succeed("systemctl is-active --quiet firewall.service")
    controller.succeed("iptables-save | grep -F -- '-i lab0' | grep -F -- '--dport 5000'; iptables-save | grep -F -- '-i lab0' | grep -F -- '--dport 8080'; iptables-save | grep -F -- '-i lab0' | grep -F -- '--dport 67'")
    controller.succeed("command -v nixorium")
    controller.succeed("command -v colmena")
    controller.succeed("mkdir /tmp/fake-colmena-bin; ln -s /run/current-system/sw/bin/nixorium-test-colmena /tmp/fake-colmena-bin/colmena")
    controller.succeed("systemctl show nixorium-harmonia.service -p LoadState --value | grep -Fx loaded")
    controller.wait_until_fails("systemctl is-active --quiet nixorium-harmonia.service")
    controller.succeed("journalctl -u harmonia.service --no-pager | grep -F 'Failed to set up credentials'")
    controller.succeed("nixorium --help | grep -F 'setup keys'")
    controller.succeed("mkdir -p /tmp/deployment")
    controller.succeed("cp /etc/nixorium-test/flake.nix /tmp/deployment/flake.nix")
    controller.succeed("cp /etc/nixorium-test/lab-settings.json /tmp/deployment/lab-settings.json")
    controller.succeed("cp /etc/nixorium-test/.gitignore /tmp/deployment/.gitignore")
    controller.succeed("jq --arg password '$6$vm$not-the-public-default' '.lab.masterDhcpIp = \"192.0.2.10\" | .lab.ifaceName = \"lab0\" | .lab.teacherPassword = $password | .lab.studentPassword = $password | .lab.adminPassword = $password' /tmp/deployment/lab-settings.json > /tmp/lab-settings.json && mv /tmp/lab-settings.json /tmp/deployment/lab-settings.json")
    controller.succeed("git -C /tmp/deployment init -q")
    controller.succeed("git -C /tmp/deployment add flake.nix lab-settings.json .gitignore")
    controller.succeed("git -C /tmp/deployment -c user.name=Test -c user.email=test@example.invalid commit -qm initial")
    controller.succeed("cp /tmp/deployment/lab-settings.json /tmp/candidate.json")
    controller.succeed("nixorium config plan --repo /tmp/deployment --file /tmp/candidate.json --json | jq -e '.operation == \"config-plan\" and .state == \"unchanged\" and (.changes | length) == 0'")
    controller.succeed("nixorium setup keys --repo /tmp/deployment --json | jq -e '.operation == \"setup-keys\" and .state == \"ready\" and all(.keys[]; .verified and .matches and .privateMode == 384)'")
    controller.succeed("test $(stat -c '%a' /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem | sort -u) = 600")
    controller.succeed("before=$(sha256sum /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem); nixorium setup keys --repo /tmp/deployment --json >/dev/null; after=$(sha256sum /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem); test \"$before\" = \"$after\"")
    controller.succeed("git -C /tmp/deployment add keys && git -C /tmp/deployment -c user.name=Test -c user.email=test@example.invalid commit -qm keys")
    controller.succeed("test -z \"$(git -C /tmp/deployment status --porcelain=v1 --untracked-files=normal)\"")
    controller.succeed("source_path=$(nix --extra-experimental-features 'nix-command flakes' flake metadata git+file:///tmp/deployment --json --no-write-lock-file | jq -r .path); test ! -e \"$source_path/secret-key\"; test ! -e \"$source_path/admin-ssh\"; test ! -e \"$source_path/veyon-private-key.pem\"")
    controller.succeed("git -C /tmp/deployment add -f secret-key")
    controller.fail("nixorium status --repo /tmp/deployment --json")
    controller.succeed("git -C /tmp/deployment rm --cached -q secret-key")
    controller.succeed("mkdir -p /home/admin/nixorium-deployment")
    controller.succeed("cp -a /tmp/deployment/. /home/admin/nixorium-deployment/")
    controller.succeed("chown -R admin:users /home/admin/nixorium-deployment")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '192.0.2.10/24'; ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'")
    controller.succeed("su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --json > /tmp/pxe-prepare-failed.json' || test $? = 1")
    controller.succeed("jq -e '.operation == \"pxe-prepare\" and .state == \"failed\" and .unit == \"nixorium-prepare-pxe.service\"' /tmp/pxe-prepare-failed.json")
    controller.succeed("journalctl -u nixorium-prepare-pxe.service --no-pager | grep -F 'Harmonia cache service is not active'")
    controller.succeed("su - admin -c 'cd ~/nixorium-deployment && nixorium setup install-secrets --json' | jq -e '.operation == \"setup-install-secrets\" and .state == \"completed\"'")
    controller.succeed("cmp /home/admin/nixorium-deployment/admin-ssh /home/admin/.ssh/id_ed25519")
    controller.succeed("cmp /home/admin/nixorium-deployment/veyon-private-key.pem /etc/veyon/keys/private/teacher/key")
    controller.succeed("cmp /home/admin/nixorium-deployment/secret-key /var/lib/nixorium/keys/harmonia-secret-key")
    controller.succeed("test $(stat -c '%a' /home/admin/.ssh/id_ed25519 /var/lib/nixorium/keys/harmonia-secret-key | sort -u) = 600")
    controller.succeed("test $(stat -c '%a' /etc/veyon/keys/private/teacher/key) = 640")
    controller.succeed("systemctl reset-failed harmonia.service harmonia.socket; systemctl restart harmonia.socket nixorium-harmonia.service")
    controller.wait_for_unit("nixorium-harmonia.service")
    controller.wait_until_succeeds("curl --fail --silent http://192.0.2.10:5000/nix-cache-info | grep -F 'StoreDir: /nix/store'")
    controller.succeed("journalctl -u harmonia.service --no-pager | grep -F 'listening on inherited fd'")
    controller.succeed("su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --json' | jq -e '.operation == \"pxe-prepare\" and .state == \"completed\"'")
    controller.succeed("jq -e '.schemaVersion == 1 and .revision != \"\" and .controller.dhcpIp == \"192.0.2.10\" and .network.ifaceName == \"lab0\" and .artifacts.kernel.relativePath == \"bzImage\" and .artifacts.firmware.relativePath == \"snponly.efi\" and (.clients | length) == 1 and .clients[0].name == \"pc01\" and (.clients[0].storePath | startswith(\"/nix/store/\"))' /var/lib/nixorium/prepared/prepared.json")
    controller.succeed("test $(stat -c '%U:%G:%a' /var/lib/nixorium/prepared/prepared.json) = admin:users:644")
    controller.succeed("revision=$(jq -r .revision /var/lib/nixorium/prepared/prepared.json); test $(find /var/lib/nixorium/prepared/roots/$revision -maxdepth 1 -type l | wc -l) = 5; nix-store --gc --print-roots | grep -F /var/lib/nixorium/prepared/roots/$revision/kernel")
    controller.succeed("test ! -e /home/admin/nixorium-deployment/result-kernel; test ! -e /home/admin/nixorium-deployment/result-initrd; test ! -e /home/admin/nixorium-deployment/result-ipxe")
    controller.succeed("mkdir /var/lib/nixorium/prepared/roots/0000000000000000000000000000000000000000; chown admin:users /var/lib/nixorium/prepared/roots/0000000000000000000000000000000000000000")
    controller.succeed("before=$(jq -c 'del(.preparedAt)' /var/lib/nixorium/prepared/prepared.json); su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --json' >/dev/null; after=$(jq -c 'del(.preparedAt)' /var/lib/nixorium/prepared/prepared.json); test \"$before\" = \"$after\"; test ! -e /var/lib/nixorium/prepared/roots/0000000000000000000000000000000000000000")
    controller.succeed("before=$(sha256sum /var/lib/nixorium/prepared/prepared.json); ip addr del 192.0.2.10/24 dev lab0; ip addr add 192.0.2.11/24 dev lab0; su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --json > /tmp/pxe-address-failed.json' || test $? = 1; after=$(sha256sum /var/lib/nixorium/prepared/prepared.json); test \"$before\" = \"$after\"")
    controller.succeed("jq -e '.state == \"failed\"' /tmp/pxe-address-failed.json")
    controller.succeed("journalctl -u nixorium-prepare-pxe.service --no-pager | grep -F 'configured DHCP address 192.0.2.10 is not assigned to lab0' | grep -F '192.0.2.11'")
    controller.succeed("ip addr del 192.0.2.11/24 dev lab0; ip addr add 192.0.2.10/24 dev lab0")
    controller.succeed("systemctl show nixorium-pxe.service nixorium-pxe-network.service nixorium-pxe-recover.service -p LoadState --value | grep -vFx not-found")
    controller.succeed("su - admin -c 'nixorium pxe start --repo ~/nixorium-deployment --json </dev/null > /tmp/pxe-confirmation-required.json' || test $? = 2")
    controller.succeed("test ! -e /var/lib/nixorium/pxe/session.json; ! su - admin -c 'systemctl start nixorium-pxe-network.service'")
    controller.succeed("su - admin -c \"(sleep 8; printf p; sleep 1; printf s; sleep 15; printf S; sleep 0.2; printf T; sleep 0.2; printf A; sleep 0.2; printf R; sleep 0.2; printf T; sleep 0.2; printf ' '; sleep 0.2; printf P; sleep 0.2; printf X; sleep 0.2; printf E; sleep 0.2; printf '\\r'; sleep 15; printf q) | TERM=xterm script -qefc 'nixorium --repo ~/nixorium-deployment' /tmp/nixorium-pxe-tui.log\"")
    controller.succeed("grep -aF 'Install computers over network' /tmp/nixorium-pxe-tui.log")
    controller.succeed("grep -aF 'Start review' /tmp/nixorium-pxe-tui.log")
    controller.succeed("systemctl is-active --quiet nixorium-pxe.service; systemctl is-active --quiet nixorium-pxe-network.service")
    controller.succeed("su - admin -c 'nixorium pxe start --repo ~/nixorium-deployment --yes --json' | jq -e '.state == \"completed\" and .mode == \"active\" and (.message | contains(\"already active\"))'")
    controller.succeed("su - admin -c 'nixorium status --repo ~/nixorium-deployment --json' | jq -e '.pxe.mode == \"active\" and .pxe.listener.active and .pxe.network.active'")
    controller.succeed("su - admin -c 'nixorium doctor --repo ~/nixorium-deployment --json' | jq -e 'any(.findings[]; .id == \"PXE-LIFECYCLE\" and .level == \"OK\") and any(.findings[]; .id == \"PXE-PORTS\" and .level == \"OK\")'")
    controller.wait_until_succeeds("curl --fail --silent http://192.0.2.10:8080/bzImage >/dev/null; curl --fail --silent http://192.0.2.10:8080/initrd >/dev/null")
    controller.succeed("grep -F 'kernel ''${base-url}/bzImage init=/nix/store/test-init' /run/nixorium/pxe-runtime/tftp/boot.ipxe")
    controller.succeed("ss -H -lun 'sport = :67' | grep -F ':67'; ss -H -lun 'sport = :69' | grep -F ':69'; ss -H -ltn 'sport = :8080' | grep -F ':8080'")
    controller.succeed("pgrep -u nobody -f 'python3 -m http.server 8080'; pid=$(pgrep -o -x dnsmasq); test $(awk '/^Uid:/ {print $3}' /proc/$pid/status) = $(id -u nixorium-pxe-dnsmasq)")
    controller.fail("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'")
    controller.succeed("su - admin -c 'nixorium pxe stop --repo ~/nixorium-deployment --json' | jq -e '.operation == \"pxe-stop\" and .state == \"completed\" and .mode == \"stopped\"'")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; test ! -e /var/lib/nixorium/pxe/session.json")
    controller.succeed("systemd-run --unit=nixorium-test-pxe-conflict --property=Type=simple python3 -m http.server 8080 --bind 192.0.2.10")
    controller.succeed("su - admin -c 'nixorium pxe start --repo ~/nixorium-deployment --yes --json > /tmp/pxe-start-failed.json' || test $? = 1")
    controller.succeed("jq -e '.operation == \"pxe-start\" and .state == \"failed\" and (.message | contains(\"nixorium-pxe.service\"))' /tmp/pxe-start-failed.json")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; test ! -e /var/lib/nixorium/pxe/session.json; ! systemctl is-active --quiet nixorium-pxe-network.service")
    controller.succeed("journalctl -u nixorium-pxe.service --no-pager | grep -F 'PXE HTTP port 8080 is already in use'")
    controller.succeed("systemctl stop nixorium-test-pxe-conflict.service; systemctl reset-failed nixorium-pxe.service nixorium-pxe-network.service")
    controller.succeed("ip addr add 198.51.100.7/24 dev lab0")
    controller.succeed("systemctl start nixorium-pxe-network.service")
    controller.succeed("systemctl is-active --quiet nixorium-pxe-network.service")
    controller.fail("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '192.0.2.10/24'; ip -4 -o addr show dev lab0 scope global | grep -F '198.51.100.7/24'")
    controller.succeed("jq -e '.schemaVersion == 1 and .state == \"network-active\" and .interface == \"lab0\" and .dhcpAddress == \"192.0.2.10\" and .staticAddress == \"10.0.0.99\" and .prefixLength == 8 and .removedStatic and (.originalAddresses | index(\"10.0.0.99/8\") != null) and .artifacts.kernel.relativePath == \"bzImage\"' /var/lib/nixorium/pxe/session.json")
    controller.succeed("test $(stat -c '%U:%G:%a' /var/lib/nixorium/pxe/session.json) = root:root:600")
    controller.succeed("systemctl start nixorium-pxe-network.service; systemctl stop nixorium-pxe-network.service")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; ip -4 -o addr show dev lab0 scope global | grep -F '198.51.100.7/24'")
    controller.succeed("test ! -e /var/lib/nixorium/pxe/session.json; jq -e '.state == \"stopped\" and .stopReason == \"normal-stop\"' /var/lib/nixorium/pxe/last-session.json")
    controller.succeed("ln -s /etc/passwd /var/lib/nixorium/pxe/session.json; ! systemctl start nixorium-pxe-recover.service; test -L /var/lib/nixorium/pxe/session.json; rm /var/lib/nixorium/pxe/session.json; systemctl reset-failed nixorium-pxe-recover.service")
    controller.succeed("ip addr del 10.0.0.99/8 dev lab0; ! systemctl start nixorium-pxe-network.service")
    controller.succeed("test ! -e /var/lib/nixorium/pxe/session.json; journalctl -u nixorium-pxe-network.service --no-pager | grep -F 'refusing an untracked transition'")
    controller.succeed("ip addr add 10.0.0.99/8 dev lab0; systemctl reset-failed nixorium-pxe-network.service")
    controller.succeed("systemctl start nixorium-pxe-network.service; su - admin -c 'nixorium pxe recover --repo /does-not-exist --json' | jq -e '.operation == \"pxe-recover\" and .state == \"completed\" and .mode == \"stopped\"'")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; test ! -e /var/lib/nixorium/pxe/session.json; jq -e '.state == \"stopped\" and .stopReason == \"normal-stop\"' /var/lib/nixorium/pxe/last-session.json")
    controller.succeed("systemctl stop nixorium-pxe-network.service")
    controller.succeed("su - admin -c 'systemctl start nixorium-install-secrets.service'")
    controller.succeed("test -z \"$(git -C /tmp/deployment status --porcelain=v1 --untracked-files=normal)\"")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.currentStage == \"apply-controller\"'")
    controller.succeed("su - admin -c 'nixorium setup apply --repo ~/nixorium-deployment --yes --json' | jq -e '.operation == \"setup-apply-controller\" and .state == \"completed\"'")
    controller.succeed("test -e /run/nixorium-controller-applied")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.currentStage == \"offer-client-installation\" and (.stages[] | select(.id == \"prepare-artifacts\").state) == \"complete\"'")
    controller.succeed("su - admin -c 'nixorium setup apply --repo ~/nixorium-deployment --yes --json' | jq -e '.state == \"completed\"'")
    controller.succeed("su - admin -c 'nixorium controller plan --repo ~/nixorium-deployment --json' > /tmp/controller-plan.json; jq -e '.operation == \"controller-plan\" and .state == \"current\" and .controller == \"pc99\" and .current and (.revision | length) == 40 and .confirmation == \"REBUILD pc99\"' /tmp/controller-plan.json")
    controller.succeed("su - admin -c 'nixorium controller apply --repo ~/nixorium-deployment --expect 0000000000000000000000000000000000000000 --yes --json >/tmp/controller-stale.json' || test $? = 1; jq -e '.state == \"blocked\" and any(.issues[]; .field == \"review\")' /tmp/controller-stale.json")
    controller.succeed("su - admin -c 'revision=$(git -C ~/nixorium-deployment rev-parse HEAD); nixorium controller apply --repo ~/nixorium-deployment --expect \"$revision\" --yes --json' > /tmp/controller-apply.json; jq -e '.operation == \"controller-apply\" and .state == \"completed\" and .phase == \"complete\" and .applied and .verified and .unit == (\"nixorium-apply-controller@\" + .revision + \".service\")' /tmp/controller-apply.json")
    controller.fail("su - admin -c 'systemctl start nixorium-apply-controller@short.service'")
    controller.succeed("printf different > /home/admin/.ssh/id_ed25519; ! systemctl start nixorium-install-secrets.service")
    controller.succeed("su - admin -c 'nixorium setup apply --repo ~/nixorium-deployment --yes --json > /tmp/apply.json' || test $? = 1")
    controller.succeed("jq -e '.operation == \"setup-apply-controller\" and .state == \"failed\" and .unit == \"nixorium-apply-controller.service\"' /tmp/apply.json")
    controller.succeed("journalctl -u nixorium-apply-controller.service --no-pager | grep -F 'installed admin SSH key is absent or differs'")
    controller.succeed("touch /home/admin/nixorium-deployment/dirty")
    controller.fail("su - admin -c 'systemctl start nixorium-apply-controller.service'")
    controller.succeed("journalctl -u nixorium-apply-controller.service --no-pager | grep -F 'deployment worktree must be clean before controller apply'")
    controller.succeed("rm /home/admin/nixorium-deployment/dirty; test -z \"$(git -C /home/admin/nixorium-deployment status --porcelain=v1 --untracked-files=normal)\"")
    controller.succeed("cp /home/admin/nixorium-deployment/admin-ssh /home/admin/.ssh/id_ed25519; chown admin:users /home/admin/.ssh/id_ed25519; chmod 0600 /home/admin/.ssh/id_ed25519; install -d -m 0700 /root/.ssh; cp /home/admin/nixorium-deployment/keys/admin-ssh.pub /root/.ssh/authorized_keys; chmod 0600 /root/.ssh/authorized_keys; systemctl reload sshd.service")
    controller.succeed("su - admin -c \"(sleep 8; printf c; sleep 5; printf 'REBUILD pc99\\r'; sleep 15; printf q) | TERM=xterm timeout 40s script -qefc 'nixorium --repo /home/admin/nixorium-deployment' /tmp/nixorium-controller-tui.log\"")
    controller.succeed("grep -aF 'Rebuild controller' /tmp/nixorium-controller-tui.log; grep -aF 'Controller rebuild review' /tmp/nixorium-controller-tui.log; grep -aF 'Last result: completed at phase complete' /tmp/nixorium-controller-tui.log; grep -aF 'Applied: true   Verified: true' /tmp/nixorium-controller-tui.log")
    controller.succeed("nixorium status --repo /tmp/deployment --json | jq -e '.operation == \"status\" and .state == \"ready\" and .lab.clients.hosts[0].name == \"pc01\" and .pxe.mode == \"ready\" and .pxePreparation.ready and all(.artifacts[]; .present) and any(.services[]; .name == \"nixorium-harmonia.service\" and .loaded and .active)'")
    controller.succeed("su - admin -c 'nixorium hosts --repo /home/admin/nixorium-deployment --json' > /tmp/hosts-current.json; jq -e '.operation == \"hosts\" and .state == \"available\" and .deployment.current == 1 and .deployment.outdated == 0 and .deployment.unknown == 0 and (.hosts | length) == 1 and .hosts[0].name == \"pc01\" and .hosts[0].role == \"client\" and .hosts[0].reachability == \"reachable\" and .hosts[0].ssh == \"available\" and .hosts[0].deployment == \"current\" and (.hosts[0].currentSystem | startswith(\"/nix/store/\")) and .hosts[0].currentRevision == .hosts[0].desiredRevision' /tmp/hosts-current.json || { cat /tmp/hosts-current.json; false; }")
    controller.succeed("nixorium deploy plan --repo /tmp/deployment --on @lab --json | jq -e '.operation == \"deploy-plan\" and .state == \"ready\" and .requested == \"@lab\" and .colmenaSelector == \"@lab\" and .buildFirst and (.revision | length) == 40 and (.targets | length) == 1 and .targets[0].name == \"pc01\"'")
    controller.succeed("nixorium deploy plan --repo /tmp/deployment --on pc01 --json | jq -e '.state == \"ready\" and .colmenaSelector == \"pc01\"'")
    controller.fail("nixorium deploy plan --repo /tmp/deployment --on pc02 --json")
    controller.succeed("su - admin -c 'revision=$(git -C /home/admin/nixorium-deployment rev-parse HEAD); NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-success PATH=/tmp/fake-colmena-bin:$PATH nixorium deploy apply --repo /home/admin/nixorium-deployment --on pc01 --expect \"$revision\" --yes --json >/tmp/deploy-success.json 2>/tmp/deploy-success.progress' || { cat /tmp/deploy-success.json /tmp/deploy-success.progress; false; }")
    controller.succeed("jq -e '.operation == \"deploy-apply\" and .state == \"completed\" and .phase == \"complete\" and .buildCompleted and .applyCompleted and .retrySafe and .colmenaSelector == \"pc01\" and .verification.attempted == 1 and .verification.verified == 1 and .verification.recorded == 1 and .verification.targets[0].name == \"pc01\" and .verification.targets[0].state == \"verified\" and (.logPath | startswith(\"/home/admin/.local/state/nixorium/operations/deploy-\"))' /tmp/deploy-success.json")
    controller.succeed("test \"$(head -n 1 /tmp/colmena-success)\" = 'build --on pc01 --verbose --color never'; test \"$(tail -n 1 /tmp/colmena-success)\" = 'apply switch --on pc01 --verbose --color never'; test \"$(wc -l </tmp/colmena-success)\" = 2")
    controller.succeed("log=$(jq -r .logPath /tmp/deploy-success.json); test \"$(stat -c '%U:%G:%a' \"$log\")\" = admin:users:600; grep -F 'fake colmena: build' \"$log\"; grep -F 'Result: completed' \"$log\"; grep -F 'Verified targets: 1/1' \"$log\"; grep -F 'fake colmena: apply' /tmp/deploy-success.progress")
    controller.succeed("history=(/home/admin/.local/state/nixorium/deployments/*.json); test \"''${#history[@]}\" = 1; test \"$(stat -c '%U:%G:%a' \"''${history[0]}\")\" = admin:users:600; jq -e '.schemaVersion == 1 and .repository == \"/home/admin/nixorium-deployment\" and (.hosts | keys) == [\"pc01\"] and (.hosts.pc01.revision | length) == 40 and (.hosts.pc01.systemPath | startswith(\"/nix/store/\")) and (.hosts.pc01.verifiedAt | endswith(\"Z\"))' \"''${history[0]}\"")
    controller.succeed("su - admin -c 'nixorium hosts --repo /home/admin/nixorium-deployment --json' > /tmp/hosts-verified.json; jq -e '.hosts[0].lastSuccessfulDeploy.revision == .hosts[0].desiredRevision and (.hosts[0].lastSuccessfulDeploy.systemPath | startswith(\"/nix/store/\")) and (.hosts[0].lastSuccessfulDeploy.verifiedAt | endswith(\"Z\"))' /tmp/hosts-verified.json")
    controller.succeed("su - admin -c 'revision=$(git -C /home/admin/nixorium-deployment rev-parse HEAD); NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-failed NIXORIUM_TEST_COLMENA_FAIL=apply PATH=/tmp/fake-colmena-bin:$PATH nixorium deploy apply --repo /home/admin/nixorium-deployment --on @lab --expect \"$revision\" --yes --json >/tmp/deploy-failed.json 2>/tmp/deploy-failed.progress' || test $? = 1")
    controller.succeed("jq -e '.state == \"failed\" and .phase == \"apply\" and .buildCompleted and (.applyCompleted | not) and .retrySafe and .verification.verified == 1 and .verification.recorded == 1 and (.message | contains(\"authenticated reconciliation\"))' /tmp/deploy-failed.json")
    controller.succeed("su - admin -c 'revision=$(git -C /home/admin/nixorium-deployment rev-parse HEAD); NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-retry PATH=/tmp/fake-colmena-bin:$PATH nixorium deploy apply --repo /home/admin/nixorium-deployment --on @lab --expect \"$revision\" --yes --json >/tmp/deploy-retry.json 2>/tmp/deploy-retry.progress'")
    controller.succeed("jq -e '.state == \"completed\" and .buildCompleted and .applyCompleted and .verification.verified == 1 and .verification.recorded == 1' /tmp/deploy-retry.json; test \"$(wc -l </tmp/colmena-retry)\" = 2")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-stale PATH=/tmp/fake-colmena-bin:$PATH nixorium deploy apply --repo /home/admin/nixorium-deployment --on pc01 --expect 0000000000000000000000000000000000000000 --yes --json >/tmp/deploy-stale.json' || test $? = 1")
    controller.succeed("jq -e '.state == \"blocked\" and .phase == \"preflight\" and any(.issues[]; .field == \"review\")' /tmp/deploy-stale.json; test ! -e /tmp/colmena-stale")
    controller.succeed("su - admin -c \"(sleep 8; printf d; sleep 1; printf a; sleep 0.2; printf '\\r'; sleep 3; printf 'DEPLOY @lab\\r'; sleep 12; printf q; sleep 2; printf q) | NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-tui PATH=/tmp/fake-colmena-bin:\$PATH TERM=xterm timeout 45s script -qefc 'nixorium --repo /home/admin/nixorium-deployment' /tmp/nixorium-deploy-tui.log\"")
    controller.succeed("grep -aF 'Deploy updates' /tmp/nixorium-deploy-tui.log; grep -aF 'Deployment review' /tmp/nixorium-deploy-tui.log; grep -aF 'Last result: completed at phase complete' /tmp/nixorium-deploy-tui.log; grep -aF 'Authenticated: 1/1' /tmp/nixorium-deploy-tui.log")
    controller.succeed("test \"$(head -n 1 /tmp/colmena-tui)\" = 'build --on @lab --verbose --color never'; test \"$(tail -n 1 /tmp/colmena-tui)\" = 'apply switch --on @lab --verbose --color never'; test \"$(wc -l </tmp/colmena-tui)\" = 2")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.operation == \"setup-status\" and .state == \"action-required\" and .currentStage == \"offer-client-installation\"'")
    controller.succeed("nixorium doctor --repo /tmp/deployment --json | jq -e '.state == \"warnings\" and any(.findings[]; .id == \"PXE-PREPARATION\" and .level == \"OK\") and any(.findings[]; .id == \"PXE-LIFECYCLE\" and .level == \"OK\") and any(.findings[]; .id == \"SERVICE-HARMONIA\" and .level == \"OK\") and any(.findings[]; .id == \"CACHE-HEALTH\" and .level == \"OK\") and any(.findings[]; .id == \"COMMAND-COLMENA\" and .level == \"OK\") and any(.findings[]; .id == \"NETWORK-INTERFACE\" and .level == \"OK\") and any(.findings[]; .id == \"CLIENT-SSH\" and .level == \"OK\") and any(.findings[]; .id == \"DISK-FREE\" and .level == \"WARNING\")'")
    controller.succeed("systemctl start nixorium-pxe-network.service; test -e /var/lib/nixorium/pxe/session.json")
    controller.crash()
    controller.wait_for_unit("multi-user.target")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; test ! -e /var/lib/nixorium/pxe/session.json; jq -e '.state == \"recovered\" and .stopReason == \"boot-or-explicit-recovery\"' /var/lib/nixorium/pxe/last-session.json")
  '';
}
