{ hostName, labSettings, lib, ... }:

let
  # Master controller: no autologin (teacher selects account)
  isMaster = hostName == labSettings.masterHostName;
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
