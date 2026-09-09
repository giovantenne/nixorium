{ lib, hostName, labSettings, ... }:
let
  isMaster = hostName == labSettings.masterHostName;
in
{
  services.logind.settings.Login = {
    HandleLidSwitch = "ignore";
    HandleLidSwitchExternalPower = "ignore";
    HandleLidSwitchDocked = "ignore";
  };

  systemd.sleep.settings.Sleep = lib.mkIf isMaster {
    AllowSuspend = "no";
    AllowHibernation = "no";
    AllowHybridSleep = "no";
    AllowSuspendThenHibernate = "no";
  };
  systemd.targets.sleep.enable = lib.mkIf isMaster false;
  systemd.targets.suspend.enable = lib.mkIf isMaster false;
  systemd.targets.hibernate.enable = lib.mkIf isMaster false;
  systemd.targets.hybrid-sleep.enable = lib.mkIf isMaster false;
}
