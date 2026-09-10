{ nixoriumPackage }:
{
  name = "nixorium-management";

  nodes.controller = { pkgs, ... }: {
    imports = [ ../modules/management.nix ];

    _module.args = {
      hostName = "pc99";
      labSettings.masterHostName = "pc99";
      inherit nixoriumPackage;
    };

    environment.systemPackages = [ pkgs.git pkgs.jq ];
    services.openssh.enable = true;
    environment.etc."nixorium-test/flake.nix".text = ''
      {
        outputs = { self }: {
          labMeta = {
            schemaVersion = 2;
            version = "vm-test";
            controller = {
              name = "pc99";
              number = 99;
              staticIp = "127.0.0.1";
              dhcpIp = "127.0.0.1";
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
              ifaceName = "lo";
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
          nixoriumValidateCandidate = candidate: builtins.deepSeq candidate true;
        };
      }
    '';
    environment.etc."nixorium-test/lab-settings.json".source = ../templates/site/lab-settings.json;
  };

  testScript = ''
    start_all()
    controller.wait_for_unit("sshd.service")
    controller.succeed("command -v nixorium")
    controller.succeed("command -v colmena")
    controller.succeed("nixorium --help | grep -F 'setup keys'")
    controller.succeed("mkdir -p /tmp/deployment")
    controller.succeed("cp /etc/nixorium-test/flake.nix /tmp/deployment/flake.nix")
    controller.succeed("cp /etc/nixorium-test/lab-settings.json /tmp/deployment/lab-settings.json")
    controller.succeed("git -C /tmp/deployment init -q")
    controller.succeed("git -C /tmp/deployment add flake.nix")
    controller.succeed("git -C /tmp/deployment -c user.name=Test -c user.email=test@example.invalid commit -qm initial")
    controller.succeed("cp /tmp/deployment/lab-settings.json /tmp/candidate.json")
    controller.succeed("nixorium config plan --repo /tmp/deployment --file /tmp/candidate.json --json | jq -e '.operation == \"config-plan\" and .state == \"unchanged\" and (.changes | length) == 0'")
    controller.succeed("nixorium setup keys --repo /tmp/deployment --json | jq -e '.operation == \"setup-keys\" and .state == \"ready\" and all(.keys[]; .verified and .matches and .privateMode == 384)'")
    controller.succeed("test $(stat -c '%a' /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem | sort -u) = 600")
    controller.succeed("before=$(sha256sum /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem); nixorium setup keys --repo /tmp/deployment --json >/dev/null; after=$(sha256sum /tmp/deployment/secret-key /tmp/deployment/admin-ssh /tmp/deployment/veyon-private-key.pem); test \"$before\" = \"$after\"")
    controller.succeed("nixorium status --repo /tmp/deployment --json | jq -e '.operation == \"status\" and .state == \"ready\" and .lab.clients.hosts[0].name == \"pc01\"'")
    controller.succeed("nixorium setup status --repo /tmp/deployment --json | jq -e '.operation == \"setup-status\" and .state == \"action-required\" and .currentStage == \"collect-network\"'")
    controller.succeed("nixorium doctor --repo /tmp/deployment --json | jq -e '.state == \"warnings\" and any(.findings[]; .id == \"COMMAND-COLMENA\" and .level == \"OK\") and any(.findings[]; .id == \"NETWORK-INTERFACE\" and .level == \"OK\") and any(.findings[]; .id == \"CLIENT-SSH\" and .level == \"OK\")'")
  '';
}
