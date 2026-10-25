{ mkLab, deploymentSelf, labConfig }:
let
  lib = subnetLab.nixosConfigurations.pc01.pkgs.lib;
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
    publicKeys = { cache = null; ssh = null; };
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
  classroomViewLab = mkLab (baseArgs // {
    labConfig = labConfig // { classroomView = true; };
  });
  assistantLab = mkLab (baseArgs // {
    labConfig = labConfig // { assistant = true; };
  });
  assistantTestLab = mkLab (baseArgs // {
    labConfig = labConfig // { assistant = true; };
    controllerModules = [ ./fixtures/assistant-options.nix ];
  });
  assistantOffLab = mkLab (baseArgs // {
    labConfig = labConfig // { assistant = false; };
  });
  roleInterfaceLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      controllerIfaceName = "eno1";
      clientIfaceName = "enp2s0";
      hostIfaceNames.pc02 = "enp3s0";
    };
  });
  hostOnlyInterfaceLab = mkLab (baseArgs // {
    labConfig = labConfig // { hostIfaceNames.pc02 = "enp3s0"; };
  });
  plainLab = mkLab baseArgs;
  # A change of laboratory network from 10.0.0.0/24 to 10.1.0.0/24.
  previousNetwork = { previousBase = "10.0.0.0"; previousPrefixLength = 24; startedAt = "2026-10-15T08:00:00Z"; };
  transitionLab = mkLab (baseArgs // {
    labConfig = labConfig // { networkBase = "10.1.0.0"; networkTransition = previousNetwork; };
  });
  transitionNamedLab = mkLab (baseArgs // {
    labConfig = labConfig // { networkBase = "10.1.0.0"; clientIfaceName = "enp2s0"; networkTransition = previousNetwork; };
  });
  movedLab = mkLab (baseArgs // { labConfig = labConfig // { networkBase = "10.1.0.0"; }; });
  transitionController = transitionLab.nixosConfigurations.pc99.config;
  transitionClient = transitionLab.nixosConfigurations.pc01.config;
  labHttpSubstituters = config: builtins.filter (lib.hasPrefix "http://10.") config.nix.settings.substituters;
  transitionRejected = transition: !(builtins.tryEval (builtins.deepSeq
    (mkLab (baseArgs // { labConfig = labConfig // { networkBase = "10.1.0.0"; networkTransition = transition; }; })).labMeta
    true)).success;
  ignoredTransitionLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      deploymentMode = "controller";
      pcCount = 0;
      teacherPassword = "$6$test$teacher";
      studentPassword = "$6$test$student";
      adminPassword = "$6$test$admin";
      # Not even validated: it names the current network.
      networkTransition = previousNetwork;
    };
    publicKeys = { cache = null; ssh = null; };
  });
  # pc03 has its own module, so its own class and system; the others share one.
  classesLab = mkLab (baseArgs // {
    hostModules.pc03 = [ ./fixtures/class-only-pc03.nix ];
  });
  classesSystem = name: classesLab.nixosConfigurations.${name}.config.system.build.toplevel.drvPath;
  # A shared configuration cannot know which computer it runs on.
  hostNameLab = mkLab (baseArgs // {
    clientModules = [ ./fixtures/class-uses-hostname.nix ];
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
        publicKeys = { cache = null; ssh = null; };
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
    "jq"
    "liberation_ttf"
    "nodejs"
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
  hostClassLab = mkLab (baseArgs // {
    labConfig = labConfig // { pcCount = 6; };
    hostModules.pc04 = [];
    updateValidationHosts = [ "pc05" ];
    labSoftware = {
      schemaVersion = 1;
      packages = [ { package = "hello"; scope = { kind = "clients"; clients = [ "pc06" ]; }; } ];
    };
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
  hasCleanGenerations = packages:
    builtins.any (package: (package.name or "") == "nixorium-clean-generations") packages;
  controllerFirewall = subnetLab.nixosConfigurations.pc99.config.networking.firewall;
  clientFirewall = subnetLab.nixosConfigurations.pc01.config.networking.firewall;
  controllerTCP = controllerFirewall.interfaces.enp0s3.allowedTCPPorts;
  controllerUDP = controllerFirewall.interfaces.enp0s3.allowedUDPPorts;
  allInterfaceTCP = firewall: lib.concatMap (iface: iface.allowedTCPPorts) (lib.attrValues firewall.interfaces);
  allInterfaceUDP = firewall: lib.concatMap (iface: iface.allowedUDPPorts) (lib.attrValues firewall.interfaces);
  clientTCP = allInterfaceTCP clientFirewall;
  nativeClient = plainLab.nixosConfigurations.pc01.config;
  nativeClientTCP = allInterfaceTCP plainLab.nixosConfigurations.pc01.config.networking.firewall;
  clientUsers = subnetLab.nixosConfigurations.pc01.config.users.users;
  clientPolkit = subnetLab.nixosConfigurations.pc01.config.security.polkit;
  clientHomeOwnership = subnetLab.nixosConfigurations.pc01.config.system.activationScripts.nixoriumUserHomeOwnership;
in
assert import ./group-modules.nix { inherit mkLab deploymentSelf labConfig lib; };
assert hostClassLab.nixoriumHostClasses == [
  { representative = "pc01"; members = [ "pc01" "pc02" "pc03" ]; }
  { representative = "pc04"; members = [ "pc04" ]; }
  { representative = "pc05"; members = [ "pc05" ]; }
  { representative = "pc06"; members = [ "pc06" ]; }
  { representative = "pc99"; members = [ "pc99" ]; }
];
assert lib.sort builtins.lessThan (lib.concatMap (class: class.members) hostClassLab.nixoriumHostClasses)
  == [ "pc01" "pc02" "pc03" "pc04" "pc05" "pc06" "pc99" ];
assert controllerOnlyLab.nixoriumHostClasses == [ { representative = "pc99"; members = [ "pc99" ]; } ];
# MySQL Workbench skips the missing keychain on every lab desktop, whatever
# the site's development module says (labs created before 3.8.0 lack it).
assert subnetLab.nixosConfigurations.pc01.config.environment.sessionVariables.WB_NO_KEYRING == "1";
assert controllerOnly.environment.sessionVariables.WB_NO_KEYRING == "1";
# OpenSSH must not rewrite the controller's link to the managed known hosts.
assert lib.hasInfix "UpdateHostKeys no" controllerOnly.programs.ssh.extraConfig;
assert controllerOnlyLab.labMeta.deploymentMode == "controller";
assert controllerOnly.virtualisation.virtualbox.guest.enable;
assert subnetLab.nixosConfigurations.pc99.config.virtualisation.virtualbox.guest.enable;
assert subnetLab.nixosConfigurations.pc01.config.virtualisation.virtualbox.guest.enable;
assert controllerOnlyLab.nixoriumUpdateTargets == [ "pc99" ];
assert subnetLab.nixoriumUpdateTargets == [ "pc99" "pc01" ];
assert plainLab.nixoriumUpdateTargets == [ "pc99" "pc01" ];
assert builtins.elem "pc02" softwareLab.nixoriumUpdateTargets;
assert (mkLab (baseArgs // { updateValidationHosts = [ "pc03" ]; })).nixoriumUpdateTargets == [ "pc99" "pc01" "pc03" ];
assert controllerOnlyLab.nixoriumOfflineCheck.drvPath != "";
assert controllerOnlyLab.labMeta.controller.staticIp == "";
assert controllerOnlyLab.labMeta.clients.count == 0;
assert controllerOnlyLab.labMeta.clients.hosts == [];
assert !(controllerOnlyLab.nixosConfigurations ? pc01);
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
assert controllerOnly.system.build.toplevel.drvPath != "";
assert subnetLab.labMeta.controller.staticIp == "10.23.4.227";
assert subnetLab.labMeta.deploymentMode == "laboratory";
assert subnetLab.deploymentStatus.controller.ready == subnetLab.deploymentStatus.ready;
assert subnetLab.deploymentStatus.controller.requiresKeys;
assert subnetLab.labMeta.network.prefixLength == 25;
assert map (host: builtins.removeAttrs host [ "ifaceName" "ifaceExplicit" "class" ]) subnetLab.labMeta.clients.hosts == [
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
# Every laboratory host has only the NetworkManager profile on its card: no
# scripted address and no dhcpcd next to NetworkManager (they raced at boot).
assert roleInterfaceLab.nixosConfigurations.pc99.config.networking.interfaces == {};
assert roleInterfaceLab.nixosConfigurations.pc99.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.interface-name == "eno1";
# pc01 shares its system with the other enp2s0 clients; pc02 alone has
# enp3s0 and keeps its own system, bound to its card in the same way.
assert roleInterfaceLab.nixosConfigurations.pc01.config.networking.interfaces == {};
assert roleInterfaceLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.interface-name == "enp2s0";
assert roleInterfaceLab.nixosConfigurations.pc02.config.networking.interfaces == {};
assert roleInterfaceLab.nixosConfigurations.pc02.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.interface-name == "enp3s0";
assert builtins.all (name: !(roleInterfaceLab.nixosConfigurations.${name}.config.systemd.services ? dhcpcd)) [ "pc99" "pc01" "pc02" ];
# Clients are configured by name only when an administrator chose one (ADR 0017);
# the template fallback name never reaches a client's network configuration.
assert !(builtins.elemAt subnetLab.labMeta.clients.hosts 0).ifaceExplicit;
assert (builtins.elemAt roleInterfaceLab.labMeta.clients.hosts 0).ifaceExplicit;
assert (builtins.elemAt roleInterfaceLab.labMeta.clients.hosts 1).ifaceExplicit;
assert !(builtins.elemAt hostOnlyInterfaceLab.labMeta.clients.hosts 0).ifaceExplicit;
assert (builtins.elemAt hostOnlyInterfaceLab.labMeta.clients.hosts 1).ifaceExplicit;
assert subnetLab.nixosConfigurations.pc01.config.networking.interfaces == {};
# Identical clients share one system; the address comes from the identity record.
assert subnetLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address1 == "$NIXORIUM_LAB_ADDRESS/25";
assert subnetLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.environmentFiles == [ "/run/nixorium/identity.env" ];
assert builtins.head subnetLab.nixosConfigurations.pc01._module.specialArgs.labSettings.clientIdentities
  == { number = 1; name = "pc01"; address = "10.23.4.129"; };
assert lib.hasInfix "1) name=pc01 address=10.23.4.129 previous= ;;" subnetLab.nixosConfigurations.pc01.config.system.activationScripts.nixoriumIdentity.text;
assert lib.hasInfix "20) name=pc20 address=10.23.4.148 previous= ;;" subnetLab.nixosConfigurations.pc01.config.system.activationScripts.nixoriumIdentity.text;
assert subnetLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.type == "ethernet";
assert !(subnetLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection ? interface-name);
assert subnetLab.nixosConfigurations.pc01.config.networking.networkmanager.settings.main.no-auto-default == "*";
# Clients answer Wake-on-LAN magic packets on their lab card; the controller
# is never switched on this way.
assert subnetLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ethernet.wake-on-lan == "64";
assert !(lib.hasInfix "wol g" subnetLab.nixosConfigurations.pc01.config.services.udev.extraRules);
assert lib.hasInfix "NAME==\"enp2s0\", RUN+=" roleInterfaceLab.nixosConfigurations.pc01.config.services.udev.extraRules;
assert lib.hasInfix "KERNEL==\"enp2s0\", RUN+=" roleInterfaceLab.nixosConfigurations.pc01.config.services.udev.extraRules;
assert lib.hasInfix "/bin/ethtool -s $name wol g" roleInterfaceLab.nixosConfigurations.pc01.config.services.udev.extraRules;
assert !(lib.hasInfix "wol g" roleInterfaceLab.nixosConfigurations.pc99.config.services.udev.extraRules);
assert !(lib.hasInfix "wol g" subnetLab.nixosConfigurations.pc99.config.services.udev.extraRules);
assert !(lib.hasInfix "iifname \"enp" subnetLab.nixosConfigurations.pc01.config.networking.firewall.extraInputRules);
assert !(lib.hasInfix "iifname \"enp" subnetLab.nixosConfigurations.pc01.config.networking.nftables.tables.nixorium-client-access.content);
assert lib.hasInfix "ip saddr 10.23.4.227 tcp dport 22 accept" subnetLab.nixosConfigurations.pc01.config.networking.firewall.extraInputRules;
# Generic Linux binaries run on the controller and every client (nix-ld).
assert subnetLab.nixosConfigurations.pc99.config.programs.nix-ld.enable
  && subnetLab.nixosConfigurations.pc01.config.programs.nix-ld.enable
  && controllerOnly.programs.nix-ld.enable;
assert builtins.all (name: builtins.elem subnetLab.nixosConfigurations.pc01.pkgs.${name}
  subnetLab.nixosConfigurations.pc01.config.programs.nix-ld.libraries) [ "zlib" "glib" "gtk3" "libgbm" ];
# The controller always keeps its configured name: its profile is bound to
# that card, and NetworkManager keeps automatic DHCP on its other cards.
assert subnetLab.nixosConfigurations.pc99.config.networking.interfaces == {};
assert subnetLab.nixosConfigurations.pc99.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.interface-name == "enp0s3";
assert subnetLab.nixosConfigurations.pc99.config.networking.networkmanager.settings.main.no-auto-default == "interface-name:enp0s3";
assert subnetLab.nixosConfigurations.pc99.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.autoconnect-priority == "100";
# An explicit client name keeps the name-based firewall; a shared system
# binds its profile to that card.
assert roleInterfaceLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address1 == "$NIXORIUM_LAB_ADDRESS/24";
assert roleInterfaceLab.nixosConfigurations.pc02.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.interface-name == "enp3s0";
assert lib.hasInfix "iifname \"enp2s0\"" roleInterfaceLab.nixosConfigurations.pc01.config.networking.firewall.extraInputRules;
assert roleInterfaceLab.nixosConfigurations.pc01.config.networking.firewall.interfaces ? enp2s0;
assert hostOnlyInterfaceLab.nixosConfigurations.pc02.config.networking.interfaces == {};
assert hostOnlyInterfaceLab.nixosConfigurations.pc02.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.interface-name == "enp3s0";
assert hostOnlyInterfaceLab.nixosConfigurations.pc01.config.networking.interfaces == {};
assert hostOnlyInterfaceLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles ? nixorium-lab;
assert hostOnlyInterfaceLab.nixosConfigurations.pc99.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.interface-name == "enp0s3";
# Without a change of laboratory network the network configuration is the
# single-network one, byte for byte (these literals are the previous output).
assert !(subnetLab.labMeta.network ? transition);
assert subnetLab.nixosConfigurations.pc01._module.specialArgs.labSettings.networkTransition == null;
assert subnetLab.nixosConfigurations.pc99.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address1 == "10.23.4.227/25";
assert !(subnetLab.nixosConfigurations.pc99.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4 ? address2);
assert subnetLab.nixosConfigurations.pc01.config.networking.firewall.extraInputRules
  == "ip saddr 10.23.4.227 tcp dport 22 accept\n";
assert subnetLab.nixosConfigurations.pc01.config.networking.nftables.tables.nixorium-client-access.content
  == "chain input {\n  type filter hook input priority -10; policy accept;\n  iifname \"lo\" accept\n  ip saddr 10.23.4.227 tcp dport 22 accept\n  tcp dport 22 drop\n  udp dport 5353 drop\n}\n";
assert roleInterfaceLab.nixosConfigurations.pc01.config.networking.firewall.extraInputRules
  == "iifname \"enp2s0\" ip saddr 10.0.0.99 tcp dport 22 accept\n";
assert labHttpSubstituters subnetLab.nixosConfigurations.pc01.config == [ "http://10.23.4.227:5000" ];
# Clients use only the controller cache; the controller keeps the public one.
assert subnetLab.nixosConfigurations.pc01.config.nix.settings.substituters == [ "http://10.23.4.227:5000" ];
assert transitionClient.nix.settings.substituters == [ "http://10.1.0.99:5000" "http://10.0.0.99:5000" ];
assert roleInterfaceLab.nixosConfigurations.pc01.config.nix.settings.substituters == [ "http://10.0.0.99:5000" ];
assert builtins.elem "https://cache.nixos.org/" subnetLab.nixosConfigurations.pc99.config.nix.settings.substituters;
assert import ../lib/client-internet-rules.nix { interface = "enp2s0"; networkCidr = "10.0.0.0/24"; } == ''
  table inet nixorium_internet {
    chain output {
      type filter hook output priority -10; policy drop;
      oifname "lo" accept
      oifname "enp2s0" ip daddr 10.0.0.0/24 accept
  # Preserve address renewal without permitting ordinary Internet traffic.
  oifname "enp2s0" udp sport 68 udp dport 67 accept
  oifname "enp2s0" ip6 daddr ff02::1:2 udp sport 546 udp dport 547 accept
  oifname "enp2s0" icmpv6 type { nd-router-solicit, nd-neighbor-solicit, nd-neighbor-advert } accept

    }
    chain forward {
      type filter hook forward priority -10; policy drop;
      oifname "enp2s0" ip daddr 10.0.0.0/24 accept
    }
  }
'';
# During a change of laboratory network the controller carries both addresses
# on its laboratory interface; the interface-scoped firewall and the cache
# listening on every address cover both.
assert transitionController.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address1 == "10.1.0.99/24";
assert transitionController.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address2 == "10.0.0.99/24";
assert builtins.all (port: builtins.elem port transitionController.networking.firewall.interfaces.enp0s3.allowedTCPPorts) [ 22 5000 8080 ];
assert builtins.all (port: builtins.elem port transitionController.networking.firewall.interfaces.enp0s3.allowedUDPPorts) [ 67 69 4011 123 ];
assert transitionController.services.harmonia.cache.settings.bind == "[::]:5000";
assert transitionLab.labMeta.controller.staticIp == "10.1.0.99";
assert transitionLab.labMeta.network.base == "10.1.0.0";
assert transitionLab.labMeta.network.transition == {
  base = "10.0.0.0";
  prefixLength = 24;
  controllerIp = "10.0.0.99";
  startedAt = "2026-10-15T08:00:00Z";
  clients = builtins.genList (index: {
    name = "pc${if index < 9 then "0" else ""}${toString (index + 1)}";
    ip = "10.0.0.${toString (index + 1)}";
  }) 20;
};
assert (builtins.head transitionLab.labMeta.clients.hosts).ip == "10.1.0.1";
assert removeAttrs transitionLab.nixosConfigurations.pc01._module.specialArgs.labSettings.networkTransition [ "hostIps" ]
  == { base = "10.0.0.0"; prefixLength = 24; masterIp = "10.0.0.99"; };
assert transitionLab.nixosConfigurations.pc01._module.specialArgs.labSettings.networkTransition.hostIps.pc20 == "10.0.0.20";
# Clients take the new address and keep the previous one, accept SSH from
# both controller addresses and try the new controller cache first; the
# running card takes the changed profile at once.
assert transitionClient.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address1 == "$NIXORIUM_LAB_ADDRESS/24";
assert transitionClient.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address2 == "$NIXORIUM_PREVIOUS_ADDRESS/24";
assert builtins.head transitionLab.nixosConfigurations.pc01._module.specialArgs.labSettings.clientIdentities
  == { number = 1; name = "pc01"; address = "10.1.0.1"; previousAddress = "10.0.0.1"; };
assert lib.hasInfix "nmcli device reapply" transitionClient.systemd.services.NetworkManager-ensure-profiles.postStart;
assert !(lib.hasInfix "nmcli device reapply" movedLab.nixosConfigurations.pc01.config.systemd.services.NetworkManager-ensure-profiles.postStart);
assert !(movedLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4 ? address2);
assert transitionClient.networking.firewall.extraInputRules
  == "ip saddr 10.1.0.99 tcp dport 22 accept\nip saddr 10.0.0.99 tcp dport 22 accept\n";
assert transitionClient.networking.nftables.tables.nixorium-client-access.content
  == "chain input {\n  type filter hook input priority -10; policy accept;\n  iifname \"lo\" accept\n  ip saddr 10.1.0.99 tcp dport 22 accept\n  ip saddr 10.0.0.99 tcp dport 22 accept\n  tcp dport 22 drop\n  udp dport 5353 drop\n}\n";
assert labHttpSubstituters transitionClient == [ "http://10.1.0.99:5000" "http://10.0.0.99:5000" ];
assert transitionNamedLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address2 == "$NIXORIUM_PREVIOUS_ADDRESS/24";
assert transitionNamedLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.connection.interface-name == "enp2s0";
assert transitionNamedLab.nixosConfigurations.pc01.config.networking.firewall.extraInputRules
  == "iifname \"enp2s0\" ip saddr 10.1.0.99 tcp dport 22 accept\niifname \"enp2s0\" ip saddr 10.0.0.99 tcp dport 22 accept\n";
# The classroom Internet block keeps both laboratory networks reachable.
assert transitionClient.systemd.services.nixorium-internet-block.serviceConfig.ExecStart
  != movedLab.nixosConfigurations.pc01.config.systemd.services.nixorium-internet-block.serviceConfig.ExecStart;
assert lib.hasInfix "ip daddr 10.1.0.0/24 accept\nip daddr 10.0.0.0/24 accept\n"
  (import ../lib/client-internet-rules.nix { interface = null; networkCidr = "10.1.0.0/24"; previousNetworkCidr = "10.0.0.0/24"; });
assert lib.hasInfix "  oifname \"enp2s0\" ip daddr 10.1.0.0/24 accept\n    oifname \"enp2s0\" ip daddr 10.0.0.0/24 accept\n  }"
  (import ../lib/client-internet-rules.nix { interface = "enp2s0"; networkCidr = "10.1.0.0/24"; previousNetworkCidr = "10.0.0.0/24"; });
# Invalid previous networks are refused; without clients the record is ignored.
assert transitionRejected (previousNetwork // { previousBase = "10.1.0.0"; });
assert transitionRejected (previousNetwork // { previousPrefixLength = 15; });
assert transitionRejected (previousNetwork // { previousBase = "10.1.0.128"; previousPrefixLength = 25; });
assert transitionRejected (previousNetwork // { previousPrefixLength = 26; });
assert transitionRejected (previousNetwork // { startedAt = "yesterday"; });
assert !(transitionRejected previousNetwork);
assert !(ignoredTransitionLab.labMeta.network ? transition);
assert ignoredTransitionLab.nixosConfigurations.pc99._module.specialArgs.labSettings.networkTransition == null;
assert builtins.elem
  "lab.networkTransition is ignored because no client computers are configured; remove it from lab-settings.json"
  ignoredTransitionLab.nixosConfigurations.pc99.config.warnings;
assert !(builtins.any (lib.hasInfix "networkTransition") controllerOnly.warnings);
# Time (modules/time.nix). The controller runs chrony, serves the laboratory
# network only and listens on its laboratory address; UDP 123 is open on the
# laboratory interface only (a client opens no port: it is a client). Every
# client takes the time from the controller alone, with no Internet pool even
# as a fallback; controller-only mode keeps the NixOS defaults.
assert subnetLab.nixosConfigurations.pc99.config.services.chrony.enable;
assert !subnetLab.nixosConfigurations.pc99.config.services.timesyncd.enable;
assert subnetLab.nixosConfigurations.pc99.config.services.chrony.extraConfig
  == "allow 10.23.4.128/25\nlocal stratum 10 waitunsynced 300\nbindaddress 10.23.4.227\n";
assert builtins.elem 123 controllerUDP;
assert !(builtins.elem 123 (allInterfaceUDP subnetLab.nixosConfigurations.pc01.config.networking.firewall));
assert subnetLab.nixosConfigurations.pc99.config.services.chrony.servers
  == subnetLab.nixosConfigurations.pc99.config.networking.timeServers;
assert builtins.length subnetLab.nixosConfigurations.pc99.config.services.chrony.servers > 0;
assert !subnetLab.nixosConfigurations.pc01.config.services.chrony.enable;
assert subnetLab.nixosConfigurations.pc01.config.services.timesyncd.enable;
assert subnetLab.nixosConfigurations.pc01.config.services.timesyncd.servers == [ "10.23.4.227" ];
assert subnetLab.nixosConfigurations.pc01.config.services.timesyncd.fallbackServers == [ ];
assert subnetLab.nixosConfigurations.pc01.config.networking.timeServers == [ "10.23.4.227" ];
assert subnetLab.nixosConfigurations.pc01.config.environment.etc."systemd/timesyncd.conf".text
  == "[Time]\nNTP=10.23.4.227\nFallbackNTP=\n";
assert roleInterfaceLab.nixosConfigurations.pc01.config.services.timesyncd.servers == [ "10.0.0.99" ];
assert roleInterfaceLab.nixosConfigurations.pc01.config.services.timesyncd.fallbackServers == [ ];
assert lib.hasInfix "allow 10.0.0.0/24\n" roleInterfaceLab.nixosConfigurations.pc99.config.services.chrony.extraConfig;
assert !controllerOnly.services.chrony.enable;
assert controllerOnly.services.timesyncd.enable;
assert controllerOnly.services.timesyncd.servers == null;
assert controllerOnly.services.timesyncd.fallbackServers == controllerOnly.networking.timeServers;
assert builtins.length controllerOnly.networking.timeServers > 0;
# During a change of laboratory network the controller serves both networks
# and clients list the new controller address first and the previous second.
assert transitionController.services.chrony.extraConfig
  == "allow 10.1.0.0/24\nallow 10.0.0.0/24\nlocal stratum 10 waitunsynced 300\n";
assert transitionClient.services.timesyncd.servers == [ "10.1.0.99" "10.0.0.99" ];
assert transitionClient.services.timesyncd.fallbackServers == [ ];
assert movedLab.nixosConfigurations.pc99.config.services.chrony.extraConfig
  == "allow 10.1.0.0/24\nlocal stratum 10 waitunsynced 300\nbindaddress 10.1.0.99\n";
assert movedLab.nixosConfigurations.pc01.config.services.timesyncd.servers == [ "10.1.0.99" ];
# Temporary Internet block rules: name-based only for explicit names.
assert builtins.length (lib.splitString "oifname" (import ../lib/client-internet-rules.nix { interface = null; networkCidr = "10.0.0.0/24"; })) == 2;
assert lib.hasInfix "oifname \"enp2s0\" ip daddr 10.0.0.0/24 accept" (import ../lib/client-internet-rules.nix { interface = "enp2s0"; networkCidr = "10.0.0.0/24"; });
assert lib.hasInfix "ip daddr 10.0.0.0/24 accept" (import ../lib/client-internet-rules.nix { interface = null; networkCidr = "10.0.0.0/24"; });
# The console follows the XKB layout; a legacy consoleKeyMap ("it2" here) is
# accepted but ignored, and plainLab omits it entirely.
assert keyboardLab.nixosConfigurations.pc99.config.services.xserver.xkb.layout == "it";
assert keyboardLab.nixosConfigurations.pc99.config.console.useXkbConfig;
assert keyboardLab.nixosConfigurations.pc99.config.console.keyMap != "it2";
assert keyboardLab.nixosConfigurations.netboot.config.services.xserver.xkb.layout == "it";
assert keyboardLab.nixosConfigurations.netboot.config.console.useXkbConfig;
assert keyboardLab.nixosConfigurations.netboot.config.console.keyMap != "it2";
assert plainLab.nixosConfigurations.pc99.config.console.useXkbConfig;
assert plainLab.nixosConfigurations.pc99.config.services.xserver.xkb.layout == "us";
# The netboot console starts the guided installer once; it stays interactive.
assert lib.hasInfix "sudo /installer/setup.sh" keyboardLab.nixosConfigurations.netboot.config.programs.bash.loginShellInit;
assert lib.hasInfix "/dev/tty1" keyboardLab.nixosConfigurations.netboot.config.programs.bash.loginShellInit;
assert keyboardLab.nixosConfigurations.netboot.config.services.getty.autologinUser == "nixos";
assert !(roleInterfaceLab.nixosConfigurations.pc99.config.systemd.services ? "network-addresses-eno1");
assert !(roleInterfaceLab.nixosConfigurations.pc01.config.systemd.services ? "network-addresses-enp2s0");
assert !(roleInterfaceLab.nixosConfigurations.pc02.config.systemd.services ? "network-addresses-enp3s0");
assert roleInterfaceLab.nixosConfigurations.pc99.config.networking.firewall.interfaces ? eno1;
assert roleInterfaceLab.nixosConfigurations.pc01.config.networking.firewall.interfaces ? enp2s0;
assert roleInterfaceLab.nixosConfigurations.pc02.config.networking.firewall.interfaces ? enp3s0;
# Deployed clients keep the role/host interface, not the shared fallback.
assert !(roleInterfaceLab.nixosConfigurations.pc01.config.systemd.services ? "network-addresses-enp0s3");
# No Colmena hive: Nixorium deploys the systems itself (ADR 0031).
assert !(subnetLab ? colmena);
# Classes of client computers (ADR 0033).
assert classesSystem "pc01" == classesSystem "pc02";
assert classesSystem "pc01" == classesSystem "pc20";
assert classesSystem "pc03" != classesSystem "pc01";
assert lib.hasInfix "-nixos-system-nixorium-client-shared-1-" (classesSystem "pc01");
assert lib.hasInfix "-nixos-system-pc03-" (classesSystem "pc03");
assert classesLab.labMeta.clients.classes == [
  { id = "shared-1"; shared = true; members = [ "pc01" "pc02" "pc04" "pc05" "pc06" "pc07" "pc08" "pc09" "pc10"
      "pc11" "pc12" "pc13" "pc14" "pc15" "pc16" "pc17" "pc18" "pc19" "pc20" ]; }
  { id = "pc03"; shared = false; members = [ "pc03" ]; }
];
assert map (host: host.class) (lib.take 4 classesLab.labMeta.clients.hosts) == [ "shared-1" "shared-1" "pc03" "shared-1" ];
assert classesLab.nixosConfigurations.pc01.config.networking.hostName == "nixorium-client";
assert !classesLab.nixosConfigurations.pc01.config.environment.etc.hostname.enable;
assert classesLab.nixosConfigurations.pc01.config.services.avahi.hostName == "";
assert classesLab.nixosConfigurations.pc01.config.networking.networkmanager.settings.main.hostname-mode == "none";
assert classesLab.nixosConfigurations.pc01.config.systemd.services ? nixorium-identity;
assert classesLab.nixosConfigurations.pc01.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.dad-timeout == "3000";
# A class of one keeps the per-host system of earlier releases.
assert classesLab.nixosConfigurations.pc03.config.networking.hostName == "pc03";
assert classesLab.nixosConfigurations.pc03.config.networking.networkmanager.ensureProfiles.profiles.nixorium-lab.ipv4.address1 == "10.0.0.3/24";
assert !(classesLab.nixosConfigurations.pc03.config.systemd.services ? nixorium-identity);
assert lib.hasInfix "printf '{\"schemaVersion\":1,\"number\":%d}\\n' 3" classesLab.nixosConfigurations.pc03.config.system.activationScripts.nixoriumIdentity.text;
assert classesLab.nixosConfigurations.pc03.config.environment.etc ? "pc03-only";
assert !(classesLab.nixosConfigurations.pc01.config.environment.etc ? "pc03-only");
assert !(builtins.tryEval hostNameLab.nixosConfigurations.pc01.config.environment.etc.who.text).success;
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
assert hasCleanGenerations subnetLab.nixosConfigurations.pc99.config.environment.systemPackages;
assert hasCleanGenerations subnetLab.nixosConfigurations.pc01.config.environment.systemPackages;
assert subnetLab.nixosConfigurations.pc01.config.boot.loader.grub.configurationLimit == 10;
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-clean-generations@";
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-clean-generations@");
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
# The student home is restored from the workspace seed, not repaired here.
assert !(subnetLab.nixosConfigurations.pc01.pkgs.lib.hasInfix
  ''HOME_DIR="/home/${labConfig.studentUser}"'' clientHomeOwnership.text);
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
# The experimental classroom view agent exists only on clients, and only when enabled.
assert classroomViewLab.nixosConfigurations.pc01.config.systemd.user.services ? nixorium-classroom-agent;
assert !(classroomViewLab.nixosConfigurations.pc99.config.systemd.user.services ? nixorium-classroom-agent);
# The classroom view is on unless a laboratory turns it off.
assert subnetLab.nixosConfigurations.pc01.config.systemd.user.services ? nixorium-classroom-agent;
assert !((mkLab (baseArgs // { labConfig = labConfig // { classroomView = false; }; })).nixosConfigurations.pc01.config.systemd.user.services ? nixorium-classroom-agent);
# A student cannot stop the agent or its extension helper from the terminal
# of the session: both refuse a manual stop and never run out of starts; the
# agent also restarts after one second however it ends (systemd rejects
# Restart=always for the oneshot helper).
assert (
  let
    units = classroomViewLab.nixosConfigurations.pc01.config.systemd.user.services;
    agent = units.nixorium-classroom-agent;
    helper = units.nixorium-classroom-extension;
  in
  agent.serviceConfig.Restart == "always"
  && agent.serviceConfig.RestartSec == 1
  && helper.serviceConfig.Restart == "on-failure"
  && helper.serviceConfig.RestartSec == 1
  && lib.all (unit: unit.unitConfig.RefuseManualStop == true && unit.unitConfig.StartLimitIntervalSec == 0) [ agent helper ]
);
# It adds no network port: the controller reaches it through SSH.
assert lib.hasInfix "tcp dport 22 accept"
  classroomViewLab.nixosConfigurations.pc01.config.networking.firewall.extraInputRules;
# The controller records whether it offers the classroom view; clients do not.
assert builtins.fromJSON classroomViewLab.nixosConfigurations.pc99.config.environment.etc."nixorium/classroom-view.json".text
  == { schemaVersion = 1; enabled = true; };
# The agent of the view opens the file without following links, so it must
# stay a regular file.
assert classroomViewLab.nixosConfigurations.pc99.config.environment.etc."nixorium/classroom-view.json".mode == "0444";
assert (builtins.fromJSON (mkLab (baseArgs // { labConfig = labConfig // { classroomView = false; }; })).nixosConfigurations.pc99.config.environment.etc."nixorium/classroom-view.json".text).enabled == false;
assert !(classroomViewLab.nixosConfigurations.pc01.config.environment.etc ? "nixorium/classroom-view.json");
# The teacher's icon stays in both states (with the view off it opens the
# Nixorium menu); the file chooser of Send files is installed only with it.
assert
  let
    has = name: lab: builtins.any (package: lib.hasPrefix name (package.name or ""))
      lab.nixosConfigurations.pc99.config.environment.systemPackages;
    offLab = mkLab (baseArgs // { labConfig = labConfig // { classroomView = false; }; });
  in
  has "nixorium-classroom" classroomViewLab && has "nixorium-classroom" offLab
  && has "zenity" classroomViewLab && !(has "zenity" offLab);
# Assistant defaults on for controllers; explicit false and client roles stay off.
assert plainLab.nixosConfigurations.pc99.config.environment.etc ? "nixorium/assistant.json";
assert controllerOnly.environment.etc ? "nixorium/assistant.json";
assert !(plainLab.nixosConfigurations.pc01.config.environment.etc ? "nixorium/assistant.json");
assert !(assistantOffLab.nixosConfigurations.pc99.config.environment.etc ? "nixorium/assistant.json");
assert !(assistantLab.nixosConfigurations.pc01.config.environment.etc ? "nixorium/assistant.json");
assert assistantLab.nixosConfigurations.pc99.config.environment.etc."nixorium/assistant.json".mode == "0444";
assert builtins.fromJSON assistantLab.nixosConfigurations.pc99.config.environment.etc."nixorium/assistant.json".text == {
  schemaVersion = 1;
  enabled = true;
  endpoint = "https://assistant.nixorium.org/v1";
  subscribeUrl = "https://nixorium.org/pricing/";
};
assert builtins.fromJSON assistantTestLab.nixosConfigurations.pc99.config.environment.etc."nixorium/assistant.json".text == {
  schemaVersion = 1;
  enabled = true;
  endpoint = "http://127.0.0.1:18480/v1";
  subscribeUrl = "https://subscribe.example.org/";
};
assert builtins.elem "d /var/lib/nixorium/assistant 0700 admin users -"
  assistantLab.nixosConfigurations.pc99.config.systemd.tmpfiles.rules;
assert builtins.elem "d /var/lib/nixorium/assistant 0700 admin users -"
  plainLab.nixosConfigurations.pc99.config.systemd.tmpfiles.rules;
assert !(builtins.elem "d /var/lib/nixorium/assistant 0700 admin users -"
  assistantLab.nixosConfigurations.pc01.config.systemd.tmpfiles.rules);
assert !(builtins.elem "d /var/lib/nixorium/assistant 0700 admin users -"
  assistantOffLab.nixosConfigurations.pc99.config.systemd.tmpfiles.rules);
# Base software is installed whatever lab-software.json declares.
assert builtins.all (host: builtins.all (pname: hasPackage subnetLab host pname)
  [ "chromium" "ghostty" "git" "terminaltexteffects" ]) [ "pc99" "pc01" ];
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
assert subnetLab.nixosConfigurations.pc99.config.systemd.services ? "nixorium-classroom";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services.nixorium-classroom.serviceConfig.User == "admin";
assert subnetLab.nixosConfigurations.pc99.config.systemd.services.nixorium-classroom.serviceConfig.Group == "nixorium-classroom";
assert builtins.elem "nixorium-classroom" subnetLab.nixosConfigurations.pc99.config.users.users.${labConfig.teacherUser}.extraGroups;
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-classroom");
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
assert builtins.all (port: builtins.elem port controllerTCP) [ 22 5000 8080 ];
assert builtins.length controllerTCP == 3;
assert builtins.all (port: builtins.elem port controllerUDP) [ 67 69 4011 5353 123 ];
assert builtins.length controllerUDP == 5;
assert !subnetLab.nixosConfigurations.pc01.config.services.harmonia.cache.enable;
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-prepare-pxe");
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-restart-cache");
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-pxe-network");
assert !(subnetLab.nixosConfigurations.pc01.config.systemd.services ? "nixorium-pxe");
assert clientFirewall.enable;
assert clientFirewall.allowedTCPPorts == [];
assert clientFirewall.allowedUDPPorts == [];
assert clientTCP == [];
assert allInterfaceUDP clientFirewall == [];
assert allInterfaceUDP roleInterfaceLab.nixosConfigurations.pc01.config.networking.firewall == [];
assert !(clientFirewall.interfaces ? enp0s3);
assert nativeClientTCP == [];
assert !(controllerOnly.systemd.services ? nixorium-internet-block);
assert !(subnetLab.nixosConfigurations.pc99.config.systemd.services ? nixorium-internet-block);
assert subnetLab.nixosConfigurations.pc01.config.systemd.services.nixorium-internet-block.wantedBy == [];
assert !subnetLab.nixosConfigurations.pc01.config.systemd.services.nixorium-internet-block.restartIfChanged;
assert subnetLab.nixosConfigurations.pc01.config.systemd.services.nixorium-internet-block.serviceConfig.RemainAfterExit;
assert subnetLab.nixosConfigurations.pc01.config.networking.nftables.enable;
assert !subnetLab.nixosConfigurations.pc01.config.networking.nftables.flushRuleset;
assert lib.hasInfix "10.23.4.227" clientFirewall.extraInputRules;
assert lib.hasInfix "tcp dport 22 accept" clientFirewall.extraInputRules;
assert !(lib.hasInfix "11100" clientFirewall.extraInputRules);
assert lib.hasInfix "priority -10" subnetLab.nixosConfigurations.pc01.config.networking.nftables.tables.nixorium-client-access.content;
assert !(subnetLab.nixosConfigurations.pc99.config.networking.nftables.tables ? nixorium-client-access);
assert !subnetLab.nixosConfigurations.pc99.config.systemd.user.services.gnome-remote-desktop.enable;
assert !nativeClient.systemd.user.services.gnome-remote-desktop.enable;
assert nativeClient.users.groups ? nixorium-staff;
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
assert builtins.all
  (preset: builtins.all
    (package: builtins.elem package preset.packages == (preset.id == "programming"))
    [ "opencode" "pi-coding-agent" ])
  sitePresetCatalog.presets;
assert builtins.all (item: item != null && item.availability == "available") siteProfileResolved;
assert siteProfilesEvaluate;
assert builtins.any (item: item.id == "hello" && item.availability == "available") softwareSearch;
assert nestedSoftware.id == "python3Packages.numpy";
assert nestedSoftware.availability == "available";
assert allowedUnfreeSoftware.availability == "available";
assert bambuStudioSoftware.availability == "available";
assert builtins.all
  (config: builtins.all (module: builtins.elem module config.boot.initrd.availableKernelModules)
    [ "virtio_blk" "virtio_pci" "virtio_scsi" ])
  [ subnetLab.nixosConfigurations.pc99.config subnetLab.nixosConfigurations.pc01.config ];
assert rejectsUnknownHost;
assert rejectsInvalidSoftwareCatalog;
assert rejectsUnsafeHomeResetPath;
true
