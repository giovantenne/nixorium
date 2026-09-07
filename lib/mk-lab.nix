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
  netbootModules ? [],
  installerSource ? null,
  nixosVersionMetadata ? null
}:
let
  lib = nixpkgs.lib;
  upstreamRoot = upstreamSelf.outPath;
  deploymentRoot = if builtins.isAttrs deploymentSelf then deploymentSelf.outPath else deploymentSelf;
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

  masterHostName = "pc${toString masterHostNumber}";
  masterIp = "${networkBase}.${toString masterHostNumber}";
  cachePort = 5000;
  pxeHttpPort = 8080;
  system = "x86_64-linux";
  pcNumbers = builtins.genList (n: n + 1) pcCount;
  clientNumbers = pcNumbers;
  padNumber = n: if n < 10 then "0${toString n}" else toString n;

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
    inherit veyonPublicKeyFile;
  };

  baseHostModules = [
    { nixpkgs.overlays = [ labOverlay ]; }
    ({ lib, ... }: {
      warnings =
        lib.optional (cachePublicKeyFile == null) "Missing cache public key"
        ++ lib.optional (adminSshKeyFile == null) "Missing admin SSH public key"
        ++ lib.optional (veyonPublicKeyFile == null) "Missing Veyon public key";
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
  ] ++ lib.optional (nixosVersionMetadata != null) ({ lib, ... }: {
    system.nixos.versionSuffix = lib.mkForce nixosVersionMetadata.versionSuffix;
    system.nixos.revision = lib.mkForce nixosVersionMetadata.revision;
  });

  modulesForHost = name: isController:
    baseHostModules
    ++ sharedModules
    ++ lib.optionals isController controllerModules
    ++ lib.optionals (!isController) clientModules
    ++ (hostModules.${name} or []);

  specialArgsForHost = name: hostIp: {
    inherit labSettings;
    inherit labAssets;
    inherit hostIp;
    hostName = name;
  };

  labMeta = {
    schemaVersion = 1;
    inherit version;
    controller = {
      name = masterHostName;
      number = masterHostNumber;
      staticIp = masterIp;
      dhcpIp = masterDhcpIp;
    };
    clients.count = pcCount;
    network = {
      base = networkBase;
      inherit ifaceName;
      inherit cachePort;
      inherit pxeHttpPort;
    };
    users = {
      student = studentUser;
      teacher = teacherUser;
    };
  };

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
      preservePathType "(nixos-lab + ${builtins.toJSON (lib.removePrefix (toString upstreamRoot) valueString)})"
    else
      throw "mkLab files must be located inside the deployment or nixos-lab source tree: ${valueString}";
  renderPathList = values: "[ ${builtins.concatStringsSep " " (map renderPath values)} ]";
  renderHostModules = "{\n${builtins.concatStringsSep "\n" (lib.mapAttrsToList
    (name: modules: "    ${builtins.toJSON name} = ${renderPathList modules};")
    hostModules)}\n  }";
  labConfigJson = builtins.toFile "lab-config.json" (builtins.toJSON config);
  extensionModules = sharedModules
    ++ controllerModules
    ++ clientModules
    ++ netbootModules
    ++ lib.concatLists (lib.attrValues hostModules);
  isModulePath = module: builtins.isPath module || builtins.isString module;
  unknownPublicKeyNames = builtins.attrNames (builtins.removeAttrs publicKeys [ "cache" "ssh" "veyon" ]);
  unknownAssetNames = builtins.attrNames (builtins.removeAttrs assets [ "logo" "backgrounds" "mimeApps" "vscodeSettings" ]);
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

  installerFlake = bootstrapPkgs.writeText "nixos-lab-installer-flake.nix" ''
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
        nixos-lab = {
          url = "path:${upstreamRoot}";
          inputs.nixpkgs.follows = "nixpkgs";
          inputs.disko.follows = "disko";
          inputs.veyon.follows = "veyon";
        };
      };

      outputs = { self, nixos-lab, site, ... }:
        nixos-lab.lib.mkLab {
          deploymentSelf = site;
          labConfig = builtins.fromJSON (builtins.readFile ./lab-config.json);
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
        };
    }
  '';

  installerBundle = bootstrapPkgs.runCommand "nixos-lab-installer-${masterHostName}" {} ''
    install -d -m 0755 "$out/lib" "$out/scripts/lib"
    install -m 0644 ${installerFlake} "$out/flake.nix"
    install -m 0644 ${labConfigJson} "$out/lab-config.json"
    install -m 0755 ${upstreamRoot}/setup.sh "$out/setup.sh"
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
      hostIp = "${networkBase}.${toString n}";
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
      hostIp = "${networkBase}.${toString n}";
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
    name = "nixos-lab-run-harmonia";
    text = ''
      export LAB_REPO_ROOT="$PWD"
      exec ${upstreamRoot}/scripts/run-harmonia.sh "$@"
    '';
  };

  runPxeProxy = bootstrapPkgs.writeShellApplication {
    name = "nixos-lab-run-pxe-proxy";
    text = ''
      export LAB_REPO_ROOT="$PWD"
      exec ${upstreamRoot}/scripts/run-pxe-proxy.sh "$@"
    '';
  };
in
assert builtins.all isModulePath extensionModules
  || throw "mkLab extension modules must be file paths so they can be included in the offline installer";
assert unknownPublicKeyNames == []
  || throw "Unknown publicKeys entries: ${builtins.concatStringsSep ", " unknownPublicKeyNames}";
assert unknownAssetNames == []
  || throw "Unknown assets entries: ${builtins.concatStringsSep ", " unknownAssetNames}";
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
          nix.settings.substituters = lib.mkForce [ "http://${masterDhcpIp}:${toString labSettings.cachePort}" ];
          networking.useDHCP = lib.mkForce true;
          boot.zfs.forceImportRoot = false;
          services.openssh.enable = true;
          environment.systemPackages = [
            disko.packages.${system}.default
            pkgs.jq
          ];
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
    run-harmonia = {
      type = "app";
      program = "${runHarmonia}/bin/nixos-lab-run-harmonia";
    };
    run-pxe-proxy = {
      type = "app";
      program = "${runPxeProxy}/bin/nixos-lab-run-pxe-proxy";
    };
  };

  packages.${system}.installerBundle = installerBundle;
}
