{ pkgs, labSettings, homeResetEphemeralPaths, ... }:

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
  ephemeralPathsFile = pkgs.writeText "nixorium-home-reset-ephemeral-paths"
    (builtins.concatStringsSep "\n" homeResetEphemeralPaths + "\n");
  # External scripts
  createTemplateScript = ../scripts/create-home-template.sh;
  homeResetScript = ../scripts/home-reset.sh;
in
{
  # Create templates at system activation (rebuild time)
  system.activationScripts.createHomeTemplates = {
    text = ''
      # Create student template
      ${pkgs.bash}/bin/bash ${createTemplateScript} "${templateDirStudent}" "${gitConfigStudent.name}" "${gitConfigStudent.email}" "${pkgs.xdg-user-dirs}/bin/xdg-user-dirs-update"
      chown -R ${labSettings.studentUser}:users "${templateDirStudent}"

      # Create admin template
      ${pkgs.bash}/bin/bash ${createTemplateScript} "${templateDirAdmin}" "${gitConfigAdmin.name}" "${gitConfigAdmin.email}" "${pkgs.xdg-user-dirs}/bin/xdg-user-dirs-update"
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
    requiredBy = [ "display-manager.service" ];
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
      ExecStart = "${pkgs.bash}/bin/bash ${homeResetScript} ${snapshotsDir} ${homeDirStudent} ${templateDirStudent} ${labSettings.studentUser}:users ${ephemeralPathsFile}";
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
