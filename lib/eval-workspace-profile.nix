{ lib }:
# Accept JSON text, not an already-decoded attrset: fromJSON alone silently
# discards duplicate object keys. Keep the textual boundary for all callers.
json:
let
  fail = message: throw "workspace profile: ${message}";
  checkedJSON = if builtins.isString json && builtins.stringLength json <= 65536
    then
      # fromJSON accepts an initial BOM, but the Go JSON decoder does not.
      if lib.hasPrefix (builtins.fromJSON ''"\uFEFF"'') json then fail "UTF-8 BOM is not supported"
      else json
    else fail "expected JSON text of at most 65536 bytes";
  raw = builtins.fromJSON checkedJSON;
  # Every colon outside a JSON string introduces one member. Comparing that
  # count with the parsed tree detects duplicate keys, including escaped names,
  # without implementing a second JSON parser. fromJSON owns syntax checking.
  gaps = builtins.filter builtins.isString (builtins.split ''"([^"\\]|\\.)*"'' checkedJSON);
  memberCount = lib.foldl' (count: gap: count + builtins.length (lib.splitString ":" gap) - 1) 0 gaps;
  countMembers = depth: value:
    if depth > 8 then fail "nesting limit exceeded"
    else if builtins.isAttrs value then
      builtins.length (builtins.attrNames value)
      + lib.foldl' (count: child: count + countMembers (depth + 1) child) 0 (builtins.attrValues value)
    else if builtins.isList value then
      lib.foldl' (count: child: count + countMembers (depth + 1) child) 0 value
    else 0;
  object = fields: value:
    if !builtins.isAttrs value then fail "expected an object"
    else if builtins.any (key: !(builtins.hasAttr key fields)) (builtins.attrNames value)
    then fail "unsupported field"
    else builtins.mapAttrs (key: item: fields.${key} item) value;
  boolean = value: if builtins.isBool value then value else fail "expected a boolean";
  integer = min: max: value:
    if builtins.isInt value && value >= min && value <= max then value
    else fail "integer outside supported range";
  enum = values: value:
    if builtins.isString value && builtins.elem value values then value else fail "unsupported enum value";
  identifier = pattern: value:
    if builtins.isString value && builtins.stringLength value <= 128 && builtins.match pattern value != null
    then value else fail "invalid identifier";
  desktopID = identifier "[A-Za-z0-9][A-Za-z0-9_.-]*\\.desktop";
  extensionID = identifier "[a-z0-9][a-z0-9-]*\\.[a-z0-9][a-z0-9-]*";
  list = max: check: value:
    if !builtins.isList value || builtins.length value > max then fail "invalid list length"
    else let checked = map check value; in
      if builtins.length (lib.unique checked) != builtins.length checked then fail "duplicate list entry"
      else checked;
  # Free-form editor defaults are reviewed text, not typed policy. Keep the
  # typed fields, managed update keys and program-launching settings out of it.
  typedSettings = [
    "editor.fontSize" "editor.tabSize" "editor.insertSpaces" "editor.wordWrap"
    "editor.formatOnSave" "editor.minimap.enabled" "files.autoSave"
  ];
  deniedSettings = typedSettings ++ [
    "update.mode" "extensions.autoUpdate" "extensions.autoCheckUpdates"
    "task.allowAutomaticTasks"
  ];
  deniedSettingPrefixes = [
    "security.workspace.trust."
    "terminal.integrated.profiles."
    "terminal.integrated.automationProfile."
    "terminal.integrated.shell."
    "terminal.integrated.shellArgs."
    "terminal.integrated.env."
  ];
  settingValue = value:
    if value == null then fail "null is not supported"
    else if builtins.isInt value && (value > 9007199254740992 || value < -9007199254740992)
    then fail "integer outside supported range"
    else if builtins.isAttrs value then builtins.mapAttrs (_: settingValue) value
    else if builtins.isList value then map settingValue value
    else value;
  extraSettings = value:
    if !builtins.isAttrs value then fail "expected an object"
    else if builtins.length (builtins.attrNames value) > 256 then fail "too many extra settings"
    else builtins.mapAttrs (key: item:
      if builtins.stringLength key > 128
        || builtins.match "[[A-Za-z0-9][]A-Za-z0-9_.[-]*" key == null
      then fail "invalid setting name"
      else if builtins.elem key deniedSettings
        || builtins.any (prefix: lib.hasPrefix prefix key) deniedSettingPrefixes
      then fail "setting is not supported as an extra setting"
      else settingValue item
    ) value;
  # Marketplace pins name exact bytes (hash) of one extension version. The
  # download URL is derived by the builder; the profile never carries one.
  marketplaceEntry = value:
    let
      entry = object {
        publisher = identifier "[A-Za-z0-9][A-Za-z0-9-]*";
        name = identifier "[A-Za-z0-9][A-Za-z0-9-]*";
        version = identifier "[0-9]+(\\.[0-9]+){1,3}";
        hash = identifier "sha256-[A-Za-z0-9+/]{43}=";
        platform = enum [ "linux-x64" ];
      } value;
    in
    if !(builtins.all (key: entry ? ${key}) [ "publisher" "name" "version" "hash" ])
    then fail "incomplete Marketplace extension"
    else entry;
  marketplaceID = entry: lib.toLower "${entry.publisher}.${entry.name}";
  marketplace = value:
    let
      entries = list 32 marketplaceEntry value;
      ids = map marketplaceID entries;
    in
    if builtins.length ids != builtins.length (lib.unique ids) then fail "duplicate Marketplace extension"
    else if !(builtins.all (id: builtins.stringLength id <= 128) ids) then fail "invalid identifier"
    else lib.sort (a: b: marketplaceID a < marketplaceID b) entries;
  profile = object {
    schemaVersion = integer 1 1;
    desktop = object {
      favorites = list 32 desktopID;
      colorScheme = enum [ "light" "dark" ];
      enableAnimations = boolean;
      dock = object {
        position = enum [ "top" "bottom" "left" "right" ];
        iconSize = integer 16 128;
        autoHide = boolean;
        extendHeight = boolean;
        showTrash = boolean;
        showMounts = boolean;
      };
    };
    vscode = object {
      extensions = value: lib.sort builtins.lessThan (list 64 extensionID value);
      settings = object {
        "editor.fontSize" = integer 8 40;
        "editor.tabSize" = integer 1 8;
        "editor.insertSpaces" = boolean;
        "editor.wordWrap" = enum [ "off" "on" "wordWrapColumn" "bounded" ];
        "editor.formatOnSave" = boolean;
        "editor.minimap.enabled" = boolean;
        "files.autoSave" = enum [ "off" "onFocusChange" "onWindowChange" ];
      };
      inherit extraSettings;
      inherit marketplace;
    };
    browser = object { defaultApplication = desktopID; };
  } raw;
in
if memberCount != countMembers 0 raw then fail "duplicate field"
else if !(profile ? schemaVersion) then fail "missing schemaVersion"
else builtins.deepSeq profile profile
