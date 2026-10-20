# A computer whose firmware lost the NixOS boot entry still starts from its
# disk: modules/boot-fallback.nix copies GRUB to the removable-media path.
{ pkgs, ... }:
{
  name = "nixorium-boot-fallback";

  nodes.machine = { ... }: {
    imports = [ ../modules/boot-fallback.nix ];
    virtualisation.useBootLoader = true;
    virtualisation.useEFIBoot = true;
    boot.loader.systemd-boot.enable = false;
    boot.loader.grub = { enable = true; efiSupport = true; device = "nodev"; };
    boot.loader.efi = { canTouchEfiVariables = true; efiSysMountPoint = "/boot"; };
    environment.systemPackages = [ pkgs.efibootmgr ];
    system.stateVersion = "26.05";
  };

  testScript = ''
    machine.wait_for_unit("multi-user.target")
    machine.succeed("cmp /boot/EFI/BOOT/BOOTX64.EFI /boot/EFI/NixOS-boot/grubx64.efi")
    print(machine.succeed("efibootmgr"))

    with subtest("without its NVRAM entry the computer starts from the fallback path"):
      machine.succeed("for entry in $(efibootmgr | sed -n 's/^Boot\\([0-9A-F]\\{4\\}\\)\\*\\{0,1\\} *NixOS.*/\\1/p'); do efibootmgr -q -b \"$entry\" -B; done")
      machine.fail("efibootmgr | grep -i nixos")
      machine.shutdown()
      machine.start()
      machine.wait_for_unit("multi-user.target")
      machine.succeed("test -d /run/current-system")
      print(machine.succeed("efibootmgr"))

    with subtest("installing the boot loader again keeps the fallback current"):
      machine.succeed("rm /boot/EFI/BOOT/BOOTX64.EFI")
      machine.succeed("NIXOS_INSTALL_BOOTLOADER=1 /run/current-system/bin/switch-to-configuration boot")
      machine.succeed("cmp /boot/EFI/BOOT/BOOTX64.EFI /boot/EFI/NixOS-boot/grubx64.efi")
  '';
}
