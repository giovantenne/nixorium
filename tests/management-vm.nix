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
      if [ "''${NIXORIUM_TEST_COLMENA_FAIL:-}" = "''${1:-}" ]; then
        exit 42
      fi
    '';
    fakeHostState = pkgs.writeShellScriptBin "nixorium-host-state" ''
      set -eu
      readlink -f /run/current-system
      git -c safe.directory=/home/admin/nixorium-deployment -C /home/admin/nixorium-deployment rev-parse HEAD
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
          fi
          while [[ $# -gt 0 ]]; do
            if [[ "$1" == --output-lock-file ]]; then
              printf '%s\n' "{\"root\":\"root\",\"nodes\":{\"root\":{\"inputs\":{\"nixorium\":\"nixorium\"}},\"nixorium\":{\"locked\":{\"rev\":\"$revision\"}}}}" > "$2"
              exit 0
            fi
            shift
          done
          exit 2
          ;;
        *"#labMeta "*)
          printf '%s\n' '{"schemaVersion":2,"controller":{"name":"pc99"},"clients":{"count":1,"hosts":[{"name":"pc01","ip":"10.0.0.1"}]}}'
          ;;
        *"#deploymentStatus "*)
          printf '%s\n' '{"ready":true,"issues":[]}'
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
      [[ "$*" == "-c credential.helper= -c core.askPass= ls-remote --refs --tags --exit-code https://github.com/giovantenne/nixorium.git refs/tags/v*" ]]
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
    environment.systemPackages = [ pkgs.curl pkgs.git pkgs.jq pkgs.python3 pkgs.util-linux fakeColmena fakeHostState fakeUpdateNix fakeUpdateGit ];
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
    controller.succeed("nixorium --help | grep -F 'setup keys'; nixorium --help | grep -F 'git review'; nixorium --help | grep -F 'git commit plan'; nixorium --help | grep -F 'update check'")
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
    controller.succeed("jq '.lab.adminPassword = \"$6$review$new-admin\"' /tmp/deployment/lab-settings.json > /tmp/review-settings.json; mv /tmp/review-settings.json /tmp/deployment/lab-settings.json; git -C /tmp/deployment add lab-settings.json; printf '\n# unstaged-review-fixture\n' >> /tmp/deployment/flake.nix; printf 'untracked contents are intentionally not opened\n' > /tmp/deployment/review-note")
    controller.succeed("before_index=$(git -C /tmp/deployment diff --cached | sha256sum); before_worktree=$(git -C /tmp/deployment diff | sha256sum); nixorium git review --repo /tmp/deployment --json > /tmp/git-review.json; after_index=$(git -C /tmp/deployment diff --cached | sha256sum); after_worktree=$(git -C /tmp/deployment diff | sha256sum); test \"$before_index:$before_worktree\" = \"$after_index:$after_worktree\"")
    controller.succeed("jq -e '.operation == \"git-review\" and .state == \"changes\" and .summary.staged == 1 and .summary.unstaged == 1 and .summary.untracked == 1 and .summary.managed == 1 and .summary.unexpected == 2 and (.changes | length) == 3 and any(.changes[]; .path == \"lab-settings.json\" and .staged == \"modified\" and .managed) and any(.changes[]; .path == \"flake.nix\" and .unstaged == \"modified\") and any(.changes[]; .path == \"review-note\" and .untracked) and (.diffs | length) == 2 and all(.diffs[]; (.content | contains(\"$6$review$new-admin\") | not)) and any(.diffs[]; .content | contains(\"<redacted>\"))' /tmp/git-review.json")
    controller.succeed("git -C /tmp/deployment restore --staged --worktree lab-settings.json; git -C /tmp/deployment restore flake.nix; rm /tmp/deployment/review-note; git -C /tmp/deployment add -f secret-key; nixorium git review --repo /tmp/deployment --json > /tmp/git-private.json || test $? = 1; jq -e '.state == \"blocked\" and .summary.private == 1 and (.diffs | length) == 0 and any(.issues[]; .field == \"private-paths\")' /tmp/git-private.json; git -C /tmp/deployment rm --cached -q secret-key")
    controller.succeed("test -z \"$(git -C /tmp/deployment status --porcelain=v1 --untracked-files=normal)\"")
    controller.succeed("cp -a /tmp/deployment /tmp/commit-deployment; git -C /tmp/commit-deployment config user.name Test; git -C /tmp/commit-deployment config user.email test@example.invalid")
    controller.succeed("printf '\n# unrelated-staged-change\n' >> /tmp/commit-deployment/flake.nix; git -C /tmp/commit-deployment add flake.nix; jq '.lab.adminPassword = \"$6$commit$new-admin\"' /tmp/commit-deployment/lab-settings.json > /tmp/commit-settings.json; mv /tmp/commit-settings.json /tmp/commit-deployment/lab-settings.json; printf 'reviewed local note\n' > /tmp/commit-deployment/review-note")
    controller.succeed("old_head=$(git -C /tmp/commit-deployment rev-parse HEAD); nixorium git commit plan --repo /tmp/commit-deployment --paths lab-settings.json,review-note --json > /tmp/git-commit-plan.json; jq -e '.operation == \"git-commit-plan\" and .state == \"ready\" and (.reviewToken | startswith(\"sha256:\")) and (.confirmation | startswith(\"COMMIT \")) and (.diff.content | contains(\"<redacted>\")) and (.diff.content | contains(\"$6$commit$new-admin\") | not)' /tmp/git-commit-plan.json; token=$(jq -r .reviewToken /tmp/git-commit-plan.json); nixorium git commit apply --repo /tmp/commit-deployment --paths lab-settings.json,review-note --expect \"$token\" --yes --json > /tmp/git-commit.json; jq -e --arg old \"$old_head\" '.operation == \"git-commit\" and .state == \"completed\" and .committed and (.retrySafe | not) and .previousRevision == $old and .revision != $old and (.message | contains(\"no remote push\"))' /tmp/git-commit.json")
    controller.succeed("git -C /tmp/commit-deployment show HEAD:review-note | grep -Fx 'reviewed local note'; git -C /tmp/commit-deployment show HEAD:lab-settings.json | grep -F '$6$commit$new-admin'; ! git -C /tmp/commit-deployment show HEAD:flake.nix | grep -qF 'unrelated-staged-change'; test \"$(git -C /tmp/commit-deployment diff --cached --name-only)\" = flake.nix; test -z \"$(git -C /tmp/commit-deployment status --porcelain=v1 -- lab-settings.json review-note)\"")
    controller.succeed("printf 'first\n' > /tmp/commit-deployment/stale-note; nixorium git commit plan --repo /tmp/commit-deployment --paths stale-note --json > /tmp/git-stale-plan.json; token=$(jq -r .reviewToken /tmp/git-stale-plan.json); printf 'second\n' > /tmp/commit-deployment/stale-note; before=$(git -C /tmp/commit-deployment rev-parse HEAD); nixorium git commit apply --repo /tmp/commit-deployment --paths stale-note --expect \"$token\" --yes --json > /tmp/git-stale.json || test $? = 1; test \"$before\" = \"$(git -C /tmp/commit-deployment rev-parse HEAD)\"; jq -e '.state == \"blocked\" and (.committed | not) and any(.issues[]; .field == \"review\")' /tmp/git-stale.json")
    controller.succeed("printf '{ token = \"plaintext-secret\"; }\n' > /tmp/commit-deployment/unsafe.nix; nixorium git commit plan --repo /tmp/commit-deployment --paths unsafe.nix --json > /tmp/git-unsafe-plan.json || test $? = 1; jq -e '.state == \"blocked\" and any(.issues[]; .field == \"proposal\" and (.message | contains(\"plaintext\")))' /tmp/git-unsafe-plan.json")
    controller.succeed("cp -a /tmp/deployment /tmp/update-deployment; printf '{\n  inputs.nixorium.url = \"github:giovantenne/nixorium/v2.0.0\";\n  outputs = { self, nixorium }: {};\n}\n' > /tmp/update-deployment/flake.nix; printf '%s\n' '{\"root\":\"root\",\"nodes\":{\"root\":{\"inputs\":{\"nixorium\":\"nixorium\"}},\"nixorium\":{\"locked\":{\"rev\":\"1111111111111111111111111111111111111111\"}}}}' > /tmp/update-deployment/flake.lock; chown -R admin:users /tmp/update-deployment; su - admin -c 'git -C /tmp/update-deployment add flake.nix flake.lock && git -C /tmp/update-deployment -c user.name=Test -c user.email=test@example.invalid commit -qm update-fixture'; mkdir /tmp/fake-update-bin /tmp/fake-update-check-bin; ln -s /run/current-system/sw/bin/nixorium-test-update-nix /tmp/fake-update-bin/nix; ln -s /run/current-system/sw/bin/nixorium-test-update-git /tmp/fake-update-check-bin/git")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_UPDATE_GIT_LOG=/tmp/update-git.log PATH=/tmp/fake-update-check-bin:$PATH nixorium update check --repo /tmp/update-deployment --json' > /tmp/update-check.json; jq -e '.operation == \"update-check\" and .state == \"available\" and .upstream == \"github:giovantenne/nixorium\" and .currentRef == \"v2.0.0\" and .currentChannel == \"stable\" and (.stable | length) == 1 and .stable[0].tag == \"v2.3.0\" and (.prerelease | length) == 1 and .prerelease[0].tag == \"v2.4.0-beta.1\" and (.truncated | not)' /tmp/update-check.json; grep -F 'credential.helper=' /tmp/update-git.log; grep -F 'prompt=0 askpass= sshaskpass= interactive=Never global=/dev/null nosystem=1' /tmp/update-git.log")
    controller.succeed("rm -f /tmp/update-nix.log; su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.1.0-beta.1 --json' > /tmp/update-prerelease.json || test $? = 1; jq -e '.operation == \"update-plan\" and .state == \"blocked\" and any(.issues[]; .field == \"target\" and (.message | contains(\"--allow-prerelease\")))' /tmp/update-prerelease.json; test ! -e /tmp/update-nix.log")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.1.0 --json' > /tmp/update-plan.json; jq -e '.operation == \"update-plan\" and .state == \"ready\" and .currentRef == \"v2.0.0\" and .target == \"v2.1.0\" and .targetChannel == \"stable\" and (.reviewToken | startswith(\"sha256:\")) and .confirmation == \"UPDATE NIXORIUM TO v2.1.0\" and (.checks | length) == 7 and all(.checks[]; .state == \"passed\") and (.diff.content | contains(\"flake.nix\")) and (.diff.content | contains(\"flake.lock\"))' /tmp/update-plan.json; test \"$(wc -l < /tmp/update-nix.log)\" = 8; test \"$(grep -c ' build ' /tmp/update-nix.log)\" = 5")
    controller.succeed("before=$(sha256sum /tmp/update-deployment/flake.nix /tmp/update-deployment/flake.lock); token=$(jq -r .reviewToken /tmp/update-plan.json); su - admin -c \"NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:\$PATH nixorium update apply --repo /tmp/update-deployment --target v2.1.0 --expect '$token' </dev/null >/tmp/update-noninteractive.out 2>/tmp/update-noninteractive.err\" || test $? = 2; after=$(sha256sum /tmp/update-deployment/flake.nix /tmp/update-deployment/flake.lock); test \"$before\" = \"$after\"; test ! -s /tmp/update-noninteractive.out; grep -F 'requires an interactive terminal or explicit --yes' /tmp/update-noninteractive.err")
    controller.succeed("before=$(sha256sum /tmp/update-deployment/flake.nix /tmp/update-deployment/flake.lock); su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update apply --repo /tmp/update-deployment --target v2.1.0 --expect sha256:stale --yes --json' > /tmp/update-stale.json || test $? = 1; after=$(sha256sum /tmp/update-deployment/flake.nix /tmp/update-deployment/flake.lock); test \"$before\" = \"$after\"; jq -e '.operation == \"update-apply\" and .state == \"blocked\" and (.updated | not) and .retrySafe and any(.issues[]; .field == \"review\")' /tmp/update-stale.json")
    controller.succeed("token=$(jq -r .reviewToken /tmp/update-plan.json); su - admin -c \"NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:\$PATH nixorium update apply --repo /tmp/update-deployment --target v2.1.0 --expect '$token' --yes --json\" > /tmp/update-apply.json; jq -e '.operation == \"update-apply\" and .state == \"completed\" and .target == \"v2.1.0\" and .updated and (.retrySafe | not) and (.message | contains(\"review and commit\"))' /tmp/update-apply.json; grep -F 'github:giovantenne/nixorium/v2.1.0' /tmp/update-deployment/flake.nix; grep -F '2222222222222222222222222222222222222222' /tmp/update-deployment/flake.lock; test \"$(git -c safe.directory=/tmp/update-deployment -C /tmp/update-deployment status --porcelain=v1 | wc -l)\" = 2; git -c safe.directory=/tmp/update-deployment -C /tmp/update-deployment status --porcelain=v1 | grep -F ' M flake.nix'; git -c safe.directory=/tmp/update-deployment -C /tmp/update-deployment status --porcelain=v1 | grep -F ' M flake.lock'; test ! -e /tmp/update-deployment/result")
    controller.succeed("su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update apply --repo /tmp/update-deployment --target v2.1.0 --expect sha256:stale --yes --json' > /tmp/update-dirty-retry.json || test $? = 1; jq -e '.operation == \"update-plan\" and .state == \"blocked\" and any(.issues[]; .field == \"git\")' /tmp/update-dirty-retry.json")
    controller.succeed("su - admin -c 'git -C /tmp/update-deployment add flake.nix flake.lock && git -C /tmp/update-deployment -c user.name=Test -c user.email=test@example.invalid commit -qm updated'; rm -f /tmp/update-nix.log; su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.1.0 --json' > /tmp/update-current.json || test $? = 1; jq -e '.state == \"blocked\" and any(.issues[]; .field == \"target\" and (.message | contains(\"already configured\")))' /tmp/update-current.json; test ! -e /tmp/update-nix.log; su - admin -c 'NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-nix.log PATH=/tmp/fake-update-bin:$PATH nixorium update plan --repo /tmp/update-deployment --target v2.2.0 --json' | jq -e '.state == \"ready\" and .target == \"v2.2.0\"'")
    controller.succeed("su - admin -c \"(sleep 8; printf u; sleep 1; printf v2.2.0; printf '\\r'; sleep 5; printf 'UPDATE NIXORIUM TO v2.2.0\\r'; sleep 5; printf '\\003') | NIXORIUM_TEST_UPDATE_NIX_LOG=/tmp/update-tui-nix.log PATH=/tmp/fake-update-bin:\\$PATH TERM=xterm timeout 35s script -qefc 'stty rows 40 cols 120; nixorium --repo /tmp/update-deployment' /tmp/nixorium-update-tui.log\"")
    controller.succeed("grep -aF 'Update Nixorium' /tmp/nixorium-update-tui.log; grep -aF 'Validated release review' /tmp/nixorium-update-tui.log; grep -aF 'Candidate checks:' /tmp/nixorium-update-tui.log; grep -aF 'No commit, push, activation, PXE action, or client deployment is implicit' /tmp/nixorium-update-tui.log; grep -aF 'Last result: completed; files updated=true; retry safe=false' /tmp/nixorium-update-tui.log; grep -F 'github:giovantenne/nixorium/v2.2.0' /tmp/update-deployment/flake.nix; grep -F '3333333333333333333333333333333333333333' /tmp/update-deployment/flake.lock; test \"$(git -c safe.directory=/tmp/update-deployment -C /tmp/update-deployment status --porcelain=v1 | wc -l)\" = 2")
    controller.succeed("cp -a /tmp/deployment /tmp/settings-deployment; chown -R admin:users /tmp/settings-deployment; su - admin -c \"(sleep 8; printf e; sleep 1; printf j; sleep 0.2; printf j; sleep 0.2; printf j; sleep 0.2; printf j; sleep 0.2; printf j; sleep 0.2; printf '\\r'; sleep 1; printf 'TUI Student'; printf '\\r'; sleep 0.2; printf '\\r'; sleep 0.2; printf '\\r'; sleep 0.2; printf '\\r'; sleep 8; printf y; sleep 8; printf q) | TERM=xterm timeout 40s script -qefc 'stty rows 40 cols 120; nixorium --repo /tmp/settings-deployment' /tmp/nixorium-settings-tui.log\"")
    controller.succeed("grep -aF 'Nixorium — Settings' /tmp/nixorium-settings-tui.log; grep -aF 'Nixorium — Edit Git' /tmp/nixorium-settings-tui.log; grep -aF 'Nixorium — Settings review' /tmp/nixorium-settings-tui.log; grep -aF 'lab.studentGitName: student' /tmp/nixorium-settings-tui.log; grep -aF 'Last apply: applied; 1 field(s)' /tmp/nixorium-settings-tui.log; ! grep -aF '$6$' /tmp/nixorium-settings-tui.log; test \"$(jq -r .lab.studentGitName /tmp/settings-deployment/lab-settings.json)\" = 'TUI Student'; test \"$(git -c safe.directory=/tmp/settings-deployment -C /tmp/settings-deployment status --porcelain=v1)\" = ' M lab-settings.json'")
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
    controller.succeed("su - admin -c \"(sleep 8; printf p; sleep 1; printf p; sleep 3; printf s; sleep 5; printf S; sleep 0.2; printf T; sleep 0.2; printf A; sleep 0.2; printf R; sleep 0.2; printf T; sleep 0.2; printf ' '; sleep 0.2; printf P; sleep 0.2; printf X; sleep 0.2; printf E; sleep 0.2; printf '\\r'; sleep 5; printf q) | TERM=xterm timeout 40s script -qefc 'stty rows 40 cols 120; nixorium --repo ~/nixorium-deployment' /tmp/nixorium-pxe-tui.log\"")
    controller.succeed("grep -aF 'Install computers over network' /tmp/nixorium-pxe-tui.log")
    controller.succeed("grep -aF 'Recent activity:' /tmp/nixorium-pxe-tui.log; grep -aF 'Prepared PXE artifacts for 1 clients' /tmp/nixorium-pxe-tui.log")
    controller.succeed("grep -aF 'Start review' /tmp/nixorium-pxe-tui.log")
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
    controller.succeed("readlink -f /run/current-system > /tmp/controller-system-before; touch /run/nixorium-test-activation-fail; su - admin -c 'nixorium setup apply --repo ~/nixorium-deployment --yes --json > /tmp/partial-apply.json' || test $? = 1")
    controller.succeed("jq -e '.operation == \"setup-apply-controller\" and .state == \"failed\"' /tmp/partial-apply.json; test \"$(readlink -f /run/current-system)\" != \"$(cat /tmp/controller-system-before)\"; test ! -e /var/lib/nixorium/controller/applied.json")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.currentStage == \"apply-controller\" and (.stages[] | select(.id == \"apply-controller\").detail | contains(\"no successful controller activation\"))'")
    controller.succeed("rm /run/nixorium-test-activation-fail; systemctl reset-failed nixorium-apply-controller.service")
    controller.succeed("su - admin -c 'nixorium setup apply --repo ~/nixorium-deployment --yes --json' | jq -e '.operation == \"setup-apply-controller\" and .state == \"completed\"'")
    controller.succeed("test -e /run/nixorium-controller-applied")
    controller.succeed("revision=$(git -c safe.directory=/home/admin/nixorium-deployment -C /home/admin/nixorium-deployment rev-parse HEAD); system_path=$(readlink -f /run/current-system); jq -e --arg revision \"$revision\" --arg systemPath \"$system_path\" '.schemaVersion == 1 and .revision == $revision and .systemPath == $systemPath and (.activatedAt | endswith(\"Z\"))' /var/lib/nixorium/controller/applied.json; test \"$(stat -c '%U:%G:%a' /var/lib/nixorium/controller/applied.json)\" = root:root:644")
    controller.succeed("test -e /home/teacher/.config/nixorium-activation-test; test -e /run/user/1000/nixorium-activation-test; test ! -e /home/admin/nixorium-deployment/activation-must-not-write")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.state == \"ready\" and (.currentStage | not) and (.stages[] | select(.id == \"prepare-artifacts\").state) == \"complete\" and (.stages[] | select(.id == \"offer-client-installation\").state) == \"complete\"'")
    controller.succeed("touch /run/nixorium-test-activation-fail; ! systemctl start nixorium-apply-controller.service; test ! -e /var/lib/nixorium/controller/applied.json; rm /run/nixorium-test-activation-fail; systemctl reset-failed nixorium-apply-controller.service")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.currentStage == \"apply-controller\"'")
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
    controller.succeed("su - admin -c \"(sleep 8; printf c; sleep 5; printf 'REBUILD pc99\\r'; sleep 15; printf q) | TERM=xterm timeout 40s script -qefc 'stty rows 40 cols 120; nixorium --repo /home/admin/nixorium-deployment' /tmp/nixorium-controller-tui.log\"")
    controller.succeed("grep -aF 'Rebuild controller' /tmp/nixorium-controller-tui.log; grep -aF 'Controller rebuild review' /tmp/nixorium-controller-tui.log; grep -aF 'Last result: completed at phase complete' /tmp/nixorium-controller-tui.log; grep -aF 'Applied: true   Verified: true' /tmp/nixorium-controller-tui.log")
    controller.succeed("nixorium status --repo /tmp/deployment --json | jq -e '.operation == \"status\" and .state == \"ready\" and .lab.clients.hosts[0].name == \"pc01\" and .pxe.mode == \"ready\" and .pxePreparation.ready and all(.artifacts[]; .present) and any(.services[]; .name == \"nixorium-harmonia.service\" and .loaded and .active)'")
    controller.succeed("su - admin -c 'nixorium services --repo /home/admin/nixorium-deployment --json' | jq -e '.operation == \"services\" and .state == \"healthy\" and (.services | length) == 2 and .services[0].id == \"cache\" and .services[0].healthy and .services[0].state == \"healthy\" and .services[0].units[0].name == \"nixorium-harmonia.service\" and .services[1].id == \"pxe\" and .services[1].healthy and .services[1].state == \"ready\" and (.services[1].actions | length) == 0'")
    controller.fail("su - admin -c 'systemctl restart harmonia.service'")
    controller.succeed("su - admin -c 'nixorium services restart cache --repo /home/admin/nixorium-deployment --yes --json' > /tmp/service-restart.json || { cat /tmp/service-restart.json; false; }; jq -e '.operation == \"service-restart\" and .state == \"completed\" and .service == \"cache\" and .unit == \"nixorium-restart-cache.service\" and .verified and .current.active' /tmp/service-restart.json || { cat /tmp/service-restart.json; false; }")
    controller.succeed("su - admin -c \"(sleep 8; printf s; sleep 5; printf r; sleep 1; printf 'RESTART CACHE\\r'; sleep 12; printf q) | TERM=xterm timeout 35s script -qefc 'stty rows 40 cols 120; nixorium --repo /home/admin/nixorium-deployment' /tmp/nixorium-services-tui.log\"")
    controller.succeed("grep -aF 'Managed services' /tmp/nixorium-services-tui.log; grep -aF 'Binary cache restart review' /tmp/nixorium-services-tui.log; grep -aF 'Last action: completed; verified=true' /tmp/nixorium-services-tui.log; grep -aF 'binary cache restarted and verified healthy' /tmp/nixorium-services-tui.log")
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
    controller.succeed("su - admin -c \"(sleep 8; printf d; sleep 1; printf a; sleep 0.2; printf '\\r'; sleep 3; printf 'DEPLOY @lab\\r'; sleep 12; printf q; sleep 2; printf q) | NIXORIUM_TEST_COLMENA_INVOCATIONS=/tmp/colmena-tui PATH=/tmp/fake-colmena-bin:\$PATH TERM=xterm timeout 45s script -qefc 'stty rows 40 cols 120; nixorium --repo /home/admin/nixorium-deployment' /tmp/nixorium-deploy-tui.log\"")
    controller.succeed("grep -aF 'Deploy updates' /tmp/nixorium-deploy-tui.log; grep -aF 'Deployment review' /tmp/nixorium-deploy-tui.log; grep -aF 'Last result: completed at phase complete' /tmp/nixorium-deploy-tui.log; grep -aF 'Authenticated: 1/1' /tmp/nixorium-deploy-tui.log")
    controller.succeed("test \"$(head -n 1 /tmp/colmena-tui)\" = 'build --on @lab --verbose --color never'; test \"$(tail -n 1 /tmp/colmena-tui)\" = 'apply switch --on @lab --verbose --color never'; test \"$(wc -l </tmp/colmena-tui)\" = 2")
    controller.succeed("su - admin -c 'cd /tmp; nixorium logs --json' > /tmp/logs.json; jq -e '.operation == \"logs-list\" and .state == \"available\" and .limit == 50 and (.logs | length) >= 4 and all(.logs[]; .kind == \"deployment\" and .available and (.id | startswith(\"deploy-\"))) and any(.records[]; .operation == \"deploy-apply\" and .state == \"completed\" and .subject == \"@lab\") and any(.records[]; .operation == \"controller-apply\" and .state == \"completed\") and any(.records[]; .operation == \"service-restart\" and .state == \"completed\") and any(.records[]; .operation == \"pxe-start\" and .state == \"completed\")' /tmp/logs.json")
    controller.succeed("test \"$(stat -c '%U:%G:%a' /home/admin/.local/state/nixorium/operations/records.json)\" = admin:users:600; test \"$(stat -c '%a' /home/admin/.local/state/nixorium/operations)\" = 700")
    controller.succeed("chmod 0644 /home/admin/.local/state/nixorium/operations/records.json; su - admin -c 'nixorium logs --json' >/tmp/records-unsafe.json || test $? = 1; jq -e '.state == \"partial\" and any(.issues[]; .field == \"records\" and (.message | contains(\"0600\"))) and (.logs | length) >= 4' /tmp/records-unsafe.json; chmod 0600 /home/admin/.local/state/nixorium/operations/records.json")
    controller.succeed("id=$(jq -r '.logs[0].id' /tmp/logs.json); su - admin -c \"nixorium logs show $id --json\" > /tmp/log-detail.json; jq -e '.operation == \"logs-show\" and .state == \"available\" and (.log.id | startswith(\"deploy-\")) and .log.available and (.content | contains(\"Result: completed\"))' /tmp/log-detail.json")
    controller.succeed("su - admin -c 'nixorium logs show ../../etc/passwd --json' >/tmp/log-invalid.json || test $? = 1; jq -e '.state == \"blocked\" and any(.issues[]; .field == \"log\" and (.message | contains(\"ID is invalid\")))' /tmp/log-invalid.json")
    controller.succeed("su - admin -c 'printf unsafe > ~/.local/state/nixorium/operations/deploy-20990101T000000.000000000Z-1.log; chmod 0644 ~/.local/state/nixorium/operations/deploy-20990101T000000.000000000Z-1.log; nixorium logs --json' >/tmp/logs-unsafe.json || test $? = 1; jq -e '.state == \"partial\" and .logs[0].state == \"unavailable\" and (.issues[0].message | contains(\"0600\"))' /tmp/logs-unsafe.json; unlink /home/admin/.local/state/nixorium/operations/deploy-20990101T000000.000000000Z-1.log")
    controller.succeed("su - admin -c \"(sleep 8; printf l; sleep 3; printf '\\r'; sleep 3; printf q) | TERM=xterm timeout 25s script -qefc 'stty rows 40 cols 120; nixorium --repo /home/admin/nixorium-deployment' /tmp/nixorium-logs-tui.log\"")
    controller.succeed("grep -aF 'Operation logs' /tmp/nixorium-logs-tui.log; grep -aF 'Recent actions' /tmp/nixorium-logs-tui.log; grep -aF 'Operation log detail' /tmp/nixorium-logs-tui.log; grep -aF 'Result: completed' /tmp/nixorium-logs-tui.log")
    controller.succeed("printf '\n# git-tui-commit\n' >> /home/admin/nixorium-deployment/flake.nix; chown admin:users /home/admin/nixorium-deployment/flake.nix; su - admin -c 'git -C ~/nixorium-deployment config user.name Test; git -C ~/nixorium-deployment config user.email test@example.invalid; nixorium git commit plan --repo ~/nixorium-deployment --paths flake.nix --json' | jq -r .confirmation > /tmp/git-tui-confirmation; su - admin -c 'git -C ~/nixorium-deployment rev-parse HEAD' > /tmp/git-tui-before; su - admin -c \"(sleep 8; printf g; sleep 5; printf c; sleep 1; printf ' '; sleep 0.2; printf '\r'; sleep 5; cat /tmp/git-tui-confirmation; sleep 0.2; printf '\r'; sleep 10; printf q) | TERM=xterm timeout 35s script -qefc 'stty rows 40 cols 120; nixorium --repo /home/admin/nixorium-deployment' /tmp/nixorium-git-tui.log\"; grep -aF 'Git change review' /tmp/nixorium-git-tui.log; grep -aF 'Select Git commit paths' /tmp/nixorium-git-tui.log; grep -aF 'Git commit review' /tmp/nixorium-git-tui.log; grep -aF 'Last commit: completed; committed=true' /tmp/nixorium-git-tui.log; grep -aF 'no remote push was attempted' /tmp/nixorium-git-tui.log; test \"$(cat /tmp/git-tui-before)\" != \"$(su - admin -c 'git -C ~/nixorium-deployment rev-parse HEAD')\"; test -z \"$(su - admin -c 'git -C ~/nixorium-deployment status --porcelain=v1 --untracked-files=normal')\"")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.operation == \"setup-status\" and .state == \"ready\" and (.currentStage | not) and (.stages[] | select(.id == \"offer-client-installation\").detail | contains(\"Install computers over network\"))'")
    controller.succeed("nixorium doctor --repo /tmp/deployment --json | jq -e '.state == \"warnings\" and any(.findings[]; .id == \"PXE-PREPARATION\" and .level == \"OK\") and any(.findings[]; .id == \"PXE-LIFECYCLE\" and .level == \"OK\") and any(.findings[]; .id == \"SERVICE-HARMONIA\" and .level == \"OK\") and any(.findings[]; .id == \"CACHE-HEALTH\" and .level == \"OK\") and any(.findings[]; .id == \"COMMAND-COLMENA\" and .level == \"OK\") and any(.findings[]; .id == \"NETWORK-INTERFACE\" and .level == \"OK\") and any(.findings[]; .id == \"CLIENT-SSH\" and .level == \"OK\") and any(.findings[]; .id == \"DISK-FREE\" and .level == \"WARNING\")'")
    controller.succeed("systemctl start nixorium-pxe-network.service; test -e /var/lib/nixorium/pxe/session.json")
    controller.crash()
    controller.wait_for_unit("multi-user.target")
    controller.succeed("ip -4 -o addr show dev lab0 scope global | grep -F '10.0.0.99/8'; test ! -e /var/lib/nixorium/pxe/session.json; jq -e '.state == \"recovered\" and .stopReason == \"boot-or-explicit-recovery\"' /var/lib/nixorium/pxe/last-session.json")
  '';
}
