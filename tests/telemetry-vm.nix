{ pkgs }:
let
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
  settings = { masterHostName = "controller"; pcCount = 2; deploymentMode = "laboratory"; };
  node = name: { ... }: {
    imports = [ ../modules/telemetry.nix ];
    _module.args = { inherit nixoriumPackage; hostName = name; labSettings = settings; };
    users.users.admin = { isNormalUser = true; };
    users.users.student = { isNormalUser = true; };
    environment.systemPackages = [ nixoriumPackage pkgs.jq ];
    # Prevent the test from contacting the real service, even if host networking exists.
    networking.extraHosts = "127.0.0.1 telemetry.nixorium.org";
    system.stateVersion = "26.05";
  };
in
pkgs.testers.runNixOSTest {
  name = "nixorium-telemetry";
  nodes.controller = node "controller";
  nodes.client = node "pc01";
  testScript = ''
    import json
    start_all()
    controller.wait_for_unit("multi-user.target")
    client.wait_for_unit("multi-user.target")
    controller.wait_for_unit("nixorium-telemetry.timer")
    controller.succeed("systemctl stop nixorium-telemetry.timer")
    controller.succeed("systemctl start nixorium-telemetry.service")
    controller.fail("test -e /var/lib/nixorium/telemetry/state.json")
    report = json.loads(controller.succeed("su - admin -c 'nixorium telemetry preview --json'"))
    assert report["configuredClients"] == "1-5" and report["clientBootVerified"] is None
    controller.fail("su - student -c 'nixorium telemetry enable'")
    client.fail("test -e /etc/nixorium/telemetry.json")
    client.fail("su - admin -c 'nixorium telemetry enable'")
    controller.succeed("su - admin -c 'nixorium telemetry enable'")
    controller.succeed("systemctl start nixorium-telemetry.service")
    state = json.loads(controller.succeed("su - admin -c 'nixorium telemetry status --json'"))
    assert state["consent"] == "enabled" and state["result"] == "network-unavailable"
    assert state["lastAttempt"] and "lastSuccess" not in state
    controller.succeed("su - admin -c 'nixorium telemetry disable'")
    controller.succeed("systemctl start nixorium-telemetry.service")
    state = json.loads(controller.succeed("cat /var/lib/nixorium/telemetry/state.json"))
    assert state["consent"] == "disabled" and "secret" not in state and "lastAttempt" not in state
    assert controller.succeed("stat -c %a /var/lib/nixorium/telemetry/state.json").strip() == "600"
  '';
}
