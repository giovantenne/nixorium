{ lib }:
let
  decode = import ../lib/eval-workspace-profile.nix { inherit lib; };
  evaluate = json: builtins.tryEval (decode json);
  cases = builtins.fromJSON (builtins.readFile ./workspace-validation-cases.json);
  checkCase = expected: case:
    let result = evaluate case.json; in
    if result.success != expected || (expected && result.value != case.normalized)
    then throw "workspace schema case failed: ${case.name}"
    else true;
  favorites = n: builtins.genList (i: "app${toString i}.desktop") n;
  extensions = n: builtins.genList (i: "publisher.extension${toString i}") n;
  accepts = profile: (evaluate (builtins.toJSON ({ schemaVersion = 1; } // profile))).success;
  repeated = n: lib.concatStrings (builtins.genList (_: "a") n);
in
assert builtins.all (checkCase true) cases.valid;
assert builtins.all (checkCase false) cases.invalid;
assert accepts { desktop.favorites = favorites 32; vscode.extensions = extensions 64; };
assert !(accepts { desktop.favorites = favorites 33; });
assert !(accepts { vscode.extensions = extensions 65; });
assert accepts { browser.defaultApplication = "${repeated 120}.desktop"; };
assert !(accepts { browser.defaultApplication = "${repeated 121}.desktop"; });
assert accepts { vscode.extensions = [ "a.${repeated 126}" ]; };
assert !(accepts { vscode.extensions = [ "a.${repeated 127}" ]; });
assert (evaluate (''{"schemaVersion":1}'' + lib.concatStrings (builtins.genList (_: " ") (65536 - 19)))).success;
assert !(evaluate (''{"schemaVersion":1}'' + lib.concatStrings (builtins.genList (_: " ") (65537 - 19)))).success;
true
