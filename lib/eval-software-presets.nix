{ lib, validPackage }:
rawCatalog:
let
  fail = message: throw "software presets: ${message}";
  catalog =
    if builtins.isAttrs rawCatalog then rawCatalog
    else fail "top-level value must be an object";
  unknownKeys = builtins.attrNames
    (builtins.removeAttrs catalog [ "schemaVersion" "defaultPreset" "presets" ]);
  validPresetId = value:
    builtins.isString value
    && builtins.match "[a-z0-9][a-z0-9-]{0,39}" value != null;
  normalizePreset = index: rawPreset:
    let
      prefix = "presets[${toString index}]";
      preset =
        if builtins.isAttrs rawPreset then rawPreset
        else fail "${prefix} must be an object";
      extras = builtins.attrNames
        (builtins.removeAttrs preset [ "id" "label" "description" "packages" ]);
      id = preset.id or (fail "${prefix}.id is required");
      label = preset.label or (fail "${prefix}.label is required");
      description = preset.description or (fail "${prefix}.description is required");
      packages = preset.packages or (fail "${prefix}.packages is required");
    in
    assert extras == []
      || fail "${prefix} has unknown fields: ${builtins.concatStringsSep ", " extras}";
    assert validPresetId id || fail "${prefix}.id is invalid";
    assert builtins.isString label && label != "" && builtins.stringLength label <= 80
      || fail "${prefix}.label must be 1 to 80 characters";
    assert builtins.isString description && description != "" && builtins.stringLength description <= 240
      || fail "${prefix}.description must be 1 to 240 characters";
    assert builtins.isList packages && packages != []
      || fail "${prefix}.packages must be a non-empty list";
    assert builtins.all validPackage packages
      || fail "${prefix}.packages contains an invalid package id";
    assert builtins.length packages == builtins.length (lib.unique packages)
      || fail "${prefix}.packages contains duplicate package ids";
    {
      inherit id label description;
      packages = lib.sort builtins.lessThan packages;
    };
  presets =
    if catalog ? presets && builtins.isList catalog.presets then
      lib.imap0 normalizePreset catalog.presets
    else
      fail "presets is required and must be a list";
  presetIds = map (preset: preset.id) presets;
  defaultPreset = catalog.defaultPreset or (fail "defaultPreset is required");
in
assert unknownKeys == []
  || fail "unknown top-level fields: ${builtins.concatStringsSep ", " unknownKeys}";
assert catalog ? schemaVersion && builtins.isInt catalog.schemaVersion
  || fail "schemaVersion is required and must be an integer";
assert catalog.schemaVersion == 1
  || fail "unsupported schemaVersion ${toString catalog.schemaVersion}; expected 1";
assert presets != [] || fail "presets must not be empty";
assert builtins.length presetIds == builtins.length (lib.unique presetIds)
  || fail "preset ids must be unique";
assert validPresetId defaultPreset && builtins.elem defaultPreset presetIds
  || fail "defaultPreset must name a catalog preset";
{
  schemaVersion = 1;
  inherit defaultPreset presets;
}
