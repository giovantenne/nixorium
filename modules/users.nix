{ hostName, labSettings, lib, pkgs, ... }:

let
  # Master controller: no autologin (teacher selects account)
  isMaster = hostName == labSettings.masterHostName;
  normalUsers = [ "admin" labSettings.teacherUser labSettings.studentUser ];
  repairUserHome = user: ''
    HOME_DIR="/home/${user}"
    for PATH_NAME in "$HOME_DIR/.config" "$HOME_DIR/.local" "$HOME_DIR/.vscode"; do
      if [ -d "$PATH_NAME" ] && [ ! -L "$PATH_NAME" ]; then
        chown ${user}:users "$PATH_NAME"
        chmod u+rwx "$PATH_NAME"
      fi
    done
    for PATH_NAME in "$HOME_DIR/.config/Code" "$HOME_DIR/.vscode/extensions" "$HOME_DIR/.local/npm"; do
      if [ -d "$PATH_NAME" ] && [ ! -L "$PATH_NAME" ]; then
        chown -hR ${user}:users "$PATH_NAME"
        chmod -R u+rwX "$PATH_NAME"
      fi
    done
    USER_ID="$(${pkgs.coreutils}/bin/id -u ${user})"
    RUNTIME_DIR="/run/user/$USER_ID"
    if [ -d "$RUNTIME_DIR" ] && [ ! -L "$RUNTIME_DIR" ]; then
      chown ${user}:users "$RUNTIME_DIR"
      chmod 0700 "$RUNTIME_DIR"
    fi
  '';
in
{
  # Passwords are managed declaratively, cannot be changed manually
  users.mutableUsers = false;

  # Disable root password login
  users.users.root = {
    hashedPassword = "!";
    openssh.authorizedKeys.keys =
      if labSettings.adminSshKey == null then
        []
      else
        [ labSettings.adminSshKey ];
  };

  users.users.${labSettings.teacherUser} = {
    isNormalUser = true;
    description = labSettings.teacherUser;
    extraGroups = [ "networkmanager" "veyon-master" ];
    hashedPassword = labSettings.teacherPassword;
  };

  users.users.${labSettings.studentUser} = {
    isNormalUser = true;
    description = labSettings.studentUser;
    extraGroups = [ "render" "video" ];
    hashedPassword = labSettings.studentPassword;
  };

  users.users.admin = {
    isNormalUser = true;
    description = "admin";
    extraGroups = [ "networkmanager" "wheel" "veyon-master" ];
    hashedPassword = labSettings.adminPassword;
    openssh.authorizedKeys.keys =
      if labSettings.adminSshKey == null then
        []
      else
        [ labSettings.adminSshKey ];
  };

  services.displayManager.autoLogin = {
    enable = !isMaster;
    user = labSettings.studentUser;
  };

  # Deployment activation may install editor defaults as root. Repair only the
  # managed application trees and XDG roots, never arbitrary user content.
  system.activationScripts.nixoriumUserHomeOwnership = {
    deps = [ "users" "createHomeTemplates" ];
    text = builtins.concatStringsSep "\n" (map repairUserHome normalUsers);
  };

  # Keep the student session online without allowing it to alter host
  # connections, radios, DNS, or other NetworkManager-managed state.
  security.polkit.extraConfig = lib.mkBefore ''
    polkit.addRule(function(action, subject) {
      if (subject.user == "${labSettings.studentUser}" &&
          action.id.indexOf("org.freedesktop.NetworkManager.") == 0) {
        return polkit.Result.NO;
      }
    });
  '';
}
