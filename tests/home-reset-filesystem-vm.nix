{ pkgs }:
let
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
      emptyDiskImages = [ 256 ];
    };
    environment.systemPackages = [ pkgs.btrfs-progs tests ];
    system.stateVersion = "26.05";
  };
  testScript = ''
    machine.start()
    machine.wait_for_unit("multi-user.target")
    # Only this disposable VM's additional disk is formatted. Tests create
    # their own temporary directories and explicitly unmount every bind mount.
    machine.succeed("mkfs.btrfs -f /dev/vdb; mkdir -p /mnt/home-reset-test; mount /dev/vdb /mnt/home-reset-test")
    machine.succeed("TMPDIR=/mnt/home-reset-test NIXORIUM_HOME_RESET_VM_TEST=1 home-reset-filesystem-tests -test.v", timeout=120)
    machine.succeed("umount /mnt/home-reset-test")
  '';
}
