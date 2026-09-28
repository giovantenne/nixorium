let
  groups = import ./validation-groups.nix;
  names = builtins.concatLists (builtins.attrValues groups);
  checks = builtins.listToAttrs (map (name: { inherit name; value.drvPath = "fixture-${name}.drv"; }) names);
  evaluate = candidate: import ./ci-eval.nix { flake.checks.x86_64-linux = candidate; };
  rejected = candidate: !(builtins.tryEval (evaluate candidate).checkGroups).success;
  lazyChecks = builtins.mapAttrs (_: _: throw "Listing check groups must not evaluate systems") checks;
  template = import ./ci-eval.nix { flake = {
    apps.x86_64-linux.nixorium.program = "fixture-command";
    packages.x86_64-linux = {
      pxeFirmware.drvPath = "fixture-firmware";
      installerBundle.drvPath = "fixture-installer";
    };
    nixosConfigurations.pc01.config.system.build.toplevel.drvPath = "fixture-client";
  }; };
in
assert template.template == {
  command = "fixture-command";
  firmware = "fixture-firmware";
  installer = "fixture-installer";
  client = "fixture-client";
};
assert (evaluate lazyChecks).checkGroups == groups;
assert builtins.all (group:
  let resolved = (evaluate checks).${group};
  in builtins.attrNames resolved == builtins.sort builtins.lessThan groups.${group}
    && builtins.all (name: resolved.${name} == checks.${name}.drvPath) groups.${group}
) (builtins.attrNames groups);
assert rejected (checks // { unexpected = {}; });
assert rejected (builtins.removeAttrs checks [ "mk-lab" ]);
assert builtins.length names == builtins.length (builtins.attrNames checks);
assert builtins.all (group: groups.${group} != []) (builtins.attrNames groups);
true
