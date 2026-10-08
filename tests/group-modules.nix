{ mkLab, deploymentSelf, labConfig, lib }:
let
  args = {
    inherit deploymentSelf;
    labConfig = labConfig // { pcCount = 4; };
    clientGroups.course = [ "pc01" "pc02" ];
    groupModules.course = [ ./fixtures/group-course.nix ];
  };
  lab = mkLab args;
  overlap = mkLab (args // {
    clientGroups = args.clientGroups // { extra = [ "pc02" ]; };
    groupModules = args.groupModules // { extra = [ ./fixtures/group-extra.nix ]; };
  });
  exceptional = mkLab (args // {
    hostModules.pc01 = [];
    updateValidationHosts = [ "pc03" ];
  });
  members = value: map (class: class.members) value.nixoriumHostClasses;
  has = value: host: package: builtins.any (p: (p.pname or "") == package)
    value.nixosConfigurations.${host}.config.environment.systemPackages;
  rejected = extra: !(builtins.tryEval (builtins.deepSeq
    (mkLab (args // extra)).nixoriumHostClasses true)).success;
in
assert members lab == [ [ "pc01" "pc02" ] [ "pc03" "pc04" ] [ "pc99" ] ];
assert members overlap == [ [ "pc01" ] [ "pc02" ] [ "pc03" "pc04" ] [ "pc99" ] ];
assert members exceptional == [ [ "pc01" ] [ "pc02" ] [ "pc03" ] [ "pc04" ] [ "pc99" ] ];
assert lib.sort builtins.lessThan lab.nixoriumUpdateTargets == [ "pc01" "pc03" "pc99" ];
assert lib.sort builtins.lessThan overlap.nixoriumUpdateTargets == [ "pc01" "pc02" "pc03" "pc99" ];
assert lib.sort builtins.lessThan exceptional.nixoriumUpdateTargets == [ "pc01" "pc02" "pc03" "pc04" "pc99" ];
assert has lab "pc01" "hello" && has lab "pc02" "hello";
assert !(has lab "pc03" "hello") && !(has lab "pc99" "hello");
assert has overlap "pc02" "hello" && has overlap "pc02" "cowsay";
assert !(has overlap "pc01" "cowsay");
assert members (mkLab (args // { groupModules.course = []; }))
  == [ [ "pc01" "pc02" "pc03" "pc04" ] [ "pc99" ] ];
assert members (mkLab (args // {
  labConfig = args.labConfig // { hostIfaceNames.pc02 = "eth1"; };
})) == members overlap;
assert members (mkLab (args // {
  labSoftware = { schemaVersion = 1; packages = [
    { package = "cowsay"; scope = { kind = "clients"; clients = [ "pc02" ]; }; }
  ]; };
})) == members overlap;
assert rejected { groupModules.unknown = []; };
assert rejected { groupModules = []; };
assert rejected { groupModules.course = ./fixtures/group-course.nix; };
assert rejected { groupModules.course = [ {} ]; };
assert rejected { clientGroups.course = [ "pc99" ]; };
assert rejected { clientGroups.course = [ "pc00" ]; };
assert rejected { clientGroups.course = [ "pc01" "pc01" ]; };
assert rejected { clientGroups.course = []; };
true
