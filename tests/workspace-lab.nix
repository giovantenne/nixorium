{ mkLab, deploymentSelf, labConfig }:
extraArgs:
mkLab ({
  inherit deploymentSelf;
  deploymentRevision = "0123456789abcdef0123456789abcdef01234567";
  labConfig = labConfig // { pcCount = 2; };
  labSoftware = {
    schemaVersion = 1;
    packages = [
      { package = "vscode"; scope.kind = "shared"; }
      { package = "jdk"; scope.kind = "shared"; }
    ];
  };
  workspaceProfileJSON = builtins.toJSON {
    schemaVersion = 1;
    desktop = {
      colorScheme = "dark";
      favorites = [ "org.gnome.TextEditor.desktop" "code.desktop" ];
    };
    vscode = {
      extensions = [ "redhat.java" ];
      settings."editor.fontSize" = 14;
    };
  };
  workspaceCatalog = {
    schemaVersion = 1;
    baseline = { schemaVersion = 1; desktop.enableAnimations = false; };
    applications = [
      { id = "org.gnome.TextEditor.desktop"; package = "gnome-text-editor"; }
      { id = "code.desktop"; package = "vscode"; }
    ];
    extensions = [
      { id = "redhat.java"; package = "vscode-extensions.redhat.java"; requiredPackages = [ "jdk" ]; }
    ];
  };
} // extraArgs)
