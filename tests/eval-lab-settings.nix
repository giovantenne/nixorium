{ lib }:
let
  evalLabSettings = import ../lib/eval-lab-settings.nix { inherit lib; };
  valid = {
    schemaVersion = 1;
    lab = import ../lab-config.nix;
  };
  evaluates = settings:
    (builtins.tryEval (builtins.deepSeq (evalLabSettings settings) true)).success;
in
assert evaluates valid;
assert !(evaluates (valid // { schemaVersion = 2; }));
assert !(evaluates (valid // { unknown = true; }));
assert !(evaluates (builtins.removeAttrs valid [ "lab" ]));
assert !(evaluates (valid // { lab = valid.lab // { unknownSetting = true; }; }));
true
