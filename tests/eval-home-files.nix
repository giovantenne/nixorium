{ lib }:
let
  decode = import ../lib/eval-home-files.nix { inherit lib; };
  cases = builtins.fromJSON (builtins.readFile ./home-files-validation-cases.json);
  check = expected: case:
    let result = builtins.tryEval (decode case.json); in
    if result.success != expected || (expected && result.value != case.normalized)
    then throw "home files schema case failed: ${case.name}" else true;
in
assert builtins.all (check true) cases.valid;
assert builtins.all (check false) cases.invalid;
true
