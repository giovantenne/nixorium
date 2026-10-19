{ pkgs }:
let
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
  genericBinary = pkgs.runCommandCC "generic-linux-binary" {
    nativeBuildInputs = [ pkgs.patchelf ];
    buildInputs = [ pkgs.zlib ];
  } ''
    cat > main.cpp <<'EOF'
    #include <iostream>
    #include <zlib.h>
    int main() { std::cout << "zlib " << zlibVersion() << std::endl; return 0; }
    EOF
    g++ main.cpp -lz -o generic
    patchelf --remove-rpath --set-interpreter /lib64/ld-linux-x86-64.so.2 generic
    install -D -m 0755 generic "$out"
  '';
  settings = { masterHostName = "controller"; pcCount = 2; deploymentMode = "laboratory"; };
  node = name: { ... }: {
    imports = [ ../modules/telemetry.nix ../modules/nix-ld.nix ];
    _module.args = { inherit nixoriumPackage; hostName = name; isController = name == settings.masterHostName; labSettings = settings; };
    users.users.admin = { isNormalUser = true; };
    users.users.student = { isNormalUser = true; };
    environment.systemPackages = [ nixoriumPackage pkgs.jq ];
    # A dynamically linked program built the way a vendor ships it: the generic
    # loader path, no run path, and zlib and libstdc++ found only through nix-ld.
    environment.etc."nixorium-test-generic-binary".source = genericBinary;
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
    for machine in [controller, client]:
      with subtest(f"{machine.name} runs generic Linux binaries through nix-ld"):
        machine.succeed("test -L /lib64/ld-linux-x86-64.so.2 && readlink /lib64/ld-linux-x86-64.so.2 | grep -q nix-ld")
        machine.succeed("/etc/nixorium-test-generic-binary | grep -q '^zlib '")
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
