{ lib, pkgs, allowUnfree ? false }:
let
  validSegment = value:
    builtins.isString value
    && value != ""
    && builtins.match "[A-Za-z0-9][A-Za-z0-9+_-]*" value != null;
  splitPath = path:
    if builtins.isString path && builtins.stringLength path <= 120 then
      lib.splitString "." path
    else
      [];
  validPath = path:
    let segments = splitPath path;
    in segments != [] && builtins.all validSegment segments;
  resolve = path:
    if validPath path then lib.attrByPath (splitPath path) null pkgs else null;
  describe = path:
    let
      inspected = builtins.tryEval (
        let
          package = resolve path;
          meta = if package != null && package ? meta && builtins.isAttrs package.meta then package.meta else {};
          vulnerabilities = meta.knownVulnerabilities or [];
          licenses =
            if !(meta ? license) then []
            else if builtins.isList meta.license then meta.license
            else [ meta.license ];
          unfree = builtins.any
            (license: builtins.isAttrs license && !(license.free or true))
            licenses;
          broken = meta.broken or false;
          platformAvailable = package != null && lib.isDerivation package && lib.meta.availableOn pkgs.stdenv.hostPlatform package;
          availability =
            if broken then "blocked-broken"
            else if builtins.isList vulnerabilities && vulnerabilities != [] then "blocked-insecure"
            else if unfree && !allowUnfree then "blocked-unfree"
            else if !platformAvailable then "unavailable-platform"
            else "available";
          label = package.pname or package.name or path;
          summary = meta.description or "No package description is available.";
          version = package.version or "";
        in
        if package == null || !lib.isDerivation package then null else {
          id = path;
          label = if builtins.isString label then label else path;
          summary = if builtins.isString summary then summary else "No package description is available.";
          version = if builtins.isString version then version else "";
          inherit availability;
        });
    in
    if inspected.success then inspected.value else null;
  search = request:
    let
      query = request.query or "";
      limit = request.limit or 40;
      segments = splitPath query;
      parentPath = if builtins.length segments > 1 then lib.init segments else [];
      needle = if segments == [] then "" else lib.last segments;
      parent = if builtins.all validSegment parentPath then lib.attrByPath parentPath null pkgs else null;
      names = if builtins.isAttrs parent then builtins.attrNames parent else [];
      matchingNames = builtins.filter
        (name: validSegment name && lib.hasInfix (lib.toLower needle) (lib.toLower name))
        names;
      paths = map
        (name: builtins.concatStringsSep "." (parentPath ++ [ name ]))
        matchingNames;
      results = builtins.filter (item: item != null) (map describe paths);
    in
    if !validPath query || !builtins.isInt limit || limit < 1 || limit > 100 then
      []
    else
      lib.take limit results;
in
{
  inherit describe resolve search validPath;
}
