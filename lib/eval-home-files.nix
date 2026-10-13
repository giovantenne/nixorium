{ lib }:
json:
let
  fail = message: throw "home files: ${message}";
  checkedJSON = if builtins.isString json && builtins.stringLength json <= 1048576
    && !(lib.hasPrefix (builtins.fromJSON ''"\uFEFF"'') json)
    # fromJSON cannot represent NUL and its primitive error bypasses tryEval.
    && builtins.length (builtins.split ''(^|[^\\])(\\\\)*\\u0000'' json) == 1
    then json else fail "expected UTF-8 JSON of at most 1 MiB";
  raw = builtins.fromJSON checkedJSON;
  gaps = builtins.filter builtins.isString (builtins.split ''"([^"\\]|\\.)*"'' checkedJSON);
  memberCount = lib.foldl' (n: gap: n + builtins.length (lib.splitString ":" gap) - 1) 0 gaps;
  countMembers = depth: value:
    if depth > 8 then fail "nesting limit exceeded"
    else if builtins.isAttrs value then builtins.length (builtins.attrNames value)
      + lib.foldl' (n: child: n + countMembers (depth + 1) child) 0 (builtins.attrValues value)
    else if builtins.isList value then lib.foldl' (n: child: n + countMembers (depth + 1) child) 0 value
    else if value == null then fail "null is not supported"
    else 0;
  policy = builtins.fromJSON (builtins.readFile ../internal/domain/home_files_policy.json);
  sensitive = path: builtins.any (part:
    builtins.elem part policy.sensitiveComponents
    || builtins.any (prefix: lib.hasPrefix prefix part) policy.sensitivePrefixes
  ) (lib.splitString "/" path);
  validDestination = directory: path:
    builtins.isString path && path != "" && builtins.stringLength path <= 512
    && builtins.match ".*[[:cntrl:]\\\\].*" path == null
    && builtins.length (lib.splitString "/" path) <= 12
    && builtins.all (part: part != "" && part != "." && part != "..") (lib.splitString "/" path)
    && !(sensitive path)
    && !(builtins.any (protected: path == protected || lib.hasPrefix "${protected}/" path
      || (!directory && lib.hasPrefix "${path}/" protected)) policy.protectedRoots)
    && (directory || !(builtins.elem path policy.scaffoldDirectories));
  object = keys: value:
    if !builtins.isAttrs value || builtins.any (key: !(builtins.elem key keys)) (builtins.attrNames value)
    then fail "unsupported field or invalid object" else value;
  sensitiveText = text: builtins.match ".*(-----begin [a-z0-9 ]*private key-----|age-secret-key-|akia[0-9a-z]{16}|(sk-|ghp_|github_pat_)[a-z0-9_-]{16,}|\\$6\\$|bearer[ \t]+[a-z0-9._~+/=-]{16,}).*" (lib.toLower text) != null;
  file = value:
    let entry = object [ "destination" "text" "source" "executable" ] value; in
    if !(entry ? destination) || !(validDestination false entry.destination)
      then fail "invalid or protected file destination"
    else if (entry ? text) == (entry ? source) then fail "expected exactly one of text or source"
    else if entry ? source && (!builtins.isString entry.source || builtins.match "imports/[0-9a-f]{64}" entry.source == null)
      then fail "invalid imported source"
    else if entry ? text && (!builtins.isString entry.text || builtins.stringLength entry.text > 65536
      || sensitiveText entry.text) then fail "invalid text or credential material"
    else if entry ? executable && !builtins.isBool entry.executable then fail "expected a boolean"
    else entry;
  profile = object [ "schemaVersion" "files" "directories" ] raw;
  files = if builtins.isList (profile.files or []) then map file (profile.files or []) else fail "expected files list";
  directories = if builtins.isList (profile.directories or [])
    && builtins.all (validDestination true) (profile.directories or [])
    then profile.directories or [] else fail "invalid directories";
  paths = (map (entry: entry.destination) files) ++ directories;
  normalized = { schemaVersion = 1; }
    // lib.optionalAttrs (files != []) { files = map (entry: lib.filterAttrs (_: value: value != false) entry)
      (lib.sort (a: b: a.destination < b.destination) files); }
    // lib.optionalAttrs (directories != []) { directories = lib.sort builtins.lessThan directories; };
in
if memberCount != countMembers 0 raw then fail "duplicate field"
else if !(profile ? schemaVersion) || profile.schemaVersion != 1 || !builtins.isInt profile.schemaVersion
  then fail "unsupported schemaVersion"
else if builtins.length paths > 512 || builtins.length paths != builtins.length (lib.unique paths)
  then fail "too many or duplicate destinations"
else if builtins.any (entry: builtins.any (path: lib.hasPrefix "${entry.destination}/" path) paths) files
  then fail "file conflicts with another destination"
else builtins.deepSeq normalized normalized
