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
    openssh.authorizedKeys.keys = [
      labSettings.adminSshKey
    ];
  };

  users.users.${labSettings.teacherUser} = {
    isNormalUser = true;
    description = labSettings.teacherUser;
    extraGroups = [ "networkmanager" "docker" "veyon-master" ];
    hashedPassword = labSettings.teacherPassword;
  };

  users.users.${labSettings.studentUser} = {
    isNormalUser = true;
    description = labSettings.studentUser;
    extraGroups = [ "networkmanager" "docker" "render" "video" ];
    hashedPassword = labSettings.studentPassword;
  };

  users.users.admin = {
    isNormalUser = true;
    description = "admin";
    extraGroups = [ "networkmanager" "wheel" "docker" "veyon-master" ];
    hashedPassword = labSettings.adminPassword;
    openssh.authorizedKeys.keys = [
      labSettings.adminSshKey
    ];
  };

  services.displayManager.autoLogin = {
    enable = !isMaster;
    user = labSettings.studentUser;
  };
}
