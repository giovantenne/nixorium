{ pkgs, editorQualification ? false }:
let
  editorPkgs = import pkgs.path { inherit (pkgs.stdenv.hostPlatform) system; config.allowUnfree = true; };
  seedPkgs = if editorQualification then editorPkgs else pkgs;
  extension = editorPkgs.vscode-extensions.ritwickdey.liveserver;
  makeSeed = size: withExtension: import ../lib/build-workspace-home.nix { inherit (pkgs) lib; pkgs = seedPkgs; } {
    resolution = {
      effective = { schemaVersion = 1; vscode = { extensions = pkgs.lib.optional withExtension "ritwickdey.liveserver"; settings."editor.fontSize" = size; }; };
      extensions = pkgs.lib.optional withExtension {
        id = "ritwickdey.liveserver";
        package = "vscode-extensions.ritwickdey.liveserver";
        inherit (extension) version;
      };
    };
    labSettings = { studentGitName = "Student"; studentGitEmail = "student@example.invalid"; };
  };
  seed = makeSeed 15 editorQualification;
  nextSeed = makeSeed 19 editorQualification;
  editorCheck = import ./workspace-editor-fixture.nix { inherit pkgs; editor = editorPkgs.vscode; inherit extension; };
  resetModule = ../modules/workspace-reset.nix;
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
  node = { lib, ... }: {
    imports = [ resetModule ] ++ lib.optional editorQualification editorCheck.module;
    _module.args = {
      labSettings.studentUser = "student";
      homeResetEphemeralPaths = [ ".local/npm" ];
      workspaceSeed = seed;
      workspaceWallpapers = [];
      inherit nixoriumPackage;
    };
    virtualisation = { memorySize = if editorQualification then 2048 else 768; emptyDiskImages = [ 512 ]; };
    users.users.student = { isNormalUser = true; uid = 2000; group = "users"; };
    users.groups.nixorium-staff = {};
    environment.systemPackages = [ pkgs.btrfs-progs ];
    # Only these disposable VM disks are formatted, and only on the first boot.
    systemd.services.prepare-reset-fixture = {
      requiredBy = [ "home-reset.service" ];
      before = [ "home-reset.service" ];
      after = [ "local-fs.target" ];
      path = [ pkgs.btrfs-progs pkgs.util-linux pkgs.coreutils ];
      serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
      script = ''
        if ! blkid /dev/vdb; then
          mkfs.btrfs -f /dev/vdb
          mkdir -p /mnt/reset-fixture
          mount /dev/vdb /mnt/reset-fixture
          btrfs subvolume create /mnt/reset-fixture/@home-student
          btrfs subvolume create /mnt/reset-fixture/@snapshots
          umount /mnt/reset-fixture
        fi
        mkdir -p /var/lib/home-snapshots
        mount -o subvol=@home-student /dev/vdb /home/student
        mount -o subvol=@snapshots /dev/vdb /var/lib/home-snapshots
        chown student:users /home/student
        chmod 0700 /home/student
      '';
      restartIfChanged = false;
    };
    # A small login consumer makes ordering and failure propagation observable
    # without claiming to exercise GNOME or a real graphical student session.
    systemd.services.display-manager = {
      wantedBy = [ "multi-user.target" ];
      after = [ "systemd-user-sessions.service" ];
      serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
      script = ''
        test ! -e /run/nologin
        touch /run/reset-fixture-login-started
      '';
      restartIfChanged = false;
    };
    specialisation = {
      updated.configuration = {
        _module.args.workspaceSeed = lib.mkForce nextSeed;
        systemd.services.home-reset.description = lib.mkForce "Updated workspace reset fixture";
      };
    } // lib.optionalAttrs editorQualification {
      without-extension.configuration._module.args.workspaceSeed = lib.mkForce (makeSeed 19 false);
    };
    system.stateVersion = "26.05";
  };
in
pkgs.testers.runNixOSTest {
  name = if editorQualification then "nixorium-workspace-editor" else "nixorium-workspace-reset-service";
  nodes.managed = node;
  testScript = if editorQualification then editorCheck.testScript else ''
    managed.start(allow_reboot=True)
    managed.wait_for_unit("multi-user.target")
    managed.wait_for_unit("display-manager.service")
    managed.succeed("test -f /run/reset-fixture-login-started; test ! -e /run/nologin")

    managed.succeed("test -f /home/student/.config/Code/User/settings.json; test -f /var/lib/home-snapshots/.workspace-reset/success.json")
    managed.succeed("grep -q '\"editor.fontSize\":15' /home/student/.config/Code/User/settings.json")
    managed.succeed("echo session-sentinel > /home/student/session-sentinel")
    managed.succeed("systemd-run --unit=student-session-fixture --uid=student /run/current-system/sw/bin/sleep infinity")
    managed.wait_for_unit("student-session-fixture.service")
    managed.fail("${nixoriumPackage}/bin/nixorium-home-reset --home /home/admin")
    managed.fail("runuser -u student -- ${nixoriumPackage}/bin/nixorium-home-reset")
    # A new seed takes effect at the next boot, never during a switch.
    managed.succeed("/run/current-system/specialisation/updated/bin/switch-to-configuration test", timeout=120)
    managed.succeed("grep -qx session-sentinel /home/student/session-sentinel")
    managed.succeed("systemctl is-active student-session-fixture.service display-manager.service systemd-user-sessions.service")
    managed.succeed("test ! -e /run/nologin; test ! -e /var/lib/home-snapshots/.workspace-reset/pending.json")
    managed.fail("${nixoriumPackage}/bin/nixorium-home-reset")
    managed.succeed("grep -qx session-sentinel /home/student/session-sentinel")
    managed.succeed("grep -q '\"editor.fontSize\":15' /home/student/.config/Code/User/settings.json")
    managed.succeed("grep -q '${seed}' /var/lib/home-snapshots/.workspace-reset/success.json")
    managed.succeed("grep -q '${nextSeed}' /etc/nixorium-workspace-reset.json")

    # The engine VM injects real failures. Here retained evidence must prevent
    # both login consumers on the next boot, without deleting the live sentinel.
    managed.succeed("install -m 0600 /dev/null /var/lib/home-snapshots/.workspace-reset/pending.json; sync")
    managed.succeed("systemctl stop student-session-fixture.service display-manager.service systemd-user-sessions.service home-reset.service")
    managed.fail("systemctl start home-reset.service")
    managed.succeed("grep -qx session-sentinel /home/student/session-sentinel")
    managed.reboot()
    managed.wait_until_succeeds("systemctl is-failed home-reset.service")
    managed.succeed("test -f /run/nologin; test ! -e /run/reset-fixture-login-started")
    managed.fail("systemctl is-active systemd-user-sessions.service")
    managed.fail("systemctl is-active display-manager.service")
    managed.succeed("grep -qx session-sentinel /home/student/session-sentinel")
    managed.fail("systemctl start home-reset.service")
    managed.succeed("grep -qx session-sentinel /home/student/session-sentinel")
  '';
}
