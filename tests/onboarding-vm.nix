{ pkgs, useKVM ? true }:
let
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
  python = pkgs.python3.withPackages (packages: [ packages.pexpect ]);
in
pkgs.testers.runNixOSTest {
  name = "nixorium-onboarding";
  requiredFeatures.kvm = useKVM;
  nodes.controller = { ... }: {
    environment.systemPackages = [ nixoriumPackage pkgs.git python ];
    environment.etc."onboarding-settings.json".source = ../templates/site/lab-settings.json;
    environment.etc."onboarding-terminal.py".source = ./onboarding-terminal.py;
    environment.etc."nixorium/telemetry.json".text = builtins.toJSON {
      schemaVersion = 1;
      version = "3.0.0";
      deploymentMode = "controller";
      configuredClients = "0";
    };
    systemd.tmpfiles.rules = [ "d /var/lib/nixorium/telemetry 0700 root root -" ];
    networking.extraHosts = "127.0.0.1 telemetry.nixorium.org github.com";
    system.stateVersion = "26.05";
  };
  testScript = ''
    start_all()
    controller.wait_for_unit("multi-user.target")
    controller.succeed("mkdir /tmp/onboarding-lab; cp /etc/onboarding-settings.json /tmp/onboarding-lab/lab-settings.json; printf '{ outputs = self: {}; }\\n' > /tmp/onboarding-lab/flake.nix; git -C /tmp/onboarding-lab init -q; git -C /tmp/onboarding-lab add .; git -C /tmp/onboarding-lab -c user.name=Test -c user.email=test@example.invalid commit -qm fixture")
    with subtest("disclaimer, skipped consent, persisted refusal and missing client notice"):
      controller.succeed("python /etc/onboarding-terminal.py", timeout=150)
  '';
}
