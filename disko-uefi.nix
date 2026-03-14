{ labSettings, ... }:
{
  disko.devices = import ./lib/disko-layout.nix {
    device = "/dev/sda";
    studentUser = labSettings.studentUser;
  };
}
