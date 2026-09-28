{ mkWorkspaceLab, workspaceLab, labConfig }:
let
  absent = mkWorkspaceLab { workspaceProfileJSON = null; workspaceCatalog = null; };
  controller = mkWorkspaceLab {
    labConfig = labConfig // { deploymentMode = "controller"; pcCount = 0; };
    publicKeys = { cache = null; ssh = null; veyon = null; };
  };
  rejected = extra: !(builtins.tryEval (builtins.deepSeq (mkWorkspaceLab extra).nixoriumWorkspace true)).success;
  candidate = workspaceLab.nixoriumValidateWorkspaceCandidate;
  scope = kind: {
    schemaVersion = 1;
    packages = [
      { package = "vscode"; scope = { inherit kind; }; }
      { package = "jdk"; scope.kind = "shared"; }
    ];
  };
  legacyTemplate = (import ../templates/site/flake.nix).outputs {
    self = {};
    nixpkgs = {};
    nixorium.lib = {
      packageBase = {};
      evalLabSettings = settings: settings.lab;
      mkLab = args:
        assert !(args ? workspaceProfileJSON) && !(args ? workspaceCatalog);
        { legacyCompatible = true; };
    };
  };
in
assert legacyTemplate.legacyCompatible;
assert absent.nixoriumWorkspace == null;
assert workspaceLab.nixoriumWorkspace.state == "prepared";
assert workspaceLab.nixoriumWorkspace.studentUser == labConfig.studentUser;
assert map (target: target.name) workspaceLab.nixoriumWorkspace.targets == [ "pc99" "pc01" "pc02" ];
assert workspaceLab.nixoriumWorkspace.effective.desktop.enableAnimations == false;
assert workspaceLab.nixoriumWorkspace.effective.desktop.favorites == [ "org.gnome.TextEditor.desktop" "code.desktop" ];
assert controller.nixoriumWorkspace.targets == [ { name = "pc99"; role = "controller"; } ];
assert !controller.nixosConfigurations.pc99.config.services.displayManager.autoLogin.enable;
# Preparation is not activation: every representative system stays identical.
assert builtins.all (name:
  absent.nixosConfigurations.${name}.config.system.build.toplevel.drvPath
  == workspaceLab.nixosConfigurations.${name}.config.system.build.toplevel.drvPath
) [ "pc99" "pc01" ];
assert candidate ''{"schemaVersion":1,"desktop":{"favorites":[]}}'';
assert !(builtins.tryEval (candidate ''{"schemaVersion":1,"vscode":{"extensions":["missing.extension"]}}'')).success;
assert !(builtins.tryEval (candidate ''{"schemaVersion":1,"schemaVersion":1}'')).success;
assert !(builtins.tryEval (candidate null)).success;
assert rejected { workspaceCatalog = null; };
assert rejected { labSoftware = scope "all-clients"; };
assert rejected { labSoftware = scope "controller"; };
assert rejected { hostModules.pc02 = [ ./fixtures/workspace-remove-editor.nix ]; };
assert !(builtins.tryEval (workspaceLab.nixoriumValidateControllerSoftwareCandidate (scope "all-clients"))).success;
true
