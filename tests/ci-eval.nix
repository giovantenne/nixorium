# Force related outputs in one evaluator so their NixOS module graphs are shared.
{ flake }:
let
  system = "x86_64-linux";
  packages = flake.packages.${system};
  apps = flake.apps.${system};
  hosts = flake.nixosConfigurations;
in
{
  checks = builtins.mapAttrs (_: check: check.drvPath) flake.checks.${system};

  systems = {
    client = hosts.pc01.config.system.build.toplevel.drvPath;
    controller = hosts.${flake.labMeta.controller.name}.config.system.build.toplevel.drvPath;
    netboot = hosts.netboot.config.system.build.netbootRamdisk.drvPath;
    disko = packages.disko.drvPath;
    installer = packages.installerBundle.drvPath;
    remoteInstaller = packages.remoteInstallerBundle.drvPath;
    firmware = packages.pxeFirmware.drvPath;
    command = packages.nixorium.drvPath;
    harmoniaApp = apps.run-harmonia.program;
    pxeApp = apps.run-pxe-proxy.program;
    commandApp = apps.nixorium.program;
    colmenaTarget = flake.colmena.pc01.deployment.targetHost;
    status = flake.deploymentStatus;
    updates = flake.nixoriumUpdateTargets;
    offline = flake.nixoriumOfflineCheck.drvPath;
  };

  template = {
    command = apps.nixorium.program;
    firmware = packages.pxeFirmware.drvPath;
    client = hosts.pc01.config.system.build.toplevel.drvPath;
    installer = packages.installerBundle.drvPath;
  };
}
