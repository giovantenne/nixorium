{ pkgs, labSettings, ... }:

let
  # Git configuration
  gitConfigStudent = {
    name = labSettings.studentGitName;
    email = labSettings.studentGitEmail;
  };

  gitConfigAdmin = {
    name = labSettings.adminGitName;
    email = labSettings.adminGitEmail;
  };

  templateDirStudent = "/var/lib/home-template/${labSettings.studentUser}";
  templateDirAdmin = "/var/lib/home-template/admin";
  snapshotsDir = "/var/lib/home-snapshots";
  homeDirStudent = "/home/${labSettings.studentUser}";
  homeDirAdmin = "/home/admin";
  # VSCode extensions to pre-install in the template
  vscodeExtensions = [
    {
      pkg = pkgs.vscode-extensions.ritwickdey.liveserver;
      dir = "ritwickdey.liveserver";
    }
    # Microsoft Extension Pack for Java (meta-pack + all individual extensions)
    {
      pkg = pkgs.vscode-extensions.vscjava.vscode-java-pack;
      dir = "vscjava.vscode-java-pack";
    }
    {
      pkg = pkgs.vscode-extensions.redhat.java;
      dir = "redhat.java";
    }
    {
      pkg = pkgs.vscode-extensions.vscjava.vscode-java-debug;
      dir = "vscjava.vscode-java-debug";
    }
    {
      pkg = pkgs.vscode-extensions.vscjava.vscode-java-test;
      dir = "vscjava.vscode-java-test";
    }
    {
      pkg = pkgs.vscode-extensions.vscjava.vscode-maven;
      dir = "vscjava.vscode-maven";
    }
    {
      pkg = pkgs.vscode-extensions.vscjava.vscode-java-dependency;
      dir = "vscjava.vscode-java-dependency";
    }
  ];

  # External scripts
  createTemplateScript = ../scripts/create-home-template.sh;
  homeResetScript = ../scripts/home-reset.sh;
  assetsDir = ../assets;

in
{
  # Create templates at system activation (rebuild time)
  system.activationScripts.createHomeTemplates = {
    text = ''
      # Create student template
      ${pkgs.bash}/bin/bash ${createTemplateScript} "${templateDirStudent}" "${gitConfigStudent.name}" "${gitConfigStudent.email}" "${pkgs.xdg-user-dirs}/bin/xdg-user-dirs-update" "${assetsDir}"
      mkdir -p "${templateDirStudent}/.vscode/extensions"
      ${builtins.concatStringsSep "\n      " (map (ext:
        ''cp -a "${ext.pkg}/share/vscode/extensions/${ext.dir}" "${templateDirStudent}/.vscode/extensions/"''
      ) vscodeExtensions)}
      # Fix Nix store read-only permissions so extensions can write temp files
      chmod -R u+w "${templateDirStudent}/.vscode/extensions"
      # Remove LiveServer announcement to suppress the "NEW" toast notification
      ${pkgs.jq}/bin/jq 'del(.announcement)' \
        "${templateDirStudent}/.vscode/extensions/ritwickdey.liveserver/package.json" \
        > "${templateDirStudent}/.vscode/extensions/ritwickdey.liveserver/package.json.tmp"
      mv "${templateDirStudent}/.vscode/extensions/ritwickdey.liveserver/package.json.tmp" \
        "${templateDirStudent}/.vscode/extensions/ritwickdey.liveserver/package.json"
      chown -R ${labSettings.studentUser}:users "${templateDirStudent}"

      # Create admin template
      ${pkgs.bash}/bin/bash ${createTemplateScript} "${templateDirAdmin}" "${gitConfigAdmin.name}" "${gitConfigAdmin.email}" "${pkgs.xdg-user-dirs}/bin/xdg-user-dirs-update" "${assetsDir}"
      chown -R admin:users "${templateDirAdmin}"

      # Setup admin home (once, not reset at boot)
      if [ ! -f "/home/admin/.home-initialized" ]; then
        cp -a "${templateDirAdmin}/." "/home/admin/"
        chown -R admin:users "/home/admin"
        touch "/home/admin/.home-initialized"
      fi
    '';
    deps = [ "users" ];
  };

  # Systemd service to reset student home at boot
  systemd.services.home-reset = {
    description = "Reset ${labSettings.studentUser} home directory from template";
    wantedBy = [ "multi-user.target" ];
    before = [ "display-manager.service" ];
    after = [ "local-fs.target" ];
    unitConfig = {
      RequiresMountsFor = [
        "/home/${labSettings.studentUser}"
        "/var/lib/home-template"
        "/var/lib/home-snapshots"
      ];
    };
    path = [ pkgs.btrfs-progs pkgs.dconf pkgs.findutils pkgs.coreutils ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.bash}/bin/bash ${homeResetScript} ${snapshotsDir} ${homeDirStudent} ${templateDirStudent} ${labSettings.studentUser}:users";
      RemainAfterExit = true;
    };
  };

  # Ensure directories have correct permissions
  systemd.tmpfiles.rules = [
    "d /var/lib/home-snapshots 0750 root veyon-master -"
    "d /var/lib/home-template 0755 root root -"
  ];

  # Add "Snapshots" bookmark in Nautilus sidebar for teacher
  system.activationScripts.teacherSnapshotBookmark = {
    text = ''
      BOOKMARK_DIR="/home/${labSettings.teacherUser}/.config/gtk-3.0"
      BOOKMARK_FILE="$BOOKMARK_DIR/bookmarks"
      ENTRY="file:///var/lib/home-snapshots Snapshots"
      mkdir -p "$BOOKMARK_DIR"
      if ! grep -q "home-snapshots" "$BOOKMARK_FILE" 2>/dev/null; then
        echo "$ENTRY" >> "$BOOKMARK_FILE"
      fi
      chown -R ${labSettings.teacherUser}:users "$BOOKMARK_DIR"
    '';
    deps = [ "users" ];
  };
}
