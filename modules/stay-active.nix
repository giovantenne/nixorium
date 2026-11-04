{ pkgs, ... }:
# Caffeine adds a "Caffeine" toggle to GNOME's quick settings for every user:
# while it is on, the screen does not blank after the idle delay (a session
# idle inhibitor), and the lab screensaver stays away too. It starts off at
# every sign-in.
let
  caffeine = pkgs.gnomeExtensions.caffeine;
in
{
  environment.systemPackages = [ caffeine ];

  # Caffeine switches itself on while any window is fullscreen. The lab
  # screensaver is a fullscreen window and stops while the screen is kept on,
  # so it would stop itself as soon as it appeared. Only the toggle counts.
  programs.dconf.profiles.user.databases = [{
    settings."org/gnome/shell/extensions/caffeine".enable-fullscreen = false;
  }];

  # Enabled in every session like the classroom extension: gnome-extensions
  # keeps the other enabled extensions, including those a persistent home
  # already lists. A failed enable is retried after one second.
  systemd.user.services.nixorium-stay-active-extension = {
    description = "Enable the Caffeine GNOME Shell extension";
    wantedBy = [ "graphical-session.target" ];
    partOf = [ "graphical-session.target" ];
    after = [ "graphical-session.target" ];
    unitConfig.StartLimitIntervalSec = 0;
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.gnome-shell}/bin/gnome-extensions enable ${caffeine.extensionUuid}";
      Restart = "on-failure";
      RestartSec = 1;
    };
  };
}
