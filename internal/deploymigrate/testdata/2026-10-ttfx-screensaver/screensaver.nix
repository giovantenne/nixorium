{ pkgs, lib, labAssets, hostSoftwarePackages, ... }:
let
  enabled = builtins.elem "ghostty" hostSoftwarePackages
    && builtins.elem "python3Packages.terminaltexteffects" hostSoftwarePackages;
in
{
  config = lib.mkIf enabled {
    # The scripts and the logo are used straight from the Nix store. The
    # scripts are always executed through bash, never directly, so a lab copy
    # that lost the executable bit (store files would then be 0444) cannot
    # break the service with 203/EXEC. The screensaver never locks the session.
    systemd.user.services.lab-screensaver = {
      description = "Lab TTE screensaver";
      wantedBy = [ "graphical-session.target" ];
      after = [ "graphical-session.target" ];
      serviceConfig = {
        Type = "simple";
        ExecStart = "${pkgs.bash}/bin/bash ${../scripts/screensaver-monitor.sh}";
        Environment = [
          "SCREENSAVER_SHELL=${pkgs.bash}/bin/bash"
          "SCREENSAVER_CMD=${../scripts/cmd-screensaver.sh}"
          "SCREENSAVER_TEXT=${labAssets.logo}"
        ];
        Restart = "on-failure";
        RestartSec = 5;
      };
      path = [ pkgs.bash pkgs.glib pkgs.gnugrep pkgs.procps pkgs.ghostty pkgs.python3Packages.terminaltexteffects pkgs.ncurses ];
    };
  };
}
