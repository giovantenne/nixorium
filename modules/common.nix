{ lib, ... }:
{
  imports = [
    ./desktop.nix
    ./firewall.nix
    ./client-internet.nix
    ./power.nix
    ./ssh.nix
  ];

  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  boot.loader.systemd-boot.enable = false;
  boot.loader.grub.enable = true;
  boot.loader.grub.efiSupport = true;
  boot.loader.grub.device = "nodev";
  boot.loader.grub.useOSProber = true;
  # Keep the boot menu and /boot bounded; Maintenance → Free disk space keeps
  # the same number of system generations (ADR 0022).
  boot.loader.grub.configurationLimit = lib.mkDefault 10;
  boot.loader.timeout = 5;
  boot.loader.efi.canTouchEfiVariables = true;
  boot.loader.efi.efiSysMountPoint = "/boot";

  networking.networkmanager.enable = true;

  # Downstream hardware modules may disable this default on bare metal.
  virtualisation.virtualbox.guest.enable = lib.mkDefault true;

  nixpkgs.config.allowUnfree = true;
  system.stateVersion = "25.11";
}
