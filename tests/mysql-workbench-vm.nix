{ pkgs }:
# A teacher opens MySQL Workbench and the "Local instance" connection it
# offers on first start: it must connect as root without a password. The lab
# desktop has no secret service (gnome-keyring is disabled), and without one
# Workbench refuses every connection with "The name is not activatable", so
# the test runs the real GNOME session and the real application. It uses the
# development module's MySQL settings and session variables unchanged.
#
# It waits on the application's accessibility tree, not on OCR: the session
# opens in the Activities overview, where the first dialog is a dimmed,
# scaled-down thumbnail that OCR read only after minutes, if at all.
let
  python = pkgs.python3.withPackages (ps: [ ps.pygobject3 ]);
  clicker = pkgs.writeShellScript "mysql-workbench-click" ''
    export GI_TYPELIB_PATH="${pkgs.glib.out}/lib/girepository-1.0:${pkgs.gobject-introspection}/lib/girepository-1.0"
    exec ${python}/bin/python3 ${./mysql-workbench-click.py} "$@"
  '';
  state = pkgs.writeShellScript "mysql-workbench-state" ''
    export GI_TYPELIB_PATH="${pkgs.glib.out}/lib/girepository-1.0:${pkgs.gobject-introspection}/lib/girepository-1.0:${pkgs.at-spi2-core}/lib/girepository-1.0"
    exec ${python}/bin/python3 ${./mysql-workbench-state.py} "$@"
  '';
  labSettings = { studentUser = "student"; teacherUser = "teacher"; };
in
pkgs.testers.runNixOSTest {
  name = "nixorium-mysql-workbench";
  nodes.machine = { lib, ... }: {
    imports = [ ../templates/site/modules/development.nix ];
    _module.args = { inherit labSettings; hostSoftwarePackages = [ "mysql84" "mysql-workbench" ]; };
    virtualisation.memorySize = 4096;
    virtualisation.resolution = { x = 1280; y = 800; };
    services.displayManager.gdm.enable = true;
    # As modules/desktop.nix: no keyring, no welcome dialog, and the screen
    # never blanks or locks. A locked screen is black and refuses the remote
    # desktop session the pointer helper drives.
    services.gnome.gnome-keyring.enable = lib.mkForce false;
    services.desktopManager.gnome.enable = true;
    services.desktopManager.gnome.extraGSettingsOverrides = ''
      [org.gnome.shell]
      welcome-dialog-last-shown-version='9999'

      [org.gnome.desktop.session]
      idle-delay=uint32 0

      [org.gnome.desktop.screensaver]
      lock-enabled=false
      lock-delay=uint32 0

      [org.gnome.settings-daemon.plugins.power]
      sleep-inactive-ac-type='nothing'
      sleep-inactive-ac-timeout=0
      idle-dim=false
    '';
    services.desktopManager.gnome.extraGSettingsOverridePackages = [ pkgs.gnome-settings-daemon ];
    services.displayManager.autoLogin = { enable = true; user = "student"; };
    users.users.student = { isNormalUser = true; uid = 1000; };
    environment.systemPackages = [ pkgs.mysql-workbench ];
    system.stateVersion = "26.05";
  };
  testScript = ''
    user = "systemd-run --user -M student@ --wait --pipe --collect "
    state = user + "${state} "
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
    # Workbench warns about the unsupported operating system. The session is
    # still in the Activities overview, which shows the window scaled down:
    # leave it before clicking. Outside the overview, Esc would close the
    # dialog instead.
    machine.wait_until_succeeds(state + "showing 'Unsupported Operating System'", timeout=120)
    if machine.execute(state + "desktop")[0] != 0:
        machine.send_key("esc")
    machine.wait_until_succeeds(state + "desktop", timeout=30)
    machine.screenshot("workbench-first-start")
    machine.succeed(user + "${clicker} 640 524")
    machine.wait_until_succeeds(state + "gone 'Unsupported Operating System'", timeout=30)
    machine.wait_until_succeeds(state + "showing 'Local instance 3306'", timeout=30)
    # Open the offered connection (a double click on its tile).
    machine.succeed(user + "${clicker} 350 640 2")
    machine.wait_until_succeeds(state + "showing 'Connection Warning'", timeout=90)
    machine.screenshot("workbench-version-warning")
    machine.succeed(user + "${clicker} 487 582")
    machine.wait_until_succeeds("grep -F \"Opened connection 'Local instance 3306'\" /home/student/.mysql/workbench/log/wb.log", timeout=90)
    machine.fail("grep -F 'could not be connected' /home/student/.mysql/workbench/log/wb.log")
    # The session is real: the server holds an authenticated root connection
    # made by Workbench itself (not by the checking client).
    machine.wait_until_succeeds("test \"$(mysql -u root -N -e \"SELECT COUNT(*) FROM performance_schema.session_connect_attrs a JOIN performance_schema.threads t ON t.PROCESSLIST_ID = a.PROCESSLIST_ID WHERE a.ATTR_NAME='program_name' AND a.ATTR_VALUE = 'MySQLWorkbench' AND t.PROCESSLIST_USER='root'\")\" -gt 0", timeout=60)
    machine.screenshot("workbench-connected")
  '';
}
