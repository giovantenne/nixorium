{ lib }:
let
  evalLabConfig = import ../lib/eval-lab-config.nix { inherit lib; };
  valid = import ../lab-config.nix;
  evaluates = config:
    (builtins.tryEval (builtins.deepSeq (evalLabConfig config) true)).success;
  sharedCases = builtins.fromJSON (builtins.readFile ./lab-settings-validation-cases.json);
in
assert evaluates valid;
assert !(evaluates (valid // { veyonNativeHosts = []; }));
assert evaluates (valid // { masterDhcpIp = "10.0.1.0"; networkBase = "10.0.0.0"; networkPrefixLength = 24; });
assert evaluates (valid // { masterDhcpIp = "9.255.255.255"; networkBase = "10.0.0.0"; networkPrefixLength = 24; });
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
# Git identities and the keyboard layout reach shell scripts: strict shapes,
# while ordinary names (accents, quotes, spaces) and XKB layouts stay valid.
assert evaluates (valid // { adminGitName = "José O'Brien \"Jo\" Núñez"; studentGitEmail = "jose.o+lab@example.org"; });
assert evaluates (valid // { keyboardLayout = "us"; });
assert evaluates (valid // { keyboardLayout = "us(intl)"; });
assert evaluates (valid // { keyboardLayout = "us,it"; });
assert evaluates (valid // { keyboardLayout = "de_CH-legacy+1"; });
assert !(evaluates (valid // { adminGitName = "x\"; touch /tmp/x #\nname"; }));
assert !(evaluates (valid // { keyboardLayout = "us')]; touch /tmp/x #"; }));
# A change of laboratory network: valid previous network, offset or zoned time.
assert (evalLabConfig valid).networkTransition == null;
assert evaluates (valid // { networkTransition = { previousBase = "10.1.0.0"; previousPrefixLength = 24; startedAt = "2026-10-15T08:00:00Z"; }; });
assert evaluates (valid // { networkTransition = { previousBase = "192.168.0.0"; previousPrefixLength = 16; startedAt = "2026-10-15T10:00:00.5+02:00"; }; });
assert (evalLabConfig (valid // { networkTransition = { previousBase = "10.1.0.0"; previousPrefixLength = 24; startedAt = "2026-10-15T08:00:00Z"; }; })).networkTransition.previousBase == "10.1.0.0";
# Without clients the record is ignored (mkLab warns), not validated further.
assert evaluates (valid // { deploymentMode = "controller"; pcCount = 0; networkTransition = { previousBase = "10.0.0.0"; previousPrefixLength = 24; startedAt = "later"; }; });
assert !(evaluates (valid // { deploymentMode = "controller"; pcCount = 0; networkTransition = { previousBase = "10.1.0.0"; }; }));
# consoleKeyMap is deprecated and ignored: present (even empty) or absent is fine.
assert !(valid ? consoleKeyMap);
assert evaluates (valid // { consoleKeyMap = "it2"; });
assert evaluates (valid // { consoleKeyMap = ""; });
assert (evalLabConfig valid).consoleKeyMap == null;
assert !(evaluates (valid // { unknownSetting = true; }));
true
