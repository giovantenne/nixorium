# Firmware that forgets its boot entries (a CMOS reset, a firmware update, or
# a vendor that drops entries it did not create) starts the removable-media
# path of each disk, \EFI\BOOT\BOOTX64.EFI. Nixorium's EFI partition is its
# own (the installers erase the disk), so GRUB is copied there as well, every
# time the boot loader is installed, and the computer still starts from its
# disk when the NVRAM entry is gone.
{ config, lib, pkgs, ... }:
let
  esp = config.boot.loader.efi.efiSysMountPoint;
  # The directory grub-install uses: distribution name and mount point
  # (nixos/modules/system/boot/loader/grub/grub.nix).
  bootloaderId = "${config.system.nixos.distroName}${lib.replaceStrings [ "/" ] [ "-" ] esp}";
in
{
  config = lib.mkIf (config.boot.loader.grub.enable && config.boot.loader.grub.efiSupport && !config.boot.loader.grub.efiInstallAsRemovable) {
    boot.loader.grub.extraInstallCommands = ''
      if [ -f ${esp}/EFI/${bootloaderId}/grubx64.efi ]; then
        ${pkgs.coreutils}/bin/install -D -m 0644 ${esp}/EFI/${bootloaderId}/grubx64.efi ${esp}/EFI/BOOT/BOOTX64.EFI
      fi
    '';
  };
}
