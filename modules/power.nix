{ lib, isController, ... }:
{
  services.logind.settings.Login = {
    HandleLidSwitch = "ignore";
    HandleLidSwitchExternalPower = "ignore";
    HandleLidSwitchDocked = "ignore";
  };

  systemd.sleep.settings.Sleep = lib.mkIf isController {
    AllowSuspend = "no";
    AllowHibernation = "no";
    AllowHybridSleep = "no";
    AllowSuspendThenHibernate = "no";
  };
  systemd.targets.sleep.enable = lib.mkIf isController false;
  systemd.targets.suspend.enable = lib.mkIf isController false;
  systemd.targets.hibernate.enable = lib.mkIf isController false;
  systemd.targets.hybrid-sleep.enable = lib.mkIf isController false;
}
