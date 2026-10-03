{ pkgs }:
# The classroom view agent starts with a real GNOME automatic login and
# answers hello through the fixed connect command, locally and over SSH as the
# controller reaches clients. Disabled laboratories get no agent at all.
let
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
  agent = "${nixoriumPackage}/bin/nixorium-classroom-agent";
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
    services.displayManager.gdm.enable = true;
    services.desktopManager.gnome.enable = true;
    services.displayManager.autoLogin = { enable = true; user = "student"; };
    users.users.student = { isNormalUser = true; uid = 1000; };
    services.openssh.enable = true;
    system.stateVersion = "26.05";
  };
  testScript = ''
    client.wait_for_unit("display-manager.service")
    client.wait_until_succeeds("pgrep -u student gnome-shell", timeout=180)
    client.wait_until_succeeds("systemctl --user -M student@ is-active nixorium-classroom-agent", timeout=120)
    client.succeed("test \"$(stat -c %a:%U /run/user/1000/nixorium-classroom.sock)\" = 600:student")

    # Local relay as root, then the same command over SSH like the controller.
    client.succeed("${agent} probe nixorium-classroom-connect | grep -F 'hello version=1' | grep -F 'user=student'")
    client.succeed("install -d -m 700 /root/.ssh && ssh-keygen -q -t ed25519 -N ''' -f /root/.ssh/id && cp /root/.ssh/id.pub /root/.ssh/authorized_keys")
    client.succeed("${agent} probe ssh -T -i /root/.ssh/id -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ClearAllForwardings=yes -o ForwardAgent=no root@127.0.0.1 nixorium-classroom-connect | grep -F 'user=student'")

    # Without a running agent the controller gets a clear error, not a hang.
    client.succeed("systemctl --user -M student@ stop nixorium-classroom-agent")
    client.fail("${agent} probe nixorium-classroom-connect")
    client.succeed("${agent} probe nixorium-classroom-connect | grep -F 'code=no-agent' || true")
    # An ordinary user cannot use the root relay.
    client.fail("su student -s /bin/sh -c '${agent} connect < /dev/null'")
  '';
}
