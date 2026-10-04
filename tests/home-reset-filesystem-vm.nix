{ pkgs }:
let
  seed = import ../lib/build-workspace-seed.nix { inherit (pkgs) lib; inherit pkgs; } {
    effective = {
      schemaVersion = 1;
      desktop = { colorScheme = "dark"; dock.position = "left"; };
      vscode = { extensions = []; settings."editor.fontSize" = 15; };
    };
    extensions = [];
  };
  badLinkSeed = pkgs.runCommand "home-reset-bad-link-seed" {} ''
    cp -a ${seed}/. "$out"
    chmod -R u+w "$out"
    ln -s /outside-home "$out/home/escape"
  '';
  badProfileSeed = pkgs.runCommand "home-reset-bad-profile-seed" {} ''
    cp -a ${seed}/. "$out"
    chmod -R u+w "$out"
    echo '{"schemaVersion":1,"profile":{"schemaVersion":99},"extensions":[]}' > "$out/manifest.json"
  '';
  failingDconf = pkgs.writeShellScriptBin "dconf" "exit 72";
  config = pkgs.writeText "home-reset-test.json" (builtins.toJSON {
    user = "student";
    inherit seed;
    ephemeralPaths = [ ".config/opencode" ".local/npm" ];
    wallpapers = [ (pkgs.writeText "fixture.jpg" "wallpaper fixture") ];
    btrfs = "${pkgs.btrfs-progs}/bin/btrfs";
    dconf = "${pkgs.dconf}/bin/dconf";
    systemctl = "${pkgs.systemd}/bin/systemctl";
  });
  tests = (pkgs.callPackage ../pkgs/nixorium.nix {}).overrideAttrs {
    pname = "nixorium-home-reset-filesystem-tests";
    doCheck = false;
    buildPhase = ''
      runHook preBuild
      go test -c -o home-reset-filesystem-tests ./internal/homereset
      runHook postBuild
    '';
    installPhase = ''
      install -D -m 0755 home-reset-filesystem-tests "$out/bin/home-reset-filesystem-tests"
    '';
    postFixup = "";
  };
in
pkgs.testers.runNixOSTest {
  name = "nixorium-home-reset-filesystem";
  nodes.machine = { pkgs, ... }: {
    virtualisation = {
      memorySize = 768;
      emptyDiskImages = [ 512 ];
    };
    environment.systemPackages = [ pkgs.btrfs-progs tests ];
    environment.etc."home-reset-test.json".source = config;
    users.users.student = { isNormalUser = true; uid = 2000; group = "users"; };
    users.groups.nixorium-staff = {};
    system.stateVersion = "26.05";
  };
  testScript = ''
    machine.start(allow_reboot=True)
    machine.wait_for_unit("multi-user.target")
    # Only this disposable VM's additional disk is formatted. Tests create
    # their own temporary directories and explicitly unmount every bind mount.
    machine.succeed("mkfs.btrfs -f /dev/vdb; mkdir -p /mnt/home-reset-test; mount /dev/vdb /mnt/home-reset-test")
    machine.succeed("btrfs subvolume create /mnt/home-reset-test/@home-student; btrfs subvolume create /mnt/home-reset-test/@snapshots")
    machine.succeed("mkdir -p /var/lib/home-snapshots; mount -o subvol=@home-student /dev/vdb /home/student; mount -o subvol=@snapshots /dev/vdb /var/lib/home-snapshots; chown student:users /home/student; chmod 0700 /home/student")
    machine.succeed("TMPDIR=/mnt/home-reset-test NIXORIUM_HOME_RESET_VM_TEST=1 NIXORIUM_BAD_SEED_LINK=${badLinkSeed} NIXORIUM_BAD_SEED_PROFILE=${badProfileSeed} NIXORIUM_FAILING_DCONF=${failingDconf}/bin/dconf home-reset-filesystem-tests -test.v", timeout=180)
    machine.reboot()
    machine.wait_for_unit("multi-user.target")
    machine.succeed("mount -o subvol=@home-student /dev/vdb /home/student; mount -o subvol=@snapshots /dev/vdb /var/lib/home-snapshots")
    machine.succeed("NIXORIUM_HOME_RESET_VM_TEST=1 NIXORIUM_HOME_RESET_REBOOT_TEST=1 home-reset-filesystem-tests -test.run TestResetFailureSurvivesRebootVM -test.v")
    machine.succeed("umount /home/student /var/lib/home-snapshots")
  '';
}
