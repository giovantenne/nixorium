{ pkgs, lib, labSettings, ... }:

let
  asUser = import ../lib/as-user.nix { inherit pkgs lib; };
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
      ${pkgs.bash}/bin/bash ${createTemplateScript} ${lib.escapeShellArgs [
        templateDirAdmin
        gitConfigAdmin.name
        gitConfigAdmin.email
        "${pkgs.xdg-user-dirs}/bin/xdg-user-dirs-update"
      ]}
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
      # Written by the teacher: the bookmark file is in the teacher's home.
      ${asUser labSettings.teacherUser ''${pkgs.bash}/bin/sh -c ${lib.escapeShellArg ''
        file="$1/.config/gtk-3.0/bookmarks"
        mkdir -p "$1/.config/gtk-3.0"
        grep -q home-snapshots "$file" 2>/dev/null || echo "file:///var/lib/home-snapshots Snapshots" >> "$file"
      ''} sh "/home/${labSettings.teacherUser}"''}
    '';
    # After the repair that gives the teacher back a ~/.config made by root.
    deps = [ "users" "nixoriumUserHomeOwnership" ];
  };
}
