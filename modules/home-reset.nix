{ pkgs, labSettings, ... }:

let
  gitConfigAdmin = {
    name = labSettings.adminGitName;
    email = labSettings.adminGitEmail;
  };

  templateDirAdmin = "/var/lib/home-template/admin";
  createTemplateScript = ../scripts/create-home-template.sh;
in
{
  # The student home is restored from the workspace seed at every boot.
  imports = [ ./workspace-reset.nix ];
  # Create templates at system activation (rebuild time)
  system.activationScripts.createHomeTemplates = {
    text = ''
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

  # Ensure directories have correct permissions
  systemd.tmpfiles.rules = [
    "d /var/lib/home-snapshots 0750 root nixorium-staff -"
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
