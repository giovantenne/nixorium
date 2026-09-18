{ lib }:
let
  evalLabConfig = import ../lib/eval-lab-config.nix { inherit lib; };
  valid = import ../lab-config.nix;
  evaluates = config:
    (builtins.tryEval (builtins.deepSeq (evalLabConfig config) true)).success;
  sharedCases = builtins.fromJSON (builtins.readFile ./lab-settings-validation-cases.json);
in
assert evaluates valid;
assert (evalLabConfig valid).deploymentMode == "laboratory";
assert evaluates (valid // { deploymentMode = "controller"; pcCount = 0; });
assert !(evaluates (valid // { deploymentMode = "controller"; }));
assert !(evaluates (valid // { pcCount = 0; }));
assert !(evaluates (valid // { deploymentMode = "unknown"; }));
assert !(evaluates (valid // { networkBase = "10.0.0"; }));
assert !(evaluates (valid // { networkBase = "10.0.0.1"; }));
assert !(evaluates (valid // { networkPrefixLength = 30; }));
assert !(evaluates (valid // { ifaceName = "interface-name-is-too-long"; }));
assert evaluates (valid // {
  controllerIfaceName = "eno1";
  clientIfaceName = "enp2s0";
  hostIfaceNames.pc01 = "enp3s0";
});
assert !(evaluates (valid // { controllerIfaceName = "interface-name-is-too-long"; }));
assert !(evaluates (valid // { clientIfaceName = "interface-name-is-too-long"; }));
assert !(evaluates (valid // { hostIfaceNames.pc00 = "enp3s0"; }));
assert !(evaluates (valid // { hostIfaceNames.pc01 = "interface-name-is-too-long"; }));
assert !(evaluates (valid // { studentUser = "Bad User"; }));
assert !(evaluates (valid // { studentUser = valid.teacherUser; }));
assert !(evaluates (valid // { adminPassword = "plaintext"; }));
assert !(evaluates (valid // { homepageUrl = "example.org"; }));
assert builtins.all (case: !(evaluates (valid // case.overrides))) sharedCases.invalid;
assert !(evaluates (valid // { unknownSetting = true; }));
true
