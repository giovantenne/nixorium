{ modulesPath, lib, ... }:
{
  imports = [ (modulesPath + "/installer/scan/not-detected.nix") ];

  # Enable firmware for common hardware (Intel/AMD microcode, WiFi, etc.)
  hardware.enableRedistributableFirmware = lib.mkDefault true;

  # Installed machines have no generated hardware-configuration, so the initrd
  # must carry the paravirtual storage drivers itself. Without them a QEMU,
  # KVM or Proxmox guest with a virtio disk never finds its root partition.
  # virtio_net is not needed: the netboot installer image already includes it
  # through netboot-minimal's all-hardware profile, and installed systems
  # mount root from local disk.
  boot.initrd.availableKernelModules = [
    "virtio_pci"
    "virtio_blk"
    "virtio_scsi"
  ];

  # The shared Disko layout uses Btrfs, so never import stray ZFS root pools.
  boot.zfs.forceImportRoot = false;

  nixpkgs.hostPlatform = lib.mkDefault "x86_64-linux";
}
