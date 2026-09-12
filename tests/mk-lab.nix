{ mkLab, deploymentSelf, labConfig }:
let
  baseArgs = {
    inherit deploymentSelf;
    inherit labConfig;
  };
  subnetLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      networkBase = "10.23.4.128";
      networkPrefixLength = 25;
    };
  });
  rejectsUnknownHost = !(builtins.tryEval (builtins.deepSeq
    (mkLab (baseArgs // {
      hostModules.pc00 = [ ../modules/common.nix ];
    })).labMeta
    true)).success;
  rejectsUnknownVeyonHost = !(builtins.tryEval (builtins.deepSeq
    (mkLab (baseArgs // {
      labConfig = labConfig // {
        veyonNativeHosts = [ "pc00" ];
      };
    })).labMeta
    true)).success;
  hasNixorium = packages:
    builtins.any (package: (package.pname or "") == "nixorium") packages;
in
assert subnetLab.labMeta.controller.staticIp == "10.23.4.227";
assert subnetLab.labMeta.network.prefixLength == 25;
assert subnetLab.labMeta.clients.hosts == [
  { name = "pc01"; ip = "10.23.4.129"; }
  { name = "pc02"; ip = "10.23.4.130"; }
  { name = "pc03"; ip = "10.23.4.131"; }
  { name = "pc04"; ip = "10.23.4.132"; }
  { name = "pc05"; ip = "10.23.4.133"; }
  { name = "pc06"; ip = "10.23.4.134"; }
  { name = "pc07"; ip = "10.23.4.135"; }
  { name = "pc08"; ip = "10.23.4.136"; }
  { name = "pc09"; ip = "10.23.4.137"; }
  { name = "pc10"; ip = "10.23.4.138"; }
  { name = "pc11"; ip = "10.23.4.139"; }
  { name = "pc12"; ip = "10.23.4.140"; }
  { name = "pc13"; ip = "10.23.4.141"; }
  { name = "pc14"; ip = "10.23.4.142"; }
  { name = "pc15"; ip = "10.23.4.143"; }
  { name = "pc16"; ip = "10.23.4.144"; }
  { name = "pc17"; ip = "10.23.4.145"; }
  { name = "pc18"; ip = "10.23.4.146"; }
  { name = "pc19"; ip = "10.23.4.147"; }
  { name = "pc20"; ip = "10.23.4.148"; }
];
assert subnetLab.colmena.pc01.deployment.targetHost == "10.23.4.129";
assert subnetLab.apps.x86_64-linux.nixorium.type == "app";
assert subnetLab.packages.x86_64-linux.nixorium.pname == "nixorium";
assert subnetLab.packages.x86_64-linux.pxeFirmware.name == "nixorium-ipxe-firmware";
assert hasNixorium subnetLab.nixosConfigurations.pc99.config.environment.systemPackages;
assert !(hasNixorium subnetLab.nixosConfigurations.pc01.config.environment.systemPackages);
assert subnetLab.nixosConfigurations.pc99.config.services.harmonia.cache.enable;
assert subnetLab.nixosConfigurations.pc99.config.services.harmonia.cache.signKeyPaths == [
  "/var/lib/nixorium/keys/harmonia-secret-key"
];
assert builtins.elem "nixorium-harmonia.service"
  subnetLab.nixosConfigurations.pc99.config.systemd.services.harmonia.aliases;
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-prepare-pxe";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-prepare-pxe".serviceConfig.User == "admin";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-prepare-pxe".serviceConfig.CapabilityBoundingSet == "";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-pxe-network";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-pxe-network".serviceConfig.CapabilityBoundingSet == [ "CAP_NET_ADMIN" ];
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-pxe-recover";
assert !subnetLab.nixosConfigurations.pc01.config.services.harmonia.cache.enable;
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-prepare-pxe");
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-pxe-network");
assert rejectsUnknownHost;
assert rejectsUnknownVeyonHost;
true
