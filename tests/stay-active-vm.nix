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
  # A window that asks to be fullscreen as it opens, like the screensaver's
  # Ghostty (which cannot draw in the test VM).
  fullscreenProbe = pkgs.stdenv.mkDerivation {
    name = "fullscreen-probe";
    dontUnpack = true;
    nativeBuildInputs = [ pkgs.wrapGAppsHook4 pkgs.gobject-introspection ];
    buildInputs = [ pkgs.gtk4 (pkgs.python3.withPackages (p: [ p.pygobject3 ])) ];
    installPhase = ''
      mkdir -p $out/bin
      cat > $out/bin/fullscreen-probe <<'PY'
      #!${pkgs.python3.withPackages (p: [ p.pygobject3 ])}/bin/python3
      import gi
      gi.require_version("Gtk", "4.0")
      from gi.repository import Gtk
      def activate(app):
          window = Gtk.ApplicationWindow(application=app)
          window.fullscreen()
          window.present()
      app = Gtk.Application(application_id="org.nixorium.FullscreenProbe")
      app.connect("activate", activate)
      app.run(None)
      PY
      chmod +x $out/bin/fullscreen-probe
    '';
  };
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

    # The lab screensaver is a fullscreen window: a fullscreen window must not
    # switch Caffeine on, or the screensaver would stop itself as it starts.
    machine.succeed(session + "WAYLAND_DISPLAY=wayland-0 XDG_RUNTIME_DIR=/run/user/1000 ${fullscreenProbe}/bin/fullscreen-probe >/dev/null 2>&1 &'")
    machine.wait_until_succeeds("pgrep -u student -f fullscreen-probe", timeout=60)
    machine.sleep(10)
    machine.screenshot("stay-active-fullscreen")
    machine.succeed(inhibited + " | grep -Fx '(false,)'")
    machine.fail(session + "${inhibitedCheck}'")
    # Control: with Caffeine's fullscreen rule back on, the same window holds
    # the inhibitor, so the window above really was fullscreen.
    machine.succeed(session + "gsettings --schemadir ${schemas} set org.gnome.shell.extensions.caffeine enable-fullscreen true'")
    machine.wait_until_succeeds(inhibited + " | grep -Fx '(true,)'", timeout=30)
    machine.succeed(session + "gsettings --schemadir ${schemas} reset org.gnome.shell.extensions.caffeine enable-fullscreen'")
    machine.wait_until_succeeds(inhibited + " | grep -Fx '(false,)'", timeout=30)
    machine.fail("journalctl -b --no-pager | grep -F 'JS ERROR' | grep -i caffeine")
  '';
}
