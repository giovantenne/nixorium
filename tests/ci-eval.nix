# Share related graphs within each group, then release the evaluator's memory.
{ flake }:
let
  system = "x86_64-linux";
  packages = flake.packages.${system};
  apps = flake.apps.${system};
  hosts = flake.nixosConfigurations;
  declaredChecks = flake.checks.${system};
  groups = import ./validation-groups.nix;
  groupedNames = builtins.concatLists (builtins.attrValues groups);
  checkedGroups =
    if builtins.all (names: names != []) (builtins.attrValues groups)
      && builtins.sort builtins.lessThan groupedNames == builtins.attrNames declaredChecks
    then groups
    else throw "Validation groups must cover every Flake check exactly once";
in
{
  checkGroups = checkedGroups;

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
} // (if flake ? checks then builtins.mapAttrs (_: names: builtins.listToAttrs (map (name: {
  inherit name;
  value = declaredChecks.${name}.drvPath;
}) names)) checkedGroups else {})
