# The guided change of laboratory network end to end, with the real
# management CLI, controller apply service, SSH trust and client networking.
#
# The controller runs the management module; its deployment Flake is a small
# fixture whose labMeta follows lab-settings.json and whose controller system
# sets the controller's laboratory addresses from the saved settings when it
# is activated. The deployment engine runs with a placeholder build and copy;
# its real SSH commands reach the client at the address it chose (a client on
# the previous network is reached there), where the activation command
# switches to the matching client configuration: the mkLab networking and firewall modules for 10.0.0.0/24,
# for the change to 10.1.0.0/24, and for 10.1.0.0/24 after it.
#
# Scenario 1: start, apply to the controller, update the client over its
# previous address, status shows it moved, undo, update it back, apply to
# the controller. Scenario 2: start again, then finish.
{ nixoriumPackage, baseLab, transitionLab, finishedLab }:
{ pkgs, lib, ... }:
let
  keys = import "${pkgs.path}/nixos/tests/ssh-keys.nix" pkgs;
  hostArgs = lab: {
    inherit (lab.nixosConfigurations.pc01._module.specialArgs) hostName hostIp isController labSettings;
  };
  forced = args: lib.mapAttrs (_: lib.mkForce) args;
  clientModules = [
    ../modules/networking.nix
    ../modules/firewall.nix
  ];
  # The controller system: activation sets the laboratory addresses the saved
  # settings call for (both during a change), like the real configuration.
  fakeControllerSystem = pkgs.runCommand "nixorium-migration-controller-system" { } ''
    mkdir -p "$out/bin"
    ln -s ${fakeRuntime} "$out/sw"
    cat > "$out/bin/switch-to-configuration" <<'SCRIPT'
    #!${pkgs.runtimeShell}
    set -eu
    [ "''${1:-}" = switch ] || exit 2
    SYSTEM_PATH="$(${pkgs.coreutils}/bin/dirname "$(${pkgs.coreutils}/bin/dirname "$(${pkgs.coreutils}/bin/readlink -f "$0")")")"
    [ "$(${pkgs.coreutils}/bin/readlink -f /nix/var/nix/profiles/system)" = "$SYSTEM_PATH" ] || exit 5
    SETTINGS=/home/admin/nixorium-deployment/lab-settings.json
    BASE="$(${pkgs.jq}/bin/jq -r .lab.networkBase "$SETTINGS")"
    PREVIOUS="$(${pkgs.jq}/bin/jq -r '.lab.networkTransition.previousBase // ""' "$SETTINGS")"
    WANTED=" ''${BASE%.0}.99/24 "
    if [ -n "$PREVIOUS" ]; then
      WANTED="$WANTED''${PREVIOUS%.0}.99/24 "
    fi
    for CIDR in 10.0.0.99/24 10.1.0.99/24; do
      case "$WANTED" in
        *" $CIDR "*) ${pkgs.iproute2}/bin/ip address replace "$CIDR" dev eth1 ;;
        *) ${pkgs.iproute2}/bin/ip address del "$CIDR" dev eth1 2>/dev/null || true ;;
      esac
    done
    ${pkgs.coreutils}/bin/ln -sfn "$SYSTEM_PATH" /run/current-system
    SCRIPT
    chmod +x "$out/bin/switch-to-configuration"
  '';
  fakeRuntime = pkgs.buildEnv {
    name = "nixorium-migration-system-path";
    paths = [
      pkgs.bash
      pkgs.coreutils
      pkgs.diffutils
      pkgs.findutils
      pkgs.gawk
      pkgs.git
      pkgs.gnugrep
      pkgs.gnused
      pkgs.iproute2
      pkgs.jq
      pkgs.nix
      pkgs.openssh
      pkgs.shadow
      pkgs.systemd
      pkgs.util-linux
      nixoriumPackage
    ];
  };
  # Deployment engine stand-ins. The build returns a placeholder system whose
  # name says which client configuration matches the reviewed settings, and
  # the copy only records its target. The engine's real SSH commands reach
  # the client at the address it chose; there a forced command activates that
  # configuration in place of the transient unit and writes the activation
  # result, while every other command runs as sent.
  fakeEngineNix = pkgs.writeShellScriptBin "nix" ''
    set -eu
    case " $* " in
      *" build "*"#nixosConfigurations."*)
        DEPLOYMENT=/home/admin/nixorium-deployment
        BASE="$(${pkgs.jq}/bin/jq -r .lab.networkBase "$DEPLOYMENT/lab-settings.json")"
        PREVIOUS="$(${pkgs.jq}/bin/jq -r '.lab.networkTransition.previousBase // ""' "$DEPLOYMENT/lab-settings.json")"
        VARIANT=base
        if [ -n "$PREVIOUS" ]; then
          VARIANT=transition
        elif [ "$BASE" = 10.1.0.0 ]; then
          VARIANT=finished
        fi
        REVISION="$(${pkgs.git}/bin/git -C "$DEPLOYMENT" rev-parse HEAD)"
        printf '[{"drvPath":"/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-x.drv","outputs":{"out":"/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-nixos-system-pc01-%s-%s"}}]\n' "$VARIANT" "$REVISION"
        ;;
      *) exec /run/current-system/sw/bin/nix "$@" ;;
    esac
  '';
  fakeEngineCopy = pkgs.writeShellScriptBin "nix-copy-closure" ''
    set -eu
    printf '%s\n' "$*" >> /tmp/engine-copy.log
  '';
  clientEngineActivation = pkgs.writeShellScript "nixorium-test-engine-activation" ''
    STATUS=0
    /run/current-system/sw/bin/nixorium-test-activate "$1" "$2" || STATUS=$?
    ${pkgs.coreutils}/bin/mkdir -p /var/lib/nixorium/activation
    printf '{"activationId":"%s","systemPath":"%s","exitCode":%d}\n' "$3" "$4" "$STATUS" > /var/lib/nixorium/activation/last.json.tmp
    ${pkgs.coreutils}/bin/mv -f /var/lib/nixorium/activation/last.json.tmp /var/lib/nixorium/activation/last.json
  '';
  clientEngineRemote = pkgs.writeShellScriptBin "nixorium-test-engine-remote" ''
    set -eu
    remote="''${SSH_ORIGINAL_COMMAND:-}"
    case "$remote" in
      *systemd-run*)
        printf '%s\n' "''${SSH_CONNECTION:-}" >> /tmp/engine-activate.log
        ID="$(printf '%s' "$remote" | ${pkgs.gnused}/bin/sed -n 's/.*NIXORIUM_ACTIVATION=\([0-9a-f]*\).*/\1/p')"
        SYSTEM="$(printf '%s' "$remote" | ${pkgs.gnused}/bin/sed -n 's/.*NIXORIUM_SYSTEM=\([^ ]*\).*/\1/p')"
        NAME="''${SYSTEM##*-nixos-system-pc01-}"
        VARIANT="''${NAME%%-*}"
        REVISION="''${NAME#*-}"
        # Like the transient unit started with --no-block: the activation
        # runs on its own and survives the connection.
        ${pkgs.util-linux}/bin/setsid ${clientEngineActivation} "$VARIANT" "$REVISION" "$ID" "$SYSTEM" </dev/null >/dev/null 2>&1 &
        ;;
      *) exec ${pkgs.bash}/bin/bash -c "$remote" ;;
    esac
  '';
  clientHostState = pkgs.writeShellScriptBin "nixorium-host-state" ''
    set -eu
    ${pkgs.coreutils}/bin/readlink -f /run/current-system
    ${pkgs.coreutils}/bin/cat /var/lib/nixorium-test/revision 2>/dev/null || printf '%040d\n' 0
  '';
  clientActivate = pkgs.writeShellScriptBin "nixorium-test-activate" ''
    set -eu
    case "$1" in
      base) TARGET=/run/booted-system ;;
      transition|finished) TARGET="/run/booted-system/specialisation/$1" ;;
      *) exit 64 ;;
    esac
    [[ "$2" =~ ^[0-9a-f]{40}$ ]] || exit 64
    "$TARGET/bin/switch-to-configuration" test
    ${pkgs.coreutils}/bin/mkdir -p /var/lib/nixorium-test
    printf '%s\n' "$2" > /var/lib/nixorium-test/revision
  '';
  deploymentFlake = ''
    {
      inputs.fakeSystem = {
        url = "path:${fakeControllerSystem}";
        flake = false;
      };
      outputs = { self, fakeSystem }:
        let
          lab = (builtins.fromJSON (builtins.readFile ./lab-settings.json)).lab;
          network = base: builtins.head (builtins.match "([0-9]+[.][0-9]+[.][0-9]+)[.]0" base);
          address = base: number: network base + "." + toString number;
          transition = lab.networkTransition or null;
        in {
          labMeta = {
            schemaVersion = 2;
            version = "vm-test";
            controller = { name = "pc99"; number = 99; staticIp = address lab.networkBase 99; dhcpIp = lab.masterDhcpIp; };
            clients = { count = 1; hosts = [ { name = "pc01"; ip = address lab.networkBase 1; } ]; };
            network = {
              base = lab.networkBase;
              prefixLength = lab.networkPrefixLength;
              ifaceName = "eth1";
              cachePort = 5000;
              pxeHttpPort = 8080;
            } // (if transition == null then { } else {
              transition = {
                base = transition.previousBase;
                prefixLength = transition.previousPrefixLength;
                controllerIp = address transition.previousBase 99;
                startedAt = transition.startedAt;
                clients = [ { name = "pc01"; ip = address transition.previousBase 1; } ];
              };
            });
            users = { student = "student"; teacher = "teacher"; };
          };
          deploymentStatus = {
            ready = true;
            issues = [ ];
            controller = { ready = true; issues = [ ]; requiresKeys = false; };
          };
          nixosConfigurations = {
            pc99.config.system.build.toplevel = fakeSystem.outPath;
            pc01.config.system.build.toplevel = fakeSystem.outPath;
          };
          nixoriumValidateCandidate = candidate: builtins.deepSeq candidate true;
        };
    }
  '';
in
{
  name = "nixorium-network-migration";

  nodes = {
    controller = { lib, pkgs, ... }: {
      imports = [
        ../modules/cache.nix
        ../modules/firewall.nix
        ../modules/management.nix
        ../modules/pxe.nix
        ../modules/users.nix
      ];
      _module.args = {
        hostName = "pc99";
        isController = true;
        labSettings = {
          masterHostName = "pc99";
          masterIp = "10.0.0.99";
          masterDhcpIp = "192.0.2.10";
          networkPrefixLength = 24;
          ifaceName = "eth1";
          cachePort = 5000;
          pxeHttpPort = 8080;
          cachePublicKey = null;
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
      virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
      virtualisation.memorySize = 1536;
      users.groups.nixorium-staff = { };
      system.activationScripts.createHomeTemplates = "";
      users.users.root.hashedPassword = lib.mkForce null;
      environment.systemPackages = [ pkgs.git pkgs.jq pkgs.openssh pkgs.iproute2 ];
      # The controller's address before any reviewed apply.
      systemd.services.nixorium-test-network = {
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        path = [ pkgs.iproute2 ];
        serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
        script = ''
          ip link set eth1 up
          ip address replace 10.0.0.99/24 dev eth1
        '';
      };
      environment.etc."nixorium-test/flake.nix".text = deploymentFlake;
      environment.etc."nixorium-test/lab-settings.json".source = ../templates/site/lab-settings.json;
      environment.etc."nixorium-test/.gitignore".source = ../templates/site/.gitignore;
      environment.etc."nixorium-test/admin-key".source = keys.snakeOilPrivateKey;
    };

    client = { ... }: {
      imports = clientModules;
      _module.args = hostArgs baseLab;
      specialisation.transition.configuration._module.args = forced (hostArgs transitionLab);
      specialisation.finished.configuration._module.args = forced (hostArgs finishedLab);
      virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
      networking.networkmanager.enable = true;
      # A PC with a single wired card: the test driver's own card is not part
      # of the laboratory.
      networking.networkmanager.unmanaged = [ "interface-name:eth0" ];
      services.openssh = { enable = true; settings.PermitRootLogin = "prohibit-password"; };
      users.users.root.openssh.authorizedKeys.keys = [ ''command="${clientEngineRemote}/bin/nixorium-test-engine-remote" ${keys.snakeOilPublicKey}'' ];
      environment.systemPackages = [ clientHostState clientActivate pkgs.iproute2 ];
      system.stateVersion = "26.05";
    };
  };

  testScript = ''
    import json
    import shlex

    def admin(command):
      return "su - admin -c " + shlex.quote(command)

    def cli(arguments):
      return json.loads(controller.succeed(admin("nixorium " + arguments + " --repo ~/nixorium-deployment --json")))

    def cli_fails(arguments):
      return json.loads(controller.succeed(admin("nixorium " + arguments + " --repo ~/nixorium-deployment --json") + " || test $? = 1"))

    def controller_addresses():
      return controller.succeed("ip -4 -o address show dev eth1 | awk '{print $4}' | sort | tr '\\n' ' '").strip()

    def client_addresses():
      return client.succeed("ip -4 -o address show dev eth1 | awk '{print $4}' | sort | tr '\\n' ' '").strip()

    def apply_controller():
      plan = cli("controller plan")
      result = cli("controller apply --expect " + plan["revision"] + " --yes")
      assert result["state"] == "completed" and result["verified"], result

    def update_computers():
      plan = cli("deploy plan --on @lab")
      assert plan["state"] == "ready", plan
      result = json.loads(controller.succeed(admin("PATH=/tmp/fake-engine-bin:$PATH nixorium deploy apply --repo ~/nixorium-deployment --on @lab --expect " + plan["revision"] + " --yes --json")))
      assert result["state"] == "completed", result
      return plan, result

    def migrate(step, extra=""):
      review = cli("network migrate " + step + extra)
      assert review["state"] == "ready" and review["reviewToken"], review
      action = "start" if step == "plan" else step
      saved = cli("network migrate " + action + extra + " --expect " + review["reviewToken"] + " --yes")
      assert saved["state"] == "saved" and saved["revision"], saved
      return review, saved

    start_all()
    controller.wait_for_unit("multi-user.target")
    controller.wait_for_unit("nixorium-test-network.service")
    client.wait_for_unit("multi-user.target")
    client.wait_until_succeeds("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.1/24 '", timeout=90)

    with subtest("a laboratory on 10.0.0.0/24 with one client"):
      controller.succeed("install -d -o admin -g users -m 0700 /home/admin/nixorium-deployment /tmp/fake-engine-bin")
      controller.succeed("ln -s ${fakeEngineNix}/bin/nix ${fakeEngineCopy}/bin/nix-copy-closure /tmp/fake-engine-bin/; chmod 0755 /tmp/fake-engine-bin")
      controller.succeed("install -o admin -g users -m 0600 /etc/nixorium-test/admin-key /home/admin/.ssh/id_ed25519")
      controller.succeed(admin(
        "set -eu; cd ~/nixorium-deployment; cp /etc/nixorium-test/flake.nix flake.nix; cp /etc/nixorium-test/.gitignore .gitignore; "
        + "jq '.lab.masterDhcpIp = \"192.0.2.10\" | .lab.ifaceName = \"eth1\" | .lab.pcCount = 1 | .lab.credentialsVersion = 1' /etc/nixorium-test/lab-settings.json > lab-settings.json; "
        + "printf '{\"schemaVersion\":1,\"packages\":[]}\\n' > lab-software.json; "
        + "jq -n --arg hash '$6$vm$not-the-public-default' '{version:1, admin:$hash, teacher:$hash, student:$hash}' > lab-credentials.json; chmod 0600 lab-credentials.json; "
        + "git init -q; git config user.name Test; git config user.email test@example.invalid; "
        + "git add flake.nix lab-settings.json lab-software.json .gitignore; git commit -qm initial"))
      apply_controller()
      assert controller_addresses() == "10.0.0.99/24", controller_addresses()
      hosts = cli("hosts")
      assert hosts["hosts"][0]["ssh"] == "available" and "network" not in hosts["hosts"][0], hosts
      update_computers()
      controller.succeed("grep -F '10.0.0.1 ' /home/admin/.ssh/nixorium-known-hosts/known_hosts")
      controller.fail("grep -F '10.1.0.1 ' /home/admin/.ssh/nixorium-known-hosts/known_hosts")

    with subtest("the change is reviewed and refused while a computer is off unless acknowledged"):
      client.succeed("systemctl stop sshd.service")
      blocked = cli_fails("network migrate plan --to 10.1.0.0/24")
      assert blocked["state"] == "blocked" and "pc01 did not answer" in blocked["issues"][0]["message"], blocked
      acknowledged = cli("network migrate plan --to 10.1.0.0/24 --acknowledge-unreachable")
      assert acknowledged["state"] == "ready" and acknowledged["acknowledged"] == ["pc01"], acknowledged
      client.succeed("systemctl start sshd.service")
      controller.wait_until_succeeds(admin("nixorium hosts --repo ~/nixorium-deployment --json | jq -e '.hosts[0].ssh == \"available\"'"))

    def start_change():
      review, saved = migrate("plan", " --to 10.1.0.0/24")
      assert review["controllerFrom"] == "10.0.0.99" and review["controllerTo"] == "10.1.0.99", review
      assert review["computers"] == [{"name": "pc01", "from": "10.0.0.1", "to": "10.1.0.1", "reachability": "reachable", "state": "reachable"}], review
      # The key trusted for the previous address is trusted for the new one
      # before anything contacts it there.
      old = controller.succeed("awk '$1 == \"10.0.0.1\" {print $2, $3}' /home/admin/.ssh/nixorium-known-hosts/known_hosts | sort").strip()
      new = controller.succeed("awk '$1 == \"10.1.0.1\" {print $2, $3}' /home/admin/.ssh/nixorium-known-hosts/known_hosts | sort -u").strip()
      assert old != "" and old == new, (old, new)
      controller.succeed("jq -e '.lab.networkBase == \"10.1.0.0\" and .lab.networkTransition.previousBase == \"10.0.0.0\"' /home/admin/nixorium-deployment/lab-settings.json")
      status = cli("network migrate status")
      assert status["state"] == "in-progress" and not status["controllerReady"] and status["next"].startswith("Apply to controller"), status
      recovery = cli("recovery status")
      assert any(c["kind"] == "network-changing" and c["next"]["code"] == "NETWORK-CHANGING" for c in recovery["conditions"]), recovery
      # Apply to controller: it answers on both networks.
      apply_controller()
      assert controller_addresses() == "10.0.0.99/24 10.1.0.99/24", controller_addresses()
      # Update computers: the client still answers only on its previous
      # address, so the update reaches it there and moves it.
      plan, result = update_computers()
      assert plan["availability"][0]["network"] == "previous" and plan["targets"][0]["previousIp"] == "10.0.0.1", plan
      controller.succeed("grep -F 'root@10.0.0.1 ' /tmp/engine-copy.log")
      client.succeed("tail -n 1 /tmp/engine-activate.log | cut -d ' ' -f 3 | grep -Fx 10.0.0.1")
      client.wait_until_succeeds("ip -4 -o address show dev eth1 | grep -F ' 10.1.0.1/24 '", timeout=60)
      assert client_addresses() == "10.0.0.1/24 10.1.0.1/24", client_addresses()
      controller.wait_until_succeeds(admin("nixorium network migrate status --repo ~/nixorium-deployment --json | jq -e '.computers[0].state == \"migrated\" and .computers[0].network == \"current\" and .canFinish'"), timeout=60)
      hosts = cli("hosts")
      assert hosts["hosts"][0]["network"] == "current" and hosts["hosts"][0]["deployment"] == "current", hosts

    with subtest("start, apply to the controller and update the client over its previous address"):
      controller.succeed("rm -f /tmp/engine-copy.log"); client.succeed("rm -f /tmp/engine-activate.log")
      start_change()

    with subtest("undo returns the client and the controller to the previous network"):
      refused = cli_fails("network migrate plan --to 10.2.0.0/24")
      assert refused["state"] == "blocked" and "already in progress" in refused["issues"][0]["message"], refused
      review, saved = migrate("undo")
      assert review["from"] == "10.1.0.0/24" and review["to"] == "10.0.0.0/24", review
      assert cli("network migrate status")["state"] == "none"
      # Update computers over the address the client keeps, then the
      # controller drops the address on the network being left.
      update_computers()
      apply_controller()
      assert controller_addresses() == "10.0.0.99/24", controller_addresses()
      client.succeed("nft list ruleset | grep -F 'ip saddr 10.0.0.99 tcp dport 22 accept'")
      client.fail("nft list ruleset | grep -F 'ip saddr 10.1.0.99'")
      hosts = cli("hosts")
      assert hosts["hosts"][0]["ip"] == "10.0.0.1" and hosts["hosts"][0]["deployment"] == "current", hosts
      # The address no longer configured disappears at the next start.
      client.shutdown()
      client.start()
      client.wait_for_unit("multi-user.target")
      client.wait_until_succeeds("ip -4 -o address show dev eth1 | grep -F ' 10.0.0.1/24 '", timeout=90)
      assert client_addresses() == "10.0.0.1/24", client_addresses()
      controller.wait_until_succeeds(admin("nixorium hosts --repo ~/nixorium-deployment --json | jq -e '.hosts[0].ssh == \"available\"'"), timeout=60)

    with subtest("start again and finish"):
      controller.succeed("rm -f /tmp/engine-copy.log"); client.succeed("rm -f /tmp/engine-activate.log")
      start_change()
      review, saved = migrate("finish")
      assert review["from"] == "10.0.0.0/24" and review["to"] == "10.1.0.0/24", review
      assert any("network installation again" in step for step in saved["next"]), saved
      controller.succeed("jq -e '.lab.networkBase == \"10.1.0.0\" and (.lab | has(\"networkTransition\") | not)' /home/admin/nixorium-deployment/lab-settings.json")
      apply_controller()
      assert controller_addresses() == "10.1.0.99/24", controller_addresses()
      plan, result = update_computers()
      assert "network" not in plan["availability"][0], plan
      client.succeed("nft list ruleset | grep -F 'ip saddr 10.1.0.99 tcp dport 22 accept'")
      client.fail("nft list ruleset | grep -F 'ip saddr 10.0.0.99'")
      status = cli("network migrate status")
      assert status["state"] == "none" and "10.1.0.0/24" in status["message"], status
      assert cli_fails("network migrate finish")["state"] == "blocked"
      hosts = cli("hosts")
      assert hosts["hosts"][0]["ip"] == "10.1.0.1" and hosts["hosts"][0]["deployment"] == "current", hosts
  '';
}
