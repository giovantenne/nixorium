{ lib, pkgs }:
# Pure preparation only. Callers supply the complete target inventory and the
# package attributes declared for each host, including framework capabilities.
# This does not observe live machines or qualify application/plugin loading.
{ profileJSON, catalog, controllerName, clientNames, hostPackages }:
let
  fail = message: throw "workspace resolution: ${message}";
  decode = import ./eval-workspace-profile.nix { inherit lib; };
  # Selected Marketplace pins join the package set for this resolution only.
  pinnedPkgs = import ./workspace-marketplace.nix { inherit lib pkgs; } (effective.vscode.marketplace or []);
  packageTools = import ./software-packages.nix { inherit lib; pkgs = pinnedPkgs; allowUnfree = true; };
  marketplaceIDs = map (entry: lib.toLower "${entry.publisher}.${entry.name}") (effective.vscode.marketplace or []);
  object = allowed: required: value:
    if !builtins.isAttrs value then fail "expected a catalog object"
    else if builtins.removeAttrs value allowed != {} then fail "unsupported catalog field"
    else if !(builtins.all (key: builtins.hasAttr key value) required) then fail "missing catalog field"
    else value;
  list = max: value:
    if builtins.isList value && builtins.length value <= max then value
    else fail "invalid catalog list";
  unique = values:
    if builtins.length values == builtins.length (lib.unique values) then values
    else fail "duplicate catalog entry";
  packagePath = value:
    if packageTools.validPath value then value else fail "invalid package attribute";
  desktopID = value: (decode (builtins.toJSON {
    schemaVersion = 1;
    browser.defaultApplication = value;
  })).browser.defaultApplication;
  extensionID = value: builtins.head (decode (builtins.toJSON {
    schemaVersion = 1;
    vscode.extensions = [ value ];
  })).vscode.extensions;
  packageList = values: lib.sort builtins.lessThan (unique (map packagePath (list 32 values)));
  extensionList = values: lib.sort builtins.lessThan (unique (map extensionID (list 64 values)));
  rawCatalog = object [ "schemaVersion" "baseline" "applications" "extensions" ]
    [ "schemaVersion" "baseline" "applications" "extensions" ] catalog;
  baseline = decode (builtins.toJSON rawCatalog.baseline);
  declared = decode profileJSON;
  # recursiveUpdate replaces lists, rather than appending them: [] must clear
  # baseline favorites/extensions. Revalidate the merged result as one profile.
  effective = decode (builtins.toJSON (lib.recursiveUpdate baseline declared));
  normalizeApplication = raw:
    let
      item = object [ "id" "package" "browser" ] [ "id" "package" ] raw;
      browser = item.browser or false;
    in
    if !builtins.isBool browser then fail "application.browser must be boolean"
    else { id = desktopID item.id; package = packagePath item.package; inherit browser; };
  normalizeExtension = raw:
    let
      item = object [ "id" "package" "requiredPackages" "requiredExtensions" "writable" ] [ "id" "package" ] raw;
      id = extensionID item.id;
      package = packagePath item.package;
      # Copied rather than linked: the extension writes into its own folder.
      writable = item.writable or false;
    in
    if package != "vscode-extensions.${id}" then fail "extension package must match its pinned vscode-extensions ID"
    else if !builtins.isBool writable then fail "extension.writable must be boolean"
    else {
      inherit id package writable;
      requiredPackages = packageList (item.requiredPackages or []);
      requiredExtensions = extensionList (item.requiredExtensions or []);
    };
  applications = map normalizeApplication (list 128 rawCatalog.applications);
  extensions = map normalizeExtension (list 128 rawCatalog.extensions);
  applicationIDs = unique (map (entry: entry.id) applications);
  extensionIDs = unique (map (entry: entry.id) extensions);
  applicationsByID = builtins.listToAttrs (map (entry: { name = entry.id; value = entry; }) applications);
  extensionsByID = builtins.listToAttrs (map (entry: { name = entry.id; value = entry; }) extensions);
  application = id: applicationsByID.${id} or (fail "desktop application is not in the deployment catalog");
  # The catalog records what a packaged extension needs; it is not an
  # allowlist. Any other ID must still be an extension of the pinned set.
  extension = id: extensionsByID.${id} or {
    inherit id;
    package = "vscode-extensions.${id}";
    requiredPackages = [];
    requiredExtensions = [];
    writable = false;
  };
  favorites = map application (effective.desktop.favorites or []);
  browser = if effective ? browser.defaultApplication
    then application effective.browser.defaultApplication else null;
  selectedExtensions = map extension (effective.vscode.extensions or []);
  selectedIDs = map (entry: entry.id) selectedExtensions;
  requiredPackages = lib.sort builtins.lessThan (lib.unique (
    map (entry: entry.package) favorites
    ++ lib.optional (browser != null) browser.package
    ++ lib.optional (effective ? vscode) "vscode"
    ++ lib.optional (effective ? desktop) "gnome-shell"
    ++ lib.optional (effective ? desktop.dock) "gnomeExtensions.dash-to-dock"
    ++ lib.concatMap (entry: entry.requiredPackages) selectedExtensions
  ));
  describe = package:
    let info = packageTools.describe package; in
    if info == null || info.availability != "available" || info.version == ""
    then fail "required package ${package} is unavailable, blocked or unversioned in the pin"
    else { inherit package; inherit (info) version; };
  resolvedPackages = map describe requiredPackages;
  resolvedExtensions = map (entry:
    let
      info = describe entry.package;
      pkg = packageTools.resolve entry.package;
    in
    # Marketplace identities are case-insensitive; profile IDs are lowercase.
    if !builtins.isString (pkg.vscodeExtUniqueId or null) || lib.toLower pkg.vscodeExtUniqueId != entry.id
    then fail "extension identity does not match its package metadata"
    else entry // { inherit (info) version; }
  ) selectedExtensions;
  validHost = value:
    builtins.isString value && builtins.match "[a-z0-9][a-z0-9-]{0,62}" value != null;
  targets = [ controllerName ] ++ clientNames;
  expectedNames = lib.sort builtins.lessThan targets;
  checkHost = name:
    let
      packages = hostPackages.${name};
      missing = builtins.filter (package: !(builtins.elem package packages)) requiredPackages;
    in
    if !builtins.isList packages || !(builtins.all packageTools.validPath packages)
    then fail "invalid host package declaration"
    else if missing != [] then fail "${name} is missing required packages: ${lib.concatStringsSep ", " missing}"
    else { inherit name; role = if name == controllerName then "controller" else "client"; };
  normalizedCatalog = {
    schemaVersion = 1;
    inherit baseline;
    applications = lib.sort (a: b: a.id < b.id) applications;
    extensions = lib.sort (a: b: a.id < b.id) extensions;
  };
  result = {
    schemaVersion = 1;
    inherit declared effective requiredPackages;
    catalog = normalizedCatalog;
    targets = map checkHost targets;
    packages = resolvedPackages;
    extensions = resolvedExtensions;
  };
in
assert builtins.isInt rawCatalog.schemaVersion && rawCatalog.schemaVersion == 1
  || fail "unsupported catalog schemaVersion";
assert validHost controllerName && builtins.isList clientNames && builtins.all validHost clientNames
  || fail "invalid target inventory";
assert builtins.length targets == builtins.length (lib.unique targets)
  || fail "duplicate target identity";
assert builtins.isAttrs hostPackages && builtins.attrNames hostPackages == expectedNames
  || fail "host packages must cover exactly the controller and all clients";
assert builtins.deepSeq [ applicationIDs extensionIDs ] true;
assert builtins.all (entry:
  !(builtins.elem entry.id entry.requiredExtensions)
  && builtins.all (id: builtins.elem id extensionIDs) entry.requiredExtensions
) extensions || fail "extension dependencies must name other catalog entries";
assert browser == null || browser.browser || fail "default application is not catalogued as a browser";
assert builtins.all (id: builtins.elem id selectedIDs) marketplaceIDs
  || fail "every Marketplace pin must be a selected extension";
assert builtins.all (entry: builtins.all (id: builtins.elem id selectedIDs) entry.requiredExtensions) selectedExtensions
  || fail "required extensions must be selected explicitly";
builtins.deepSeq result result
