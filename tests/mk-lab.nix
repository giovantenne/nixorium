{ mkLab, deploymentSelf, labConfig }:
let
  baseArgs = {
    inherit deploymentSelf;
    inherit labConfig;
    deploymentRevision = "0123456789abcdef0123456789abcdef01234567";
  };
  subnetLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      networkBase = "10.23.4.128";
      networkPrefixLength = 25;
    };
  });
  nativeVeyonLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      veyonNativeHosts = [ "pc01" ];
    };
  });
  softwareLab = mkLab (baseArgs // {
    clientGroups.graphics = [ "pc01" ];
    labSoftware = {
      schemaVersion = 1;
      packages = [
        { package = "vlc"; scope = { kind = "group"; group = "graphics"; }; }
      ];
    };
  });
  softwareSearch = softwareLab.nixoriumSearchSoftwarePackages { query = "hello"; limit = 20; };
  nestedSoftware = softwareLab.nixoriumResolveSoftwarePackage "python3Packages.numpy";
  blockedSoftware = softwareLab.nixoriumResolveSoftwarePackage "hello-unfree";
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
  hasHostState = packages:
    builtins.any (package: (package.name or "") == "nixorium-host-state") packages;
  hasSessionState = packages:
    builtins.any (package: (package.name or "") == "nixorium-session-state") packages;
  controllerFirewall = subnetLab.nixosConfigurations.pc99.config.networking.firewall;
  clientFirewall = subnetLab.nixosConfigurations.pc01.config.networking.firewall;
  controllerTCP = controllerFirewall.interfaces.enp0s3.allowedTCPPorts;
  controllerUDP = controllerFirewall.interfaces.enp0s3.allowedUDPPorts;
  clientTCP = clientFirewall.interfaces.enp0s3.allowedTCPPorts;
  nativeClientTCP = nativeVeyonLab.nixosConfigurations.pc01.config.networking.firewall.interfaces.enp0s3.allowedTCPPorts;
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
assert hasHostState subnetLab.nixosConfigurations.pc99.config.environment.systemPackages;
assert hasHostState subnetLab.nixosConfigurations.pc01.config.environment.systemPackages;
assert hasSessionState subnetLab.nixosConfigurations.pc99.config.environment.systemPackages;
assert hasSessionState subnetLab.nixosConfigurations.pc01.config.environment.systemPackages;
assert subnetLab.nixosConfigurations.pc01.config.system.configurationRevision == "0123456789abcdef0123456789abcdef01234567";
assert subnetLab.nixosConfigurations.pc99.config.services.harmonia.cache.enable;
assert subnetLab.nixosConfigurations.pc99.config.services.harmonia.cache.signKeyPaths == [
  "/var/lib/nixorium/keys/harmonia-secret-key"
];
assert builtins.elem "nixorium-harmonia.service"
  subnetLab.nixosConfigurations.pc99.config.systemd.services.harmonia.aliases;
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-apply-controller".serviceConfig.ProtectHome == false;
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-apply-controller@".serviceConfig.ProtectHome == false;
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-apply-controller".serviceConfig.StateDirectory == "nixorium/controller";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-apply-controller@".serviceConfig.StateDirectory == "nixorium/controller";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-apply-controller".serviceConfig.StateDirectoryMode == "0755";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-apply-controller@".serviceConfig.StateDirectoryMode == "0755";
assert builtins.elem "/home/admin/nixorium-deployment"
  subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-apply-controller".serviceConfig.ReadOnlyPaths;
assert builtins.elem "/home/admin/nixorium-deployment"
  subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-apply-controller@".serviceConfig.ReadOnlyPaths;
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-prepare-pxe";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-restart-cache";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-restart-cache".serviceConfig.CapabilityBoundingSet == "";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-prepare-pxe".serviceConfig.User == "admin";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-prepare-pxe".serviceConfig.CapabilityBoundingSet == "";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-pxe-network";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-pxe-network".serviceConfig.CapabilityBoundingSet == [ "CAP_NET_ADMIN" ];
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-pxe-recover";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-pxe";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-pxe".serviceConfig.CapabilityBoundingSet == [
  "CAP_KILL"
  "CAP_NET_ADMIN"
  "CAP_NET_BIND_SERVICE"
  "CAP_NET_RAW"
  "CAP_SETGID"
  "CAP_SETUID"
];
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-pxe".serviceConfig.AmbientCapabilities == [ "CAP_SETGID" "CAP_SETUID" ];
assert subnetLab.nixosConfigurations.pc99.config.users.users.nixorium-pxe-dnsmasq.isSystemUser;
assert controllerFirewall.enable;
assert controllerFirewall.allowedTCPPorts == [];
assert controllerFirewall.allowedUDPPorts == [];
assert builtins.all (port: builtins.elem port controllerTCP) [ 22 11100 5900 5000 8080 ];
assert builtins.length controllerTCP == 5;
assert builtins.all (port: builtins.elem port controllerUDP) [ 67 69 4011 5353 ];
assert builtins.length controllerUDP == 4;
assert !subnetLab.nixosConfigurations.pc01.config.services.harmonia.cache.enable;
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-prepare-pxe");
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-restart-cache");
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-pxe-network");
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-pxe");
assert clientFirewall.enable;
assert clientFirewall.allowedTCPPorts == [];
assert clientFirewall.allowedUDPPorts == [];
assert builtins.all (port: builtins.elem port clientTCP) [ 22 11100 5900 ];
assert builtins.length clientTCP == 3;
assert clientFirewall.interfaces.enp0s3.allowedUDPPorts == [ 5353 ];
assert builtins.all (port: builtins.elem port nativeClientTCP) [ 22 11100 ];
assert !(builtins.elem 5900 nativeClientTCP);
assert builtins.length nativeClientTCP == 2;
assert builtins.any (package: (package.pname or "") == "vlc")
  softwareLab.nixosConfigurations.pc01.config.environment.systemPackages;
assert !(builtins.any (package: (package.pname or "") == "vlc")
  softwareLab.nixosConfigurations.pc02.config.environment.systemPackages);
assert softwareLab.nixoriumSoftware.groups.graphics == [ "pc01" ];
assert (builtins.head softwareLab.nixoriumSoftware.packages).origin == "managed";
assert builtins.any (item: item.id == "hello" && item.availability == "available") softwareSearch;
assert nestedSoftware.id == "python3Packages.numpy";
assert nestedSoftware.availability == "available";
assert blockedSoftware.availability == "blocked-unfree";
assert rejectsUnknownHost;
assert rejectsUnknownVeyonHost;
true
