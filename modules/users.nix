{ hostName, labSettings, ... }:

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
    autoSubUidGidRange = true;
  };

  users.users.${labSettings.studentUser} = {
    isNormalUser = true;
    description = labSettings.studentUser;
    extraGroups = [ "networkmanager" "render" "video" ];
    hashedPassword = labSettings.studentPassword;
    autoSubUidGidRange = true;
  };

  users.users.admin = {
    isNormalUser = true;
    description = "admin";
    extraGroups = [ "networkmanager" "wheel" "veyon-master" ];
    hashedPassword = labSettings.adminPassword;
    autoSubUidGidRange = true;
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
}
