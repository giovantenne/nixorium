{ lib }:
let
  evaluate = raw:
    (builtins.tryEval (builtins.deepSeq (import ../lib/eval-software-presets.nix {
      inherit lib;
      validPackage = value:
        builtins.isString value
        && builtins.match "[A-Za-z0-9][A-Za-z0-9+_-]*(\\.[A-Za-z0-9][A-Za-z0-9+_-]*)*" value != null;
    } raw) true)).success;
  cases = builtins.fromJSON (builtins.readFile ./software-preset-validation-cases.json);
in
assert builtins.all (case: evaluate case.catalog) cases.valid;
assert builtins.all (case: !(evaluate case.catalog)) cases.invalid;
true
