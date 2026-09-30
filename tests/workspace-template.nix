{ lib, pkgs }:
let
  catalog = import ../templates/site/workspace-catalog.nix;
  profileJSON = builtins.readFile ../templates/site/workspace-profile.json;
  resolve = import ../lib/resolve-workspace-profile.nix { inherit lib pkgs; };
  core = [ "gnome-shell" "gnomeExtensions.dash-to-dock" "nautilus" "gnome-text-editor" ];
  presets = (builtins.fromJSON (builtins.readFile ../templates/site/software-presets.json)).presets;
  resolveFor = packages: clients: candidate: resolve {
    inherit catalog;
    profileJSON = candidate;
    controllerName = "pc99";
    clientNames = clients;
    hostPackages = lib.genAttrs ([ "pc99" ] ++ clients) (_: packages ++ core);
  };
  initial = builtins.fromJSON (builtins.readFile ../templates/site/lab-software.json);
  initialPackages = map (entry: entry.package) initial.packages;
  example = resolveFor initialPackages [ "pc01" ] profileJSON;
  rejects = value: !(builtins.tryEval (builtins.deepSeq value true)).success;
  templateArguments =
    ((import ../templates/site/flake.nix).outputs {
      self = {};
      nixpkgs = {};
      nixorium.lib = {
        packageBase = {};
        evalLabSettings = settings: settings.lab;
        mkLab = args: { capturedArguments = args; };
      };
    }).capturedArguments;
  # Workspace validation can inspect system packages, whose modules need flake
  # revision metadata. Output names must exist before that guarded value runs.
  recursiveOutputs = (import ../templates/site/flake.nix).outputs {
    self = recursiveOutputs // { rev = "fixture-revision"; outPath = ../templates/site; };
    nixpkgs = {};
    nixorium.lib = {
      packageBase = {};
      evalLabSettings = settings: settings.lab;
      workspaceProfileSchemaVersion = 1;
      mkLab = args:
        assert !(args ? workspaceCatalog) || args.deploymentSelf.rev == "fixture-revision";
        { guarded = true; validationStillRuns = throw "fixture guard"; };
    };
  };
  homeArgs = {
    inherit pkgs lib;
    labSettings = { studentUser = "learner"; teacherUser = "instructor"; };
    labAssets = {
      mimeApps = ../templates/site/assets/mimeapps.list;
      vscodeSettings = ../templates/site/assets/vscode-settings.json;
    };
    hostSoftwarePackages = [ "chromium" "vscode" "nodejs" ];
  };
  home = enabled: import ../templates/site/modules/home-profile.nix
    (homeArgs // { workspaceRuntimeEnabled = enabled; });
  legacyHome = (home false).system.activationScripts.siteHomeProfile.text;
  managedHome = (home true).system.activationScripts.siteHomeProfile.text;
  staffLines = script: builtins.filter (line:
    lib.hasInfix "/home/admin/" line || lib.hasInfix "/home/instructor/" line
  ) (lib.splitString "\n" script);
in
assert builtins.pathExists ../templates/site/workspace-profile.json;
assert profileJSON == builtins.readFile ../templates/site/workspace-profile.example.json;
assert recursiveOutputs.guarded;
assert rejects recursiveOutputs.validationStillRuns;
assert templateArguments.workspaceCatalog == catalog;
assert templateArguments.workspaceProfileJSON == profileJSON;
assert !(templateArguments ? workspaceRuntimeEnabled);
assert catalog.baseline == { schemaVersion = 1; };
assert builtins.all (preset: builtins.all (clients:
  let result = resolveFor preset.packages clients profileJSON;
  in builtins.deepSeq result (builtins.length result.targets == 1 + builtins.length clients)
) [ [] [ "pc01" "pc02" ] ]) presets;
assert !(example.effective ? vscode) && example.extensions == [];
assert !(builtins.elem "vscode" example.requiredPackages);
assert !(builtins.elem "io.veyon.desktop" example.effective.desktop.favorites);
assert rejects (resolveFor (lib.remove "chromium" initialPackages) [] profileJSON);
assert rejects (resolveFor initialPackages [] ''{"schemaVersion":1,"vscode":{"extensions":["ritwickdey.liveserver"]}}'');
assert builtins.deepSeq (resolveFor (initialPackages ++ [ "vscode" ]) [] ''{"schemaVersion":1,"vscode":{"extensions":["ritwickdey.liveserver"]}}'') true;
assert lib.hasInfix "/var/lib/home-template/learner" legacyHome;
assert !(lib.hasInfix "/var/lib/home-template/learner" managedHome);
assert staffLines legacyHome != [] && staffLines legacyHome == staffLines managedHome;
assert (home false).system.activationScripts.nixoriumUserHomeOwnership.deps
  == (home true).system.activationScripts.nixoriumUserHomeOwnership.deps;
true
