{ mkLab, deploymentSelf, labConfig }:
let
  baseArgs = {
    inherit deploymentSelf;
    inherit labConfig;
    deploymentRevision = "0123456789abcdef0123456789abcdef01234567";
  };
  controllerOnlyLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      deploymentMode = "controller";
      pcCount = 0;
      teacherPassword = "$6$test$teacher";
      studentPassword = "$6$test$student";
      adminPassword = "$6$test$admin";
    };
    publicKeys = { cache = null; ssh = null; veyon = null; };
  });
  controllerOnly = controllerOnlyLab.nixosConfigurations.pc99.config;
  insecureController = mkLab (baseArgs // {
    labConfig = labConfig // { deploymentMode = "controller"; pcCount = 0; };
  });
  subnetLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      networkBase = "10.23.4.128";
      networkPrefixLength = 25;
    };
  });
  roleInterfaceLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      controllerIfaceName = "eno1";
      clientIfaceName = "enp2s0";
      hostIfaceNames.pc02 = "enp3s0";
    };
  });
  nativeVeyonLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      veyonNativeHosts = [ "pc01" ];
    };
  });
  keyboardLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      keyboardLayout = "it";
      consoleKeyMap = "it2";
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
  catalogLab = mkLab (baseArgs // {
    softwareCatalog = [
      { id = "vlc"; label = "Video"; summary = "Play classroom media"; }
    ];
  });
  presetLab = mkLab (baseArgs // {
    softwarePresets = {
      schemaVersion = 1;
      defaultPreset = "essential";
      presets = [
        {
          id = "essential";
          label = "Essential";
          description = "Browser, terminal, and common fonts";
          packages = [ "liberation_ttf" "chromium" "ghostty" ];
        }
      ];
    };
  });
  sitePresetCatalog = builtins.fromJSON (builtins.readFile ../templates/site/software-presets.json);
  siteDefaultSoftware = builtins.fromJSON (builtins.readFile ../templates/site/lab-software.json);
  siteDefaultPreset = builtins.head
    (builtins.filter (preset: preset.id == sitePresetCatalog.defaultPreset) sitePresetCatalog.presets);
  softwareForPreset = preset: {
    schemaVersion = 1;
    packages = map (package: {
      inherit package;
      scope.kind = "shared";
    }) preset.packages;
  };
  siteProfileLabs = map
    (preset: {
      laboratory = mkLab (baseArgs // {
        softwarePresets = sitePresetCatalog;
        labSoftware = softwareForPreset preset;
      });
      controller = mkLab (baseArgs // {
        labConfig = labConfig // {
          deploymentMode = "controller";
          pcCount = 0;
          teacherPassword = "$6$test$teacher";
          studentPassword = "$6$test$student";
          adminPassword = "$6$test$admin";
        };
        publicKeys = { cache = null; ssh = null; veyon = null; };
        softwarePresets = sitePresetCatalog;
        labSoftware = softwareForPreset preset;
      });
    })
    sitePresetCatalog.presets;
  siteProfilePackages = builtins.concatLists
    (map (preset: preset.packages) sitePresetCatalog.presets);
  siteProfileResolved = map presetLab.nixoriumResolveSoftwarePackage siteProfilePackages;
  commonProfilePackages = [
    "chromium"
    "ghostty"
    "git"
    "liberation_ttf"
    "nodejs"
    "opencode"
    "pi-coding-agent"
    "python3Packages.terminaltexteffects"
  ];
  siteProfilesEvaluate = builtins.all
    (labs:
      labs.laboratory.nixoriumSoftwarePresets.defaultPreset == "essential"
      && builtins.length labs.laboratory.nixosConfigurations.pc99.config.environment.systemPackages > 0
      && builtins.length labs.laboratory.nixosConfigurations.pc01.config.environment.systemPackages > 0
      && labs.controller.nixoriumSoftwarePresets.defaultPreset == "essential"
      && builtins.length labs.controller.nixosConfigurations.pc99.config.environment.systemPackages > 0)
    siteProfileLabs;
  softwareSearch = softwareLab.nixoriumSearchSoftwarePackages { query = "hello"; limit = 20; };
  scopedSoftware = {
    schemaVersion = 1;
    packages = [
      { package = "hello"; scope.kind = "shared"; }
      { package = "cowsay"; scope.kind = "controller"; }
      { package = "figlet"; scope.kind = "all-clients"; }
    ];
  };
  scopedLab = mkLab (baseArgs // { labSoftware = scopedSoftware; });
  scopedController = mkLab (baseArgs // {
    labConfig = labConfig // { deploymentMode = "controller"; pcCount = 0; };
    labSoftware = scopedSoftware;
  });
  hasPackage = lab: host: pname:
    builtins.any (package: (package.pname or "") == pname)
      lab.nixosConfigurations.${host}.config.environment.systemPackages;
  nestedSoftware = softwareLab.nixoriumResolveSoftwarePackage "python3Packages.numpy";
  allowedUnfreeSoftware = softwareLab.nixoriumResolveSoftwarePackage "hello-unfree";
  bambuStudioSoftware = softwareLab.nixoriumResolveSoftwarePackage "bambu-studio";
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
  rejectsInvalidSoftwareCatalog = !(builtins.tryEval (builtins.deepSeq
    (mkLab (baseArgs // {
      softwareCatalog = [
        { id = "vlc"; label = ""; summary = "Invalid empty label"; }
      ];
    })).nixoriumSoftware
    true)).success;
  rejectsUnsafeHomeResetPath = !(builtins.tryEval (builtins.deepSeq
    (mkLab (baseArgs // {
      homeResetEphemeralPaths = [ "../outside-home" ];
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
  clientUsers = subnetLab.nixosConfigurations.pc01.config.users.users;
  clientPolkit = subnetLab.nixosConfigurations.pc01.config.security.polkit;
  clientHomeOwnership = subnetLab.nixosConfigurations.pc01.config.system.activationScripts.nixoriumUserHomeOwnership;
  colmenaBaseGnomeRemoteDesktop = subnetLab.colmena.meta.nixpkgs.gnome-remote-desktop;
  clientGnomeRemoteDesktop = subnetLab.nixosConfigurations.pc01.pkgs.gnome-remote-desktop;
in
assert controllerOnlyLab.labMeta.deploymentMode == "controller";
assert controllerOnlyLab.nixoriumUpdateTargets == [ "pc99" ];
assert subnetLab.nixoriumUpdateTargets == [ "pc99" "pc01" ];
assert builtins.elem "pc02" nativeVeyonLab.nixoriumUpdateTargets;
assert builtins.elem "pc02" softwareLab.nixoriumUpdateTargets;
assert (mkLab (baseArgs // { updateValidationHosts = [ "pc03" ]; })).nixoriumUpdateTargets == [ "pc99" "pc01" "pc03" ];
assert controllerOnlyLab.nixoriumOfflineCheck.drvPath != "";
assert controllerOnlyLab.labMeta.controller.staticIp == "";
assert controllerOnlyLab.labMeta.clients.count == 0;
assert controllerOnlyLab.labMeta.clients.hosts == [];
assert !(controllerOnlyLab.nixosConfigurations ? pc01);
assert !(controllerOnlyLab.colmena ? pc01);
assert !controllerOnlyLab.deploymentStatus.ready;
assert controllerOnlyLab.deploymentStatus.controller.ready;
assert !controllerOnlyLab.deploymentStatus.controller.requiresKeys;
assert !insecureController.deploymentStatus.controller.ready;
assert builtins.length insecureController.deploymentStatus.controller.issues == 3;
assert !(controllerOnly.networking.interfaces ? enp0s3);
assert controllerOnly.networking.networkmanager.enable;
assert controllerOnly.networking.firewall.enable;
assert controllerOnly.networking.firewall.interfaces == {};
assert !controllerOnly.services.harmonia.cache.enable;
assert !(controllerOnly.systemd.user.services ? veyon-server);
assert controllerOnly.system.build.toplevel.drvPath != "";
assert subnetLab.labMeta.controller.staticIp == "10.23.4.227";
assert subnetLab.labMeta.deploymentMode == "laboratory";
assert subnetLab.deploymentStatus.controller.ready == subnetLab.deploymentStatus.ready;
assert subnetLab.deploymentStatus.controller.requiresKeys;
assert subnetLab.labMeta.network.prefixLength == 25;
assert map (host: builtins.removeAttrs host [ "ifaceName" ]) subnetLab.labMeta.clients.hosts == [
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
assert subnetLab.labMeta.controller.ifaceName == "enp0s3";
assert subnetLab.labMeta.network.ifaceName == "enp0s3";
assert subnetLab.labMeta.network.clientIfaceName == "enp0s3";
assert (builtins.elemAt subnetLab.labMeta.clients.hosts 0).ifaceName == "enp0s3";
assert roleInterfaceLab.labMeta.controller.ifaceName == "eno1";
assert roleInterfaceLab.labMeta.network.ifaceName == "eno1";
assert roleInterfaceLab.labMeta.network.clientIfaceName == "enp2s0";
assert (builtins.elemAt roleInterfaceLab.labMeta.clients.hosts 0).ifaceName == "enp2s0";
assert (builtins.elemAt roleInterfaceLab.labMeta.clients.hosts 1).ifaceName == "enp3s0";
assert roleInterfaceLab.nixosConfigurations.pc99.config.networking.interfaces ? eno1;
assert roleInterfaceLab.nixosConfigurations.pc01.config.networking.interfaces ? enp2s0;
assert roleInterfaceLab.nixosConfigurations.pc02.config.networking.interfaces ? enp3s0;
assert keyboardLab.nixosConfigurations.pc99.config.console.keyMap == "it2";
assert keyboardLab.nixosConfigurations.netboot.config.console.keyMap == "it2";
assert builtins.elem "network.target"
  roleInterfaceLab.nixosConfigurations.pc99.config.systemd.services."network-addresses-eno1".wantedBy;
assert !(builtins.elem "network.target"
  roleInterfaceLab.nixosConfigurations.pc01.config.systemd.services."network-addresses-enp2s0".wantedBy);
assert roleInterfaceLab.nixosConfigurations.pc99.config.networking.firewall.interfaces ? eno1;
assert roleInterfaceLab.nixosConfigurations.pc01.config.networking.firewall.interfaces ? enp2s0;
assert roleInterfaceLab.nixosConfigurations.pc02.config.networking.firewall.interfaces ? enp3s0;
assert subnetLab.colmena.pc01.deployment.targetHost == "10.23.4.129";
# Colmena starts from the unmodified package set. Each host's module graph
# applies the laboratory overlay exactly once while evaluating that node.
assert !(builtins.elem "-Dvnc=true" (colmenaBaseGnomeRemoteDesktop.mesonFlags or [ ]));
assert builtins.elem "-Dvnc=true" (clientGnomeRemoteDesktop.mesonFlags or [ ]);
assert subnetLab.apps.x86_64-linux.nixorium.type == "app";
assert subnetLab.packages.x86_64-linux.nixorium.pname == "nixorium";
assert subnetLab.packages.x86_64-linux.pxeFirmware.name == "nixorium-ipxe-firmware";
assert subnetLab.packages.x86_64-linux.remoteInstallerBundle.name == "nixorium-remote-installer-pc99";
assert hasNixorium subnetLab.nixosConfigurations.pc99.config.environment.systemPackages;
assert !(hasNixorium subnetLab.nixosConfigurations.pc01.config.environment.systemPackages);
assert hasPackage subnetLab "pc99" "git";
assert hasHostState subnetLab.nixosConfigurations.pc99.config.environment.systemPackages;
assert hasHostState subnetLab.nixosConfigurations.pc01.config.environment.systemPackages;
assert hasSessionState subnetLab.nixosConfigurations.pc99.config.environment.systemPackages;
assert hasSessionState subnetLab.nixosConfigurations.pc01.config.environment.systemPackages;
assert !(builtins.elem "networkmanager" clientUsers.${labConfig.studentUser}.extraGroups);
assert !(builtins.elem "networkmanager"
  subnetLab.nixosConfigurations.pc99.config.users.users.${labConfig.studentUser}.extraGroups);
assert builtins.elem "networkmanager" clientUsers.${labConfig.teacherUser}.extraGroups;
assert builtins.elem "networkmanager" clientUsers.admin.extraGroups;
assert builtins.elem "users" clientHomeOwnership.deps;
assert builtins.elem "createHomeTemplates" clientHomeOwnership.deps;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''HOME_DIR="/home/admin"'' clientHomeOwnership.text;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''HOME_DIR="/home/${labConfig.teacherUser}"'' clientHomeOwnership.text;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''HOME_DIR="/home/${labConfig.studentUser}"'' clientHomeOwnership.text;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''.config/Code'' clientHomeOwnership.text;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''.vscode/extensions'' clientHomeOwnership.text;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''RUNTIME_DIR="/run/user/$USER_ID"'' clientHomeOwnership.text;
assert clientPolkit.enable;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''subject.user == "${labConfig.studentUser}"'' clientPolkit.extraConfig;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''action.id.indexOf("org.freedesktop.NetworkManager.") == 0'' clientPolkit.extraConfig;
assert subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  "return polkit.Result.NO;" clientPolkit.extraConfig;
assert !(hasPackage subnetLab "pc01" "chromium");
assert !(hasPackage subnetLab "pc01" "vscode");
assert !(hasPackage subnetLab "pc01" "opencode");
assert !(hasPackage subnetLab "pc01" "pi-coding-agent");
assert !subnetLab.nixosConfigurations.pc01.config.virtualisation.docker.rootless.enable;
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
assert subnetLab.nixosConfigurations.pc99.config.systemd.services."nixorium-pxe-recover".unitConfig.ConditionPathExists == "/var/lib/nixorium/pxe/session.json";
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
assert scopedLab.nixoriumSoftware.controller == "pc99";
assert hasPackage scopedLab "pc99" "hello";
assert hasPackage scopedLab "pc01" "hello";
assert hasPackage scopedLab "pc02" "hello";
assert hasPackage scopedLab "pc99" "cowsay";
assert !(hasPackage scopedLab "pc01" "cowsay");
assert hasPackage scopedLab "pc01" "figlet";
assert !(hasPackage scopedLab "pc99" "figlet");
assert !(hasPackage softwareLab "pc99" "vlc");
assert hasPackage scopedController "pc99" "hello";
assert hasPackage scopedController "pc99" "cowsay";
assert !(hasPackage scopedController "pc99" "figlet");
assert scopedController.nixoriumValidateControllerSoftwareCandidate scopedSoftware;
assert !(builtins.tryEval (scopedController.nixoriumValidateControllerSoftwareCandidate {
  schemaVersion = 1;
  packages = [{ package = "not-a-real-package"; scope.kind = "controller"; }];
})).success;
assert (builtins.head softwareLab.nixoriumSoftware.packages).origin == "managed";
assert softwareLab.nixoriumSoftware.catalog == [];
assert (builtins.head catalogLab.nixoriumSoftware.catalog).label == "Video";
assert (builtins.head catalogLab.nixoriumSoftware.catalog).id == "vlc";
assert softwareLab.nixoriumSoftwarePresets == null;
assert presetLab.nixoriumSoftwarePresets.defaultPreset == "essential";
assert (builtins.head presetLab.nixoriumSoftwarePresets.presets).packages
  == [ "chromium" "ghostty" "liberation_ttf" ];
assert map (preset: preset.id) sitePresetCatalog.presets == [
  "essential"
  "general-education"
  "programming"
  "graphics"
  "multimedia"
  "cad-3d"
  "stem"
];
assert sitePresetCatalog.defaultPreset == "essential";
assert map (entry: entry.package) siteDefaultSoftware.packages == siteDefaultPreset.packages;
assert builtins.all (entry: entry.scope.kind == "shared") siteDefaultSoftware.packages;
assert builtins.all
  (preset: builtins.all (package: builtins.elem package preset.packages) commonProfilePackages)
  sitePresetCatalog.presets;
assert builtins.all
  (preset: builtins.elem "vscode" preset.packages == (preset.id == "programming"))
  sitePresetCatalog.presets;
assert builtins.all (item: item != null && item.availability == "available") siteProfileResolved;
assert siteProfilesEvaluate;
assert builtins.any (item: item.id == "hello" && item.availability == "available") softwareSearch;
assert nestedSoftware.id == "python3Packages.numpy";
assert nestedSoftware.availability == "available";
assert allowedUnfreeSoftware.availability == "available";
assert bambuStudioSoftware.availability == "available";
assert rejectsUnknownHost;
assert rejectsUnknownVeyonHost;
assert rejectsInvalidSoftwareCatalog;
assert rejectsUnsafeHomeResetPath;
true
