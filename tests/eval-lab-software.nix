{ lib, pkgs }:
let
  evaluate = import ../lib/eval-lab-software.nix {
    inherit lib pkgs;
    clientNames = [ "pc01" "pc02" ];
    clientGroups.graphics = [ "pc01" ];
  };
  valid = evaluate {
    schemaVersion = 1;
    packages = [
      { package = "vlc"; scope.kind = "all-clients"; }
      { package = "gimp"; scope = { kind = "group"; group = "graphics"; }; }
      { package = "inkscape"; scope = { kind = "clients"; clients = [ "pc02" ]; }; }
    ];
  };
  rejects = value: !(builtins.tryEval (builtins.deepSeq (evaluate value) true)).success;
  rejectsGroups = clientGroups: !(builtins.tryEval (builtins.deepSeq ((import ../lib/eval-lab-software.nix {
    inherit lib pkgs clientGroups;
    clientNames = [ "pc01" ];
  }) { schemaVersion = 1; packages = []; }) true)).success;
in
assert builtins.length valid.packages == 3;
assert rejects { schemaVersion = 2; packages = []; };
assert rejects { schemaVersion = 1; packages = []; unexpected = true; };
assert rejects { schemaVersion = 1; packages = [{ package = "not-a-real-package"; scope.kind = "all-clients"; }]; };
assert !rejects { schemaVersion = 1; packages = [{ package = "hello"; scope.kind = "all-clients"; }]; };
assert !rejects { schemaVersion = 1; packages = [{ package = "python3Packages.pip"; scope.kind = "all-clients"; }]; };
assert rejects { schemaVersion = 1; packages = [{ package = "vlc"; scope = { kind = "all-clients"; group = "graphics"; }; }]; };
assert rejects { schemaVersion = 1; packages = [{ package = "vlc"; scope = { kind = "clients"; clients = [ "pc99" ]; }; }]; };
assert rejects { schemaVersion = 1; packages = [{ package = "vlc"; scope = { kind = "group"; group = "missing"; }; }]; };
assert rejects { schemaVersion = 1; packages = [
  { package = "vlc"; scope.kind = "all-clients"; }
  { package = "vlc"; scope.kind = "all-clients"; }
]; };
assert rejectsGroups { graphics = [ "pc01" "pc01" ]; };
assert rejectsGroups { "../graphics" = [ "pc01" ]; };
true
