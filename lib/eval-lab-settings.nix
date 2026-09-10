{ lib }:
rawSettings:
let
  fail = message: throw "lab-settings.json: ${message}";
  settings =
    if builtins.isAttrs rawSettings then rawSettings
    else fail "top-level value must be an object";
  unknownKeys = builtins.attrNames (builtins.removeAttrs settings [ "schemaVersion" "lab" ]);
  evalLabConfig = import ./eval-lab-config.nix { inherit lib; };
in
assert unknownKeys == []
  || fail "unknown top-level settings: ${builtins.concatStringsSep ", " unknownKeys}";
assert settings ? schemaVersion
  || fail "schemaVersion is required";
assert builtins.isInt settings.schemaVersion
  || fail "schemaVersion must be an integer";
assert settings.schemaVersion == 1
  || fail "unsupported schemaVersion ${toString settings.schemaVersion}; expected 1";
assert settings ? lab
  || fail "lab is required";
assert builtins.isAttrs settings.lab
  || fail "lab must be an object";
evalLabConfig settings.lab
