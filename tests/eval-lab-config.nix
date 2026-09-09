{ lib }:
let
  evalLabConfig = import ../lib/eval-lab-config.nix { inherit lib; };
  valid = import ../lab-config.nix;
  evaluates = config:
    (builtins.tryEval (builtins.deepSeq (evalLabConfig config) true)).success;
in
assert evaluates valid;
assert !(evaluates (valid // { networkBase = "10.0.0"; }));
assert !(evaluates (valid // { networkBase = "10.0.0.1"; }));
assert !(evaluates (valid // { networkPrefixLength = 30; }));
assert !(evaluates (valid // { ifaceName = "interface-name-is-too-long"; }));
assert !(evaluates (valid // { studentUser = "Bad User"; }));
assert !(evaluates (valid // { studentUser = valid.teacherUser; }));
assert !(evaluates (valid // { adminPassword = "plaintext"; }));
assert !(evaluates (valid // { homepageUrl = "example.org"; }));
assert !(evaluates (valid // { unknownSetting = true; }));
true
