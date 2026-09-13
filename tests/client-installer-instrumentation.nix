{ modulesPath, ... }:
{
  imports = [ (modulesPath + "/testing/test-instrumentation.nix") ];

  boot.initrd.availableKernelModules = [
    "virtio_blk"
    "virtio_pci"
  ];
}
