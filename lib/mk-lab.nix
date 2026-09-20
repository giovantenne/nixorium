{ upstreamSelf, nixpkgs, disko, veyon }:
args@{
  deploymentSelf,
  labConfig,
  publicKeys ? {},
  assets ? {},
  homeResetEphemeralPaths ? [],
  sharedModules ? [],
  controllerModules ? [],
  clientModules ? [],
  hostModules ? {},
  labSoftware ? { schemaVersion = 1; packages = []; },
  softwareCatalog ? [],
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
    logo = assets.logo or ../assets/empty-logo.txt;
    backgrounds = assets.backgrounds or [];
    mimeApps = assets.mimeApps or ../assets/empty-mimeapps.list;
    vscodeSettings = assets.vscodeSettings or ../assets/empty-vscode-settings.json;
  };

  inherit (config) masterDhcpIp;
  inherit (config) deploymentMode;
  laboratoryEnabled = deploymentMode == "laboratory";
  inherit (config) networkBase;
  inherit (config) networkPrefixLength;
  inherit (config) pcCount;
  inherit (config) masterHostNumber;
  inherit (config) ifaceName;
  inherit (config) controllerIfaceName;
  inherit (config) clientIfaceName;
  inherit (config) hostIfaceNames;
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
  ifaceForHost = name:
    hostIfaceNames.${name} or (
      if name == masterHostName then
        if controllerIfaceName == null then ifaceName else controllerIfaceName
      else if clientIfaceName == null then ifaceName else clientIfaceName);
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
    inherit deploymentMode;
    inherit masterIp;
    inherit masterDhcpIp;
    inherit masterHostName;
    inherit masterHostNumber;
    inherit networkBase;
    inherit networkPrefixLength;
    inherit clientIps;
    inherit pcCount;
    inherit ifaceName;
    inherit controllerIfaceName;
    inherit clientIfaceName;
    inherit hostIfaceNames;
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
        warnings = lib.optionals laboratoryEnabled (
          lib.optional (cachePublicKeyFile == null) "Missing cache public key"
          ++ lib.optional (adminSshKeyFile == null) "Missing admin SSH public key"
          ++ lib.optional (veyonPublicKeyFile == null) "Missing Veyon public key");
        environment.systemPackages = [ hostState hostSessionState ];
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
    ++ lib.optionals isController ([ managedSoftwareModule ] ++ controllerModules)
    ++ lib.optionals (!isController) ([ managedSoftwareModule ] ++ clientModules)
    ++ (hostModules.${name} or []);

  specialArgsForHost = name: hostIp: {
    labSettings = labSettings // { ifaceName = ifaceForHost name; };
    inherit labAssets;
    inherit homeResetEphemeralPaths;
    hostSoftwarePackages = map (entry: entry.package)
      (builtins.filter (entry: softwareAppliesTo name entry.scope) labSoftwareConfig.packages);
    inherit nixoriumPackage;
    inherit hostIp;
    hostName = name;
  };

  labMeta = {
    schemaVersion = 2;
    inherit version;
    inherit deploymentMode;
    controller = {
      name = masterHostName;
      number = masterHostNumber;
      staticIp = if laboratoryEnabled then masterIp else "";
      dhcpIp = masterDhcpIp;
      ifaceName = ifaceForHost masterHostName;
    };
    clients = {
      count = pcCount;
      hosts = map (n: {
        name = "pc${padNumber n}";
        ip = mkHostIp n;
        ifaceName = ifaceForHost "pc${padNumber n}";
      }) clientNumbers;
      groups = clientGroups;
    };
    network = {
      base = networkBase;
      prefixLength = networkPrefixLength;
      ifaceName = ifaceForHost masterHostName;
      clientIfaceName = if clientIfaceName == null then ifaceName else clientIfaceName;
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
  softwareCatalogJson = builtins.toFile "software-catalog.json" (builtins.toJSON softwareCatalog);
  homeResetEphemeralPathsJson = builtins.toFile "home-reset-ephemeral-paths.json"
    (builtins.toJSON homeResetEphemeralPaths);
  clientGroupsJson = builtins.toFile "client-groups.json" (builtins.toJSON clientGroups);
  extensionModules = sharedModules
    ++ controllerModules
    ++ clientModules
    ++ netbootModules
    ++ lib.concatLists (lib.attrValues hostModules);
  isModulePath = module: builtins.isPath module || builtins.isString module;
  unknownPublicKeyNames = builtins.attrNames (builtins.removeAttrs publicKeys [ "cache" "ssh" "veyon" ]);
  unknownAssetNames = builtins.attrNames (builtins.removeAttrs assets [ "logo" "backgrounds" "mimeApps" "vscodeSettings" ]);
  validHomeResetEphemeralPath = path:
    builtins.isString path
    && path != ""
    && !(lib.hasPrefix "/" path)
    && !(builtins.elem ".." (lib.splitString "/" path));
  validClientNames = map (n: "pc${padNumber n}") pcNumbers;
  validHostNames = validClientNames ++ [ masterHostName ];
  unknownHostModuleNames = builtins.attrNames (builtins.removeAttrs hostModules validHostNames);
  unknownVeyonNativeHosts = builtins.filter (name: !builtins.elem name validHostNames) veyonNativeHosts;
  defaultPasswordHash = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0";
  credentialIssues =
    lib.optional (teacherPassword == defaultPasswordHash) "teacherPassword still uses the public default"
    ++ lib.optional (studentPassword == defaultPasswordHash) "studentPassword still uses the public default"
    ++ lib.optional (adminPassword == defaultPasswordHash) "adminPassword still uses the public default";
  deploymentIssues =
    lib.optional (!laboratoryEnabled) "Client installation is not configured"
    ++ lib.optional (masterDhcpIp == "MASTER_DHCP_IP") "masterDhcpIp still uses the template placeholder"
    ++ lib.optional (cachePublicKeyFile == null) "cache public key is missing"
    ++ lib.optional (adminSshKeyFile == null) "admin SSH public key is missing"
    ++ lib.optional (veyonPublicKeyFile == null) "Veyon public key is missing"
    ++ credentialIssues;
  controllerIssues = if laboratoryEnabled then deploymentIssues else credentialIssues;
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
  softwarePkgs = import nixpkgs {
    inherit system;
    overlays = [ labOverlay ];
    config.allowUnfree = true;
  };
  softwarePackageTools = import ./software-packages.nix {
    inherit lib;
    pkgs = softwarePkgs;
    allowUnfree = true;
  };
  normalizeSoftwareCatalogItem = index: definition:
    let
      prefix = "softwareCatalog[${toString index}]";
      extras = if builtins.isAttrs definition then
        builtins.attrNames (builtins.removeAttrs definition [ "id" "label" "summary" ])
      else
        [];
      id = definition.id or (throw "${prefix}.id is required");
      label = definition.label or (throw "${prefix}.label is required");
      summary = definition.summary or (throw "${prefix}.summary is required");
      resolved = if softwarePackageTools.validPath id then softwarePackageTools.describe id else null;
    in
    assert builtins.isAttrs definition || throw "${prefix} must be an attribute set";
    assert extras == [] || throw "${prefix} contains unknown fields: ${builtins.concatStringsSep ", " extras}";
    assert softwarePackageTools.validPath id || throw "${prefix}.id is invalid";
    assert builtins.isString label && label != "" || throw "${prefix}.label must be a non-empty string";
    assert builtins.isString summary && summary != "" || throw "${prefix}.summary must be a non-empty string";
    if resolved == null || resolved.availability != "available" then null else
    resolved // {
      inherit label;
      inherit summary;
    };
  softwareCatalogIds = map
    (definition: definition.id or (throw "softwareCatalog entries require id"))
    softwareCatalog;
  resolvedSoftwareCatalog = builtins.filter (item: item != null)
    (lib.imap0 normalizeSoftwareCatalogItem softwareCatalog);
  labSoftwareConfig = import ./eval-lab-software.nix {
    inherit lib;
    pkgs = softwarePkgs;
    clientNames = validClientNames;
    inherit clientGroups;
    requireAvailable = false;
    allowUnfree = true;
  } labSoftware;
  softwareAppliesTo = name: scope:
    scope.kind == "shared"
    || (scope.kind == "controller" && name == masterHostName)
    || (scope.kind == "all-clients" && builtins.elem name validClientNames)
    || (scope.kind == "group" && builtins.elem name clientGroups.${scope.group})
    || (scope.kind == "clients" && builtins.elem name scope.clients);
  managedSoftwareModule = { pkgs, hostName, ... }: {
    environment.systemPackages = map
      (entry:
        let package = lib.attrByPath (lib.splitString "." entry.package) null pkgs;
        in if package != null && lib.isDerivation package then package
        else throw "lab-software.json: package ${entry.package} is unavailable in the pinned package set")
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
  hostSessionState = bootstrapPkgs.writeShellApplication {
    name = "nixorium-session-state";
    runtimeInputs = [ bootstrapPkgs.coreutils bootstrapPkgs.systemd ];
    text = ''
      sessions="$(loginctl list-sessions --no-legend --no-pager)" \
        || { echo "session inventory is unavailable" >&2; exit 2; }
      while read -r session uid _rest; do
        [[ -z "''${session:-}" ]] && continue
        [[ "$uid" =~ ^[0-9]+$ ]] \
          || { echo "session inventory is invalid" >&2; exit 2; }
        if (( uid >= 1000 )); then
          printf 'active\n'
          exit 0
        fi
      done <<< "$sessions"
      printf 'idle\n'
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
          softwareCatalog = builtins.fromJSON (builtins.readFile ./software-catalog.json);
          homeResetEphemeralPaths = builtins.fromJSON (builtins.readFile ./home-reset-ephemeral-paths.json);
          clientGroups = builtins.fromJSON (builtins.readFile ./client-groups.json);
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
    install -m 0644 ${softwareCatalogJson} "$out/software-catalog.json"
    install -m 0644 ${homeResetEphemeralPathsJson} "$out/home-reset-ephemeral-paths.json"
    install -m 0644 ${clientGroupsJson} "$out/client-groups.json"
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
assert builtins.isList homeResetEphemeralPaths && builtins.all validHomeResetEphemeralPath homeResetEphemeralPaths
  || throw "mkLab homeResetEphemeralPaths must contain safe relative paths";
assert builtins.isList softwareCatalog
  || throw "mkLab softwareCatalog must be a list";
assert builtins.length softwareCatalogIds == builtins.length (lib.unique softwareCatalogIds)
  || throw "mkLab softwareCatalog contains duplicate ids";
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
        labSettings = labSettings // { ifaceName = ifaceForHost masterHostName; };
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
          console.keyMap = labSettings.consoleKeyMap;
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
    controller = masterHostName;
    clients = validClientNames;
    groups = clientGroups;
    catalog = resolvedSoftwareCatalog;
    packages = map (entry: entry // { origin = "managed"; }) labSoftwareConfig.packages;
  };

  nixoriumSearchSoftwarePackages = softwarePackageTools.search;
  nixoriumResolveSoftwarePackage = softwarePackageTools.describe;
  # Keep controller validation upstream: older site templates validate only
  # clients in their software hook. Reuse every downstream extension unchanged.
  nixoriumValidateControllerSoftwareCandidate = rawSoftware:
    let
      candidate = import ./mk-lab.nix { inherit upstreamSelf nixpkgs disko veyon; }
        (args // { labSoftware = rawSoftware; });
    in builtins.deepSeq
      candidate.nixosConfigurations.${masterHostName}.config.system.build.toplevel.drvPath
      true;

  deploymentStatus = {
    ready = deploymentIssues == [];
    issues = deploymentIssues;
    controller = {
      ready = controllerIssues == [];
      issues = controllerIssues;
      requiresKeys = laboratoryEnabled;
    };
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
