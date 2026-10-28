{ pkgs }:
# The Caffeine toggle in a real GNOME automatic login: the extension is
# enabled, starts off, and while on holds a session idle inhibitor, which both
# GNOME's screen blanking and the lab screensaver's check respect.
let
  caffeine = pkgs.gnomeExtensions.caffeine;
  schemas = "${caffeine}/share/gnome-shell/extensions/${caffeine.extensionUuid}/schemas";
  # The check the lab screensaver runs, taken from the template script.
  inhibitedCheck = pkgs.writeShellScript "stay-active-inhibited" ''
    export PATH=${pkgs.lib.makeBinPath [ pkgs.glib pkgs.gnugrep ]}
    eval "$(${pkgs.gnused}/bin/sed -n '/^idle_inhibited() {/,/^}/p' ${../templates/site/scripts/screensaver-monitor.sh})"
    idle_inhibited
  '';
in
pkgs.testers.runNixOSTest {
  name = "nixorium-stay-active";
  nodes.machine = { ... }: {
    imports = [ ../modules/stay-active.nix ];
    virtualisation.memorySize = 3072;
    services.displayManager.gdm.enable = true;
    services.desktopManager.gnome.enable = true;
    services.displayManager.autoLogin = { enable = true; user = "student"; };
    users.users.student = { isNormalUser = true; uid = 1000; };
    system.stateVersion = "26.05";
  };
  testScript = ''
    session = "su - student -c 'DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus "
    inhibited = session + "gdbus call --session --dest org.gnome.SessionManager --object-path /org/gnome/SessionManager --method org.gnome.SessionManager.IsInhibited 8'"
    caffeine = session + "gsettings --schemadir ${schemas} set org.gnome.shell.extensions.caffeine cli-toggle "

    machine.wait_for_unit("display-manager.service")
    machine.wait_until_succeeds("pgrep -u student gnome-shell", timeout=180)
    machine.wait_until_succeeds("test -S /run/user/1000/bus", timeout=60)
    machine.wait_until_succeeds(session + "gnome-extensions info ${caffeine.extensionUuid}' | grep -E 'State: (ENABLED|ACTIVE)'", timeout=120)

    # Off at sign-in: nothing keeps the screen on.
    machine.succeed(inhibited + " | grep -Fx '(false,)'")
    machine.fail(session + "${inhibitedCheck}'")

    machine.succeed(caffeine + "true'")
    machine.wait_until_succeeds(inhibited + " | grep -Fx '(true,)'", timeout=30)
    machine.succeed(session + "${inhibitedCheck}'")
    machine.screenshot("stay-active-on")

    machine.succeed(caffeine + "false'")
    machine.wait_until_succeeds(inhibited + " | grep -Fx '(false,)'", timeout=30)
    machine.fail(session + "${inhibitedCheck}'")
    machine.fail("journalctl -b --no-pager | grep -F 'JS ERROR' | grep -i caffeine")
  '';
}
