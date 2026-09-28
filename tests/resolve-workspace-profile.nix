{ lib, realPkgs }:
let
  package = name: {
    type = "derivation";
    inherit name;
    version = "1.2.3";
    meta.platforms = [ "x86_64-linux" ];
  };
  java = package "java-extension" // { vscodeExtUniqueId = "redhat.java"; };
  pkgs = {
    stdenv.hostPlatform = lib.systems.elaborate "x86_64-linux";
    vscode = package "vscode";
    chromium = package "chromium";
    nautilus = package "nautilus";
    jdk = package "jdk";
    gnome-shell = package "gnome-shell";
    gnomeExtensions.dash-to-dock = package "dock";
    vscode-extensions = {
      redhat.java = java;
      vscjava.vscode-java-pack = package "java-pack" // { vscodeExtUniqueId = "vscjava.vscode-java-pack"; };
    };
  };
  resolve = import ../lib/resolve-workspace-profile.nix { inherit lib pkgs; };
  catalog = {
    schemaVersion = 1;
    baseline = {
      schemaVersion = 1;
      desktop = { favorites = [ "org.gnome.Nautilus.desktop" ]; colorScheme = "light"; dock.iconSize = 40; };
    };
    applications = [
      { id = "org.gnome.Nautilus.desktop"; package = "nautilus"; }
      { id = "chromium-browser.desktop"; package = "chromium"; browser = true; }
      { id = "code.desktop"; package = "vscode"; }
    ];
    extensions = [
      { id = "redhat.java"; package = "vscode-extensions.redhat.java"; requiredPackages = [ "jdk" ]; }
      { id = "vscjava.vscode-java-pack"; package = "vscode-extensions.vscjava.vscode-java-pack"; requiredExtensions = [ "redhat.java" ]; }
    ];
  };
  software = [ "vscode" "chromium" "nautilus" "jdk" "gnome-shell" "gnomeExtensions.dash-to-dock" ];
  args = {
    inherit catalog;
    profileJSON = builtins.toJSON {
      schemaVersion = 1;
      desktop = { colorScheme = "dark"; dock.autoHide = false; };
      vscode.extensions = [ "vscjava.vscode-java-pack" "redhat.java" ];
      browser.defaultApplication = "chromium-browser.desktop";
    };
    controllerName = "pc99";
    clientNames = [ "pc01" "pc02" ];
    hostPackages = { pc99 = software; pc01 = software; pc02 = software; };
  };
  result = resolve args;
  profile = value: args // { profileJSON = builtins.toJSON ({ schemaVersion = 1; } // value); };
  accepted = input: (builtins.tryEval (resolve input)).success;
  rejected = name: input:
    if accepted input then throw "workspace resolver accepted invalid case: ${name}" else true;
  tests = [
    (rejected "unknown desktop entry" (profile { desktop.favorites = [ "unknown.desktop" ]; }))
    (rejected "non-browser default" (profile { browser.defaultApplication = "code.desktop"; }))
    (rejected "unknown extension" (profile { vscode.extensions = [ "unknown.extension" ]; }))
    (rejected "extension dependency missing" (profile { vscode.extensions = [ "vscjava.vscode-java-pack" ]; }))
    (rejected "controller missing editor" (args // { hostPackages = args.hostPackages // { pc99 = lib.remove "vscode" software; }; }))
    (rejected "client missing editor" (args // { hostPackages = args.hostPackages // { pc02 = lib.remove "vscode" software; }; }))
    (rejected "runtime missing" (args // { hostPackages = args.hostPackages // { pc01 = lib.remove "jdk" software; }; }))
    (rejected "dock missing" (args // { hostPackages = args.hostPackages // { pc99 = lib.remove "gnomeExtensions.dash-to-dock" software; }; }))
    (rejected "desktop missing" (args // { hostPackages = args.hostPackages // { pc01 = lib.remove "gnome-shell" software; }; }))
    (rejected "controller omitted" (args // { hostPackages = builtins.removeAttrs args.hostPackages [ "pc99" ]; }))
    (rejected "client omitted" (args // { hostPackages = builtins.removeAttrs args.hostPackages [ "pc02" ]; }))
    (rejected "extra target" (args // { hostPackages = args.hostPackages // { pc03 = software; }; }))
    (rejected "duplicate client" (args // { clientNames = [ "pc01" "pc01" "pc02" ]; }))
    (rejected "controller listed as client" (args // { clientNames = [ "pc99" "pc01" "pc02" ]; }))
    (rejected "invalid host name" (args // { controllerName = "../pc99"; }))
    (rejected "invalid host declaration" (args // { hostPackages = args.hostPackages // { pc99 = null; }; }))
    (rejected "unknown catalog field" (args // { catalog = catalog // { arbitraryFiles = []; }; }))
    (rejected "future catalog" (args // { catalog = catalog // { schemaVersion = 2; }; }))
    (rejected "float catalog version" (args // { catalog = catalog // { schemaVersion = 1.0; }; }))
    (rejected "missing catalog field" (args // { catalog = builtins.removeAttrs catalog [ "baseline" ]; }))
    (rejected "bad baseline" (args // { catalog = catalog // { baseline = { schemaVersion = 1; gimp = {}; }; }; }))
    (rejected "duplicate app" (args // { catalog = catalog // { applications = catalog.applications ++ [ (builtins.head catalog.applications) ]; }; }))
    (rejected "duplicate extension" (args // { catalog = catalog // { extensions = catalog.extensions ++ [ (builtins.head catalog.extensions) ]; }; }))
    (rejected "browser type" (args // { catalog = catalog // { applications = [ { id = "code.desktop"; package = "vscode"; browser = "true"; } ]; }; }))
    (rejected "arbitrary extension package" (args // { catalog = catalog // { extensions = [ { id = "redhat.java"; package = "chromium"; } ]; }; }))
    (rejected "unknown extension dependency" (args // { catalog = catalog // { extensions = [ { id = "redhat.java"; package = "vscode-extensions.redhat.java"; requiredExtensions = [ "unknown.extension" ]; } ]; }; }))
    (rejected "self dependency" (args // { catalog = catalog // { extensions = [ { id = "redhat.java"; package = "vscode-extensions.redhat.java"; requiredExtensions = [ "redhat.java" ]; } ]; }; }))
    (rejected "no implicit vscode" ((profile { vscode = {}; }) // { hostPackages = { pc99 = []; pc01 = []; pc02 = []; }; }))
  ];
  withExtension = extensionPackage:
    import ../lib/resolve-workspace-profile.nix { inherit lib; pkgs = pkgs // {
      vscode-extensions = pkgs.vscode-extensions // { redhat.java = extensionPackage; };
    }; } args;
  withEditor = editor:
    import ../lib/resolve-workspace-profile.nix { inherit lib; pkgs = pkgs // { vscode = editor; }; } args;
  rejectsValue = value: !(builtins.tryEval (builtins.deepSeq value true)).success;
  empty = resolve ((profile { desktop.favorites = []; vscode.extensions = []; }) // {
    catalog = catalog // { baseline = catalog.baseline // { vscode.extensions = [ "redhat.java" ]; }; };
  });
  controllerOnly = resolve (args // { clientNames = []; hostPackages = { pc99 = software; }; });
  minimal = resolve {
    catalog = catalog // { baseline = { schemaVersion = 1; }; };
    profileJSON = ''{"schemaVersion":1}'';
    controllerName = "pc99";
    clientNames = [];
    hostPackages.pc99 = [];
  };
  # Real pinned attributes, without building an editor or reading its manifest
  # during evaluation. Synthetic packages above own failure-mode coverage.
  pinned = import ../lib/resolve-workspace-profile.nix { inherit lib; pkgs = realPkgs; } {
    catalog = catalog // { baseline = { schemaVersion = 1; }; };
    profileJSON = ''{"schemaVersion":1,"vscode":{"extensions":["redhat.java"]}}'';
    controllerName = "pc99";
    clientNames = [];
    hostPackages.pc99 = [ "vscode" "jdk" ];
  };
in
assert import ./workspace-template.nix { inherit lib; pkgs = realPkgs; };
assert builtins.all (value: value) tests;
assert result.effective.desktop.favorites == [ "org.gnome.Nautilus.desktop" ];
assert result.effective.desktop.colorScheme == "dark";
assert result.effective.desktop.dock == { iconSize = 40; autoHide = false; };
assert result.declared.desktop.colorScheme == "dark";
assert !(result.declared.desktop ? favorites);
assert result.catalog.baseline.desktop.colorScheme == "light";
assert map (target: target.name) result.targets == [ "pc99" "pc01" "pc02" ];
assert (builtins.head result.targets).role == "controller";
assert (builtins.elemAt result.targets 1).role == "client";
assert map (entry: entry.id) result.extensions == [ "redhat.java" "vscjava.vscode-java-pack" ];
assert builtins.all (entry: entry.version == "1.2.3") result.extensions;
assert empty.effective.desktop.favorites == [] && empty.effective.vscode.extensions == [];
assert !(builtins.elem "jdk" empty.requiredPackages);
assert controllerOnly.targets == [ { name = "pc99"; role = "controller"; } ];
assert minimal.requiredPackages == [] && minimal.extensions == [];
assert rejectsValue (withExtension (java // { vscodeExtUniqueId = "another.extension"; }));
assert rejectsValue (withExtension (java // { version = ""; }));
assert rejectsValue (withExtension (java // { meta = java.meta // { broken = true; }; }));
assert rejectsValue (withExtension (java // { meta = java.meta // { knownVulnerabilities = [ "test advisory" ]; }; }));
assert rejectsValue (withExtension (java // { meta.platforms = [ "aarch64-linux" ]; }));
assert rejectsValue (withExtension null);
assert rejectsValue (withEditor null);
assert rejectsValue (withEditor (pkgs.vscode // { meta.broken = true; }));
assert builtins.deepSeq pinned true;
assert (builtins.head pinned.extensions).version == realPkgs.vscode-extensions.redhat.java.version;
true
