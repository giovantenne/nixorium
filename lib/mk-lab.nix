{ upstreamSelf, nixpkgs, disko, veyon }:
{
  deploymentSelf,
  labConfig,
  publicKeys ? {},
  assets ? {},
  sharedModules ? [],
  controllerModules ? [],
  clientModules ? [],
  hostModules ? {},
  labSoftware ? { schemaVersion = 1; packages = []; },
  clientGroups ? {},
  netbootModules ? [],
  installerSource ? null,
  nixosVersionMetadata ? null,
  deploymentRevision ? null
}:
let
  lib = nixpkgs.lib;
  upstreamRoot = upstreamSelf.outPath;
  deploymentRoot = if builtins.isAttrs deploymentSelf then deploymentSelf.outPath else deploymentSelf;
  inferredDeploymentRevision =
    if builtins.isAttrs deploymentSelf then
      deploymentSelf.rev or deploymentSelf.dirtyRev or null
    else
      null;
  effectiveDeploymentRevision =
    if deploymentRevision != null then deploymentRevision else inferredDeploymentRevision;
  version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile (upstreamRoot + "/VERSION"));
  config = import ./eval-lab-config.nix { inherit lib; } labConfig;

  fileExists = file: file != null && builtins.pathExists file;
  readSingleLine = file:
    if fileExists file then
      builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile file)
    else
      null;

  configuredCachePublicKeyFile = publicKeys.cache or ../public-key;
  configuredAdminSshKeyFile = publicKeys.ssh or ../id_ed25519.pub;
  configuredVeyonPublicKeyFile = publicKeys.veyon or ../veyon-public-key.pem;
  cachePublicKeyFile = if fileExists configuredCachePublicKeyFile then configuredCachePublicKeyFile else null;
  adminSshKeyFile = if fileExists configuredAdminSshKeyFile then configuredAdminSshKeyFile else null;
  veyonPublicKeyFile = if fileExists configuredVeyonPublicKeyFile then configuredVeyonPublicKeyFile else null;
  cachePublicKey = readSingleLine cachePublicKeyFile;
  adminSshKey = readSingleLine adminSshKeyFile;

  labAssets = {
    logo = assets.logo or ../assets/logo.txt;
    backgrounds = assets.backgrounds or [
      ../assets/backgrounds/1-ristretto.jpg
      ../assets/backgrounds/2-ristretto.jpg
      ../assets/backgrounds/3-ristretto.jpg
    ];
    mimeApps = assets.mimeApps or ../assets/mimeapps.list;
    vscodeSettings = assets.vscodeSettings or ../assets/vscode-settings.json;
  };

  inherit (config) masterDhcpIp;
  inherit (config) networkBase;
  inherit (config) networkPrefixLength;
  inherit (config) pcCount;
  inherit (config) masterHostNumber;
  inherit (config) ifaceName;
  inherit (config) teacherUser;
  inherit (config) studentUser;
  inherit (config) teacherPassword;
  inherit (config) studentPassword;
  inherit (config) adminPassword;
  inherit (config) homepageUrl;
  inherit (config) studentGitName;
  inherit (config) studentGitEmail;
  inherit (config) adminGitName;
  inherit (config) adminGitEmail;
  inherit (config) timeZone;
  inherit (config) defaultLocale;
  inherit (config) extraLocale;
  inherit (config) keyboardLayout;
  inherit (config) consoleKeyMap;
  inherit (config) veyonNativeHosts;

  networkOctets = map lib.toInt (lib.splitString "." networkBase);
  networkAddress =
    builtins.elemAt networkOctets 0 * 16777216
      + builtins.elemAt networkOctets 1 * 65536
      + builtins.elemAt networkOctets 2 * 256
      + builtins.elemAt networkOctets 3;
  ipv4FromInt = address:
    builtins.concatStringsSep "." (map toString [
      (builtins.div address 16777216)
      (lib.mod (builtins.div address 65536) 256)
      (lib.mod (builtins.div address 256) 256)
      (lib.mod address 256)
    ]);
  mkHostIp = number: ipv4FromInt (networkAddress + number);
  padNumber = n: if n < 10 then "0${toString n}" else toString n;
  masterHostName = "pc${padNumber masterHostNumber}";
  masterIp = mkHostIp masterHostNumber;
  cachePort = 5000;
  pxeHttpPort = 8080;
  system = "x86_64-linux";
  pcNumbers = builtins.genList (n: n + 1) pcCount;
  clientNumbers = pcNumbers;
  clientIps = map mkHostIp clientNumbers;

  veyonWaylandOverlay = final: prev: {
    veyon = prev.veyon.overrideAttrs (oldAttrs: {
      buildInputs = (oldAttrs.buildInputs or []) ++ [ final.pipewire ];
      postPatch = (oldAttrs.postPatch or "") + ''
        substituteInPlace plugins/platform/linux/input-helper/CMakeLists.txt \
          --replace-fail 'OWNER_READ OWNER_WRITE OWNER_EXECUTE SETUID' \
                         'OWNER_READ OWNER_WRITE OWNER_EXECUTE'
      '';
      postInstall = (oldAttrs.postInstall or "") + ''
        if [ ! -f "$out/lib/veyon/pipewire-vnc-server.so" ]; then
          echo "ERROR: Veyon PipeWire VNC plugin was not built" >&2
          exit 1
        fi
      '';
    });
  };

  labOverlay = lib.composeManyExtensions [
    veyon.overlays.default
    veyonWaylandOverlay
    (final: prev: {
      gnome-remote-desktop = import (upstreamRoot + "/pkgs/gnome-remote-desktop.nix") { inherit prev; };
    })
  ];

  labSettings = {
    inherit masterIp;
    inherit masterDhcpIp;
    inherit masterHostName;
    inherit masterHostNumber;
    inherit networkBase;
    inherit networkPrefixLength;
    inherit clientIps;
    inherit pcCount;
    inherit ifaceName;
    inherit teacherUser;
    inherit studentUser;
    inherit teacherPassword;
    inherit studentPassword;
    inherit adminPassword;
    inherit adminSshKey;
    inherit homepageUrl;
    inherit studentGitName;
    inherit studentGitEmail;
    inherit adminGitName;
    inherit adminGitEmail;
    inherit timeZone;
    inherit defaultLocale;
    inherit extraLocale;
    inherit keyboardLayout;
    inherit consoleKeyMap;
    inherit veyonNativeHosts;
    inherit cachePublicKey;
    inherit cachePort;
    inherit pxeHttpPort;
    inherit veyonPublicKeyFile;
  };

  baseHostModules = [
    { nixpkgs.overlays = [ labOverlay ]; }
    ({ lib, ... }:
      {
        warnings =
          lib.optional (cachePublicKeyFile == null) "Missing cache public key"
          ++ lib.optional (adminSshKeyFile == null) "Missing admin SSH public key"
          ++ lib.optional (veyonPublicKeyFile == null) "Missing Veyon public key";
        environment.systemPackages = [ hostState ];
      }
      // lib.optionalAttrs (effectiveDeploymentRevision != null) {
        system.configurationRevision = effectiveDeploymentRevision;
      })
    disko.nixosModules.disko
    (upstreamRoot + "/disko-uefi.nix")
    (upstreamRoot + "/modules/hardware.nix")
    (upstreamRoot + "/modules/common.nix")
    (upstreamRoot + "/modules/users.nix")
    (upstreamRoot + "/modules/networking.nix")
    (upstreamRoot + "/modules/cache.nix")
    (upstreamRoot + "/modules/filesystems.nix")
    (upstreamRoot + "/modules/home-reset.nix")
    (upstreamRoot + "/modules/docker.nix")
    (upstreamRoot + "/modules/development.nix")
    (upstreamRoot + "/modules/veyon.nix")
    (upstreamRoot + "/modules/management.nix")
    (upstreamRoot + "/modules/pxe.nix")
  ] ++ lib.optional (nixosVersionMetadata != null) ({ lib, ... }: {
    system.nixos.versionSuffix = lib.mkForce nixosVersionMetadata.versionSuffix;
    system.nixos.revision = lib.mkForce nixosVersionMetadata.revision;
  });

  modulesForHost = name: isController:
    baseHostModules
    ++ sharedModules
    ++ lib.optionals isController controllerModules
    ++ lib.optionals (!isController) ([ managedSoftwareModule ] ++ clientModules)
    ++ (hostModules.${name} or []);

  specialArgsForHost = name: hostIp: {
    inherit labSettings;
    inherit labAssets;
    inherit nixoriumPackage;
    inherit hostIp;
    hostName = name;
  };

  labMeta = {
    schemaVersion = 2;
    inherit version;
    controller = {
      name = masterHostName;
      number = masterHostNumber;
      staticIp = masterIp;
      dhcpIp = masterDhcpIp;
    };
    clients = {
      count = pcCount;
      hosts = map (n: {
        name = "pc${padNumber n}";
        ip = mkHostIp n;
      }) clientNumbers;
      groups = clientGroups;
    };
    network = {
      base = networkBase;
      prefixLength = networkPrefixLength;
      inherit ifaceName;
      inherit cachePort;
      inherit pxeHttpPort;
    };
    users = {
      student = studentUser;
      teacher = teacherUser;
    };
  };
  labMetaJson = builtins.toFile "nixorium-installer-lab-meta.json" (builtins.toJSON labMeta);

  renderPath = value:
    let
      valueString = toString value;
      deploymentPrefix = "${toString deploymentRoot}/";
      upstreamPrefix = "${toString upstreamRoot}/";
      preservePathType = expression:
        if builtins.isPath value then
          "(/. + builtins.unsafeDiscardStringContext ${expression})"
        else
          expression;
    in
    if value == null then
      "null"
    else if lib.hasPrefix deploymentPrefix valueString then
      preservePathType "(site + ${builtins.toJSON (lib.removePrefix (toString deploymentRoot) valueString)})"
    else if lib.hasPrefix upstreamPrefix valueString then
      preservePathType "(nixorium + ${builtins.toJSON (lib.removePrefix (toString upstreamRoot) valueString)})"
    else
      throw "mkLab files must be located inside the deployment or nixorium source tree: ${valueString}";
  renderPathList = values: "[ ${builtins.concatStringsSep " " (map renderPath values)} ]";
  renderHostModules = "{\n${builtins.concatStringsSep "\n" (lib.mapAttrsToList
    (name: modules: "    ${builtins.toJSON name} = ${renderPathList modules};")
    hostModules)}\n  }";
  labConfigJson = builtins.toFile "lab-config.json" (builtins.toJSON config);
  labSoftwareJson = builtins.toFile "lab-software.json" (builtins.toJSON labSoftwareConfig);
  extensionModules = sharedModules
    ++ controllerModules
    ++ clientModules
    ++ netbootModules
    ++ lib.concatLists (lib.attrValues hostModules);
  isModulePath = module: builtins.isPath module || builtins.isString module;
  unknownPublicKeyNames = builtins.attrNames (builtins.removeAttrs publicKeys [ "cache" "ssh" "veyon" ]);
  unknownAssetNames = builtins.attrNames (builtins.removeAttrs assets [ "logo" "backgrounds" "mimeApps" "vscodeSettings" ]);
  validClientNames = map (n: "pc${padNumber n}") pcNumbers;
  validHostNames = validClientNames ++ [ masterHostName ];
  unknownHostModuleNames = builtins.attrNames (builtins.removeAttrs hostModules validHostNames);
  unknownVeyonNativeHosts = builtins.filter (name: !builtins.elem name validHostNames) veyonNativeHosts;
  defaultPasswordHash = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0";
  deploymentIssues =
    lib.optional (masterDhcpIp == "MASTER_DHCP_IP") "masterDhcpIp still uses the template placeholder"
    ++ lib.optional (cachePublicKeyFile == null) "cache public key is missing"
    ++ lib.optional (adminSshKeyFile == null) "admin SSH public key is missing"
    ++ lib.optional (veyonPublicKeyFile == null) "Veyon public key is missing"
    ++ lib.optional (teacherPassword == defaultPasswordHash) "teacherPassword still uses the public default"
    ++ lib.optional (studentPassword == defaultPasswordHash) "studentPassword still uses the public default"
    ++ lib.optional (adminPassword == defaultPasswordHash) "adminPassword still uses the public default";
  sourceNixosVersionMetadata =
    if nixpkgs ? rev && nixpkgs ? lastModifiedDate then
      {
        versionSuffix = ".${builtins.substring 0 8 nixpkgs.lastModifiedDate}.${nixpkgs.shortRev}";
        revision = nixpkgs.rev;
      }
    else if nixosVersionMetadata != null then
      nixosVersionMetadata
    else
      {
        versionSuffix = ".local";
        revision = null;
      };
  bootstrapPkgs = import nixpkgs {
    inherit system;
  };
  softwareCatalog = import ./software-catalog.nix {
    inherit lib;
    pkgs = bootstrapPkgs;
  };
  labSoftwareConfig = import ./eval-lab-software.nix {
    inherit lib;
    pkgs = bootstrapPkgs;
    clientNames = validClientNames;
    inherit clientGroups;
    allowedPackages = map (entry: entry.id) softwareCatalog;
  } labSoftware;
  softwareAppliesTo = name: scope:
    scope.kind == "all-clients"
    || (scope.kind == "group" && builtins.elem name clientGroups.${scope.group})
    || (scope.kind == "clients" && builtins.elem name scope.clients);
  managedSoftwareModule = { pkgs, hostName, ... }: {
    environment.systemPackages = map
      (entry: builtins.getAttr entry.package pkgs)
      (builtins.filter (entry: softwareAppliesTo hostName entry.scope) labSoftwareConfig.packages);
  };
  installerDiskoRuntimePackages = disko.lib.packages {
    disko.devices = import (upstreamRoot + "/lib/disko-layout.nix") {
      device = "/dev/nixorium-install-target";
      inherit studentUser;
    };
  } bootstrapPkgs;
  installerDiskoScript = disko.lib._cliDestroyFormatMountNoDeps {
    disko.devices = import (upstreamRoot + "/lib/disko-layout.nix") {
      device = "/dev/\${NIXORIUM_INSTALL_DISK}";
      inherit studentUser;
    };
  } bootstrapPkgs;
  nixoriumPackage = bootstrapPkgs.callPackage (upstreamRoot + "/pkgs/nixorium.nix") {};
  hostState = bootstrapPkgs.writeShellApplication {
    name = "nixorium-host-state";
    runtimeInputs = [ bootstrapPkgs.coreutils ];
    text = ''
      system_path="$(readlink -f /run/current-system)"
      [[ "$system_path" == /nix/store/* && "$system_path" != *[[:space:]]* ]] \
        || { echo "active system path is invalid" >&2; exit 1; }
      revision="$(/run/current-system/sw/bin/nixos-version --configuration-revision)" \
        || { echo "active deployment revision is unavailable" >&2; exit 1; }
      [[ "$revision" =~ ^[0-9a-f]{40,64}$ ]] \
        || { echo "active deployment revision is invalid" >&2; exit 1; }
      printf '%s\n%s\n' "$system_path" "$revision"
    '';
  };

  installerFlake = bootstrapPkgs.writeText "nixorium-installer-flake.nix" ''
    {
      inputs = {
        nixpkgs.url = "path:${nixpkgs.outPath}";
        site = {
          url = "path:${deploymentRoot}";
          flake = false;
        };
        systems.url = "path:${veyon.inputs.flake-utils.inputs.systems.outPath}";
        flake-utils = {
          url = "path:${veyon.inputs.flake-utils.outPath}";
          inputs.systems.follows = "systems";
        };
        disko = {
          url = "path:${disko.outPath}";
          inputs.nixpkgs.follows = "nixpkgs";
        };
        veyon = {
          url = "path:${veyon.outPath}";
          inputs.nixpkgs.follows = "nixpkgs";
          inputs.flake-utils.follows = "flake-utils";
        };
        nixorium = {
          url = "path:${upstreamRoot}";
          inputs.nixpkgs.follows = "nixpkgs";
          inputs.disko.follows = "disko";
          inputs.veyon.follows = "veyon";
        };
      };

      outputs = { self, nixorium, site, ... }:
        nixorium.lib.mkLab {
          deploymentSelf = site;
          labConfig = builtins.fromJSON (builtins.readFile ./lab-config.json);
          labSoftware = builtins.fromJSON (builtins.readFile ./lab-software.json);
          clientGroups = ${builtins.toJSON clientGroups};
          publicKeys = {
            cache = ${renderPath cachePublicKeyFile};
            ssh = ${renderPath adminSshKeyFile};
            veyon = ${renderPath veyonPublicKeyFile};
          };
          assets = {
            logo = ${renderPath labAssets.logo};
            backgrounds = ${renderPathList labAssets.backgrounds};
            mimeApps = ${renderPath labAssets.mimeApps};
            vscodeSettings = ${renderPath labAssets.vscodeSettings};
          };
          sharedModules = ${renderPathList sharedModules};
          controllerModules = ${renderPathList controllerModules};
          clientModules = ${renderPathList clientModules};
          hostModules = ${renderHostModules};
          netbootModules = ${renderPathList netbootModules};
          installerSource = self;
          nixosVersionMetadata = {
            versionSuffix = ${builtins.toJSON sourceNixosVersionMetadata.versionSuffix};
            revision = ${builtins.toJSON sourceNixosVersionMetadata.revision};
          };
          deploymentRevision = ${builtins.toJSON effectiveDeploymentRevision};
        };
    }
  '';

  installerBundle = bootstrapPkgs.runCommand "nixorium-installer-${masterHostName}" {} ''
    install -d -m 0755 "$out/lib" "$out/scripts/lib"
    install -m 0644 ${installerFlake} "$out/flake.nix"
    install -m 0644 ${labConfigJson} "$out/lab-config.json"
    install -m 0644 ${labSoftwareJson} "$out/lab-software.json"
    install -m 0644 ${labMetaJson} "$out/lab-meta.json"
    install -m 0755 ${upstreamRoot}/setup.sh "$out/setup.sh"
    install -m 0755 ${installerDiskoScript}/bin/disko-destroy-format-mount "$out/disko-install"
    install -m 0644 ${upstreamRoot}/lib/disko-layout.nix "$out/lib/disko-layout.nix"
    install -m 0644 ${upstreamRoot}/scripts/lib/lab-meta.sh "$out/scripts/lib/lab-meta.sh"
    ${lib.optionalString (cachePublicKeyFile != null) ''
      install -m 0644 ${cachePublicKeyFile} "$out/public-key"
    ''}
  '';

  effectiveInstallerSource = if installerSource == null then installerBundle else installerSource;

  mkHost = n:
    let
      name = "pc${padNumber n}";
      hostIp = mkHostIp n;
    in
    {
      inherit name;
      value = nixpkgs.lib.nixosSystem {
        inherit system;
        specialArgs = specialArgsForHost name hostIp;
        modules = modulesForHost name false;
      };
    };

  mkColmenaHost = n:
    let
      name = "pc${padNumber n}";
      hostIp = mkHostIp n;
    in
    {
      inherit name;
      value = {
        _module.args = specialArgsForHost name hostIp;
        imports = modulesForHost name false;
        deployment = {
          targetHost = hostIp;
          tags = [ "lab" ];
        };
      };
    };

  runHarmonia = bootstrapPkgs.writeShellApplication {
    name = "nixorium-run-harmonia";
    text = ''
      export LAB_REPO_ROOT="$PWD"
      exec ${upstreamRoot}/scripts/run-harmonia.sh "$@"
    '';
  };

  runPxeProxy = bootstrapPkgs.writeShellApplication {
    name = "nixorium-run-pxe-proxy";
    text = ''
      export LAB_REPO_ROOT="$PWD"
      exec ${upstreamRoot}/scripts/run-pxe-proxy.sh "$@"
    '';
  };

  runDisko = bootstrapPkgs.writeShellApplication {
    name = "nixorium-disko";
    text = ''
      exec ${disko.packages.${system}.default}/bin/disko "$@"
    '';
  };
  pxeFirmware = bootstrapPkgs.runCommand "nixorium-ipxe-firmware" {} ''
    install -D -m 0644 ${bootstrapPkgs.ipxe}/snp.efi "$out/snponly.efi"
  '';
in
assert builtins.all isModulePath extensionModules
  || throw "mkLab extension modules must be file paths so they can be included in the offline installer";
assert unknownPublicKeyNames == []
  || throw "Unknown publicKeys entries: ${builtins.concatStringsSep ", " unknownPublicKeyNames}";
assert unknownAssetNames == []
  || throw "Unknown assets entries: ${builtins.concatStringsSep ", " unknownAssetNames}";
assert unknownHostModuleNames == []
  || throw "hostModules contains unknown hosts: ${builtins.concatStringsSep ", " unknownHostModuleNames}";
assert unknownVeyonNativeHosts == []
  || throw "veyonNativeHosts contains unknown hosts: ${builtins.concatStringsSep ", " unknownVeyonNativeHosts}";
{
  nixosConfigurations = builtins.listToAttrs (map mkHost pcNumbers) // {
    ${masterHostName} = nixpkgs.lib.nixosSystem {
      inherit system;
      specialArgs = specialArgsForHost masterHostName masterIp;
      modules = modulesForHost masterHostName true;
    };
    netboot = nixpkgs.lib.nixosSystem {
      inherit system;
      specialArgs = {
        inherit labSettings;
        inherit labAssets;
        hostName = "netboot";
        hostIp = masterDhcpIp;
      };
      modules = [
        "${nixpkgs}/nixos/modules/installer/netboot/netboot-minimal.nix"
        (upstreamRoot + "/modules/cache.nix")
        ({ pkgs, lib, ... }: {
          # The managed listener passes its preparation-time address at boot;
          # the installer uses that address explicitly for signed closure pulls.
          nix.settings.substituters = lib.mkForce [ ];
          networking.useDHCP = lib.mkForce true;
          boot.zfs.forceImportRoot = false;
          services.openssh.enable = true;
          environment.systemPackages = [
            disko.packages.${system}.default
            pkgs.iputils
            pkgs.jq
            pkgs.util-linux
          ] ++ installerDiskoRuntimePackages;
          system.stateVersion = "25.11";
          system.activationScripts.copyFlakeToRamdisk.text = ''
            install -d -m 0755 /installer
            cp -a ${effectiveInstallerSource}/. /installer/
          '';
        })
      ] ++ netbootModules;
    };
  };

  inherit labMeta;

  nixoriumSoftware = {
    schemaVersion = 1;
    managedFile = "lab-software.json";
    clients = validClientNames;
    groups = clientGroups;
    catalog = softwareCatalog;
    packages = map (entry: entry // { origin = "managed"; }) labSoftwareConfig.packages;
  };

  deploymentStatus = {
    ready = deploymentIssues == [];
    issues = deploymentIssues;
  };

  colmena = {
    meta = {
      nixpkgs = import nixpkgs {
        inherit system;
        overlays = [ labOverlay ];
      };
      specialArgs = {
        inherit labSettings;
        inherit labAssets;
      };
    };
    defaults.deployment = {
      targetUser = "root";
      buildOnTarget = false;
      sshOptions = [ "-o" "StrictHostKeyChecking=accept-new" ];
    };
    ${masterHostName} = {
      _module.args = specialArgsForHost masterHostName masterIp;
      imports = modulesForHost masterHostName true;
      deployment = {
        targetHost = "localhost";
        tags = [ "master" ];
      };
    };
  } // builtins.listToAttrs (map mkColmenaHost clientNumbers);

  apps.${system} = {
    nixorium = {
      type = "app";
      program = "${nixoriumPackage}/bin/nixorium";
      meta.description = "Manage the Nixorium laboratory";
    };
    run-harmonia = {
      type = "app";
      program = "${runHarmonia}/bin/nixorium-run-harmonia";
      meta.description = "Run the laboratory Harmonia binary cache";
    };
    run-pxe-proxy = {
      type = "app";
      program = "${runPxeProxy}/bin/nixorium-run-pxe-proxy";
      meta.description = "Run the laboratory ProxyDHCP, TFTP, and HTTP services";
    };
    disko = {
      type = "app";
      program = "${runDisko}/bin/nixorium-disko";
      meta.description = "Run the Disko revision pinned by this deployment";
    };
  };

  packages.${system} = {
    inherit installerBundle pxeFirmware;
    disko = runDisko;
    nixorium = nixoriumPackage;
  };
}
