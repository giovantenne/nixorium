{ pkgs }:
# The classroom view agent starts with a real GNOME automatic login and
# answers hello through the fixed connect command, locally and over SSH as the
# controller reaches clients. Disabled laboratories get no agent at all.
let
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
  agent = "${nixoriumPackage}/bin/nixorium-classroom-agent";
  python = pkgs.python3.withPackages (ps: [ ps.pygobject3 ]);
  clicker = pkgs.writeShellScript "classroom-view-click" ''
    export GI_TYPELIB_PATH="${pkgs.glib.out}/lib/girepository-1.0:${pkgs.gobject-introspection}/lib/girepository-1.0"
    exec ${python}/bin/python3 ${./classroom-view-click.py} "$@"
  '';
  labSettings = {
    classroomView = true;
    deploymentMode = "laboratory";
    masterHostName = "pc99";
  };
in
pkgs.testers.runNixOSTest {
  name = "nixorium-classroom-view";
  nodes.client = { ... }: {
    imports = [ ../modules/classroom-view.nix ];
    _module.args = { inherit labSettings nixoriumPackage; hostName = "pc01"; };
    virtualisation.memorySize = 3072;
    virtualisation.resolution = { x = 1280; y = 800; };
    services.displayManager.gdm.enable = true;
    services.desktopManager.gnome.enable = true;
    # As on Nixorium workstations: no modal welcome dialog over the top bar.
    services.desktopManager.gnome.extraGSettingsOverrides = "[org.gnome.shell]\nwelcome-dialog-last-shown-version='9999'\n";
    services.displayManager.autoLogin = { enable = true; user = "student"; };
    users.users.student = { isNormalUser = true; uid = 1000; };
    services.openssh.enable = true;
    system.stateVersion = "26.05";
  };
  testScript = ''
    client.wait_for_unit("display-manager.service")
    client.wait_until_succeeds("pgrep -u student gnome-shell", timeout=180)
    client.wait_until_succeeds("systemctl --user -M student@ is-active nixorium-classroom-agent", timeout=120)
    client.wait_until_succeeds("test \"$(stat -c %a:%U /run/user/1000/nixorium-classroom.sock)\" = 600:student", timeout=30)

    # Local relay as root, then the same command over SSH like the controller.
    client.succeed("${agent} probe nixorium-classroom-connect | grep -F 'hello version=1' | grep -F 'user=student'")
    client.succeed("install -d -m 700 /root/.ssh && ssh-keygen -q -t ed25519 -N ''' -f /root/.ssh/id && cp /root/.ssh/id.pub /root/.ssh/authorized_keys")
    client.succeed("${agent} probe ssh -T -i /root/.ssh/id -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ClearAllForwardings=yes -o ForwardAgent=no root@127.0.0.1 nixorium-classroom-connect | grep -F 'user=student'")

    # A real thumbnail of the student's screen, through the same relay.
    client.succeed("${agent} probe --thumbnail /tmp/thumb.jpg nixorium-classroom-connect | grep -E '^thumbnail width=320 height=200 bytes=[0-9]{3,}'")
    client.succeed("test \"$(head -c 2 /tmp/thumb.jpg | od -An -tx1 | tr -d ' ')\" = ffd8")
    client.copy_from_machine("/tmp/thumb.jpg")

    # Remote control: input through the agent reaches the session (GNOME's
    # idle time restarts), with every key released afterwards.
    idle = "su - student -c 'DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus gdbus call --session --dest org.gnome.Mutter.IdleMonitor --object-path /org/gnome/Mutter/IdleMonitor/Core --method org.gnome.Mutter.IdleMonitor.GetIdletime' | sed -n 's/.*uint64 \\([0-9]*\\).*/\\1/p'"
    client.sleep(6)
    client.succeed("test $(" + idle + ") -gt 4000")
    client.succeed("${agent} probe --input --thumbnail /tmp/thumb-input.jpg nixorium-classroom-connect | grep -F 'input.done'")
    client.succeed("test $(" + idle + ") -lt 3000")

    # The indicator stays visible but a student's click no longer stops the view.
    extensions = "su - student -c 'DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus gnome-extensions "
    client.wait_until_succeeds(extensions + "info nixorium-classroom@nixorium.org' | grep -E 'State: (ENABLED|ACTIVE)'", timeout=60)
    client.succeed("${agent} probe --thumbnail /tmp/thumb2.jpg nixorium-classroom-connect")
    client.sleep(2)
    client.screenshot("sharing-indicator")
    client.succeed("systemd-run --user -M student@ --wait --pipe --collect ${clicker} 1160 16")
    # Control: without the extension the same click stops screen sharing.
    client.succeed(extensions + "disable nixorium-classroom@nixorium.org'")
    client.sleep(2)
    client.succeed("${agent} probe --thumbnail /tmp/thumb3.jpg nixorium-classroom-connect")
    client.fail("systemd-run --user -M student@ --wait --pipe --collect ${clicker} 1160 16")

    # A new screen size closes Mutter's stream: the next request records the
    # new screen and input still works (no stale frame, no "Unknown stream").
    client.succeed("${agent} probe --thumbnail /tmp/thumb4.jpg nixorium-classroom-connect | grep -F 'height=200'")
    connector = client.succeed("for status in /sys/class/drm/card*-*/status; do grep -qx connected $status && basename $(dirname $status) | cut -d- -f2-; done | head -1").strip()
    gdctl = "su - student -c 'DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus ${pkgs.mutter}/bin/gdctl "
    mode = client.succeed(gdctl + "show --modes' | grep -o '1024x768@[0-9.]*' | head -1").strip()
    client.succeed(gdctl + "set --logical-monitor --primary --monitor " + connector + " --mode " + mode + "'")
    client.sleep(2)
    client.succeed("${agent} probe --input --thumbnail /tmp/thumb5.jpg nixorium-classroom-connect | tee /dev/stderr | grep -F 'input.done'")
    client.succeed("${agent} probe --thumbnail /tmp/thumb6.jpg nixorium-classroom-connect | grep -F 'width=320 height=240'")

    # Without a running agent the controller gets a clear error, not a hang.
    client.succeed("systemctl --user -M student@ stop nixorium-classroom-agent")
    client.fail("${agent} probe nixorium-classroom-connect")
    client.succeed("${agent} probe nixorium-classroom-connect | grep -F 'code=no-agent' || true")
    # An ordinary user cannot use the root relay.
    client.fail("su student -s /bin/sh -c '${agent} connect < /dev/null'")
  '';
}
