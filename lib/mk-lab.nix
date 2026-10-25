{ upstreamSelf, nixpkgs, disko }:
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
  groupModules ? {},
  updateValidationHosts ? [],
  labSoftware ? { schemaVersion = 1; packages = []; },
  softwareCatalog ? [],
  softwarePresets ? null,
  # Without a deployment profile the student home still resets at every boot,
  # to the system defaults.
  workspaceProfileJSON ? builtins.toJSON { schemaVersion = 1; },
  workspaceCatalog ? { schemaVersion = 1; baseline.schemaVersion = 1; applications = []; extensions = []; },
  # Adopt tracked initial-file declarations on existing deployment flakes too.
  # A missing declaration keeps the previous empty additional-file baseline.
  homeFilesJSON ? let root = if builtins.isAttrs deploymentSelf then deploymentSelf.outPath else deploymentSelf;
    in if builtins.pathExists (root + "/home-files.json") then builtins.readFile (root + "/home-files.json")
      else builtins.toJSON { schemaVersion = 1; },
  homeFilesRoot ? let root = if builtins.isAttrs deploymentSelf then deploymentSelf.outPath else deploymentSelf;
    directory = root + "/home-files";
    in if builtins.pathExists directory then /. + builtins.unsafeDiscardStringContext (toString directory) else null,
  clientGroups ? {},
  netbootModules ? [],
  installerSource ? null,
  nixosVersionMetadata ? null,
  deploymentRevision ? null
}:
let
  lib = nixpkgs.lib;
  # Same deployment with a constant revision: the Assistant compares host
  # derivations without the Git revision that every candidate changes.
  comparisonLab = import ./mk-lab.nix { inherit upstreamSelf nixpkgs disko; }
    (args // { deploymentRevision = "nixorium-comparison"; });
  upstreamRoot = upstreamSelf.outPath;
  deploymentRoot = if builtins.isAttrs deploymentSelf then deploymentSelf.outPath else deploymentSelf;
  inferredDeploymentRevision =
    if builtins.isAttrs deploymentSelf then
      deploymentSelf.rev or deploymentSelf.dirtyRev or null
    else
      null;
  effectiveDeploymentRevision =
    if deploymentRevision != null then deploymentRevision
    else if builtins.pathExists (deploymentRoot + "/.nixorium-source-revision.json") then
      builtins.fromJSON (builtins.readFile (deploymentRoot + "/.nixorium-source-revision.json"))
    else inferredDeploymentRevision;
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
  cachePublicKeyFile = if fileExists configuredCachePublicKeyFile then configuredCachePublicKeyFile else null;
  adminSshKeyFile = if fileExists configuredAdminSshKeyFile then configuredAdminSshKeyFile else null;
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
  inherit (config) classroomView;
  inherit (config) assistant;

  ipv4ToInt = address:
    let octets = map lib.toInt (lib.splitString "." address);
    in builtins.elemAt octets 0 * 16777216
      + builtins.elemAt octets 1 * 65536
      + builtins.elemAt octets 2 * 256
      + builtins.elemAt octets 3;
  networkAddress = ipv4ToInt networkBase;
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
  # The controller always keeps its configured name. A client is configured by
  # name only when the administrator chose one for it (hostIfaceNames or
  # clientIfaceName); the template fallback `ifaceName` is a VirtualBox-era
  # default that real hardware rarely shares, so unnamed clients match the
  # wired NIC by type instead (ADR 0017).
  ifaceExplicitForHost = name:
    name == masterHostName || hostIfaceNames ? ${name} || clientIfaceName != null;
  cachePort = 5000;
  pxeHttpPort = 8080;
  system = "x86_64-linux";
  pcNumbers = builtins.genList (n: n + 1) pcCount;
  clientNumbers = pcNumbers;
  clientIps = map mkHostIp clientNumbers;

  # A change of laboratory network in progress (managed by the guided change).
  # The same host numbers keep their previous addresses on the previous
  # network: the controller and every client carry both, clients trust both
  # controller addresses. Without clients the record is ignored and reported.
  networkTransitionRecord = config.networkTransition;
  networkTransitionIgnored = networkTransitionRecord != null && pcCount == 0;
  networkTransition =
    if networkTransitionRecord == null || networkTransitionIgnored then null
    else {
      base = networkTransitionRecord.previousBase;
      prefixLength = networkTransitionRecord.previousPrefixLength;
      masterIp = previousMasterIp;
      # Each client's previous address, by host name.
      hostIps = builtins.listToAttrs (lib.zipListsWith
        (n: ip: { name = "pc${padNumber n}"; value = ip; })
        clientNumbers previousClientIps);
    };
  mkPreviousHostIp = number:
    ipv4FromInt (ipv4ToInt networkTransitionRecord.previousBase + number);
  previousMasterIp = mkPreviousHostIp masterHostNumber;
  previousClientIps = map mkPreviousHostIp clientNumbers;

  labSettings = {
    inherit deploymentMode;
    inherit masterIp;
    inherit masterDhcpIp;
    inherit masterHostName;
    inherit masterHostNumber;
    inherit networkBase;
    inherit networkPrefixLength;
    inherit clientIps;
    # Every client's number, name and addresses: a shared system selects its
    # row with the number in the computer's identity record.
    clientIdentities = map (n: {
      number = n;
      name = "pc${padNumber n}";
      address = mkHostIp n;
    } // lib.optionalAttrs (networkTransition != null) {
      previousAddress = mkPreviousHostIp n;
    }) clientNumbers;
    inherit networkTransition;
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
    inherit classroomView;
    inherit assistant;
    inherit cachePublicKey;
    inherit cachePort;
    inherit pxeHttpPort;
  };

  baseHostModules = [
    ({ lib, isController, ... }:
      {
        warnings = lib.optionals laboratoryEnabled (
          lib.optional (cachePublicKeyFile == null) "Missing cache public key"
          ++ lib.optional (adminSshKeyFile == null) "Missing admin SSH public key")
          ++ lib.optional networkTransitionIgnored
            "lab.networkTransition is ignored because no client computers are configured; remove it from lab-settings.json";
        environment.systemPackages = [ hostState hostSessionState hostCleanGenerations hostDiagnostics ];
        environment.etc."nixorium-host-diagnostics.json".text = builtins.toJSON {
          controller = isController;
          systemctl = "${bootstrapPkgs.systemd}/bin/systemctl";
          journalctl = "${bootstrapPkgs.systemd}/bin/journalctl";
          lpstat = "${bootstrapPkgs.cups}/bin/lpstat";
        };
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
    (upstreamRoot + "/modules/identity.nix")
    (upstreamRoot + "/modules/cache.nix")
    (upstreamRoot + "/modules/filesystems.nix")
    (upstreamRoot + "/modules/home-reset.nix")
    (upstreamRoot + "/modules/classroom-view.nix")
    (upstreamRoot + "/modules/management.nix")
    (upstreamRoot + "/modules/telemetry.nix")
    (upstreamRoot + "/modules/assistant.nix")
    (upstreamRoot + "/modules/pxe.nix")
  ] ++ lib.optional (nixosVersionMetadata != null) ({ lib, ... }: {
    system.nixos.versionSuffix = lib.mkForce nixosVersionMetadata.versionSuffix;
    system.nixos.revision = lib.mkForce nixosVersionMetadata.revision;
  });

  modulesForHost = name: isController:
    baseHostModules
    ++ sharedModules
    ++ lib.optionals isController ([ managedSoftwareModule ] ++ controllerModules)
    ++ lib.optionals (!isController) ([ managedSoftwareModule ] ++ clientModules
      ++ lib.concatMap (group: groupModules.${group}) (moduleGroupsForHost name))
    ++ (hostModules.${name} or []);

  specialArgsForHost = name: hostIp: {
    labSettings = labSettings // {
      ifaceName = ifaceForHost name;
      ifaceExplicit = ifaceExplicitForHost name;
    };
    inherit labAssets;
    inherit homeResetEphemeralPaths;
    inherit workspaceSeed;
    workspaceWallpapers = map (background: "${background}") labAssets.backgrounds;
    hostSoftwarePackages = map (entry: entry.package) (hostSoftwareEntries name);
    inherit nixoriumPackage;
    # The lab screensaver's effects (pkgs/ttfx.nix) until nixpkgs ships ttfx.
    nixoriumTtfx = bootstrapPkgs.callPackage (upstreamRoot + "/pkgs/ttfx.nix") {};
    inherit hostIp;
    hostName = name;
    # The role is explicit so modules never infer it from the host name.
    nixoriumRole = if name == masterHostName then "controller" else "client";
    isController = name == masterHostName;
    nixoriumClass = if name == masterHostName then controllerClass else nixoriumClassFor clientClassOf.${name};
  };

  # A shared system knows the computers of its class, not which one it runs
  # on: modules that need a computer's own name or address fail clearly.
  specialArgsForClass = class:
    let
      members = builtins.concatStringsSep ", " class.members;
      unknown = what: throw "${what} is not known in the configuration shared by ${members}. A module that only tests whether it runs on the controller should use isController (in modules/workstation.nix of deployments created before 3.9.0, replace the hostName comparison with: isController = args.isController or false;); a module that needs the computer's own name or address needs its own configuration with hostModules or updateValidationHosts";
    in
    specialArgsForHost class.representative null // {
      hostName = unknown "hostName";
      hostIp = unknown "hostIp";
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
        ifaceExplicit = ifaceExplicitForHost "pc${padNumber n}";
        class = classIdFor clientClassOf."pc${padNumber n}";
      }) clientNumbers;
      classes = map (class: { inherit (nixoriumClassFor class) id shared members; }) clientClasses;
      groups = clientGroups;
    };
    network = {
      base = networkBase;
      prefixLength = networkPrefixLength;
      ifaceName = ifaceForHost masterHostName;
      clientIfaceName = if clientIfaceName == null then ifaceName else clientIfaceName;
      inherit cachePort;
      inherit pxeHttpPort;
    } // lib.optionalAttrs (networkTransition != null) {
      # Present only during a change of laboratory network.
      transition = {
        base = networkTransition.base;
        prefixLength = networkTransition.prefixLength;
        controllerIp = previousMasterIp;
        inherit (networkTransitionRecord) startedAt;
        clients = lib.zipListsWith (n: ip: { name = "pc${padNumber n}"; inherit ip; })
          clientNumbers previousClientIps;
      };
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
  safeHomeFilesRoot =
    if homeFilesRoot == null then true
    else let
      root = if lib.hasPrefix "${toString deploymentRoot}/" (toString homeFilesRoot) then deploymentRoot
        else if lib.hasPrefix "${toString upstreamRoot}/" (toString homeFilesRoot) then upstreamRoot
        else throw "home files: source root is outside the deployment";
      parts = lib.splitString "/" (lib.removePrefix "${toString root}/" (toString homeFilesRoot));
      walk = directory: remaining:
        if remaining == [] then true
        else let name = builtins.head remaining; in
          ((builtins.readDir directory).${name} or null) == "directory"
          && walk (directory + "/${name}") (builtins.tail remaining);
    in walk root parts;
  # Defer source inspection until a value is requested: forcing deploymentSelf
  # while discovering the output names creates a recursive flake self.
  checkedHomeFilesRoot =
    assert homeFilesRoot == null || (builtins.isPath homeFilesRoot && builtins.isString (renderPath homeFilesRoot))
      || throw "mkLab homeFilesRoot must be a path inside the deployment or upstream source";
    assert safeHomeFilesRoot || throw "mkLab homeFilesRoot must not traverse symbolic links";
    homeFilesRoot;
  renderModuleMap = entries: "{\n${builtins.concatStringsSep "\n" (lib.mapAttrsToList
    (name: modules: "    ${builtins.toJSON name} = ${renderPathList modules};")
    entries)}\n  }";
  labConfigJson = builtins.toFile "lab-config.json" (builtins.toJSON config);
  labSoftwareJson = builtins.toFile "lab-software.json" (builtins.toJSON labSoftwareConfig);
  softwareCatalogJson = builtins.toFile "software-catalog.json" (builtins.toJSON softwareCatalog);
  softwarePresetsJson = builtins.toFile "software-presets.json" (builtins.toJSON softwarePresetsConfig);
  workspaceProfileFile = builtins.toFile "workspace-profile.json" workspaceProfileJSON;
  homeFilesProfile = import ./eval-home-files.nix { inherit lib; } homeFilesJSON;
  homeFilesFile = builtins.toFile "home-files.json" homeFilesJSON;
  workspaceCatalogFile = builtins.toFile "workspace-catalog.json" (builtins.toJSON workspaceCatalog);
  homeResetEphemeralPathsJson = builtins.toFile "home-reset-ephemeral-paths.json"
    (builtins.toJSON homeResetEphemeralPaths);
  clientGroupsJson = builtins.toFile "client-groups.json" (builtins.toJSON clientGroups);
  extensionModules = sharedModules
    ++ controllerModules
    ++ clientModules
    ++ netbootModules
    ++ lib.concatLists (lib.attrValues hostModules)
    ++ lib.concatLists (lib.attrValues groupModules);
  isModulePath = module: builtins.isPath module || builtins.isString module;
  unknownPublicKeyNames = builtins.attrNames (builtins.removeAttrs publicKeys [ "cache" "ssh" ]);
  unknownAssetNames = builtins.attrNames (builtins.removeAttrs assets [ "logo" "backgrounds" "mimeApps" "vscodeSettings" ]);
  validHomeResetEphemeralPath = path:
    builtins.isString path
    && path != ""
    && !(lib.hasPrefix "/" path)
    && !(builtins.elem ".." (lib.splitString "/" path));
  validRuntimeEphemeralPaths =
    builtins.length homeResetEphemeralPaths <= 128
    && builtins.length (lib.unique homeResetEphemeralPaths) == builtins.length homeResetEphemeralPaths
    && builtins.all (path:
      builtins.stringLength path <= 4096
      && builtins.match "[^[:cntrl:]]+" path != null
      && !(lib.hasInfix "\\" path)
      && builtins.all (part: part != "" && part != ".") (lib.splitString "/" path)
      && !(builtins.any (parent: lib.hasPrefix "${parent}/" path) homeResetEphemeralPaths)
    ) homeResetEphemeralPaths;
  validClientNames = map (n: "pc${padNumber n}") pcNumbers;
  validHostNames = validClientNames ++ [ masterHostName ];
  unknownHostModuleNames = builtins.attrNames (builtins.removeAttrs hostModules validHostNames);
  unknownModuleGroups = builtins.attrNames (builtins.removeAttrs groupModules (builtins.attrNames clientGroups));
  moduleGroupsForHost = name: builtins.filter
    (group: groupModules.${group} != [] && builtins.elem name clientGroups.${group})
    (builtins.attrNames groupModules);
  updateClientVariants = lib.groupBy (name: builtins.toJSON {
    moduleGroups = moduleGroupsForHost name;
    packages = lib.sort builtins.lessThan (lib.unique (map (entry: entry.package)
      (builtins.filter (entry: softwareAppliesTo name entry.scope) labSoftwareConfig.packages)));
    interface = ifaceForHost name;
    interfaceExplicit = ifaceExplicitForHost name;
  }) validClientNames;
  singleClassHosts = lib.unique ([ masterHostName ] ++ builtins.attrNames hostModules ++ updateValidationHosts);
  hostClasses = lib.sort (a: b: a.representative < b.representative) (
    map (name: { representative = name; members = [ name ]; }) singleClassHosts
    ++ map (members: { representative = builtins.head members; inherit members; })
      (builtins.filter (members: members != []) (map
        (members: builtins.filter (name: !(builtins.elem name singleClassHosts)) members)
        (builtins.attrValues updateClientVariants)))
  );
  # Classes of client computers (ADR 0033). The members of a class with
  # several computers share one system: each takes its name and laboratory
  # address from its identity record. A class with one member keeps the
  # per-host system of earlier releases. Ids are the computer's name for a
  # class of one and `shared-<n>` otherwise, numbered by first member, so a
  # software change does not rename every class.
  clientClasses = builtins.filter (class: class.representative != masterHostName) hostClasses;
  sharedClientClasses = builtins.filter (class: builtins.length class.members > 1) clientClasses;
  classIdFor = class:
    if builtins.length class.members == 1 then class.representative
    else "shared-${toString (1 + lib.lists.findFirstIndex
      (other: other.representative == class.representative) null sharedClientClasses)}";
  nixoriumClassFor = class: {
    id = classIdFor class;
    shared = builtins.length class.members > 1;
    inherit (class) members representative;
  };
  clientClassOf = builtins.listToAttrs (lib.concatMap
    (class: map (member: { name = member; value = class; }) class.members) clientClasses);
  controllerClass = {
    id = masterHostName;
    shared = false;
    members = [ masterHostName ];
    representative = masterHostName;
  };
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
  softwarePresetsConfig =
    if softwarePresets == null then null
    else import ./eval-software-presets.nix {
      inherit lib;
      validPackage = softwarePackageTools.validPath;
    } softwarePresets;
  labSoftwareConfig = import ./eval-lab-software.nix {
    inherit lib;
    pkgs = softwarePkgs;
    clientNames = validClientNames;
    inherit clientGroups;
    requireAvailable = false;
    allowUnfree = true;
  } labSoftware;
  # Base software is installed on every computer whatever lab-software.json
  # declares: Nixorium's own features need it. The Go review refuses to
  # remove it, reading the same list.
  baseSoftware = builtins.fromJSON (builtins.readFile ../internal/domain/base-software.json);
  hostSoftwareEntries = name:
    let declared = builtins.filter (entry: softwareAppliesTo name entry.scope) labSoftwareConfig.packages;
    in declared ++ map (package: { inherit package; scope = { kind = "shared"; }; })
      (builtins.filter (package: !(builtins.any (entry: entry.package == package) declared)) baseSoftware);
  softwareAppliesTo = name: scope:
    scope.kind == "shared"
    || (scope.kind == "controller" && name == masterHostName)
    || (scope.kind == "all-clients" && builtins.elem name validClientNames)
    || (scope.kind == "group" && builtins.elem name clientGroups.${scope.group})
    || (scope.kind == "clients" && builtins.elem name scope.clients);
  # Members of a class share their managed software by construction.
  managedSoftwareModule = { pkgs, nixoriumClass, ... }: {
    environment.systemPackages = map
      (entry:
        let package = lib.attrByPath (lib.splitString "." entry.package) null pkgs;
        in if package != null && lib.isDerivation package then package
        else throw "lab-software.json: package ${entry.package} is unavailable in the pinned package set")
      (hostSoftwareEntries nixoriumClass.representative);
  };
  # Resolve against actual declarative system packages, including GNOME core
  # packages and downstream modules, not just managed software declarations.
  # No live machine is inspected and no application is installed by this path.
  workspacePackagePaths = lib.unique (
    [ "vscode" "gnome-shell" "gnomeExtensions.dash-to-dock" ]
    ++ map (entry: entry.package) workspaceCatalog.applications
    ++ lib.concatMap (entry: entry.requiredPackages or []) workspaceCatalog.extensions
  );
  workspaceHostPackages = name:
    let installed = map toString hostConfigurations.${name}.config.environment.systemPackages; in
    builtins.filter (path:
      let
        info = softwarePackageTools.describe path;
        package = if info != null && info.availability == "available" then
          builtins.tryEval (toString (softwarePackageTools.resolve path))
        else { success = false; };
      in package.success && builtins.elem package.value installed
    ) workspacePackagePaths;
  # Hosts that share their role, interface, managed software and module groups
  # evaluate the same modules, so their system packages agree; hostModules
  # make a host its own class. Resolving one representative per class keeps
  # validation memory independent of the number of computers. A shared module
  # that changes packages by host name alone is outside this contract and
  # requires an explicit hostModules or updateValidationHosts exception.
  workspaceHostClass = name:
    if hostModules ? ${name} || builtins.elem name updateValidationHosts then "host ${name}"
    else builtins.toJSON {
      controller = name == masterHostName;
      moduleGroups = moduleGroupsForHost name;
      interface = ifaceForHost name;
      interfaceExplicit = ifaceExplicitForHost name;
      software = lib.sort builtins.lessThan (map (entry: entry.package) (hostSoftwareEntries name));
    };
  # The first host of each class, in inventory order, represents it.
  workspaceClassPackages = builtins.mapAttrs (_: workspaceHostPackages)
    (builtins.listToAttrs (map (name: { name = workspaceHostClass name; value = name; }) validHostNames));
  workspaceResolution =
    import ./resolve-workspace-profile.nix { inherit lib; pkgs = softwarePkgs; } {
      profileJSON = workspaceProfileJSON;
      catalog = workspaceCatalog;
      controllerName = masterHostName;
      clientNames = validClientNames;
      hostPackages = lib.genAttrs validHostNames (name: workspaceClassPackages.${workspaceHostClass name});
    };
  workspaceSeed =
    import ./build-workspace-home.nix { inherit lib; pkgs = softwarePkgs; } {
      resolution = workspaceResolution;
      inherit labSettings;
      inherit homeFilesProfile;
      homeFilesRoot = checkedHomeFilesRoot;
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
  remoteInstallerMetadata = bootstrapPkgs.runCommand "nixorium-remote-installer-metadata" {} ''
    install -d -m 0755 "$out"
    install -m 0644 ${labMetaJson} "$out/lab-meta.json"
    ${lib.optionalString (cachePublicKeyFile != null) ''
      install -m 0644 ${cachePublicKeyFile} "$out/cache-public-key"
    ''}
    ${lib.optionalString (adminSshKeyFile != null) ''
      install -m 0644 ${adminSshKeyFile} "$out/admin-ssh.pub"
    ''}
    printf '%s\n' 1 > "$out/protocol-version"
  '';
  remoteInstallerProgram = bootstrapPkgs.writeShellApplication {
    name = "nixorium-remote-client-installer";
    runtimeInputs = [
      bootstrapPkgs.bash
      bootstrapPkgs.btrfs-progs
      bootstrapPkgs.coreutils
      bootstrapPkgs.dosfstools
      bootstrapPkgs.efibootmgr
      bootstrapPkgs.findutils
      bootstrapPkgs.gawk
      bootstrapPkgs.gnugrep
      bootstrapPkgs.gnused
      bootstrapPkgs.gptfdisk
      bootstrapPkgs.iproute2
      bootstrapPkgs.jq
      bootstrapPkgs.nix
      bootstrapPkgs.nixos-install-tools
      bootstrapPkgs.parted
      bootstrapPkgs.systemd
      bootstrapPkgs.util-linux
    ] ++ installerDiskoRuntimePackages;
    text = ''
      export NIXORIUM_INSTALLER_LIB=${upstreamRoot}/scripts/lib/client-installer.sh
      export NIXORIUM_DISKO_SCRIPT=${installerDiskoScript}/bin/disko-destroy-format-mount
      export NIXORIUM_BUNDLE_SHARE=${remoteInstallerMetadata}
      export NIXORIUM_PLAN_VALIDATOR=${nixoriumPackage}/bin/nixorium-remote-validator
      NIXORIUM_REMOTE_PROGRAM="$(${bootstrapPkgs.coreutils}/bin/readlink -f -- "$0")"
      export NIXORIUM_REMOTE_PROGRAM
      export NIXORIUM_OPERATION_ROOT=/run/nixorium-remote-install
      export NIXORIUM_SYSTEMD_RUN=/run/current-system/sw/bin/systemd-run
      export NIXORIUM_OPERATION_OWNER=0:0
      unset NIXORIUM_TESTING
      exec ${upstreamRoot}/scripts/remote-client-installer.sh "$@"
    '';
  };
  remoteInstallerBundle = bootstrapPkgs.runCommand "nixorium-remote-installer-${masterHostName}" {} ''
    install -d -m 0755 "$out/bin" "$out/share/nixorium"
    ln -s ${remoteInstallerProgram}/bin/nixorium-remote-client-installer \
      "$out/bin/nixorium-remote-client-installer"
    ln -s ${installerDiskoScript}/bin/disko-destroy-format-mount \
      "$out/bin/disko-destroy-format-mount"
    cp -a ${remoteInstallerMetadata}/. "$out/share/nixorium/"
  '';
  # Laboratories build Nixorium from source on the controller. The Go tests
  # run in CI (tests/source-checks.nix builds the package with them); on a
  # busy controller they only add minutes, and a timing-sensitive test there
  # once failed an installation.
  nixoriumPackage = (bootstrapPkgs.callPackage (upstreamRoot + "/pkgs/nixorium.nix") {}).overrideAttrs { doCheck = false; };
  hostState = bootstrapPkgs.writeShellApplication {
    name = "nixorium-host-state";
    runtimeInputs = [ bootstrapPkgs.coreutils bootstrapPkgs.gnused ];
    text = ''
      system_path="$(readlink -f /run/current-system)"
      [[ "$system_path" == /nix/store/* && "$system_path" != *[[:space:]]* ]] \
        || { echo "active system path is invalid" >&2; exit 1; }
      revision="$(/run/current-system/sw/bin/nixos-version --configuration-revision)" \
        || { echo "active deployment revision is unavailable" >&2; exit 1; }
      [[ "$revision" =~ ^[0-9a-f]{40,64}$ ]] \
        || { echo "active deployment revision is invalid" >&2; exit 1; }
      # A third line names the computer number of a valid identity record,
      # which a shared system needs to know which computer it is (ADR 0033).
      number="$(sed -n 's/^{"schemaVersion":1,"number":\([1-9][0-9]\{0,3\}\)}$/\1/p' /var/lib/nixorium/identity.json 2>/dev/null || true)"
      if [[ -n "$number" ]]; then
        printf '%s\n%s\n%s\n' "$system_path" "$revision" "$number"
      else
        printf '%s\n%s\n' "$system_path" "$revision"
      fi
    '';
  };
  # Clients need only this helper in their system profile. The controller also
  # installs the complete package; use its identical target to avoid collisions.
  hostDiagnostics = bootstrapPkgs.runCommand "nixorium-host-diagnostics" {} ''
    mkdir -p "$out/bin"
    ln -s ${nixoriumPackage}/bin/nixorium-host-diagnostics "$out/bin/nixorium-host-diagnostics"
  '';
  hostSessionState = bootstrapPkgs.writeShellApplication {
    name = "nixorium-session-state";
    runtimeInputs = [ bootstrapPkgs.coreutils bootstrapPkgs.systemd bootstrapPkgs.util-linux bootstrapPkgs.glib ];
    text = builtins.readFile (upstreamRoot + "/scripts/session-state.sh");
  };

  # Reviewed removal of old system generations (ADR 0022). Its --plan mode is
  # read-only; --apply runs as root over SSH or in the controller unit.
  hostCleanGenerations = bootstrapPkgs.writeShellApplication {
    name = "nixorium-clean-generations";
    runtimeInputs = [ bootstrapPkgs.coreutils bootstrapPkgs.gnused ];
    text = builtins.readFile (upstreamRoot + "/scripts/clean-generations.sh");
  };

  installerFlake = bootstrapPkgs.writeText "nixorium-installer-flake.nix" ''
    {
      inputs = {
        nixpkgs.url = "path:${nixpkgs.outPath}";
        site = {
          url = "path:${deploymentRoot}";
          flake = false;
        };
        disko = {
          url = "path:${disko.outPath}";
          inputs.nixpkgs.follows = "nixpkgs";
        };
        nixorium = {
          url = "path:${upstreamRoot}";
          inputs.nixpkgs.follows = "nixpkgs";
          inputs.disko.follows = "disko";
        };
      };

      outputs = { self, nixorium, site, ... }:
        nixorium.lib.mkLab {
          deploymentSelf = site;
          labConfig = builtins.fromJSON (builtins.readFile ./lab-config.json);
          labSoftware = builtins.fromJSON (builtins.readFile ./lab-software.json);
          softwareCatalog = builtins.fromJSON (builtins.readFile ./software-catalog.json);
          softwarePresets = builtins.fromJSON (builtins.readFile ./software-presets.json);
          workspaceProfileJSON = builtins.readFile ./workspace-profile.json;
          workspaceCatalog = builtins.fromJSON (builtins.readFile ./workspace-catalog.json);
          homeFilesJSON = builtins.readFile ./home-files.json;
          homeFilesRoot = ${renderPath checkedHomeFilesRoot};
          homeResetEphemeralPaths = builtins.fromJSON (builtins.readFile ./home-reset-ephemeral-paths.json);
          clientGroups = builtins.fromJSON (builtins.readFile ./client-groups.json);
          publicKeys = {
            cache = ${renderPath cachePublicKeyFile};
            ssh = ${renderPath adminSshKeyFile};
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
          hostModules = ${renderModuleMap hostModules};
          groupModules = ${renderModuleMap groupModules};
          updateValidationHosts = builtins.fromJSON ${builtins.toJSON (builtins.toJSON updateValidationHosts)};
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
    install -m 0644 ${softwarePresetsJson} "$out/software-presets.json"
    install -m 0644 ${workspaceProfileFile} "$out/workspace-profile.json"
    install -m 0644 ${workspaceCatalogFile} "$out/workspace-catalog.json"
    install -m 0644 ${homeFilesFile} "$out/home-files.json"
    install -m 0644 ${homeResetEphemeralPathsJson} "$out/home-reset-ephemeral-paths.json"
    install -m 0644 ${clientGroupsJson} "$out/client-groups.json"
    install -m 0755 ${upstreamRoot}/setup.sh "$out/setup.sh"
    install -m 0755 ${installerDiskoScript}/bin/disko-destroy-format-mount "$out/disko-install"
    install -m 0644 ${upstreamRoot}/lib/disko-layout.nix "$out/lib/disko-layout.nix"
    install -m 0644 ${upstreamRoot}/scripts/lib/lab-meta.sh "$out/scripts/lib/lab-meta.sh"
    install -m 0644 ${upstreamRoot}/scripts/lib/client-installer.sh "$out/scripts/lib/client-installer.sh"
    install -m 0644 ${upstreamRoot}/scripts/lib/ui.sh "$out/scripts/lib/ui.sh"
    ${lib.optionalString (cachePublicKeyFile != null) ''
      install -m 0644 ${cachePublicKeyFile} "$out/public-key"
    ''}
  '';

  effectiveInstallerSource = if installerSource == null then installerBundle else installerSource;

  sharedClassSystems = builtins.listToAttrs (map (class: {
    name = classIdFor class;
    value = nixpkgs.lib.nixosSystem {
      inherit system;
      specialArgs = specialArgsForClass class;
      modules = modulesForHost class.representative false;
    };
  }) sharedClientClasses);

  # Every member of a shared class is an alias of the same value, so it is
  # evaluated and built once.
  mkHost = n:
    let
      name = "pc${padNumber n}";
      hostIp = mkHostIp n;
      class = clientClassOf.${name};
    in
    {
      inherit name;
      value =
        if builtins.length class.members > 1 then sharedClassSystems.${classIdFor class}
        else nixpkgs.lib.nixosSystem {
          inherit system;
          specialArgs = specialArgsForHost name hostIp;
          modules = modulesForHost name false;
        };
    };

  hostConfigurations = builtins.listToAttrs (map mkHost pcNumbers) // {
    ${masterHostName} = nixpkgs.lib.nixosSystem {
      inherit system;
      specialArgs = specialArgsForHost masterHostName masterIp;
      modules = modulesForHost masterHostName true;
    };
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
assert builtins.isAttrs groupModules && builtins.all builtins.isList (builtins.attrValues groupModules)
  || throw "groupModules must be an attribute set of module lists";
assert unknownModuleGroups == []
  || throw "groupModules contains unknown client groups: ${builtins.concatStringsSep ", " unknownModuleGroups}";
# Reuse the existing strict client-group validation, including controller exclusion.
assert groupModules == {} || builtins.seq labSoftwareConfig true;
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
assert builtins.isList updateValidationHosts && builtins.all
  (name: builtins.isString name && builtins.elem name validHostNames) updateValidationHosts
  || throw "updateValidationHosts must contain configured host names";
assert unknownHostModuleNames == []
  || throw "hostModules contains unknown hosts: ${builtins.concatStringsSep ", " unknownHostModuleNames}";
assert validRuntimeEphemeralPaths
  || throw "homeResetEphemeralPaths must be bounded, canonical and non-overlapping";
assert builtins.length labAssets.backgrounds <= 128
  || throw "the student home reset supports at most 128 wallpapers";
# Discovery does not authorize an operation. Keep inventory and package queries
# independent of all host module graphs; readiness, validators and every build
# or deployment output still require the complete workspace prerequisite check.
builtins.mapAttrs (name: value:
  if builtins.elem name [
    "labMeta" "nixoriumSoftware" "nixoriumSoftwarePresets"
    "nixoriumSearchSoftwarePackages" "nixoriumResolveSoftwarePackage"
    "nixoriumUpdateTargets" "nixoriumHostClasses"
  ] then value
  # Nix probes packages.<system>.<name> before a top-level installable. Keep
  # those namespace lookups cheap; force validation when selecting an entry.
  else if builtins.elem name [ "packages" "apps" ] then
    builtins.mapAttrs (_: entries:
      builtins.mapAttrs (_: entry: builtins.deepSeq workspaceResolution entry) entries
    ) value
  else builtins.deepSeq workspaceResolution value
) (rec {
  nixoriumComparisonConfigurations = comparisonLab.nixosConfigurations;
  nixosConfigurations = hostConfigurations // {
    netboot = nixpkgs.lib.nixosSystem {
      inherit system;
      specialArgs = {
        labSettings = labSettings // {
          ifaceName = ifaceForHost masterHostName;
          ifaceExplicit = true;
        };
        inherit labAssets;
        hostName = "netboot";
        hostIp = masterDhcpIp;
        # The installer image is not the controller; it trusts the controller cache.
        nixoriumRole = "client";
        isController = false;
      };
      modules = [
        "${nixpkgs}/nixos/modules/installer/netboot/netboot-minimal.nix"
        (upstreamRoot + "/modules/cache.nix")
        ({ pkgs, lib, ... }: {
          # The managed listener passes its preparation-time address at boot;
          # the installer uses that address explicitly for signed closure pulls.
          nix.settings.substituters = lib.mkForce [ ];
          networking.useDHCP = lib.mkForce true;
          # The console follows the same XKB layout as the installed system.
          services.xserver.xkb.layout = labSettings.keyboardLayout;
          console.useXkbConfig = true;
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
          # Start the guided installer once on the first console. It still
          # asks for the identity and the typed erase confirmation; nothing
          # runs unattended. Leaving it returns to an ordinary shell.
          programs.bash.loginShellInit = ''
            if [ "$(tty)" = /dev/tty1 ] && [ -x /installer/setup.sh ] && [ ! -e /tmp/nixorium-installer-offered ]; then
              : > /tmp/nixorium-installer-offered
              sudo /installer/setup.sh || true
              echo "To start the guided installer again, run: sudo /installer/setup.sh"
            fi
          '';
          services.getty.helpLine = lib.mkForce ''

            The Nixorium guided installer starts on the first console.
            To run it again: sudo /installer/setup.sh
          '';
        })
      ] ++ netbootModules;
    };
  };

  inherit labMeta;

  # Every configured host belongs to exactly one cheaply discovered class.
  nixoriumHostClasses =
    assert lib.sort builtins.lessThan (lib.concatMap (class: class.members) hostClasses)
      == lib.sort builtins.lessThan validHostNames
      || throw "nixoriumHostClasses must contain every configured host exactly once";
    hostClasses;

  # Build each materially different graph, not repeated identical client roles.
  # Private modules branching on hostName must declare extra validation hosts.
  nixoriumUpdateTargets = lib.unique (
    [ masterHostName ]
    ++ lib.optional (validClientNames != []) (builtins.head validClientNames)
    ++ builtins.attrNames hostModules
    # An exceptional first member must not hide the remaining group's graph.
    ++ map (class: class.representative) hostClasses
    ++ updateValidationHosts
  );

  # Build-time offline evaluation avoids import-from-derivation. Compare every
  # selected role with the self-contained installer, using its exact sources.
  nixoriumOfflineCheck = bootstrapPkgs.runCommand "nixorium-offline-equivalence" {
    nativeBuildInputs = [ bootstrapPkgs.nix ];
    expected = builtins.toJSON {
      systems = builtins.listToAttrs (map (name: {
        inherit name;
        # These are comparison strings, not build inputs. Keeping .drv context
        # would pull the entire build-time source closure into this check.
        value = builtins.unsafeDiscardStringContext nixosConfigurations.${name}.config.system.build.toplevel.drvPath;
      }) nixoriumUpdateTargets);
      # Check the complete preference metadata as well as system equivalence.
      workspace = nixoriumWorkspace;
    };
    passAsFile = [ "expected" ];
  } ''
    export XDG_CACHE_HOME="$TMPDIR/cache"
    mkdir -p "$XDG_CACHE_HOME"
    export NIX_CONFIG="experimental-features = nix-command flakes"
    nix eval --offline --store "$TMPDIR/nix" --no-write-lock-file --impure --json \
      --expr 'let f = builtins.getFlake "path:${installerBundle}"; in {
        systems = builtins.mapAttrs (name: _: f.nixosConfigurations.''${name}.config.system.build.toplevel.drvPath)
          (builtins.fromJSON (builtins.readFile ${builtins.toJSON (bootstrapPkgs.writeText "update-targets.json" (builtins.toJSON (builtins.listToAttrs (map (name: { inherit name; value = true; }) nixoriumUpdateTargets))))}));
        workspace = f.nixoriumWorkspace;
      }' > actual.json
    ${bootstrapPkgs.jq}/bin/jq -S . "$expectedPath" > expected.json
    ${bootstrapPkgs.jq}/bin/jq -S . actual.json > actual-sorted.json
    diff -u expected.json actual-sorted.json
    touch "$out"
  '';

  nixoriumSoftware = {
    schemaVersion = 1;
    managedFile = "lab-software.json";
    controller = masterHostName;
    clients = validClientNames;
    groups = clientGroups;
    catalog = resolvedSoftwareCatalog;
    packages = map (entry: entry // { origin = "managed"; }) labSoftwareConfig.packages;
  };

  nixoriumSoftwarePresets = softwarePresetsConfig;

  # Configured preferences are not an active-home receipt. Build and distribute
  # the systems, then boot them normally to restore the student home.
  nixoriumWorkspace = workspaceResolution // {
    state = "prepared";
    seed = toString workspaceSeed;
    managedFile = "workspace-profile.json";
    inherit studentUser;
  };
  # Return the exact resolved proposal without replacing deployment files.
  # Consumers must also invoke the public validation hook: a deployment may
  # wrap that hook with additional local policy.
  nixoriumResolveWorkspaceCandidate = rawJSON:
    assert builtins.isString rawJSON || throw "workspace candidate must be JSON text";
    let
      declared = (import ./eval-workspace-profile.nix { inherit lib; }) rawJSON;
      candidate = import ./mk-lab.nix { inherit upstreamSelf nixpkgs disko; }
        (args // { workspaceProfileJSON = rawJSON; });
    in
    # Update review validates the current declaration again through this hook.
    # Reuse only this evaluator's fully checked resolution of the same profile;
    # another process or changed source still resolves all prerequisites afresh.
    if declared == workspaceResolution.declared then
      nixoriumWorkspace
    else builtins.deepSeq candidate.nixoriumWorkspace candidate.nixoriumWorkspace;
  nixoriumValidateWorkspaceCandidate = rawJSON:
    builtins.deepSeq (nixoriumResolveWorkspaceCandidate rawJSON) true;

  nixoriumHomeFiles = {
    schemaVersion = 1;
    state = "prepared";
    managedFile = "home-files.json";
    declared = homeFilesProfile;
    inherit studentUser;
    targets = workspaceResolution.targets;
    seed = toString workspaceSeed;
  };
  nixoriumResolveHomeFilesCandidate = rawJSON:
    let candidate = import ./mk-lab.nix { inherit upstreamSelf nixpkgs disko; }
      (args // { homeFilesJSON = rawJSON; });
    in builtins.deepSeq candidate.nixoriumHomeFiles candidate.nixoriumHomeFiles;
  nixoriumValidateHomeFilesCandidate = rawJSON:
    builtins.deepSeq (nixoriumResolveHomeFilesCandidate rawJSON) true;

  nixoriumSearchSoftwarePackages = softwarePackageTools.search;
  nixoriumResolveSoftwarePackage = softwarePackageTools.describe;
  # Controller validation stays upstream; the site software hook validates
  # clients. Reuse every downstream extension unchanged.
  nixoriumValidateControllerSoftwareCandidate = rawSoftware:
    let
      candidate = import ./mk-lab.nix { inherit upstreamSelf nixpkgs disko; }
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

  apps.${system} = {
    nixorium = {
      type = "app";
      program = "${nixoriumPackage}/bin/nixorium";
      meta.description = "Manage the Nixorium laboratory";
    };
    disko = {
      type = "app";
      program = "${runDisko}/bin/nixorium-disko";
      meta.description = "Run the Disko revision pinned by this deployment";
    };
  };

  packages.${system} = {
    inherit installerBundle pxeFirmware remoteInstallerBundle;
    disko = runDisko;
    nixorium = nixoriumPackage;
  };
})
