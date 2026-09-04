{ modulesPath, lib, ... }:
{
  imports = [ (modulesPath + "/installer/scan/not-detected.nix") ];

  # Enable firmware for common hardware (Intel/AMD microcode, WiFi, etc.)
  hardware.enableRedistributableFirmware = lib.mkDefault true;

  # The shared Disko layout uses Btrfs, so never import stray ZFS root pools.
  boot.zfs.forceImportRoot = false;

  nixpkgs.hostPlatform = lib.mkDefault "x86_64-linux";
}
