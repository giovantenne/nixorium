{ pkgs, editorQualification ? false, useKVM ? true }:
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
    homeFilesRoot = ./fixtures/home-files;
    homeFilesProfile = if editorQualification then { schemaVersion = 1; } else {
      schemaVersion = 1;
      directories = [ "Documents/Teaching/Empty" ];
      files = [
        { destination = "Documents/Teaching/lesson.txt"; text = if size == 15 then "initial lesson\n" else "updated lesson\n"; }
        { destination = "Documents/Teaching/image.bin"; source = "imports/0e65a074a920df375483f2139a86aad84b3932e03a5f08f013ef142de1d008a9"; }
        { destination = ".config/ghostty/config"; text = if size == 15 then "theme = light\n" else "theme = dark\n"; }
        { destination = "bin/teaching-script"; text = "#!/bin/sh\nprintf 'lesson script\\n'\n"; executable = true; }
      ] ++ pkgs.lib.optional (size == 15) { destination = "Documents/Teaching/removed.txt"; text = "removed in next seed\n"; }
        ++ pkgs.lib.optional (size != 15) { destination = "Documents/Teaching/added.txt"; text = "added in next seed\n"; };
    };
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
    virtualisation = { memorySize = if editorQualification then 2048 else 768; emptyDiskImages = [ 512 ]; useBootLoader = true; };
    # The reset-service fixture has only the test driver's serial channel.
    # The separate editor qualification retains its own loopback-only policy.
    virtualisation.vlans = lib.mkIf (!editorQualification) (lib.mkForce []);
    virtualisation.qemu.networkingOptions = lib.mkIf (!editorQualification) (lib.mkForce []);
    virtualisation.qemu.options = lib.mkIf (!editorQualification) [ "-nic" "none" ];
    networking.useDHCP = lib.mkIf (!editorQualification) (lib.mkForce false);
    boot.loader.grub.enable = true;
    boot.loader.timeout = 1;
    users.users.student = { isNormalUser = true; uid = 2000; group = "users"; };
    users.groups.nixorium-staff = {};
    environment.systemPackages = [ pkgs.btrfs-progs ];
    environment.etc = lib.optionalAttrs (!editorQualification) {
      home-files-test-fixture.source = ./fixtures/home-files/imports/0e65a074a920df375483f2139a86aad84b3932e03a5f08f013ef142de1d008a9;
    };
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
  requiredFeatures.kvm = useKVM;
  qemu.forceAccel = useKVM;
  testScript = if editorQualification then editorCheck.testScript else ''
    managed.start(allow_reboot=True)
    managed.wait_for_unit("multi-user.target")
    managed.wait_for_unit("display-manager.service")
    managed.succeed("test \"$(ls /sys/class/net)\" = lo; test -z \"$(ip route show default)\"; test -z \"$(ip -6 route show default)\"")
    managed.succeed("test -f /run/reset-fixture-login-started; test ! -e /run/nologin")

    managed.succeed("test -f /home/student/.config/Code/User/settings.json; test -f /var/lib/home-snapshots/.workspace-reset/success.json")
    managed.succeed("grep -q '\"editor.fontSize\":15' /home/student/.config/Code/User/settings.json")
    managed.succeed("grep -qx 'initial lesson' /home/student/Documents/Teaching/lesson.txt; grep -qx 'theme = light' /home/student/.config/ghostty/config")
    managed.succeed("cmp /etc/home-files-test-fixture /home/student/Documents/Teaching/image.bin; test -d /home/student/Documents/Teaching/Empty; test -z \"$(ls -A /home/student/Documents/Teaching/Empty)\"")

    def check_home_modes():
        for path, mode in [
            ("Documents/Teaching/lesson.txt", "600"),
            ("Documents/Teaching/image.bin", "600"),
            (".config/ghostty/config", "600"),
            ("bin/teaching-script", "700"),
            ("Documents/Teaching/Empty", "700"),
        ]:
            actual = managed.succeed(f"stat -c '%u:%g:%a' /home/student/{path}").strip()
            assert actual == f"2000:100:{mode}", (path, actual)

    check_home_modes()
    managed.succeed("runuser -u student -- /home/student/bin/teaching-script | grep -qx 'lesson script'")
    # Student ownership permits ordinary session edits and mode changes.
    managed.succeed("runuser -u student -- sh -c 'printf session-edit > /home/student/Documents/Teaching/lesson.txt; chmod 0744 /home/student/Documents/Teaching/lesson.txt; printf session-config > /home/student/.config/ghostty/config; rm /home/student/Documents/Teaching/image.bin; touch /home/student/Documents/Teaching/Empty/session-file'")
    managed.succeed("echo session-sentinel > /home/student/session-sentinel")
    managed.succeed("readlink -f /run/current-system > /root/reset-base-system")
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
    managed.succeed("grep -qx session-edit /home/student/Documents/Teaching/lesson.txt; test \"$(stat -c %a /home/student/Documents/Teaching/lesson.txt)\" = 744; grep -qx session-config /home/student/.config/ghostty/config; test ! -e /home/student/Documents/Teaching/image.bin; test -f /home/student/Documents/Teaching/Empty/session-file; test -f /home/student/Documents/Teaching/removed.txt; test ! -e /home/student/Documents/Teaching/added.txt")

    # Persist the reviewed generation and actually boot it. The update must
    # restore captured bytes/modes, additions, removals and empty directories.
    managed.succeed("target=$(cat /root/reset-base-system)/specialisation/updated; nix-env -p /nix/var/nix/profiles/system --set $target; $target/bin/switch-to-configuration switch", timeout=120)
    managed.reboot()
    managed.wait_for_unit("display-manager.service")
    managed.succeed("test \"$(ls /sys/class/net)\" = lo; test -z \"$(ip route show default)\"; test -z \"$(ip -6 route show default)\"")
    managed.succeed("test ! -e /run/nologin; test ! -e /var/lib/home-snapshots/.workspace-reset/pending.json; test ! -e /home/student/session-sentinel")
    managed.succeed("grep -q '\"editor.fontSize\":19' /home/student/.config/Code/User/settings.json; grep -qx 'updated lesson' /home/student/Documents/Teaching/lesson.txt; grep -qx 'theme = dark' /home/student/.config/ghostty/config")
    managed.succeed("cmp /etc/home-files-test-fixture /home/student/Documents/Teaching/image.bin; test -f /home/student/Documents/Teaching/added.txt; test ! -e /home/student/Documents/Teaching/removed.txt; test -z \"$(ls -A /home/student/Documents/Teaching/Empty)\"")
    check_home_modes()
    managed.succeed("grep -q '${nextSeed}' /var/lib/home-snapshots/.workspace-reset/success.json; echo session-sentinel > /home/student/session-sentinel")
    managed.succeed("systemd-run --unit=student-session-fixture --uid=student /run/current-system/sw/bin/sleep infinity")
    managed.wait_for_unit("student-session-fixture.service")

    # The engine VM injects real failures. Here retained evidence must prevent
    # both login consumers on the next boot, without deleting the live sentinel.
    managed.succeed("install -m 0600 /dev/null /var/lib/home-snapshots/.workspace-reset/pending.json; sync")
    managed.succeed("systemctl stop student-session-fixture.service display-manager.service systemd-user-sessions.service home-reset.service")
    managed.fail("systemctl start home-reset.service")
    managed.succeed("grep -qx session-sentinel /home/student/session-sentinel")
    managed.reboot()
    managed.wait_until_succeeds("systemctl is-failed home-reset.service")
    managed.succeed("test \"$(ls /sys/class/net)\" = lo; test -z \"$(ip route show default)\"; test -z \"$(ip -6 route show default)\"")
    managed.succeed("test -f /run/nologin; test ! -e /run/reset-fixture-login-started")
    managed.fail("systemctl is-active systemd-user-sessions.service")
    managed.fail("systemctl is-active display-manager.service")
    managed.succeed("grep -qx session-sentinel /home/student/session-sentinel")
    managed.fail("systemctl start home-reset.service")
    managed.succeed("grep -qx session-sentinel /home/student/session-sentinel")
  '';
}
