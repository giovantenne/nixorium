{ lib, ... }:
{
  imports = [
    ./desktop.nix
    ./packages.nix
    ./power.nix
    ./screensaver.nix
    ./shell.nix
    ./ssh.nix
  ];

  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  boot.loader.systemd-boot.enable = false;
  boot.loader.grub.enable = true;
  boot.loader.grub.efiSupport = true;
  boot.loader.grub.device = "nodev";
  boot.loader.grub.useOSProber = true;
  boot.loader.timeout = 5;
  boot.loader.efi.canTouchEfiVariables = true;
  boot.loader.efi.efiSysMountPoint = "/boot";

  networking.networkmanager.enable = true;

  # Kept intentionally until the Veyon/VNC network policy is redesigned.
  networking.firewall.enable = false;

  # Downstream hardware modules may disable this default on bare metal.
  virtualisation.virtualbox.guest.enable = lib.mkDefault true;

  nixpkgs.config.allowUnfree = true;
  system.stateVersion = "25.11";
}
