{ mkWorkspaceLab, workspaceLab, workspaceRuntimeLab, workspaceRuntimeControllerLab, labConfig }:
let
  lib = workspaceLab.nixosConfigurations.pc99.pkgs.lib;
  absent = mkWorkspaceLab { workspaceProfileJSON = null; workspaceCatalog = null; };
  controller = mkWorkspaceLab {
    labConfig = labConfig // { deploymentMode = "controller"; pcCount = 0; };
    publicKeys = { cache = null; ssh = null; };
  };
  rejected = extra: !(builtins.tryEval (builtins.deepSeq (mkWorkspaceLab extra).nixoriumWorkspace true)).success;
  discoveryLab = mkWorkspaceLab {
    sharedModules = [ ./fixtures/workspace-forbid-host-evaluation.nix ];
  };
  candidate = workspaceLab.nixoriumValidateWorkspaceCandidate;
  resolveCandidate = workspaceLab.nixoriumResolveWorkspaceCandidate;
  cleared = resolveCandidate ''{"schemaVersion":1,"desktop":{"favorites":[]}}'';
  managedCandidate = workspaceRuntimeLab.nixoriumResolveWorkspaceCandidate
    ''{"schemaVersion":1,"vscode":{"settings":{"editor.fontSize":19}}}'';
  scope = kind: {
    schemaVersion = 1;
    packages = [
      { package = "vscode"; scope = { inherit kind; }; }
      { package = "jdk"; scope.kind = "shared"; }
    ];
  };
  # The editor reaches clients through one declaration with the given scope.
  clientEditor = editorScope: {
    controllerModules = [ ./fixtures/workspace-controller-editor.nix ];
    clientGroups = { editors = [ "pc01" ]; };
    labSoftware = {
      schemaVersion = 1;
      packages = [
        { package = "vscode"; scope = editorScope; }
        { package = "jdk"; scope.kind = "shared"; }
      ];
    };
  };
  templateArgs = {
    workspaceProfileJSON = null;
    workspaceCatalog = import ../templates/site/workspace-catalog.nix;
    labSoftware = builtins.fromJSON (builtins.readFile ../templates/site/lab-software.json);
    sharedModules = [ ../templates/site/modules/shared.nix ];
  };
  templateLab = mkWorkspaceLab templateArgs;
  templateProfile = builtins.readFile ../templates/site/workspace-profile.json;
  templateManagedLab = mkWorkspaceLab (templateArgs // {
    workspaceProfileJSON = templateProfile;
  });
  templateController = mkWorkspaceLab (templateArgs // {
    labConfig = labConfig // { deploymentMode = "controller"; pcCount = 0; };
  });
in
{
  template =
    assert templateLab.nixoriumWorkspace == null;
    assert templateLab.nixoriumValidateWorkspaceCandidate templateProfile;
    assert templateController.nixoriumValidateWorkspaceCandidate templateProfile;
    assert (templateLab.nixoriumResolveWorkspaceCandidate templateProfile).declared
      == builtins.fromJSON templateProfile;
    assert (templateLab.nixoriumResolveWorkspaceCandidate templateProfile).runtimeEnabled;
    assert templateManagedLab.nixoriumWorkspace.runtimeEnabled;
    assert !(templateManagedLab.nixoriumWorkspace.effective ? vscode);
    assert builtins.all (name:
      let cfg = templateManagedLab.nixosConfigurations.${name}.config;
      in !(lib.hasInfix "/var/lib/home-template/${labConfig.studentUser}" cfg.system.activationScripts.siteHomeProfile.text)
        && lib.hasInfix "APPEARANCE_ROLE=managed-student" cfg.environment.etc."lab/gnome-user-setup.sh".text
    ) [ "pc99" "pc01" ];
    true;

  runtime =
    assert workspaceRuntimeLab.nixoriumWorkspace.state == "prepared";
    assert workspaceRuntimeLab.nixoriumWorkspace.runtimeEnabled;
    assert builtins.isString workspaceRuntimeLab.nixoriumWorkspace.seed;
    assert builtins.all (name:
      let cfg = workspaceRuntimeLab.nixosConfigurations.${name}.config;
      in !cfg.systemd.services.home-reset.restartIfChanged
        && cfg.systemd.services.home-reset.unitConfig.RefuseManualStart
        && builtins.elem "systemd-user-sessions.service" cfg.systemd.services.home-reset.requiredBy
        && cfg.environment.etc ? "nixorium-workspace-reset.json"
        && !(lib.hasInfix "/var/lib/home-template/${labConfig.studentUser}" cfg.system.activationScripts.createHomeTemplates.text)
        && !(lib.hasInfix "/home/${labConfig.studentUser}" cfg.system.activationScripts.nixoriumUserHomeOwnership.text)
    ) [ "pc99" "pc01" "pc02" ];
    assert !workspaceRuntimeLab.nixosConfigurations.pc99.config.services.displayManager.autoLogin.enable;
    assert workspaceRuntimeLab.nixosConfigurations.pc01.config.services.displayManager.autoLogin.enable;
    assert !workspaceRuntimeControllerLab.nixosConfigurations.pc99.config.services.displayManager.autoLogin.enable;
    assert workspaceRuntimeControllerLab.nixoriumWorkspace.runtimeEnabled;
    assert workspaceRuntimeControllerLab.nixoriumWorkspace.targets == [ { name = "pc99"; role = "controller"; } ];
    true;

  preparation =
    assert absent.nixoriumWorkspace == null;
    assert workspaceLab.nixoriumWorkspace.state == "prepared";
    assert workspaceLab.nixoriumWorkspace.runtimeEnabled;
    assert builtins.isString workspaceLab.nixoriumWorkspace.seed;
    assert workspaceLab.nixoriumWorkspace.studentUser == labConfig.studentUser;
    assert map (target: target.name) workspaceLab.nixoriumWorkspace.targets == [ "pc99" "pc01" "pc02" ];
    assert workspaceLab.nixoriumWorkspace.effective.desktop.enableAnimations == false;
    assert workspaceLab.nixoriumWorkspace.effective.desktop.favorites == [ "org.gnome.TextEditor.desktop" "code.desktop" ];
    assert controller.nixoriumWorkspace.targets == [ { name = "pc99"; role = "controller"; } ];
    assert !controller.nixosConfigurations.pc99.config.services.displayManager.autoLogin.enable;
    # A profile changes the configured system without a second opt-in.
    assert builtins.all (name:
      absent.nixosConfigurations.${name}.config.system.build.toplevel.drvPath
      != workspaceLab.nixosConfigurations.${name}.config.system.build.toplevel.drvPath
    ) [ "pc99" "pc01" ];
    true;

  candidate =
    assert resolveCandidate (builtins.toJSON workspaceLab.nixoriumWorkspace.declared)
      == workspaceLab.nixoriumWorkspace;
    assert workspaceRuntimeLab.nixoriumResolveWorkspaceCandidate
      (builtins.toJSON workspaceRuntimeLab.nixoriumWorkspace.declared)
      == workspaceRuntimeLab.nixoriumWorkspace;
    assert candidate ''{"schemaVersion":1,"desktop":{"favorites":[]}}'';
    assert cleared.declared.desktop.favorites == [];
    assert cleared.effective.desktop.favorites == [];
    assert cleared.effective.desktop.enableAnimations == false;
    assert cleared.targets == workspaceLab.nixoriumWorkspace.targets;
    assert cleared.catalog == workspaceLab.nixoriumWorkspace.catalog;
    assert cleared.studentUser == labConfig.studentUser;
    assert cleared.state == "prepared" && cleared.runtimeEnabled && builtins.isString cleared.seed;
    assert cleared.extensions == [] && !(builtins.elem "vscode" cleared.requiredPackages);
    assert workspaceLab.nixoriumWorkspace.effective.desktop.favorites == [ "org.gnome.TextEditor.desktop" "code.desktop" ];
    assert managedCandidate.state == "prepared" && managedCandidate.runtimeEnabled;
    assert managedCandidate.effective.vscode.settings."editor.fontSize" == 19;
    assert managedCandidate.seed != workspaceRuntimeLab.nixoriumWorkspace.seed;
    assert builtins.all (raw: !(builtins.tryEval (resolveCandidate raw)).success) [
      null ''{"schemaVersion":1,"schemaVersion":1}''
      ''{"schemaVersion":1,"vscode":{"extensions":["missing.extension"]}}''
    ];
    assert !(builtins.tryEval (absent.nixoriumResolveWorkspaceCandidate ''{"schemaVersion":1}'')).success;
    true;

  rejection =
    # Reading identities and software must not evaluate any host. Every output
    # that authorizes, validates or builds still fails on the same invalid lab.
    assert discoveryLab.labMeta.clients.count == 2;
    assert builtins.deepSeq discoveryLab.nixoriumSoftware true;
    assert discoveryLab.nixoriumSoftwarePresets == null;
    assert (discoveryLab.nixoriumResolveSoftwarePackage "hello").availability == "available";
    assert builtins.deepSeq (discoveryLab.nixoriumSearchSoftwarePackages { query = "hello"; limit = 1; }) true;
    assert builtins.deepSeq discoveryLab.nixoriumUpdateTargets true;
    assert builtins.all (name: !(builtins.tryEval discoveryLab.${name}).success) [
      "deploymentStatus" "nixosConfigurations" "colmena"
      "nixoriumWorkspace" "nixoriumOfflineCheck"
      "nixoriumValidateWorkspaceCandidate" "nixoriumResolveWorkspaceCandidate"
      "nixoriumValidateControllerSoftwareCandidate"
    ];
    assert !(builtins.tryEval discoveryLab.packages.x86_64-linux.nixorium).success;
    assert !(builtins.tryEval discoveryLab.apps.x86_64-linux.nixorium).success;
    assert !(builtins.tryEval (candidate ''{"schemaVersion":1,"vscode":{"extensions":["missing.extension"]}}'')).success;
    assert !(builtins.tryEval (candidate ''{"schemaVersion":1,"schemaVersion":1}'')).success;
    assert !(builtins.tryEval (candidate null)).success;
    assert rejected { workspaceCatalog = null; };
    assert !(builtins.functionArgs (import ../lib/mk-lab.nix {
      upstreamSelf = {}; nixpkgs = {}; disko = {};
    }) ? workspaceRuntimeEnabled);
    assert builtins.all (paths: rejected { homeResetEphemeralPaths = paths; }) [
      [ "." ] [ "a//b" ] [ "a/./b" ] [ "a/" ] [ "a\\b" ] [ "a\nb" ]
      [ ".cache" ".cache" ] [ ".cache" ".cache/tool" ] [ ".cache/tool" ".cache" ]
    ];
    assert rejected { labSoftware = scope "all-clients"; };
    assert rejected { labSoftware = scope "controller"; };
    assert rejected { hostModules.pc02 = [ ./fixtures/workspace-remove-editor.nix ]; };
    # Clients with different managed software are separate classes, so a
    # client outside the editor's scope is still checked.
    assert rejected (clientEditor { kind = "clients"; clients = [ "pc01" ]; });
    assert rejected (clientEditor { kind = "group"; group = "editors"; });
    assert !rejected (clientEditor { kind = "clients"; clients = [ "pc01" "pc02" ]; });
    # Identical clients are resolved once, whatever the size of the lab.
    assert !rejected { sharedModules = [ ./fixtures/workspace-forbid-second-client.nix ]; };
    assert !(builtins.tryEval (workspaceLab.nixoriumValidateControllerSoftwareCandidate (scope "all-clients"))).success;
    true;
}
