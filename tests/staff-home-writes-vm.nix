{ pkgs }:
# The root activation steps that write into the administrator's and teacher's
# homes (core users, home-reset and desktop modules, and the template's home
# profile) run on a machine where the teacher has replaced each target with a
# symbolic link to a system file. The system files stay untouched, the steps
# still finish, and without the links the files land owned by their users.
let
  inherit (pkgs) lib;
  labSettings = {
    teacherUser = "teacher";
    adminGitName = "Admin";
    adminGitEmail = "admin@example.invalid";
  };
  labAssets = {
    backgrounds = [];
    mimeApps = ../assets/empty-mimeapps.list;
    vscodeSettings = pkgs.writeText "vscode-settings.json" "{\"nixorium\": true}\n";
  };
  # Each module is called as a function, so only the steps under test run.
  users = import ../modules/users.nix { isController = true; inherit labSettings lib pkgs; };
  homeReset = import ../modules/home-reset.nix { inherit labSettings lib pkgs; };
  desktop = import ../modules/desktop.nix { isController = true; inherit labSettings labAssets lib pkgs; };
  homeProfile = import ../templates/site/modules/home-profile.nix {
    inherit labSettings labAssets lib pkgs;
    hostSoftwarePackages = [ "vscode" "nodejs" "chromium" ];
  };
  steps = [
    (pkgs.writeShellScript "repair-ownership" users.system.activationScripts.nixoriumUserHomeOwnership.text)
    (pkgs.writeShellScript "teacher-bookmark" homeReset.system.activationScripts.teacherSnapshotBookmark.text)
    (pkgs.writeShellScript "site-home-profile" homeProfile.system.activationScripts.siteHomeProfile.text)
    (pkgs.writeShellScript "gdm-monitor-config" ''
      export PATH=${lib.makeBinPath [ pkgs.coreutils ]}:$PATH
      ${desktop.systemd.services.gdm-monitor-config.content.script}
    '')
  ];
  runSteps = pkgs.writeShellScript "run-staff-home-steps" ''
    set -e
    ${lib.concatMapStringsSep "\n" (step: "${step}") steps}
  '';
in
pkgs.testers.runNixOSTest {
  name = "nixorium-staff-home-writes";
  nodes.machine = { ... }: {
    users.users.admin = { isNormalUser = true; };
    users.users.teacher = { isNormalUser = true; };
    users.groups.nixorium-staff = {};
    system.stateVersion = "26.05";
  };
  testScript = ''
    def snapshot():
        return machine.succeed("find /etc/victim /var/lib/victim -printf '%u:%g %m %s %p\\n' | sort && find /etc/victim /var/lib/victim -type f -exec sha256sum {} + | sort")

    machine.wait_for_unit("multi-user.target")
    machine.succeed("echo 'root:secret' > /etc/victim && chmod 0600 /etc/victim")
    machine.succeed("install -d -m 0750 /var/lib/victim && echo keep > /var/lib/victim/settings.json && echo keep > /var/lib/victim/argv.json")
    # The admin's layout is private: the teacher's copy must not need to read it.
    machine.succeed("su - admin -c 'mkdir -p ~/.config && echo \"<monitors/>\" > ~/.config/monitors.xml && chmod 0600 ~/.config/monitors.xml'")

    with subtest("without links the files land in the homes, owned by their users"):
        machine.succeed("${runSteps}")
        machine.succeed("test \"$(stat -c %U /home/teacher/.config/monitors.xml)\" = teacher")
        machine.succeed("grep -q monitors /home/teacher/.config/monitors.xml")
        machine.succeed("grep -q home-snapshots /home/teacher/.config/gtk-3.0/bookmarks")
        machine.succeed("test \"$(stat -c %U /home/teacher/.config/gtk-3.0/bookmarks)\" = teacher")
        for user in ["admin", "teacher"]:
            machine.succeed(f"test \"$(stat -c %U /home/{user}/.config/Code/User/settings.json)\" = {user}")
            machine.succeed(f"test \"$(stat -c %U /home/{user}/.vscode/argv.json)\" = {user}")
            machine.succeed(f"test -d /home/{user}/.local/npm")
        machine.succeed("test \"$(stat -c %U /home/admin/.config/mimeapps.list)\" = admin")

    with subtest("links left by the teacher do not redirect root writes"):
        before = snapshot()
        # Set up as root, then handed to the teacher: what the teacher could do
        # in a home they own (chown -h and -R never follow the links).
        machine.succeed(
            "cd /home/teacher && "
            "rm -rf .config/monitors.xml .config/gtk-3.0 .config/Code .vscode .local/npm && "
            "mkdir -p .config/gtk-3.0 .vscode .local && "
            "ln -s /etc/victim .config/monitors.xml && "
            "ln -s /etc/victim .config/gtk-3.0/bookmarks && "
            "ln -s /var/lib/victim .config/Code && "
            "ln -s /var/lib/victim .vscode/extensions && ln -s /etc/victim .vscode/argv.json && "
            "ln -s /var/lib/victim .local/npm && "
            "chown -hR teacher:users .config .vscode .local"
        )
        machine.succeed("${runSteps}")
        after = snapshot()
        assert before == after, f"a system file changed:\n{before}\n{after}"
        # The teacher's own link was replaced by a regular file, written as the teacher.
        machine.succeed("test ! -L /home/teacher/.config/monitors.xml")
        machine.succeed("test \"$(stat -c %U /home/teacher/.config/monitors.xml)\" = teacher")

    with subtest("a FIFO left by the teacher does not stall the steps"):
        machine.succeed("cd /home/teacher/.config/gtk-3.0 && rm -f bookmarks && mkfifo bookmarks && chown -h teacher:users bookmarks")
        machine.succeed("timeout 120 ${runSteps}")
  '';
}
