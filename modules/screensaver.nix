{ config, pkgs, lib, labAssets, hostSoftwarePackages, nixoriumTtfx, ... }:
# The lab screensaver: after 5 idle minutes a fullscreen Ghostty animates the
# lab logo (assets/logo.txt in the deployment) with random ttfx text effects;
# after 30 GNOME blanks the screen and the screensaver stops. It never locks
# the session. Every built-in profile declares Ghostty and the text effects,
# whose presence switches it on for that host.
let
  enabled = builtins.elem "ghostty" hostSoftwarePackages
    && builtins.elem "python3Packages.terminaltexteffects" hostSoftwarePackages;
  # A deployment whose template still carries its own screensaver (a
  # migration left as a manual change) keeps that one, so two never run.
  templateScreensaver = config.systemd.user.services ? lab-screensaver;
in
{
  config = lib.mkIf enabled {
    # The scripts are always executed through bash, never directly, so they
    # need no executable bit.
    systemd.user.services.nixorium-screensaver = {
      description = "Lab text-effects screensaver";
      enable = !templateScreensaver;
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
      path = [ pkgs.bash pkgs.glib pkgs.gnugrep pkgs.procps pkgs.ghostty nixoriumTtfx pkgs.ncurses ];
    };
  };
}
