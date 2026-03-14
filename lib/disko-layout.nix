{ device, studentUser }:
{
  disk.main = {
    type = "disk";
    inherit device;
    content = {
      type = "gpt";
      partitions = {
        esp = {
          size = "512M";
          type = "EF00";
          content = {
            type = "filesystem";
            format = "vfat";
            mountpoint = "/boot";
            mountOptions = [ "umask=0077" ];
          };
        };
        root = {
          size = "100%";
          content = {
            type = "btrfs";
            extraArgs = [ "-f" "-L" "nixos" ];
            subvolumes = {
              "@root" = {
                mountpoint = "/";
                mountOptions = [ "compress=zstd" "noatime" ];
              };
              "@home-${studentUser}" = {
                mountpoint = "/home/${studentUser}";
                mountOptions = [ "compress=zstd" "noatime" ];
              };
              "@snapshots" = {
                mountpoint = "/var/lib/home-snapshots";
                mountOptions = [ "compress=zstd" "noatime" ];
              };
            };
          };
        };
      };
    };
  };
}
