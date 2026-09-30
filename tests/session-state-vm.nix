{ pkgs }:
# The power-control session helper against a real GNOME automatic login: an
# untouched session is "unused", keyboard input makes it "active", and the
# user's systemd manager session is not mistaken for a login.
let
  helper = pkgs.writeShellApplication {
    name = "nixorium-session-state";
    runtimeInputs = [ pkgs.coreutils pkgs.systemd pkgs.util-linux pkgs.glib ];
    text = builtins.readFile ../scripts/session-state.sh;
  };
in
pkgs.testers.runNixOSTest {
  name = "nixorium-session-state";
  nodes.machine = { ... }: {
    virtualisation.memorySize = 3072;
    services.displayManager.gdm.enable = true;
    services.desktopManager.gnome.enable = true;
    services.displayManager.autoLogin = { enable = true; user = "student"; };
    users.users.student = { isNormalUser = true; uid = 1000; };
    environment.systemPackages = [ helper ];
    system.stateVersion = "26.05";
  };
  testScript = ''
    machine.wait_for_unit("display-manager.service")
    machine.wait_until_succeeds("pgrep -u student gnome-shell", timeout=180)
    machine.wait_until_succeeds("test -S /run/user/1000/bus", timeout=60)
    machine.wait_until_succeeds("test \"$(nixorium-session-state)\" = unused", timeout=120)
    # Input within the first 30 seconds still counts as "since login".
    machine.sleep(40)
    machine.succeed("test \"$(nixorium-session-state)\" = unused")
    machine.send_key("shift")
    machine.wait_until_succeeds("test \"$(nixorium-session-state)\" = active", timeout=30)
  '';
}
