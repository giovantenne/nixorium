{ lib, pkgs, clientNames, clientGroups ? {}, requireAvailable ? true }:
rawSoftware:
let
  packageTools = import ./software-packages.nix { inherit lib pkgs; };
  fail = message: throw "lab-software.json: ${message}";
  software =
    if builtins.isAttrs rawSoftware then rawSoftware
    else fail "top-level value must be an object";
  unknownKeys = builtins.attrNames (builtins.removeAttrs software [ "schemaVersion" "packages" ]);
  validPackageName = value:
    builtins.isString value
    && builtins.stringLength value <= 80
    && packageTools.validPath value;
  validClient = name: builtins.isString name && builtins.elem name clientNames;
  groupNames = builtins.attrNames clientGroups;
  invalidGroups = builtins.filter
    (name:
      let clients = clientGroups.${name}; in
      builtins.match "[A-Za-z0-9][A-Za-z0-9._-]{0,39}" name == null
      || !builtins.isList clients
      || clients == []
      || builtins.length clients != builtins.length (lib.unique clients))
    groupNames;
  invalidGroupClients = lib.concatLists (lib.mapAttrsToList
    (name: clients:
      if !builtins.isList clients then [ name ]
      else builtins.filter (client: !validClient client) clients)
    clientGroups);
  normalizeScope = index: scope:
    let
      prefix = "packages[${toString index}].scope";
      value = if builtins.isAttrs scope then scope else fail "${prefix} must be an object";
      kind = value.kind or (fail "${prefix}.kind is required");
      allowedKeys =
        if builtins.elem kind [ "shared" "controller" "all-clients" ] then [ "kind" ]
        else if kind == "group" then [ "kind" "group" ]
        else if kind == "clients" then [ "kind" "clients" ]
        else fail "${prefix}.kind must be shared, controller, all-clients, group, or clients";
      extras = builtins.attrNames (builtins.removeAttrs value allowedKeys);
      group = value.group or null;
      clients = value.clients or [];
    in
    assert extras == [] || fail "${prefix} has unknown fields: ${builtins.concatStringsSep ", " extras}";
    assert kind != "group" || (builtins.isString group && builtins.elem group groupNames)
      || fail "${prefix}.group must name an evaluated client group";
    assert kind != "clients" || (builtins.isList clients && clients != [] && builtins.all validClient clients)
      || fail "${prefix}.clients must contain configured client identities";
    assert kind != "clients" || builtins.length clients == builtins.length (lib.unique clients)
      || fail "${prefix}.clients contains duplicate identities";
    if builtins.elem kind [ "shared" "controller" "all-clients" ] then { inherit kind; }
    else if kind == "group" then { inherit kind group; }
    else { inherit kind; clients = lib.sort builtins.lessThan clients; };
  normalizePackage = index: entry:
    let
      prefix = "packages[${toString index}]";
      value = if builtins.isAttrs entry then entry else fail "${prefix} must be an object";
      extras = builtins.attrNames (builtins.removeAttrs value [ "package" "scope" ]);
      package = value.package or (fail "${prefix}.package is required");
      scope = normalizeScope index (value.scope or (fail "${prefix}.scope is required"));
      resolved = if validPackageName package then packageTools.resolve package else null;
      packageInfo = if validPackageName package then packageTools.describe package else null;
    in
    assert extras == [] || fail "${prefix} has unknown fields: ${builtins.concatStringsSep ", " extras}";
    assert validPackageName package || fail "${prefix}.package is invalid";
    assert !requireAvailable || (resolved != null && lib.isDerivation resolved && packageInfo != null && packageInfo.availability == "available")
      || fail "${prefix}.package is unavailable or blocked in the pinned package set";
    { inherit package scope; };
  packages =
    if software ? packages && builtins.isList software.packages then
      lib.imap0 normalizePackage software.packages
    else
      fail "packages must be a list";
  packageNames = map (entry: entry.package) packages;
in
assert unknownKeys == []
  || fail "unknown top-level fields: ${builtins.concatStringsSep ", " unknownKeys}";
assert software ? schemaVersion && builtins.isInt software.schemaVersion
  || fail "schemaVersion is required and must be an integer";
assert software.schemaVersion == 1
  || fail "unsupported schemaVersion ${toString software.schemaVersion}; expected 1";
assert invalidGroupClients == []
  || fail "clientGroups contains invalid client identities";
assert invalidGroups == []
  || fail "clientGroups contains an empty, duplicate, or invalid group";
assert builtins.length packageNames == builtins.length (lib.unique packageNames)
  || fail "each package may be declared only once";
{
  schemaVersion = 1;
  inherit packages;
}
