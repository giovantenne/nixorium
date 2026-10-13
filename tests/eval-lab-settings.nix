{ lib }:
let
  evalLabSettings = import ../lib/eval-lab-settings.nix { inherit lib; };
  valid = {
    schemaVersion = 1;
    lab = import ../lab-config.nix;
  };
  evaluates = settings:
    (builtins.tryEval (builtins.deepSeq (evalLabSettings settings) true)).success;
  template = builtins.fromJSON (builtins.readFile ../templates/site/lab-settings.json);
in
assert evaluates valid;
assert (evalLabSettings valid).assistant;
assert (evalLabSettings template).assistant;
assert (evalLabSettings (valid // { lab = valid.lab // { assistant = true; }; })).assistant;
assert !(evalLabSettings (valid // { lab = valid.lab // { assistant = false; }; })).assistant;
assert evaluates (valid // { lab = valid.lab // { assistant = true; }; });
assert evaluates (valid // { lab = valid.lab // { assistant = false; }; });
assert !(evaluates (valid // { lab = valid.lab // { assistant = "true"; }; }));
assert !(evaluates (valid // { schemaVersion = 2; }));
assert !(evaluates (valid // { unknown = true; }));
assert !(evaluates (builtins.removeAttrs valid [ "lab" ]));
assert !(evaluates (valid // { lab = valid.lab // { unknownSetting = true; }; }));
true
