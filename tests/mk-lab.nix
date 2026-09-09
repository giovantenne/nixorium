{ mkLab, deploymentSelf, labConfig }:
let
  baseArgs = {
    inherit deploymentSelf;
    inherit labConfig;
  };
  subnetLab = mkLab (baseArgs // {
    labConfig = labConfig // {
      networkBase = "10.23.4.128";
      networkPrefixLength = 25;
    };
  });
  rejectsUnknownHost = !(builtins.tryEval (builtins.deepSeq
    (mkLab (baseArgs // {
      hostModules.pc00 = [ ../modules/common.nix ];
    })).labMeta
    true)).success;
  rejectsUnknownVeyonHost = !(builtins.tryEval (builtins.deepSeq
    (mkLab (baseArgs // {
      labConfig = labConfig // {
        veyonNativeHosts = [ "pc00" ];
      };
    })).labMeta
    true)).success;
in
assert subnetLab.labMeta.controller.staticIp == "10.23.4.227";
assert subnetLab.labMeta.network.prefixLength == 25;
assert subnetLab.colmena.pc01.deployment.targetHost == "10.23.4.129";
assert rejectsUnknownHost;
assert rejectsUnknownVeyonHost;
true
