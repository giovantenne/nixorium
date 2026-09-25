{ nixoriumPackage }:
{
  name = "nixorium-management";

  nodes.controller = { lib, pkgs, ... }:
  let
    fakeRuntime = pkgs.buildEnv {
      name = "nixorium-test-system-path";
      paths = [
        pkgs.bash
        pkgs.colmena
        pkgs.coreutils
        pkgs.git
        pkgs.gnugrep
        pkgs.iproute2
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
      if ${pkgs.coreutils}/bin/touch /home/admin/nixorium-deployment/activation-must-not-write 2>/dev/null; then
        exit 3
      fi
      ${pkgs.coreutils}/bin/mkdir -p /home/teacher/.config /run/user/1000
      ${pkgs.coreutils}/bin/touch /home/teacher/.config/nixorium-activation-test /run/user/1000/nixorium-activation-test
      ${pkgs.coreutils}/bin/ln -sfn "$SYSTEM_PATH" /run/current-system
      if [[ -e /run/nixorium-test-activation-fail ]]; then
        exit 4
      fi
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
      if [ "''${1:-}" = apply ]; then
        test -n "''${SSH_CONFIG_FILE:-}"
        test "$(stat -c %a "$SSH_CONFIG_FILE")" = 600
        grep -Fx '  BatchMode yes' "$SSH_CONFIG_FILE"
        grep -Fx '  PasswordAuthentication no' "$SSH_CONFIG_FILE"
        grep -Fx '  StrictHostKeyChecking accept-new' "$SSH_CONFIG_FILE"
        touch /tmp/nixorium-test-colmena-ssh-policy
      fi
      if [ "''${NIXORIUM_TEST_COLMENA_FAIL:-}" = "''${1:-}" ]; then
        exit 42
      fi
    '';
    fakeHostState = pkgs.writeShellScriptBin "nixorium-host-state" ''
      set -eu
      readlink -f /run/current-system
      git -c safe.directory=/home/admin/nixorium-deployment -C /home/admin/nixorium-deployment rev-parse HEAD
    '';
    fakeShutdownRemote = pkgs.writeShellScriptBin "nixorium-test-shutdown-remote" ''
      set -eu
      printf '%s\n' "''${SSH_ORIGINAL_COMMAND:-}" >> /tmp/nixorium-test-shutdown-ssh.log
      case "''${SSH_ORIGINAL_COMMAND:-}" in
        nixorium-session-state)
          if [[ -e /tmp/nixorium-test-session-state ]]; then
            cat /tmp/nixorium-test-session-state
          else
            printf 'idle\n'
          fi
          ;;
        "systemctl poweroff --no-block")
          printf 'accepted\n' >> /tmp/nixorium-test-shutdown-dispatch.log
          ;;
        *)
          exit 64
          ;;
      esac
    '';
    fakeUpdateNix = pkgs.writeShellScriptBin "nixorium-test-update-nix" ''
      set -eu
      : "''${NIXORIUM_TEST_UPDATE_NIX_LOG:?}"
      printf '%s\n' "$*" >> "$NIXORIUM_TEST_UPDATE_NIX_LOG"
      case " $* " in
        *" flake lock "*)
          revision=2222222222222222222222222222222222222222
          if [[ " $* " == *"/v2.2.0 "* ]]; then
            revision=3333333333333333333333333333333333333333
          elif [[ " $* " == *"/v2.3.0 "* ]]; then
            revision=4444444444444444444444444444444444444444
          fi
          while [[ $# -gt 0 ]]; do
            if [[ "$1" == --output-lock-file ]]; then
              if [[ -n "''${NIXORIUM_TEST_BASE_CANDIDATE-}" ]]; then
                cp "$NIXORIUM_TEST_BASE_CANDIDATE" "$2"
                exit 0
              fi
              printf '%s\n' "{\"root\":\"root\",\"nodes\":{\"root\":{\"inputs\":{\"nixorium\":\"nixorium\"}},\"nixorium\":{\"locked\":{\"rev\":\"$revision\"}}}}" > "$2"
              exit 0
            fi
            shift
          done
          exit 2
          ;;
        *"#labMeta "*)
          if [[ "''${NIXORIUM_TEST_UPDATE_MODE-}" == controller ]]; then
            printf '%s\n' '{"schemaVersion":2,"deploymentMode":"controller","controller":{"name":"pc99"},"clients":{"count":0,"hosts":[]}}'
          else
            printf '%s\n' '{"schemaVersion":2,"controller":{"name":"pc99"},"clients":{"count":1,"hosts":[{"name":"pc01","ip":"10.0.0.1"}]}}'
          fi
          ;;
        *"#deploymentStatus "*)
          if [[ "''${NIXORIUM_TEST_UPDATE_MODE-}" == controller ]]; then
            printf '%s\n' '{"ready":false,"issues":["Client installation is not configured"],"controller":{"ready":true,"issues":[],"requiresKeys":false}}'
          else
            printf '%s\n' '{"ready":true,"issues":[]}'
          fi
          ;;
        *"#nixoriumUpdateTargets "*)
          printf '%s\n' '["pc99","pc01"]'
          ;;
        *" build "*)
          ;;
        *)
          exit 3
          ;;
      esac
    '';
    fakeUpdateGit = pkgs.writeShellScriptBin "nixorium-test-update-git" ''
      set -eu
      if [[ "$*" != "-c credential.helper= -c core.askPass= ls-remote --refs --exit-code https://github.com/giovantenne/nixorium.git refs/heads/master refs/tags/v*" ]]; then
        exec ${pkgs.git}/bin/git "$@"
      fi
      : "''${NIXORIUM_TEST_UPDATE_GIT_LOG:?}"
      printf '%s\n' "$*" > "$NIXORIUM_TEST_UPDATE_GIT_LOG"
      printf 'prompt=%s askpass=%s sshaskpass=%s interactive=%s global=%s nosystem=%s\n' \
        "''${GIT_TERMINAL_PROMPT-}" "''${GIT_ASKPASS-}" "''${SSH_ASKPASS-}" \
        "''${GCM_INTERACTIVE-}" "''${GIT_CONFIG_GLOBAL-}" "''${GIT_CONFIG_NOSYSTEM-}" \
        >> "$NIXORIUM_TEST_UPDATE_GIT_LOG"
      [[ "''${GIT_TERMINAL_PROMPT-}" == 0 ]]
      [[ "''${GIT_ASKPASS+x}" == x && -z "''${GIT_ASKPASS}" ]]
      [[ "''${SSH_ASKPASS+x}" == x && -z "''${SSH_ASKPASS}" ]]
      [[ "''${GCM_INTERACTIVE-}" == Never ]]
      [[ "''${GIT_CONFIG_GLOBAL-}" == /dev/null ]]
      [[ "''${GIT_CONFIG_NOSYSTEM-}" == 1 ]]
      [[ "$*" == "-c credential.helper= -c core.askPass= ls-remote --refs --exit-code https://github.com/giovantenne/nixorium.git refs/heads/master refs/tags/v*" ]]
      printf '%s\t%s\n' 7777777777777777777777777777777777777777 refs/heads/master
      printf '%s\t%s\n' 4444444444444444444444444444444444444444 refs/tags/v2.3.0
      printf '%s\t%s\n' 5555555555555555555555555555555555555555 refs/tags/v2.4.0-beta.1
      printf '%s\t%s\n' 6666666666666666666666666666666666666666 refs/tags/version-two
    '';
  in
  {
    imports = [
      ../modules/cache.nix
      ../modules/firewall.nix
      ../modules/management.nix
      ../modules/pxe.nix
      ../modules/users.nix
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
        teacherUser = "teacher";
        studentUser = "student";
        teacherPassword = "!";
        studentPassword = "!";
        adminPassword = "!";
        adminSshKey = null;
      };
      inherit nixoriumPackage;
    };

    networking.hostName = "pc99";
    # This test applies systems after power-loss recovery. Keep fetched Flake
    # sources across reboot rather than losing the writable store's tmpfs.
    virtualisation.writableStoreUseTmpfs = false;
    environment.systemPackages = [ pkgs.curl pkgs.git pkgs.jq pkgs.python3 pkgs.util-linux fakeColmena fakeHostState fakeShutdownRemote fakeUpdateNix fakeUpdateGit ];
    users.groups.veyon-master = {};
    system.activationScripts.createHomeTemplates = "";
    system.activationScripts.siteHomeProfile = {
      deps = [ "users" ];
      text = ''
        for user in admin teacher student; do
          mkdir -p "/home/$user/.config/Code/User/globalStorage" "/home/$user/.vscode/extensions"
          chown -R root:root "/home/$user/.config" "/home/$user/.vscode"
          user_id="$(${pkgs.coreutils}/bin/id -u "$user")"
          mkdir -p "/run/user/$user_id"
          chown root:root "/run/user/$user_id"
          chmod 0755 "/run/user/$user_id"
        done
      '';
    };
    system.activationScripts.nixoriumUserHomeOwnership.deps = [ "siteHomeProfile" ];
    users.users.root.hashedPassword = lib.mkForce null;
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
    environment.etc."nixorium-test/controller-only.nix".text = ''
      {
        inputs.fakeSystem = {
          url = "path:${fakeControllerSystem}";
          flake = false;
        };
        outputs = inputs:
          let original = (import ./laboratory-flake.nix).outputs inputs; in original // {
            labMeta = original.labMeta // {
              deploymentMode = "controller";
              clients = { count = 0; hosts = []; };
              controller = original.labMeta.controller // { staticIp = ""; };
            };
            deploymentStatus = {
              ready = false;
              issues = [ "Client installation is not configured" ];
              controller = { ready = true; issues = []; requiresKeys = false; };
            };
            nixoriumSoftware = original.nixoriumSoftware // {
              controller = "pc99";
              clients = [];
              groups = {};
            };
            nixoriumValidateControllerSoftwareCandidate = candidate:
              assert builtins.all (entry: entry.package != "vlc" || !builtins.elem entry.scope.kind [ "shared" "controller" ]) candidate.packages
                || throw "controller-specific package policy rejected vlc";
              builtins.deepSeq candidate true;
          };
      }
    '';
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
          nixoriumSoftware = {
            schemaVersion = 1;
            managedFile = "lab-software.json";
            clients = [ "pc01" ];
            groups.graphics = [ "pc01" ];
            catalog = [
              { id = "gimp"; label = "GIMP"; summary = "Edit bitmap images"; availability = "available"; }
              { id = "vlc"; label = "VLC"; summary = "Play audio and video"; availability = "available"; }
            ];
            packages = map (entry: entry // { origin = "managed"; })
              (builtins.fromJSON (builtins.readFile ./lab-software.json)).packages;
          };
          nixoriumSoftwarePresets = {
            schemaVersion = 1;
            defaultPreset = "essential";
            presets = [
              {
                id = "essential";
                label = "Essential";
                description = "Common software for everyday work";
                packages = [ "gimp" "vlc" ];
              }
            ];
          };
          nixoriumSearchSoftwarePackages = request:
            if request.query == "hell" then [
              { id = "hello"; label = "hello"; summary = "A friendly greeting program"; version = "2.12"; availability = "available"; }
            ] else [];
          nixoriumResolveSoftwarePackage = package:
            if package == "hello" then
              { id = "hello"; label = "hello"; summary = "A friendly greeting program"; version = "2.12"; availability = "available"; }
            else if package == "gimp" then
              { id = "gimp"; label = "GIMP"; summary = "Edit bitmap images"; availability = "available"; }
            else if package == "vlc" then
              { id = "vlc"; label = "VLC"; summary = "Play audio and video"; availability = "available"; }
            else null;
          nixoriumValidateSoftwareCandidate = candidate: builtins.deepSeq candidate true;
        };
      }
    '';
    environment.etc."nixorium-test/lab-settings.json".source = ../templates/site/lab-settings.json;
    environment.etc."nixorium-test/lab-software.json".text = builtins.toJSON {
      schemaVersion = 1;
      packages = [];
    };
    environment.etc."nixorium-test/.gitignore".source = ../templates/site/.gitignore;
  };

  testScript = ''
    import json
    import shlex

    start_all()
    controller.wait_for_unit("sshd.service")
    controller.wait_for_unit("nixorium-test-network.service")
    controller.succeed("systemctl is-active --quiet firewall.service")
    controller.succeed("iptables-save | grep -F -- '-i lab0' | grep -F -- '--dport 5000'; iptables-save | grep -F -- '-i lab0' | grep -F -- '--dport 8080'; iptables-save | grep -F -- '-i lab0' | grep -F -- '--dport 67'")
    controller.succeed("command -v nixorium")
    controller.succeed("command -v colmena")
    controller.succeed("id -nG admin | tr ' ' '\n' | grep -Fx nixorium-operations; test \"$(stat -c '%U:%G:%a' /var/lib/nixorium/coordination)\" = root:nixorium-operations:770; test \"$(stat -c '%U:%G:%a' /var/lib/nixorium/coordination/operation.lock)\" = root:nixorium-operations:660")
    controller.succeed("test \"$(cat /etc/nixorium/deployment-path)\" = /home/admin/nixorium-deployment; test \"$(stat -c '%U:%G:%a' /etc/nixorium/deployment-path)\" = root:root:444; systemctl cat nixorium-remote-install.service | grep -F '/etc/nixorium/deployment-path'; systemctl show nixorium-remote-install.service -p LimitCORE --value | grep -Fx 0")
    controller.succeed("test \"$(stat -c '%U:%G:%a' /home/admin/.ssh/known_hosts)\" = admin:users:600; test \"$(stat -c '%U:%G:%a' /home/admin/.ssh/.nixorium-known-hosts.lock)\" = admin:users:600; test \"$(stat -c '%U:%G:%a' /home/admin/.local/state/nixorium/operations)\" = admin:users:700")
    controller.succeed("systemctl show nixorium-remote-install.service -p Environment --value | grep -F 'XDG_STATE_HOME=/home/admin/.local/state'; systemctl cat nixorium-remote-install.service | grep -F -- '-/home/admin/.ssh/known_hosts'; systemctl cat nixorium-remote-install.service | grep -F -- '-/home/admin/.local/state/nixorium/operations'")
    controller.succeed("su - admin -c 'systemctl start nixorium-remote-install.service'")
    controller.wait_for_unit("nixorium-remote-install.service")
    controller.succeed("test \"$(stat -c '%U:%G:%a' /run/nixorium/remote-install/control.sock)\" = admin:users:600; test \"$(stat -c '%U:%G:%a' /var/lib/nixorium/remote-install)\" = admin:users:700")
    ipc_probe = 'import json,socket; s=socket.socket(socket.AF_UNIX); s.connect("/run/nixorium/remote-install/control.sock"); s.sendall(b\'{"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","operation":"worker-probe"}\\n\'); value=json.loads(s.makefile().readline()); assert value["requestId"] == "0123456789abcdef0123456789abcdef" and value["state"] == "ready"'
    controller.succeed("su - admin -c " + shlex.quote("python3 -c " + shlex.quote(ipc_probe)))
    orphan_id = "abcdefabcdefabcdefabcdefabcdefab"
    orphan_session = {
      "schemaVersion": 1,
      "operationId": orphan_id,
      "logId": f"usb-install-{orphan_id}.log",
      "state": "artifacts-ready",
      "plan": {},
      "artifacts": {
        "schemaVersion": 1,
        "operationId": orphan_id,
        "repository": "/home/admin/nixorium-deployment",
        "deploymentRevision": "0" * 40,
        "bundlePath": "/nix/store/11111111111111111111111111111111-remote-installer",
        "bundleClosureBytes": 1024,
        "systemPath": "/nix/store/22222222222222222222222222222222-nixos-system-pc01-test",
        "systemClosureBytes": 2048,
        "hostName": "pc01",
        "hostInterface": "lab0",
        "hostStaticIp": "10.0.0.1",
        "cachePublicKey": "cache.example:YWJjZA==",
        "adminPublicKey": "ssh-ed25519 YWJjZA== admin@test",
        "preparedAt": "2026-01-01T00:00:00Z",
        "issues": [],
      },
      "tokenConsumed": False,
      "dispatchUncertain": False,
      "rebootRequested": False,
      "bootVerified": False,
      "events": [],
    }
    orphan_marker = {
      "schemaVersion": 1,
      "operationId": orphan_id,
      "statePath": f"/var/lib/nixorium/remote-install/{orphan_id}.json",
      "tokenDigest": "",
    }
    write_orphan = (
      "import json,pathlib; "
      f"pathlib.Path('/var/lib/nixorium/remote-install/{orphan_id}.json').write_text({json.dumps(json.dumps(orphan_session) + chr(10))}); "
      f"pathlib.Path('/var/lib/nixorium/coordination/usb-reservation.json').write_text({json.dumps(json.dumps(orphan_marker) + chr(10))})"
    )
    controller.succeed("systemctl stop nixorium-remote-install.service; su - admin -c " + shlex.quote("python3 -c " + shlex.quote(write_orphan)))
    controller.succeed(f"chmod 0600 /var/lib/nixorium/remote-install/{orphan_id}.json /var/lib/nixorium/coordination/usb-reservation.json; systemctl start nixorium-remote-install.service")
    controller.wait_for_unit("nixorium-remote-install.service")
    orphan_probe = f'import json,socket; s=socket.socket(socket.AF_UNIX); s.connect("/run/nixorium/remote-install/control.sock"); s.sendall(b\'{{"schemaVersion":1,"requestId":"fedcba9876543210fedcba9876543210","operation":"worker-probe"}}\\n\'); value=json.loads(s.makefile().readline()); assert value["operationId"] == "{orphan_id}" and value["state"] == "artifacts-ready"'
    orphan_probe_command = "su - admin -c " + shlex.quote("python3 -c " + shlex.quote(orphan_probe))
    controller.succeed(orphan_probe_command)
    controller.succeed("systemctl kill --kill-who=main --signal=SIGKILL nixorium-remote-install.service")
    controller.wait_until_succeeds("systemctl is-active --quiet nixorium-remote-install.service && test -S /run/nixorium/remote-install/control.sock && " + orphan_probe_command)
    controller.succeed("test -f /var/lib/nixorium/coordination/usb-reservation.json")
    controller.fail("systemctl start nixorium-pxe-network.service")
    controller.succeed("journalctl -u nixorium-pxe-network.service --no-pager | grep -F 'USB installation remains reserved'; systemctl reset-failed nixorium-pxe-network.service; systemctl stop nixorium-remote-install.service; rm /var/lib/nixorium/coordination/usb-reservation.json /var/lib/nixorium/remote-install/" + orphan_id + ".json; systemctl start nixorium-remote-install.service")
    controller.wait_for_unit("nixorium-remote-install.service")
    controller.succeed("systemd-run --quiet --unit=nixorium-test-operation-holder --uid=admin /run/current-system/sw/bin/flock /var/lib/nixorium/coordination/operation.lock /run/current-system/sw/bin/sleep infinity; systemctl is-active --quiet nixorium-test-operation-holder.service")
    controller.wait_until_succeeds("! flock -n /var/lib/nixorium/coordination/operation.lock true")
    controller.fail("su - admin -c 'systemctl start nixorium-restart-cache.service'")
    controller.succeed("journalctl -u nixorium-restart-cache.service --no-pager | grep -F 'another Nixorium controller or client operation is already running'; systemctl reset-failed nixorium-restart-cache.service")
    controller.fail("systemctl start nixorium-pxe-network.service")
    controller.succeed("journalctl -u nixorium-pxe-network.service --no-pager | grep -F 'another Nixorium controller or client operation is already running'; systemctl stop nixorium-test-operation-holder.service; systemctl reset-failed nixorium-pxe-network.service")
    controller.succeed("install -m 0600 -o admin -g users /dev/null /var/lib/nixorium/coordination/usb-reservation.json")
    controller.fail("systemctl start nixorium-pxe-network.service")
    controller.succeed("journalctl -u nixorium-pxe-network.service --no-pager | grep -F 'USB installation remains reserved'; rm /var/lib/nixorium/coordination/usb-reservation.json; systemctl reset-failed nixorium-pxe-network.service")
    controller.succeed("for user in admin teacher student; do uid=$(id -u $user); test \"$(stat -c %U:%G /home/$user/.config/Code/User/globalStorage)\" = $user:users; test \"$(stat -c %U:%G /home/$user/.vscode/extensions)\" = $user:users; test \"$(stat -c %U:%G:%a /run/user/$uid)\" = $user:users:700; su -s /bin/sh $user -c \"test -w /home/$user/.config/Code/User/globalStorage && test -w /home/$user/.vscode/extensions && test -w /run/user/$uid\"; done")
    controller.succeed("grep -Fx 'X-RestartIfChanged=false' /etc/systemd/system/nixorium-apply-controller.service; grep -Fx 'X-RestartIfChanged=false' /etc/systemd/system/nixorium-apply-controller@.service")
    controller.succeed("mkdir /tmp/fake-colmena-bin; ln -s /run/current-system/sw/bin/nixorium-test-colmena /tmp/fake-colmena-bin/colmena")
    controller.succeed("systemctl show nixorium-harmonia.service -p LoadState --value | grep -Fx loaded")
    controller.wait_until_fails("systemctl is-active --quiet nixorium-harmonia.service")
    controller.succeed("journalctl -u harmonia.service --no-pager | grep -F 'Failed to set up credentials'")
    controller.succeed("nixorium --help | grep -F 'setup keys'; nixorium --help | grep -F 'git review'; nixorium --help | grep -F 'git commit plan'; nixorium --help | grep -F 'update check'; nixorium --help | grep -F 'software catalog'; nixorium --help | grep -F 'software preset plan'; nixorium --help | grep -F 'shutdown plan'")
    controller.succeed("mkdir -p /tmp/deployment")
    controller.succeed("cp /etc/nixorium-test/flake.nix /tmp/deployment/flake.nix")
    controller.succeed("cp /etc/nixorium-test/lab-settings.json /tmp/deployment/lab-settings.json")
    controller.succeed("cp /etc/nixorium-test/lab-software.json /tmp/deployment/lab-software.json")
    controller.succeed("cp /etc/nixorium-test/.gitignore /tmp/deployment/.gitignore")
    controller.succeed("jq --arg password '$6$vm$not-the-public-default' '.lab.masterDhcpIp = \"192.0.2.10\" | .lab.ifaceName = \"lab0\" | .lab.teacherPassword = $password | .lab.studentPassword = $password | .lab.adminPassword = $password' /tmp/deployment/lab-settings.json > /tmp/lab-settings.json && mv /tmp/lab-settings.json /tmp/deployment/lab-settings.json")
    controller.succeed("git -C /tmp/deployment init -q")
    controller.succeed("git -C /tmp/deployment config user.name Test; git -C /tmp/deployment config user.email test@example.invalid")
    controller.succeed("git -C /tmp/deployment add flake.nix lab-settings.json lab-software.json .gitignore")
    controller.succeed("git -C /tmp/deployment -c user.name=Test -c user.email=test@example.invalid commit -qm initial")
    controller.succeed("mkdir /tmp/first-setup-deployment; cp /etc/nixorium-test/flake.nix /tmp/first-setup-deployment/flake.nix; cp /etc/nixorium-test/lab-settings.json /tmp/first-setup-deployment/lab-settings.json; cp /etc/nixorium-test/lab-software.json /tmp/first-setup-deployment/lab-software.json; cp /etc/nixorium-test/.gitignore /tmp/first-setup-deployment/.gitignore; git -C /tmp/first-setup-deployment init -q; git -C /tmp/first-setup-deployment add flake.nix lab-settings.json lab-software.json .gitignore; git -C /tmp/first-setup-deployment -c user.name=Test -c user.email=test@example.invalid commit -qm initial")
    controller.succeed("(sleep 8; for _ in {1..12}; do printf '\\r'; sleep 0.2; done; sleep 2; printf 'VM-Admin-2026!\\rVM-Admin-2026!\\rVM-Teacher-2026!\\rVM-Teacher-2026!\\rVM-Student-2026!\\rVM-Student-2026!\\r'; sleep 10; printf y) | TERM=xterm timeout 45s script -qefc 'stty rows 40 cols 120; nixorium setup configure --repo /tmp/first-setup-deployment' /tmp/nixorium-first-setup-tui.log")
    controller.succeed("grep -aF 'All settings are collected first; passwords and one complete validation follow.' /tmp/nixorium-first-setup-tui.log")
    controller.succeed("grep -aF 'Student password accepted (3/3)' /tmp/nixorium-first-setup-tui.log")
    controller.succeed("grep -aF 'Configuration apply: APPLIED' /tmp/nixorium-first-setup-tui.log")
    controller.succeed("! grep -aF 'Caught panic' /tmp/nixorium-first-setup-tui.log")
    controller.succeed("nixorium setup status --repo /tmp/first-setup-deployment --json | jq -e '.currentStage == \"reconcile-keys\"'")
    controller.succeed("test -n \"$(git -C /tmp/first-setup-deployment status --porcelain=v1 -- lab-settings.json)\"")
    controller.succeed("install -d -m 0700 /root/.ssh; ssh-keygen -q -t ed25519 -N \"\" -f /root/.ssh/id_ed25519; printf 'restrict,command=\"/run/current-system/sw/bin/nixorium-test-shutdown-remote\" %s\n' \"$(cat /root/.ssh/id_ed25519.pub)\" > /root/.ssh/authorized_keys; chmod 0600 /root/.ssh/authorized_keys; systemctl reload sshd.service")
    controller.succeed("nixorium shutdown plan --repo /tmp/deployment --on @lab --json > /tmp/shutdown-plan.json || { cat /tmp/shutdown-plan.json; false; }; jq -e '.operation == \"shutdown-plan\" and .state == \"ready\" and .requested == \"@lab\" and .policy == \"protect-unknown\" and .eligible == 1 and (.targets | length) == 1 and .targets[0].name == \"pc01\" and .targets[0].eligible and .targets[0].session == \"idle\" and (.reviewToken | startswith(\"sha256:\")) and .confirmation == \"SHUTDOWN\"' /tmp/shutdown-plan.json || { cat /tmp/shutdown-plan.json; false; }")
    controller.succeed("rm -f /tmp/nixorium-test-shutdown-dispatch.log; printf 'active\n' > /tmp/nixorium-test-session-state; nixorium shutdown plan --repo /tmp/deployment --on @lab --json > /tmp/shutdown-active.json; jq -e '.state == \"ready\" and .eligible == 1 and .targets[0].session == \"active\" and .targets[0].eligible and (.targets[0].detail | contains(\"unsaved work\")) and .confirmation == \"SHUTDOWN\"' /tmp/shutdown-active.json; test ! -e /tmp/nixorium-test-shutdown-dispatch.log")
    controller.succeed("printf 'unknown\n' > /tmp/nixorium-test-session-state; nixorium shutdown plan --repo /tmp/deployment --on @lab --json > /tmp/shutdown-unknown.json || test $? = 1; jq -e '.state == \"blocked\" and .eligible == 0 and .targets[0].session == \"unknown\"' /tmp/shutdown-unknown.json; nixorium shutdown plan --repo /tmp/deployment --on @lab --acknowledge-unknown-sessions --json | jq -e '.state == \"ready\" and .eligible == 1 and .policy == \"acknowledge-unknown\" and .targets[0].eligible'; test ! -e /tmp/nixorium-test-shutdown-dispatch.log; rm /tmp/nixorium-test-session-state")
    controller.succeed("token=$(jq -r .reviewToken /tmp/shutdown-plan.json); nixorium shutdown apply --repo /tmp/deployment --on @lab --expect \"$token\" </dev/null >/tmp/shutdown-noninteractive.out 2>/tmp/shutdown-noninteractive.err || test $? = 2; grep -F 'requires an interactive terminal or explicit --yes' /tmp/shutdown-noninteractive.err; test ! -e /tmp/nixorium-test-shutdown-dispatch.log")
    controller.succeed("nixorium shutdown apply --repo /tmp/deployment --on @lab --expect sha256:stale --yes --json > /tmp/shutdown-stale.json || test $? = 1; jq -e '.operation == \"shutdown-apply\" and .state == \"blocked\" and .retrySafe and any(.issues[]; .field == \"review\")' /tmp/shutdown-stale.json; test ! -e /tmp/nixorium-test-shutdown-dispatch.log")
    controller.succeed("nixorium shutdown plan --repo /tmp/deployment --on @lab --json > /tmp/shutdown-fresh-plan.json; token=$(jq -r .reviewToken /tmp/shutdown-fresh-plan.json); nixorium shutdown apply --repo /tmp/deployment --on @lab --expect \"$token\" --yes --json > /tmp/shutdown-apply.json; jq -e '.operation == \"shutdown-apply\" and .state == \"completed\" and .accepted == 1 and .notSent == 0 and .unconfirmed == 0 and (.retrySafe | not) and .targets[0].name == \"pc01\" and .targets[0].state == \"accepted\"' /tmp/shutdown-apply.json; test \"$(wc -l < /tmp/nixorium-test-shutdown-dispatch.log)\" = 1; grep -Fx 'nixorium-session-state' /tmp/nixorium-test-shutdown-ssh.log; grep -Fx 'systemctl poweroff --no-block' /tmp/nixorium-test-shutdown-ssh.log")
    controller.succeed("cp /tmp/deployment/lab-settings.json /tmp/candidate.json")
    controller.succeed("nixorium config plan --repo /tmp/deployment --file /tmp/candidate.json --json | jq -e '.operation == \"config-plan\" and .state == \"unchanged\" and (.changes | length) == 0'")
    controller.succeed("nixorium setup keys --repo /tmp/deployment --json | jq -e '.operation == \"setup-keys\" and .state == \"ready\" and all(.keys[]; .verified and .matches and .privateMode == 384)'")
    controller.succeed("test $(stat -c '%a' /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem | sort -u) = 600")
    controller.succeed("before=$(sha256sum /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem); nixorium setup keys --repo /tmp/deployment --json >/dev/null; after=$(sha256sum /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem); test \"$before\" = \"$after\"")
    controller.succeed("git -C /tmp/deployment add keys && git -C /tmp/deployment -c user.name=Test -c user.email=test@example.invalid commit -qm keys")
    controller.succeed("test -z \"$(git -C /tmp/deployment status --porcelain=v1 --untracked-files=normal)\"")
    controller.succeed("cp /tmp/deployment/admin-ssh /tmp/existing-admin-key; chmod 0600 /tmp/existing-admin-key; sha256sum /tmp/existing-admin-key > /tmp/existing-admin-key.sha256; rm /tmp/deployment/admin-ssh /tmp/deployment/keys/admin-ssh.pub; ((sleep 8; printf a; sleep 1; printf e; sleep 4; printf k; sleep 4; printf j; printf i; sleep 1; printf '/tmp/existing-admin-key\\r'; sleep 6; printf '\\r'; sleep 6) | TERM=xterm timeout 35s script -qefc 'stty rows 40 cols 120; nixorium --repo /tmp/deployment' /tmp/nixorium-key-import-tui.log) || test $? = 124")
    controller.succeed("sha256sum -c /tmp/existing-admin-key.sha256; cmp /tmp/existing-admin-key /tmp/deployment/admin-ssh; test \"$(stat -c '%a' /tmp/deployment/admin-ssh)\" = 600; nixorium setup status --repo /tmp/deployment --json | jq -e '.currentStage == \"apply-controller\"'; ! git -C /tmp/deployment status --porcelain=v1 | grep -F 'keys/admin-ssh.pub'")
    controller.succeed("nixorium software catalog --repo /tmp/deployment --json > /tmp/software-catalog.json; jq -e '.operation == \"software-catalog\" and .state == \"ready\" and .managedFile == \"lab-software.json\" and (.catalog | length) == 2 and (.packages | length) == 0 and .groups.graphics == [\"pc01\"]' /tmp/software-catalog.json")
    controller.succeed("cp -a /tmp/deployment /tmp/preset-deployment; nixorium software presets --repo /tmp/preset-deployment --json > /tmp/software-presets.json; jq -e '.operation == \"software-presets\" and .state == \"ready\" and .catalog.defaultPreset == \"essential\" and .catalog.presets[0].packages == [\"gimp\",\"vlc\"] and (.fingerprint | startswith(\"sha256:\"))' /tmp/software-presets.json")
    controller.succeed("cp -a /tmp/deployment /tmp/tui-preset-deployment; old_head=$(git -C /tmp/tui-preset-deployment rev-parse HEAD); (sleep 8; printf w; sleep 4; printf p; sleep 4; printf '\\r'; sleep 2; printf j; printf ' '; sleep 1; printf '\\r'; sleep 4; printf '\\r'; sleep 10; printf '\\r'; sleep 1; printf q) | TERM=xterm timeout 50s script -qefc 'stty rows 30 cols 120; nixorium --repo /tmp/tui-preset-deployment' /tmp/nixorium-software-profile-tui.log; test \"$old_head\" != \"$(git -C /tmp/tui-preset-deployment rev-parse HEAD)\"; test -z \"$(git -C /tmp/tui-preset-deployment status --porcelain=v1)\"; jq -e '.packages == [{\"package\":\"gimp\",\"scope\":{\"kind\":\"all-clients\"}}]' /tmp/tui-preset-deployment/lab-software.json")
    controller.succeed("grep -aF 'Add a software profile' /tmp/nixorium-software-profile-tui.log; grep -aF 'Essential packages' /tmp/nixorium-software-profile-tui.log; grep -aF 'Validated together against the pinned package set' /tmp/nixorium-software-profile-tui.log; grep -aF 'Essential saved' /tmp/nixorium-software-profile-tui.log; grep -aF 'Distribute affected computers' /tmp/nixorium-software-profile-tui.log; ! grep -aF 'Caught panic' /tmp/nixorium-software-profile-tui.log")
    controller.succeed("nixorium software preset plan --repo /tmp/preset-deployment --preset essential --scope all-clients --exclude vlc --json > /tmp/software-preset-plan.json; jq -e '.operation == \"software-preset-plan\" and .state == \"ready\" and .request.exclude == [\"vlc\"] and (.selectedPackages | map(.id)) == [\"gimp\"] and (.additions | length) == 1 and .affectedClients == [\"pc01\"] and .confirmation == \"ADD PROFILE\" and (.reviewToken | startswith(\"sha256:\"))' /tmp/software-preset-plan.json")
    controller.succeed("before=$(sha256sum /tmp/preset-deployment/lab-software.json); nixorium software preset apply --repo /tmp/preset-deployment --preset essential --scope all-clients --exclude vlc --expect sha256:stale --yes --json > /tmp/software-preset-stale.json || test $? = 1; after=$(sha256sum /tmp/preset-deployment/lab-software.json); test \"$before\" = \"$after\"; jq -e '.operation == \"software-preset-apply\" and .state == \"conflict\" and any(.issues[]; .field == \"reviewToken\")' /tmp/software-preset-stale.json")
    controller.succeed("token=$(jq -r .reviewToken /tmp/software-preset-plan.json); nixorium software preset apply --repo /tmp/preset-deployment --preset essential --scope all-clients --exclude vlc --expect \"$token\" --yes --json > /tmp/software-preset-apply.json; jq -e '.operation == \"software-preset-apply\" and .state == \"applied\" and (.additions | length) == 1 and .affectedClients == [\"pc01\"]' /tmp/software-preset-apply.json; jq -e '.packages == [{\"package\":\"gimp\",\"scope\":{\"kind\":\"all-clients\"}}]' /tmp/preset-deployment/lab-software.json; nixorium software preset apply --repo /tmp/preset-deployment --preset essential --scope all-clients --exclude vlc --expect \"$token\" --yes --json | jq -e '.state == \"unchanged\"'")
    controller.succeed("nixorium software search --repo /tmp/deployment --query hell --json > /tmp/software-search.json; jq -e '.operation == \"software-search\" and .state == \"ready\" and .query == \"hell\" and .results == [{\"id\":\"hello\",\"label\":\"hello\",\"summary\":\"A friendly greeting program\",\"version\":\"2.12\",\"availability\":\"available\"}]' /tmp/software-search.json")
    controller.succeed("nixorium software plan --repo /tmp/deployment --package vlc --scope group:graphics --json > /tmp/software-plan.json; jq -e '.operation == \"software-change-plan\" and .state == \"ready\" and .request.package == \"vlc\" and .request.scope.group == \"graphics\" and .affectedClients == [\"pc01\"] and (.reviewToken | startswith(\"sha256:\")) and .confirmation == \"SAVE\"' /tmp/software-plan.json")
    controller.succeed("before=$(sha256sum /tmp/deployment/lab-software.json); token=$(jq -r .reviewToken /tmp/software-plan.json); nixorium software apply --repo /tmp/deployment --package vlc --scope group:graphics --expect \"$token\" </dev/null >/tmp/software-noninteractive.out 2>/tmp/software-noninteractive.err || test $? = 2; after=$(sha256sum /tmp/deployment/lab-software.json); test \"$before\" = \"$after\"; grep -F 'requires an interactive terminal or explicit --yes' /tmp/software-noninteractive.err")
    controller.succeed("before=$(sha256sum /tmp/deployment/lab-software.json); nixorium software apply --repo /tmp/deployment --package vlc --scope group:graphics --expect sha256:stale --yes --json > /tmp/software-stale.json || test $? = 1; after=$(sha256sum /tmp/deployment/lab-software.json); test \"$before\" = \"$after\"; jq -e '.operation == \"software-change-apply\" and .state == \"conflict\" and any(.issues[]; .field == \"reviewToken\")' /tmp/software-stale.json")
    controller.succeed("token=$(jq -r .reviewToken /tmp/software-plan.json); nixorium software apply --repo /tmp/deployment --package vlc --scope group:graphics --expect \"$token\" --yes --json > /tmp/software-apply.json; jq -e '.operation == \"software-change-apply\" and .state == \"applied\" and .managedFile == \"lab-software.json\" and .affectedClients == [\"pc01\"]' /tmp/software-apply.json; jq -e '.packages == [{\"package\":\"vlc\",\"scope\":{\"kind\":\"group\",\"group\":\"graphics\"}}]' /tmp/deployment/lab-software.json; test \"$(git -C /tmp/deployment status --porcelain=v1)\" = ' M lab-software.json'")
    controller.succeed("nixorium git commit plan --repo /tmp/deployment --paths lab-software.json --json > /tmp/software-commit-plan.json; jq -e '.state == \"ready\" and .commitMessage == \"chore: update laboratory software\"' /tmp/software-commit-plan.json; token=$(jq -r .reviewToken /tmp/software-commit-plan.json); nixorium git commit apply --repo /tmp/deployment --paths lab-software.json --expect \"$token\" --yes --json > /tmp/software-commit.json; jq -e '.state == \"completed\" and .committed' /tmp/software-commit.json; test -z \"$(git -C /tmp/deployment status --porcelain=v1)\"")
    controller.succeed("jq '.lab.adminPassword = \"$6$review$new-admin\"' /tmp/deployment/lab-settings.json > /tmp/review-settings.json; mv /tmp/review-settings.json /tmp/deployment/lab-settings.json; git -C /tmp/deployment add lab-settings.json; printf '\n# unstaged-review-fixture\n' >> /tmp/deployment/flake.nix; printf 'untracked contents are intentionally not opened\n' > /tmp/deployment/review-note")
    controller.succeed("before_index=$(git -C /tmp/deployment diff --cached | sha256sum); before_worktree=$(git -C /tmp/deployment diff | sha256sum); nixorium git review --repo /tmp/deployment --json > /tmp/git-review.json; after_index=$(git -C /tmp/deployment diff --cached | sha256sum); after_worktree=$(git -C /tmp/deployment diff | sha256sum); test \"$before_index:$before_worktree\" = \"$after_index:$after_worktree\"")
    controller.succeed("jq -e '.operation == \"git-review\" and .state == \"changes\" and .summary.staged == 1 and .summary.unstaged == 1 and .summary.untracked == 1 and .summary.managed == 1 and .summary.unexpected == 2 and (.changes | length) == 3 and any(.changes[]; .path == \"lab-settings.json\" and .staged == \"modified\" and .managed) and any(.changes[]; .path == \"flake.nix\" and .unstaged == \"modified\") and any(.changes[]; .path == \"review-note\" and .untracked) and (.diffs | length) == 2 and all(.diffs[]; (.content | contains(\"$6$review$new-admin\") | not)) and any(.diffs[]; .content | contains(\"<redacted>\"))' /tmp/git-review.json")
    controller.succeed("git -C /tmp/deployment restore --staged --worktree lab-settings.json; git -C /tmp/deployment restore flake.nix; rm /tmp/deployment/review-note; git -C /tmp/deployment add -f secret-key; nixorium git review --repo /tmp/deployment --json > /tmp/git-private.json || test $? = 1; jq -e '.state == \"blocked\" and .summary.private == 1 and (.diffs | length) == 0 and any(.issues[]; .field == \"private-paths\")' /tmp/git-private.json; git -C /tmp/deployment rm --cached -q secret-key")
    controller.succeed("test -z \"$(git -C /tmp/deployment status --porcelain=v1 --untracked-files=normal)\"")
    controller.succeed("cp -a /tmp/deployment /tmp/commit-deployment; git -C /tmp/commit-deployment config user.name Test; git -C /tmp/commit-deployment config user.email test@example.invalid")
    controller.succeed("printf '\n# unrelated-staged-change\n' >> /tmp/commit-deployment/flake.nix; git -C /tmp/commit-deployment add flake.nix; jq '.lab.adminPassword = \"$6$commit$new-admin\"' /tmp/commit-deployment/lab-settings.json > /tmp/commit-settings.json; mv /tmp/commit-settings.json /tmp/commit-deployment/lab-settings.json; printf 'reviewed local note\n' > /tmp/commit-deployment/review-note")
    controller.succeed("old_head=$(git -C /tmp/commit-deployment rev-parse HEAD); nixorium git commit plan --repo /tmp/commit-deployment --paths lab-settings.json,review-note --json > /tmp/git-commit-plan.json; jq -e '.operation == \"git-commit-plan\" and .state == \"ready\" and (.reviewToken | startswith(\"sha256:\")) and .confirmation == \"COMMIT\" and (.diff.content | contains(\"<redacted>\")) and (.diff.content | contains(\"$6$commit$new-admin\") | not)' /tmp/git-commit-plan.json; token=$(jq -r .reviewToken /tmp/git-commit-plan.json); nixorium git commit apply --repo /tmp/commit-deployment --paths lab-settings.json,review-note --expect \"$token\" --yes --json > /tmp/git-commit.json; jq -e --arg old \"$old_head\" '.operation == \"git-commit\" and .state == \"completed\" and .committed and (.retrySafe | not) and .previousRevision == $old and .revision != $old and (.message | contains(\"no remote push\"))' /tmp/git-commit.json")
    controller.succeed("git -C /tmp/commit-deployment show HEAD:review-note | grep -Fx 'reviewed local note'; git -C /tmp/commit-deployment show HEAD:lab-settings.json | grep -F '$6$commit$new-admin'; ! git -C /tmp/commit-deployment show HEAD:flake.nix | grep -qF 'unrelated-staged-change'; test \"$(git -C /tmp/commit-deployment diff --cached --name-only)\" = flake.nix; test -z \"$(git -C /tmp/commit-deployment status --porcelain=v1 -- lab-settings.json review-note)\"")
    controller.succeed("printf 'first\n' > /tmp/commit-deployment/stale-note; nixorium git commit plan --repo /tmp/commit-deployment --paths stale-note --json > /tmp/git-stale-plan.json; token=$(jq -r .reviewToken /tmp/git-stale-plan.json); printf 'second\n' > /tmp/commit-deployment/stale-note; before=$(git -C /tmp/commit-deployment rev-parse HEAD); nixorium git commit apply --repo /tmp/commit-deployment --paths stale-note --expect \"$token\" --yes --json > /tmp/git-stale.json || test $? = 1; test \"$before\" = \"$(git -C /tmp/commit-deployment rev-parse HEAD)\"; jq -e '.state == \"blocked\" and (.committed | not) and any(.issues[]; .field == \"review\")' /tmp/git-stale.json")
    controller.succeed("printf '{ token = \"plaintext-secret\"; }\n' > /tmp/commit-deployment/unsafe.nix; nixorium git commit plan --repo /tmp/commit-deployment --paths unsafe.nix --json > /tmp/git-unsafe-plan.json || test $? = 1; jq -e '.state == \"blocked\" and any(.issues[]; .field == \"proposal\" and (.message | contains(\"plaintext\")))' /tmp/git-unsafe-plan.json")
    controller.succeed("cp -a /tmp/deployment /tmp/update-deployment; printf '{\n  inputs.nixorium.url = \"github:giovantenne/nixorium/v2.0.0\";\n  outputs = { self, nixorium }: {};\n}\n' > /tmp/update-deployment/flake.nix; printf '%s\n' '{\"root\":\"root\",\"nodes\":{\"root\":{\"inputs\":{\"nixorium\":\"nixorium\"}},\"nixorium\":{\"locked\":{\"rev\":\"1111111111111111111111111111111111111111\"}}}}' > /tmp/update-deployment/flake.lock; chown -R admin:users /tmp/update-deployment; su - admin -c 'git -C /tmp/update-deployment add flake.nix flake.lock && git -C /tmp/update-deployment -c user.name=Test -c user.email=test@example.invalid commit -qm update-fixture'; mkdir /tmp/fake-update-bin /tmp/fake-update-check-bin; ln -s /run/current-system/sw/bin/nixorium-test-update-nix /tmp/fake-update-bin/nix; ln -s /run/current-system/sw/bin/nixorium-test-update-git /tmp/fake-update-check-bin/git")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_UPDATE_GIT_LOG=/tmp/update-git.log PATH=/tmp/fake-update-check-bin:$PATH nixorium update check --repo /tmp/update-deployment --json' > /tmp/update-check.json; jq -e '.operation == \"update-check\" and .state == \"available\" and .upstream == \"github:giovantenne/nixorium\" and .currentRef == \"v2.0.0\" and .currentChannel == \"stable\" and .development == [{\"tag\":\"master\",\"objectId\":\"7777777777777777777777777777777777777777\",\"channel\":\"moving\"}] and (.stable | length) == 1 and .stable[0].tag == \"v2.3.0\" and (.prerelease | length) == 1 and .prerelease[0].tag == \"v2.4.0-beta.1\" and (.truncated | not)' /tmp/update-check.json; grep -F 'credential.helper=' /tmp/update-git.log; grep -F 'prompt=0 askpass= sshaskpass= interactive=Never global=/dev/null nosystem=1' /tmp/update-git.log")
    controller.succeed("rm -f /tmp/update-nix.log; su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.1.0-beta.1 --json' > /tmp/update-prerelease.json || test $? = 1; jq -e '.operation == \"update-plan\" and .state == \"blocked\" and any(.issues[]; .field == \"target\" and (.message | contains(\"--allow-prerelease\")))' /tmp/update-prerelease.json; test ! -e /tmp/update-nix.log")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target master --json' > /tmp/update-master.json; jq -e '.operation == \"update-plan\" and .state == \"ready\" and .target == \"master\" and .targetChannel == \"moving\" and .confirmation == \"UPDATE\" and (.reviewToken | startswith(\"sha256:\"))' /tmp/update-master.json; rm -f /tmp/update-nix.log")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.1.0 --json' > /tmp/update-plan.json; jq -e '.operation == \"update-plan\" and .state == \"ready\" and .currentRef == \"v2.0.0\" and .target == \"v2.1.0\" and .targetChannel == \"stable\" and (.reviewToken | startswith(\"sha256:\")) and .confirmation == \"UPDATE\" and (.checks | length) == 7 and all(.checks[]; .state == \"passed\") and (.diff.content | contains(\"flake.nix\")) and (.diff.content | contains(\"flake.lock\"))' /tmp/update-plan.json; test \"$(wc -l < /tmp/update-nix.log)\" = 8; test \"$(grep -c ' build ' /tmp/update-nix.log)\" = 5")
    controller.succeed("rm -f /tmp/update-controller-nix.log; su - admin -c 'NIXORIUM_TEST_UPDATE_MODE=controller NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-controller-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.2.0 --json' > /tmp/update-controller-plan.json; jq -e '.state == \"ready\" and (.checks | length) == 3 and .checks[1].message == \"candidate controller is ready\" and .checks[2].id == \"controller\"' /tmp/update-controller-plan.json; test \"$(wc -l < /tmp/update-controller-nix.log)\" = 4; test \"$(grep -c ' build ' /tmp/update-controller-nix.log)\" = 1; grep -F '#nixosConfigurations.pc99.config.system.build.toplevel' /tmp/update-controller-nix.log; ! grep -E '#(nixosConfigurations\\.pc01|nixosConfigurations\\.netboot|pxeFirmware|installerBundle)' /tmp/update-controller-nix.log")
    controller.succeed("before=$(sha256sum /tmp/update-deployment/flake.nix /tmp/update-deployment/flake.lock); token=$(jq -r .reviewToken /tmp/update-plan.json); su - admin -c \"NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:\$PATH nixorium update apply --repo /tmp/update-deployment --target v2.1.0 --expect '$token' </dev/null >/tmp/update-noninteractive.out 2>/tmp/update-noninteractive.err\" || test $? = 2; after=$(sha256sum /tmp/update-deployment/flake.nix /tmp/update-deployment/flake.lock); test \"$before\" = \"$after\"; test ! -s /tmp/update-noninteractive.out; grep -F 'requires an interactive terminal or explicit --yes' /tmp/update-noninteractive.err")
    controller.succeed("before=$(sha256sum /tmp/update-deployment/flake.nix /tmp/update-deployment/flake.lock); su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update apply --repo /tmp/update-deployment --target v2.1.0 --expect sha256:stale --yes --json' > /tmp/update-stale.json || test $? = 1; after=$(sha256sum /tmp/update-deployment/flake.nix /tmp/update-deployment/flake.lock); test \"$before\" = \"$after\"; jq -e '.operation == \"update-apply\" and .state == \"blocked\" and (.updated | not) and .retrySafe and any(.issues[]; .field == \"review\")' /tmp/update-stale.json")
    controller.succeed("token=$(jq -r .reviewToken /tmp/update-plan.json); su - admin -c \"NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:\$PATH nixorium update apply --repo /tmp/update-deployment --target v2.1.0 --expect '$token' --yes --json\" > /tmp/update-apply.json; jq -e '.operation == \"update-apply\" and .state == \"completed\" and .target == \"v2.1.0\" and .updated and (.retrySafe | not) and (.message | contains(\"review and commit\"))' /tmp/update-apply.json; grep -F 'github:giovantenne/nixorium/v2.1.0' /tmp/update-deployment/flake.nix; grep -F '2222222222222222222222222222222222222222' /tmp/update-deployment/flake.lock; test \"$(git -c safe.directory=/tmp/update-deployment -C /tmp/update-deployment status --porcelain=v1 | wc -l)\" = 2; git -c safe.directory=/tmp/update-deployment -C /tmp/update-deployment status --porcelain=v1 | grep -F ' M flake.nix'; git -c safe.directory=/tmp/update-deployment -C /tmp/update-deployment status --porcelain=v1 | grep -F ' M flake.lock'; test ! -e /tmp/update-deployment/result")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update apply --repo /tmp/update-deployment --target v2.1.0 --expect sha256:stale --yes --json' > /tmp/update-dirty-retry.json || test $? = 1; jq -e '.operation == \"update-plan\" and .state == \"blocked\" and any(.issues[]; .field == \"git\")' /tmp/update-dirty-retry.json")
    controller.succeed("su - admin -c 'git -C /tmp/update-deployment add flake.nix flake.lock && git -C /tmp/update-deployment -c user.name=Test -c user.email=test@example.invalid commit -qm updated'; rm -f /tmp/update-nix.log; su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.1.0 --json' > /tmp/update-current.json || test $? = 1; jq -e '.state == \"blocked\" and any(.issues[]; .field == \"target\" and (.message | contains(\"already configured\")))' /tmp/update-current.json; test ! -e /tmp/update-nix.log; su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.2.0 --json' | jq -e '.state == \"ready\" and .target == \"v2.2.0\"'")
    with subtest("autonomous package-base planning and reviewed apply"):
      base_flake = '{\n  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";\n  inputs.nixorium.url = "github:giovantenne/nixorium/v2.1.0";\n  inputs.nixorium.inputs.nixpkgs.follows = "nixpkgs";\n}\n'
      base_lock = {"version": 7, "root": "root", "nodes": {
        "root": {"inputs": {"nixpkgs": "nixpkgs", "nixorium": "nixorium"}},
        "nixorium": {"inputs": {"nixpkgs": ["nixpkgs"]}, "locked": {"rev": "2" * 40}},
        "nixpkgs": {"original": {"type": "github", "owner": "NixOS", "repo": "nixpkgs", "ref": "nixos-26.05"}, "locked": {"type": "github", "owner": "NixOS", "repo": "nixpkgs", "rev": "a" * 40, "narHash": "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}
      }}
      controller.succeed("cp -a /tmp/update-deployment /tmp/base-deployment")
      for name, content in [("flake.nix", base_flake), ("flake.lock", json.dumps(base_lock))]:
        controller.succeed("printf %s " + shlex.quote(content) + " > /tmp/base-deployment/" + name)
      controller.succeed("chown -R admin:users /tmp/base-deployment; su - admin -c 'git -C /tmp/base-deployment add flake.nix flake.lock && git -C /tmp/base-deployment -c user.name=Test -c user.email=test@example.invalid commit -qm base-fixture'")
      base_lock["nodes"]["nixpkgs"]["locked"]["rev"] = "b" * 40
      controller.succeed("printf %s " + shlex.quote(json.dumps(base_lock)) + " > /tmp/base-candidate.json; chmod 644 /tmp/base-candidate.json")
      base_command = "NIXORIUM_TEST_BASE_CANDIDATE=/tmp/base-candidate.json NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/base-update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium package-base "
      def run_base(arguments):
        return "su - admin -c " + shlex.quote(base_command + arguments + " --repo /tmp/base-deployment --json")
      status = json.loads(controller.succeed(run_base("status")))
      assert status["channel"] == "nixos-26.05" and status["revision"] == "a" * 40
      controller.succeed(run_base("plan --target nixos-26.11") + " > /tmp/base-policy.json || test $? = 1")
      controller.succeed("jq -e '.state == \"blocked\" and any(.issues[]; .field == \"policy\")' /tmp/base-policy.json; test ! -e /tmp/base-update-nix.log")
      plan = json.loads(controller.succeed(run_base("plan")))
      assert plan["state"] == "ready" and plan["kind"] == "package-base"
      assert plan["packageBase"]["targetRevision"] == "b" * 40
      controller.succeed("git -c safe.directory=/tmp/base-deployment -C /tmp/base-deployment diff --exit-code; grep -F '#nixoriumOfflineCheck' /tmp/base-update-nix.log")
      controller.succeed(run_base("apply --expect sha256:stale --yes") + " > /tmp/base-stale.json || test $? = 1")
      controller.succeed("jq -e '.operation == \"package-base-apply\" and .state == \"blocked\" and (.updated | not)' /tmp/base-stale.json; git -c safe.directory=/tmp/base-deployment -C /tmp/base-deployment diff --exit-code")
      applied = json.loads(controller.succeed(run_base("apply --expect " + plan["reviewToken"] + " --yes")))
      assert applied["state"] == "completed" and applied["updated"]
      actual = json.loads(controller.succeed("cat /tmp/base-deployment/flake.lock"))
      assert actual == base_lock
      assert controller.succeed("cat /tmp/base-deployment/flake.nix") == base_flake

    controller.succeed("source_path=$(nix --extra-experimental-features 'nix-command flakes' flake metadata git+file:///tmp/deployment --json --no-write-lock-file | jq -r .path); test ! -e \"$source_path/secret-key\"; test ! -e \"$source_path/admin-ssh\"; test ! -e \"$source_path/veyon-private-key.pem\"")
    controller.succeed("git -C /tmp/deployment add -f secret-key")
    controller.fail("nixorium status --repo /tmp/deployment --json")
    controller.succeed("git -C /tmp/deployment rm --cached -q secret-key")
    controller.succeed("mkdir -p /home/admin/nixorium-deployment")
    controller.succeed("cp -a /tmp/deployment/. /home/admin/nixorium-deployment/")
    controller.succeed("chown -R admin:users /home/admin/nixorium-deployment")
    controller.succeed("su - admin -c 'cd /home/admin/nixorium-deployment && nixorium install usb status --id 0123456789abcdef0123456789abcdef --json' >/tmp/usb-status.json || test $? = 1; jq -e '.operationId == \"0123456789abcdef0123456789abcdef\" and .state == \"unavailable\"' /tmp/usb-status.json")
    controller.fail("cd /home/admin/nixorium-deployment && nixorium install usb status --id 0123456789abcdef0123456789abcdef --json")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '192.0.2.10/24'; ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'")
    controller.succeed("su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --json > /tmp/pxe-prepare-failed.json' || test $? = 1")
    controller.succeed("jq -e '.operation == \"pxe-prepare\" and .state == \"failed\" and .unit == \"nixorium-prepare-pxe.service\"' /tmp/pxe-prepare-failed.json")
    controller.succeed("jq -e '.schemaVersion == 1 and .operation == \"pxe-prepare\" and .state == \"failed\" and .phase == \"network\" and (.recent | length) <= 5 and (.recent[-1] | contains(\"Harmonia cache service is not active\"))' /var/lib/nixorium/prepared/progress.json")
    controller.succeed("test $(stat -c '%U:%G:%a' /var/lib/nixorium/prepared/progress.json) = admin:users:600")
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
    controller.succeed("jq -e '.schemaVersion == 1 and .operation == \"pxe-prepare\" and .state == \"completed\" and .phase == \"complete\" and .current == 1 and .total == 1 and (.recent | length) <= 5 and .recent[-1] == \"Prepared PXE artifacts for 1 clients\"' /var/lib/nixorium/prepared/progress.json")
    controller.succeed("jq -e '.schemaVersion == 1 and .revision != \"\" and .controller.dhcpIp == \"192.0.2.10\" and .network.ifaceName == \"lab0\" and .artifacts.kernel.relativePath == \"bzImage\" and .artifacts.firmware.relativePath == \"snponly.efi\" and (.clients | length) == 1 and .clients[0].name == \"pc01\" and (.clients[0].storePath | startswith(\"/nix/store/\"))' /var/lib/nixorium/prepared/prepared.json")
    controller.succeed("test $(stat -c '%U:%G:%a' /var/lib/nixorium/prepared/prepared.json) = admin:users:644")
    controller.succeed("revision=$(jq -r .revision /var/lib/nixorium/prepared/prepared.json); test $(find /var/lib/nixorium/prepared/roots/$revision -maxdepth 1 -type l | wc -l) = 5; nix-store --gc --print-roots | grep -F /var/lib/nixorium/prepared/roots/$revision/kernel")
    controller.succeed("test ! -e /home/admin/nixorium-deployment/result-kernel; test ! -e /home/admin/nixorium-deployment/result-initrd; test ! -e /home/admin/nixorium-deployment/result-ipxe")
    controller.succeed("mkdir /var/lib/nixorium/prepared/roots/0000000000000000000000000000000000000000; chown admin:users /var/lib/nixorium/prepared/roots/0000000000000000000000000000000000000000")
    controller.succeed("before=$(jq -c 'del(.preparedAt)' /var/lib/nixorium/prepared/prepared.json); su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --json' >/dev/null; after=$(jq -c 'del(.preparedAt)' /var/lib/nixorium/prepared/prepared.json); test \"$before\" = \"$after\"; test ! -e /var/lib/nixorium/prepared/roots/0000000000000000000000000000000000000000")
    controller.succeed("ip addr del 192.0.2.10/24 dev lab0; ip addr add 192.0.2.11/24 dev lab0; su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --json' | jq -e '.state == \"completed\"'")
    controller.succeed("jq -e '.controller.dhcpIp == \"192.0.2.11\"' /var/lib/nixorium/prepared/prepared.json; journalctl -u nixorium-prepare-pxe.service --no-pager | grep -F 'preparing PXE for the unambiguous live address 192.0.2.11'")
    controller.succeed("su - admin -c 'nixorium status --repo ~/nixorium-deployment --json' | jq -e '.pxePreparation.ready and .pxePreparation.dhcpAddress == \"192.0.2.11\"'; su - admin -c 'nixorium doctor --repo ~/nixorium-deployment --json' | jq -e 'any(.findings[]; .id == \"NETWORK-DHCP-IP\" and .level == \"OK\" and (.evidence | contains(\"192.0.2.11\")))'")
    controller.succeed("ip addr add 192.0.2.12/24 dev lab0; before=$(sha256sum /var/lib/nixorium/prepared/prepared.json); su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --json > /tmp/pxe-address-ambiguous.json' || test $? = 1; after=$(sha256sum /var/lib/nixorium/prepared/prepared.json); test \"$before\" = \"$after\"; jq -e '.state == \"failed\"' /tmp/pxe-address-ambiguous.json; journalctl -u nixorium-prepare-pxe.service --no-pager | grep -F 'live controller address is ambiguous' | grep -F '192.0.2.11 192.0.2.12'; ip addr del 192.0.2.12/24 dev lab0")
    controller.succeed("systemctl show nixorium-pxe.service nixorium-pxe-network.service nixorium-pxe-recover.service -p LoadState --value | grep -vFx not-found")
    controller.succeed("su - admin -c 'nixorium pxe start --repo ~/nixorium-deployment --json </dev/null > /tmp/pxe-confirmation-required.json' || test $? = 2")
    controller.succeed("test ! -e /var/lib/nixorium/pxe/session.json; ! su - admin -c 'systemctl start nixorium-pxe-network.service'")
    controller.succeed("su - admin -c 'nixorium pxe start --repo ~/nixorium-deployment --yes --json' | jq -e '.operation == \"pxe-start\" and .state == \"completed\" and .mode == \"active\"'")
    controller.succeed("systemctl is-active --quiet nixorium-pxe.service; systemctl is-active --quiet nixorium-pxe-network.service")
    controller.succeed("su - admin -c 'nixorium pxe start --repo ~/nixorium-deployment --yes --json' | jq -e '.state == \"completed\" and .mode == \"active\" and (.message | contains(\"already active\"))'")
    controller.succeed("su - admin -c 'nixorium status --repo ~/nixorium-deployment --json' | jq -e '.pxe.mode == \"active\" and .pxe.listener.active and .pxe.network.active'")
    controller.succeed("su - admin -c 'nixorium doctor --repo ~/nixorium-deployment --json' | jq -e 'any(.findings[]; .id == \"PXE-LIFECYCLE\" and .level == \"OK\") and any(.findings[]; .id == \"PXE-PORTS\" and .level == \"OK\")'")
    controller.wait_until_succeeds("curl --fail --silent http://192.0.2.11:8080/bzImage >/dev/null; curl --fail --silent http://192.0.2.11:8080/initrd >/dev/null")
    controller.succeed("grep -F 'kernel ''${base-url}/bzImage init=/nix/store/test-init nixorium.controller-dhcp-ip=192.0.2.11' /run/nixorium/pxe-runtime/tftp/boot.ipxe")
    controller.succeed("ss -H -lun 'sport = :67' | grep -F ':67'; ss -H -lun 'sport = :69' | grep -F ':69'; ss -H -ltn 'sport = :8080' | grep -F ':8080'")
    controller.succeed("pgrep -u nobody -f 'python3 -m http.server 8080'; pid=$(pgrep -o -x dnsmasq); test $(awk '/^Uid:/ {print $3}' /proc/$pid/status) = $(id -u nixorium-pxe-dnsmasq)")
    controller.fail("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'")
    controller.succeed("su - admin -c 'nixorium pxe stop --repo ~/nixorium-deployment --json' | jq -e '.operation == \"pxe-stop\" and .state == \"completed\" and .mode == \"stopped\"'")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; test ! -e /var/lib/nixorium/pxe/session.json")
    controller.succeed("systemd-run --unit=nixorium-test-pxe-conflict --property=Type=simple python3 -m http.server 8080 --bind 192.0.2.11")
    controller.succeed("su - admin -c 'nixorium pxe start --repo ~/nixorium-deployment --yes --json > /tmp/pxe-start-failed.json' || test $? = 1")
    controller.succeed("jq -e '.operation == \"pxe-start\" and .state == \"failed\" and (.message | contains(\"nixorium-pxe.service\"))' /tmp/pxe-start-failed.json")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; test ! -e /var/lib/nixorium/pxe/session.json; ! systemctl is-active --quiet nixorium-pxe-network.service")
    controller.succeed("journalctl -u nixorium-pxe.service --no-pager | grep -F 'PXE HTTP port 8080 is already in use'")
    controller.succeed("systemctl stop nixorium-test-pxe-conflict.service; systemctl reset-failed nixorium-pxe.service nixorium-pxe-network.service")
    controller.succeed("ip addr add 198.51.100.7/24 dev lab0")
    controller.succeed("systemctl start nixorium-pxe-network.service")
    controller.succeed("systemctl is-active --quiet nixorium-pxe-network.service")
    controller.fail("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '192.0.2.11/24'; ip -4 -o addr show dev lab0 scope global | grep -F '198.51.100.7/24'")
    controller.succeed("jq -e '.schemaVersion == 1 and .state == \"network-active\" and .interface == \"lab0\" and .dhcpAddress == \"192.0.2.11\" and .staticAddress == \"10.0.0.99\" and .prefixLength == 8 and .removedStatic and (.originalAddresses | index(\"10.0.0.99/8\") != null) and .artifacts.kernel.relativePath == \"bzImage\"' /var/lib/nixorium/pxe/session.json")
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
    controller.succeed("readlink -f /run/current-system > /tmp/controller-system-before; touch /run/nixorium-test-activation-fail; su - admin -c 'nixorium setup apply --repo ~/nixorium-deployment --yes --json > /tmp/partial-apply.json 2>/tmp/partial-apply.progress' || test $? = 1")
    controller.succeed("jq -e '.operation == \"setup-apply-controller\" and .state == \"failed\"' /tmp/partial-apply.json; grep -F 'Progress [activate] (2/4): Controller apply stopped unexpectedly' /tmp/partial-apply.progress; test \"$(readlink -f /run/current-system)\" != \"$(cat /tmp/controller-system-before)\"; test ! -e /var/lib/nixorium/controller/applied.json")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.currentStage == \"apply-controller\" and (.stages[] | select(.id == \"apply-controller\").detail | contains(\"no successful controller activation\"))'")
    controller.succeed("rm /run/nixorium-test-activation-fail; systemctl reset-failed nixorium-apply-controller.service")
    controller.succeed("ip addr del 10.0.0.99/8 dev lab0; ! ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'")
    controller.succeed("su - admin -c 'nixorium setup apply --repo ~/nixorium-deployment --yes --json 2>/tmp/setup-apply.progress' | jq -e '.operation == \"setup-apply-controller\" and .state == \"completed\"'; grep -F 'Progress [complete] (4/4): Controller revision activated and verified' /tmp/setup-apply.progress")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'")
    controller.succeed("test -e /run/nixorium-controller-applied")
    controller.succeed("revision=$(git -c safe.directory=/home/admin/nixorium-deployment -C /home/admin/nixorium-deployment rev-parse HEAD); system_path=$(readlink -f /run/current-system); jq -e --arg revision \"$revision\" --arg systemPath \"$system_path\" '.schemaVersion == 1 and .revision == $revision and .systemPath == $systemPath and (.activatedAt | endswith(\"Z\"))' /var/lib/nixorium/controller/applied.json; test \"$(stat -c '%U:%G:%a' /var/lib/nixorium/controller/applied.json)\" = root:root:644")
    controller.succeed("jq -e '.schemaVersion == 1 and .operation == \"controller-apply\" and .state == \"completed\" and .phase == \"complete\" and .current == 4 and .total == 4 and (.recent | length) <= 5' /var/lib/nixorium/controller/progress.json; test \"$(stat -c '%U:%G:%a' /var/lib/nixorium/controller/progress.json)\" = admin:users:600")
    controller.succeed("test -e /home/teacher/.config/nixorium-activation-test; test -e /run/user/1000/nixorium-activation-test; test ! -e /home/admin/nixorium-deployment/activation-must-not-write")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.state == \"ready\" and (.currentStage | not) and (.stages | length) == 9 and (.stages[] | select(.id == \"prepare-artifacts\").state) == \"complete\"'")
    controller.succeed("touch /run/nixorium-test-activation-fail; ! systemctl start nixorium-apply-controller.service; test ! -e /var/lib/nixorium/controller/applied.json; rm /run/nixorium-test-activation-fail; systemctl reset-failed nixorium-apply-controller.service")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.currentStage == \"apply-controller\"'")
    controller.succeed("su - admin -c 'nixorium setup apply --repo ~/nixorium-deployment --yes --json' | jq -e '.state == \"completed\"'")
    controller.succeed("su - admin -c 'nixorium controller plan --repo ~/nixorium-deployment --json' > /tmp/controller-plan.json; jq -e '.operation == \"controller-plan\" and .state == \"current\" and .controller == \"pc99\" and .current and (.revision | length) == 40 and .confirmation == \"REBUILD\"' /tmp/controller-plan.json")
    controller.succeed("su - admin -c 'nixorium controller apply --repo ~/nixorium-deployment --expect 0000000000000000000000000000000000000000 --yes --json >/tmp/controller-stale.json' || test $? = 1; jq -e '.state == \"blocked\" and any(.issues[]; .field == \"review\")' /tmp/controller-stale.json")
    controller.succeed("su - admin -c 'revision=$(git -C ~/nixorium-deployment rev-parse HEAD); nixorium controller apply --repo ~/nixorium-deployment --expect \"$revision\" --yes --json 2>/tmp/controller-apply.progress' > /tmp/controller-apply.json; jq -e '.operation == \"controller-apply\" and .state == \"completed\" and .phase == \"complete\" and .applied and .verified and .unit == (\"nixorium-apply-controller@\" + .revision + \".service\")' /tmp/controller-apply.json; grep -F 'Progress [complete] (4/4): Controller revision activated and verified' /tmp/controller-apply.progress")
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
    controller.succeed("nixorium status --repo /tmp/deployment --json | jq -e '.operation == \"status\" and .state == \"ready\" and .lab.clients.hosts[0].name == \"pc01\" and .pxe.mode == \"ready\" and .pxePreparation.ready and all(.artifacts[]; .present) and any(.services[]; .name == \"nixorium-harmonia.service\" and .loaded and .active)'")
    controller.succeed("su - admin -c 'nixorium services --repo /home/admin/nixorium-deployment --json' | jq -e '.operation == \"services\" and .state == \"healthy\" and (.services | length) == 2 and .services[0].id == \"cache\" and .services[0].healthy and .services[0].state == \"healthy\" and .services[0].units[0].name == \"nixorium-harmonia.service\" and .services[1].id == \"pxe\" and .services[1].healthy and .services[1].state == \"ready\" and (.services[1].actions | length) == 0'")
    controller.fail("su - admin -c 'systemctl restart harmonia.service'")
    controller.succeed("su - admin -c 'nixorium services restart cache --repo /home/admin/nixorium-deployment --yes --json' > /tmp/service-restart.json || { cat /tmp/service-restart.json; false; }; jq -e '.operation == \"service-restart\" and .state == \"completed\" and .service == \"cache\" and .unit == \"nixorium-restart-cache.service\" and .verified and .current.active' /tmp/service-restart.json || { cat /tmp/service-restart.json; false; }")
    controller.succeed("su - admin -c 'nixorium hosts --repo /home/admin/nixorium-deployment --json' > /tmp/hosts-current.json; jq -e '.operation == \"hosts\" and .state == \"available\" and .deployment.current == 1 and .deployment.outdated == 0 and .deployment.unknown == 0 and (.hosts | length) == 1 and .hosts[0].name == \"pc01\" and .hosts[0].role == \"client\" and .hosts[0].reachability == \"reachable\" and .hosts[0].ssh == \"available\" and .hosts[0].deployment == \"current\" and (.hosts[0].currentSystem | startswith(\"/nix/store/\")) and .hosts[0].currentRevision == .hosts[0].desiredRevision' /tmp/hosts-current.json || { cat /tmp/hosts-current.json; false; }")
    controller.succeed("nixorium deploy plan --repo /tmp/deployment --on @lab --json | jq -e '.operation == \"deploy-plan\" and .state == \"ready\" and .requested == \"@lab\" and .colmenaSelector == \"@lab\" and .buildFirst and (.revision | length) == 40 and (.targets | length) == 1 and .targets[0].name == \"pc01\"'")
    controller.succeed("nixorium deploy plan --repo /tmp/deployment --on pc01 --json | jq -e '.state == \"ready\" and .colmenaSelector == \"pc01\"'")
    controller.fail("nixorium deploy plan --repo /tmp/deployment --on pc02 --json")
    controller.succeed("su - admin -c 'revision=$(git -C /home/admin/nixorium-deployment rev-parse HEAD); NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-success PATH=/tmp/fake-colmena-bin:$PATH nixorium deploy apply --repo /home/admin/nixorium-deployment --on pc01 --expect \"$revision\" --yes --json >/tmp/deploy-success.json 2>/tmp/deploy-success.progress' || { cat /tmp/deploy-success.json /tmp/deploy-success.progress; false; }")
    controller.succeed("jq -e '.operation == \"deploy-apply\" and .state == \"completed\" and .phase == \"complete\" and .buildCompleted and .applyCompleted and .retrySafe and .colmenaSelector == \"pc01\" and .verification.attempted == 1 and .verification.verified == 1 and .verification.recorded == 1 and .verification.targets[0].name == \"pc01\" and .verification.targets[0].state == \"verified\" and (.logPath | startswith(\"/home/admin/.local/state/nixorium/operations/deploy-\"))' /tmp/deploy-success.json")
    controller.succeed("test \"$(head -n 1 /tmp/colmena-success)\" = 'build --on pc01 --verbose --color never'; test \"$(tail -n 1 /tmp/colmena-success)\" = 'apply switch --on pc01 --verbose --color never'; test \"$(wc -l </tmp/colmena-success)\" = 2; test -e /tmp/nixorium-test-colmena-ssh-policy")
    controller.succeed("log=$(jq -r .logPath /tmp/deploy-success.json); test \"$(stat -c '%U:%G:%a' \"$log\")\" = admin:users:600; grep -F 'fake colmena: build' \"$log\"; grep -F 'Result: completed' \"$log\"; grep -F 'Verified targets: 1/1' \"$log\"; grep -F 'fake colmena: apply' /tmp/deploy-success.progress")
    controller.succeed("history=(/home/admin/.local/state/nixorium/deployments/*.json); test \"''${#history[@]}\" = 1; test \"$(stat -c '%U:%G:%a' \"''${history[0]}\")\" = admin:users:600; jq -e '.schemaVersion == 1 and .repository == \"/home/admin/nixorium-deployment\" and (.hosts | keys) == [\"pc01\"] and (.hosts.pc01.revision | length) == 40 and (.hosts.pc01.systemPath | startswith(\"/nix/store/\")) and (.hosts.pc01.verifiedAt | endswith(\"Z\"))' \"''${history[0]}\"")
    controller.succeed("su - admin -c 'nixorium hosts --repo /home/admin/nixorium-deployment --json' > /tmp/hosts-verified.json; jq -e '.hosts[0].lastSuccessfulDeploy.revision == .hosts[0].desiredRevision and (.hosts[0].lastSuccessfulDeploy.systemPath | startswith(\"/nix/store/\")) and (.hosts[0].lastSuccessfulDeploy.verifiedAt | endswith(\"Z\"))' /tmp/hosts-verified.json")
    controller.succeed("su - admin -c 'revision=$(git -C /home/admin/nixorium-deployment rev-parse HEAD); NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-failed NIXORIUM_TEST_COLMENA_FAIL=apply PATH=/tmp/fake-colmena-bin:$PATH nixorium deploy apply --repo /home/admin/nixorium-deployment --on @lab --expect \"$revision\" --yes --json >/tmp/deploy-failed.json 2>/tmp/deploy-failed.progress' || test $? = 1")
    controller.succeed("jq -e '.state == \"failed\" and .phase == \"apply\" and .buildCompleted and (.applyCompleted | not) and .retrySafe and .verification.verified == 1 and .verification.recorded == 1 and (.message | contains(\"authenticated reconciliation\"))' /tmp/deploy-failed.json")
    controller.succeed("su - admin -c 'revision=$(git -C /home/admin/nixorium-deployment rev-parse HEAD); NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-retry PATH=/tmp/fake-colmena-bin:$PATH nixorium deploy apply --repo /home/admin/nixorium-deployment --on @lab --expect \"$revision\" --yes --json >/tmp/deploy-retry.json 2>/tmp/deploy-retry.progress'")
    controller.succeed("jq -e '.state == \"completed\" and .buildCompleted and .applyCompleted and .verification.verified == 1 and .verification.recorded == 1' /tmp/deploy-retry.json; test \"$(wc -l </tmp/colmena-retry)\" = 2")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-stale PATH=/tmp/fake-colmena-bin:$PATH nixorium deploy apply --repo /home/admin/nixorium-deployment --on pc01 --expect 0000000000000000000000000000000000000000 --yes --json >/tmp/deploy-stale.json' || test $? = 1")
    controller.succeed("jq -e '.state == \"blocked\" and .phase == \"preflight\" and any(.issues[]; .field == \"review\")' /tmp/deploy-stale.json; test ! -e /tmp/colmena-stale")
    controller.succeed("su - admin -c 'cd /tmp; nixorium logs --json' > /tmp/logs.json; jq -e '.operation == \"logs-list\" and .state == \"available\" and .limit == 50 and (.logs | length) >= 3 and all(.logs[]; .kind == \"deployment\" and .available and (.id | startswith(\"deploy-\"))) and any(.records[]; .operation == \"deploy-apply\" and .state == \"completed\" and .subject == \"@lab\") and any(.records[]; .operation == \"controller-apply\" and .state == \"completed\") and any(.records[]; .operation == \"service-restart\" and .state == \"completed\") and any(.records[]; .operation == \"pxe-start\" and .state == \"completed\")' /tmp/logs.json")
    controller.succeed("test \"$(stat -c '%U:%G:%a' /home/admin/.local/state/nixorium/operations/records.json)\" = admin:users:600; test \"$(stat -c '%a' /home/admin/.local/state/nixorium/operations)\" = 700")
    controller.succeed("chmod 0644 /home/admin/.local/state/nixorium/operations/records.json; su - admin -c 'nixorium logs --json' >/tmp/records-unsafe.json || test $? = 1; jq -e '.state == \"partial\" and any(.issues[]; .field == \"records\" and (.message | contains(\"0600\"))) and (.logs | length) >= 3' /tmp/records-unsafe.json; chmod 0600 /home/admin/.local/state/nixorium/operations/records.json")
    controller.succeed("id=$(jq -r '.logs[0].id' /tmp/logs.json); su - admin -c \"nixorium logs show $id --json\" > /tmp/log-detail.json; jq -e '.operation == \"logs-show\" and .state == \"available\" and (.log.id | startswith(\"deploy-\")) and .log.available and (.content | contains(\"Result: completed\"))' /tmp/log-detail.json")
    controller.succeed("su - admin -c 'nixorium logs show ../../etc/passwd --json' >/tmp/log-invalid.json || test $? = 1; jq -e '.state == \"blocked\" and any(.issues[]; .field == \"log\" and (.message | contains(\"ID is invalid\")))' /tmp/log-invalid.json")
    controller.succeed("su - admin -c 'printf unsafe > ~/.local/state/nixorium/operations/deploy-20990101T000000.000000000Z-1.log; chmod 0644 ~/.local/state/nixorium/operations/deploy-20990101T000000.000000000Z-1.log; nixorium logs --json' >/tmp/logs-unsafe.json || test $? = 1; jq -e '.state == \"partial\" and .logs[0].state == \"unavailable\" and (.issues[0].message | contains(\"0600\"))' /tmp/logs-unsafe.json; unlink /home/admin/.local/state/nixorium/operations/deploy-20990101T000000.000000000Z-1.log")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.operation == \"setup-status\" and .state == \"ready\" and (.currentStage | not) and (.stages | length) == 9 and (.stages[] | select(.id == \"prepare-artifacts\").state) == \"complete\"'")
    controller.succeed("nixorium doctor --repo /tmp/deployment --json | jq -e '.state == \"warnings\" and any(.findings[]; .id == \"PXE-PREPARATION\" and .level == \"OK\") and any(.findings[]; .id == \"PXE-LIFECYCLE\" and .level == \"OK\") and any(.findings[]; .id == \"SERVICE-HARMONIA\" and .level == \"OK\") and any(.findings[]; .id == \"CACHE-HEALTH\" and .level == \"OK\") and any(.findings[]; .id == \"COMMAND-COLMENA\" and .level == \"OK\") and any(.findings[]; .id == \"NETWORK-INTERFACE\" and .level == \"OK\") and any(.findings[]; .id == \"CLIENT-SSH\" and .level == \"OK\") and any(.findings[]; .id == \"DISK-FREE\" and .level == \"WARNING\")'")
    controller.succeed("systemctl start nixorium-pxe-network.service; test -e /var/lib/nixorium/pxe/session.json")
    controller.succeed("sync")
    controller.crash()
    controller.wait_for_unit("multi-user.target")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; test ! -e /var/lib/nixorium/pxe/session.json; jq -e '.state == \"recovered\" and .stopReason == \"boot-or-explicit-recovery\"' /var/lib/nixorium/pxe/last-session.json")
    with subtest("controller-only activation without laboratory keys"):
      controller.succeed("install -d -m 0700 /tmp/controller-only-secrets; mv /home/admin/nixorium-deployment/admin-ssh /tmp/controller-only-secrets/repository-ssh")
      controller.fail("systemctl start nixorium-apply-controller.service")
      controller.succeed("journalctl -u nixorium-apply-controller.service --no-pager | grep -F 'deployment key correspondence verification failed'; systemctl reset-failed nixorium-apply-controller.service")
      controller.succeed("mv /home/admin/nixorium-deployment/secret-key /tmp/controller-only-secrets/repository-cache; mv /home/admin/nixorium-deployment/veyon-private-key.pem /tmp/controller-only-secrets/repository-veyon; mv /home/admin/.ssh/id_ed25519 /tmp/controller-only-secrets/installed-ssh; mv /var/lib/nixorium/keys/harmonia-secret-key /tmp/controller-only-secrets/installed-cache; mv /etc/veyon/keys/private/teacher/key /tmp/controller-only-secrets/installed-veyon")
      controller.succeed("cp /home/admin/nixorium-deployment/flake.nix /home/admin/nixorium-deployment/laboratory-flake.nix; cp /etc/nixorium-test/controller-only.nix /home/admin/nixorium-deployment/flake.nix; jq '.lab.deploymentMode = \"controller\" | .lab.pcCount = 0 | .lab.masterDhcpIp = \"MASTER_DHCP_IP\"' /home/admin/nixorium-deployment/lab-settings.json > /tmp/controller-only-settings.json; cp /tmp/controller-only-settings.json /home/admin/nixorium-deployment/lab-settings.json; chown admin:users /home/admin/nixorium-deployment/laboratory-flake.nix /home/admin/nixorium-deployment/flake.nix /home/admin/nixorium-deployment/lab-settings.json")
      controller.succeed("su - admin -c 'cd ~/nixorium-deployment; cp /etc/nixorium-test/lab-software.json lab-software.json; git add flake.nix laboratory-flake.nix lab-settings.json lab-software.json; git -c user.name=Test -c user.email=test@example.invalid commit -qm controller-only; nixorium config validate --json'")
      controller.succeed("su - admin -c 'nixorium controller plan --repo ~/nixorium-deployment --json' | jq -e '.state == \"ready\" and (.issues | length) == 0'")
      controller.succeed("su - admin -c 'revision=$(git -C ~/nixorium-deployment rev-parse HEAD); nixorium controller apply --repo ~/nixorium-deployment --expect \"$revision\" --yes --json' | jq -e '.state == \"completed\" and .verified'")
      controller.succeed("su - admin -c 'nix --extra-experimental-features \"nix-command flakes\" eval ~/nixorium-deployment#deploymentStatus --json --no-write-lock-file' | jq -e '(.ready | not) and .controller.ready and (.controller.requiresKeys | not)'")
      controller.fail("su - admin -c 'nixorium pxe prepare --repo ~/nixorium-deployment --yes --json'")
      controller.fail("su - admin -c 'nixorium deploy plan --repo ~/nixorium-deployment --on @lab --json'")
      controller.succeed("test ! -e /var/lib/nixorium/pxe/session.json; ! systemctl is-active --quiet nixorium-pxe.service")
    with subtest("shared software on a controller without configured clients"):
      controller.succeed("su - admin -c 'nixorium software plan --repo ~/nixorium-deployment --package vlc --scope shared --json' > /tmp/shared-rejected.json || test $? = 1; jq -e '.state == \"invalid\" and any(.issues[]; .message | contains(\"controller-specific package policy\"))' /tmp/shared-rejected.json")
      controller.succeed("su - admin -c 'nixorium software plan --repo ~/nixorium-deployment --package hello --scope shared --json' > /tmp/shared-plan.json; jq -e '.state == \"ready\" and .affectedController == \"pc99\" and (.affectedClients | length) == 0' /tmp/shared-plan.json")
      controller.succeed("token=$(jq -r .reviewToken /tmp/shared-plan.json); su - admin -c \"nixorium software apply --repo ~/nixorium-deployment --package hello --scope shared --expect $token --yes --json\" | jq -e '.state == \"applied\" and .affectedController == \"pc99\"'; jq -e 'any(.packages[]; .package == \"hello\" and .scope.kind == \"shared\")' /home/admin/nixorium-deployment/lab-software.json")
      controller.succeed("su - admin -c 'nixorium software plan --repo ~/nixorium-deployment --package hello --scope shared --remove --json' > /tmp/shared-remove.json; jq -e '.state == \"ready\" and .affectedController == \"pc99\" and .candidate.packages == []' /tmp/shared-remove.json")
      controller.succeed("token=$(jq -r .reviewToken /tmp/shared-remove.json); su - admin -c \"nixorium software apply --repo ~/nixorium-deployment --package hello --scope shared --remove --expect $token --yes --json\" | jq -e '.state == \"applied\"'; jq -e '.packages == []' /home/admin/nixorium-deployment/lab-software.json")
  '';
}
