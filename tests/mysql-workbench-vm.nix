{ pkgs }:
# A teacher opens MySQL Workbench and the "Local instance" connection it
# offers on first start: it must connect as root without a password. The lab
# desktop has no secret service (gnome-keyring is disabled), and without one
# Workbench refuses every connection with "The name is not activatable", so
# the test runs the real GNOME session and the real application. It uses the
# development module's MySQL settings and session variables unchanged.
let
  python = pkgs.python3.withPackages (ps: [ ps.pygobject3 ]);
  clicker = pkgs.writeShellScript "mysql-workbench-click" ''
    export GI_TYPELIB_PATH="${pkgs.glib.out}/lib/girepository-1.0:${pkgs.gobject-introspection}/lib/girepository-1.0"
    exec ${python}/bin/python3 ${./mysql-workbench-click.py} "$@"
  '';
  labSettings = { studentUser = "student"; teacherUser = "teacher"; };
in
pkgs.testers.runNixOSTest {
  name = "nixorium-mysql-workbench";
  enableOCR = true;
  nodes.machine = { lib, ... }: {
    imports = [ ../templates/site/modules/development.nix ];
    _module.args = { inherit labSettings; hostSoftwarePackages = [ "mysql84" "mysql-workbench" ]; };
    virtualisation.memorySize = 4096;
    virtualisation.resolution = { x = 1280; y = 800; };
    services.displayManager.gdm.enable = true;
    # As modules/desktop.nix: no keyring, no welcome dialog, no remote desktop.
    services.gnome.gnome-keyring.enable = lib.mkForce false;
    services.desktopManager.gnome.enable = true;
    services.desktopManager.gnome.extraGSettingsOverrides = "[org.gnome.shell]\nwelcome-dialog-last-shown-version='9999'\n";
    services.displayManager.autoLogin = { enable = true; user = "student"; };
    users.users.student = { isNormalUser = true; uid = 1000; };
    environment.systemPackages = [ pkgs.mysql-workbench ];
    system.stateVersion = "26.05";
  };
  testScript = ''
    user = "systemd-run --user -M student@ --wait --pipe --collect "
    machine.wait_for_unit("display-manager.service")
    machine.wait_until_succeeds("pgrep -u student gnome-shell", timeout=180)
    machine.wait_for_unit("mysql.service")
    # The precondition of the failure: nothing provides the secret service.
    machine.fail("systemctl --user -M student@ is-active gnome-keyring-daemon")
    machine.fail("ls /run/current-system/sw/share/dbus-1/services | grep -i secrets")
    # The session the dock starts applications from carries the setting.
    machine.succeed("tr '\\0' '\\n' < /proc/$(pgrep -u student -o gnome-shell)/environ | grep -Fx WB_NO_KEYRING=1")
    machine.succeed("systemctl --user -M student@ show-environment | grep -Fx WB_NO_KEYRING=1")

    # First start, as the dock launches it.
    machine.succeed("systemd-run --user -M student@ --unit=workbench /run/current-system/sw/bin/mysql-workbench")
    machine.wait_until_succeeds("grep -F 'System info' /home/student/.mysql/workbench/log/wb.log", timeout=120)
    machine.wait_until_succeeds("test -s /home/student/.mysql/workbench/server_instances.xml", timeout=60)
    machine.sleep(10)
    machine.send_key("esc")
    machine.screenshot("workbench-first-start")
    machine.succeed(user + "${clicker} 640 524")
    machine.wait_for_text("Local instance 3306")
    # Open the offered connection (a double click on its tile).
    machine.succeed(user + "${clicker} 350 640 2")
    machine.wait_for_text("Connection Warning")
    machine.screenshot("workbench-version-warning")
    machine.succeed(user + "${clicker} 487 582")
    machine.wait_until_succeeds("grep -F \"Opened connection 'Local instance 3306'\" /home/student/.mysql/workbench/log/wb.log", timeout=90)
    machine.fail("grep -F 'could not be connected' /home/student/.mysql/workbench/log/wb.log")
    # The session is real: a query runs as root on the teaching server.
    machine.sleep(5)
    machine.succeed(user + "${clicker} 700 400")
    machine.send_chars("SELECT CURRENT_USER();")
    machine.send_key("ctrl-ret")
    machine.wait_for_text("root@localhost")
    machine.screenshot("workbench-query")
  '';
}
