{ pkgs, lib, hostName, labSettings, labAssets, ... }:
let
  isMaster = hostName == labSettings.masterHostName;
  backgroundFiles = builtins.listToAttrs (map (background: {
    name = "lab/backgrounds/${builtins.baseNameOf (builtins.unsafeDiscardStringContext (toString background))}";
    value.source = background;
  }) labAssets.backgrounds);
in
{
  imports = [
    { environment.etc = backgroundFiles; }
  ];

  time.timeZone = labSettings.timeZone;
  i18n.defaultLocale = labSettings.defaultLocale;
  i18n.extraLocaleSettings = {
    LC_ADDRESS = labSettings.extraLocale;
    LC_IDENTIFICATION = labSettings.extraLocale;
    LC_MEASUREMENT = labSettings.extraLocale;
    LC_MONETARY = labSettings.extraLocale;
    LC_NAME = labSettings.extraLocale;
    LC_NUMERIC = labSettings.extraLocale;
    LC_PAPER = labSettings.extraLocale;
    LC_TELEPHONE = labSettings.extraLocale;
    LC_TIME = labSettings.extraLocale;
  };
  services.xserver.xkb.layout = labSettings.keyboardLayout;
  console.keyMap = labSettings.consoleKeyMap;

  services.xserver.enable = true;
  services.displayManager.gdm.enable = true;
  services.desktopManager.gnome.enable = true;

  systemd.services.gdm-monitor-config = lib.mkIf isMaster {
    description = "Copy admin monitor layout to GDM and teacher";
    wantedBy = [ "display-manager.service" ];
    before = [ "display-manager.service" ];
    after = [ "local-fs.target" ];
    serviceConfig.Type = "oneshot";
    script = ''
      ADMIN_MONITORS="/home/admin/.config/monitors.xml"
      GDM_CONFIG_DIR="/var/lib/gdm/seat0/config"
      GDM_MONITORS="$GDM_CONFIG_DIR/monitors.xml"
      TEACHER_CONFIG_DIR="/home/${labSettings.teacherUser}/.config"
      TEACHER_MONITORS="$TEACHER_CONFIG_DIR/monitors.xml"

      if [ ! -f "$ADMIN_MONITORS" ]; then
        exit 0
      fi

      install -d -m 0700 "$GDM_CONFIG_DIR"
      cp "$ADMIN_MONITORS" "$GDM_MONITORS"
      chmod 0600 "$GDM_MONITORS"
      chown "$(stat -c '%u:%g' "$GDM_CONFIG_DIR")" "$GDM_MONITORS"

      install -d -o "${labSettings.teacherUser}" -g users -m 0700 "$TEACHER_CONFIG_DIR"
      cp "$ADMIN_MONITORS" "$TEACHER_MONITORS"
      chmod 0600 "$TEACHER_MONITORS"
      chown "${labSettings.teacherUser}:users" "$TEACHER_MONITORS"
    '';
    path = [ pkgs.coreutils ];
  };

  services.gnome.gnome-keyring.enable = lib.mkForce false;
  services.desktopManager.gnome.extraGSettingsOverrides = ''
    [org.gnome.desktop.session]
    idle-delay=uint32 0

    [org.gnome.desktop.screensaver]
    lock-enabled=false
    lock-delay=uint32 0

    [org.gnome.settings-daemon.plugins.power]
    sleep-inactive-ac-type='nothing'
    sleep-inactive-battery-type='nothing'
    sleep-inactive-ac-timeout=0
    sleep-inactive-battery-timeout=0
    idle-dim=false

    [org.gnome.desktop.input-sources]
    sources=[('xkb', '${labSettings.keyboardLayout}')]
  '';
  services.desktopManager.gnome.extraGSettingsOverridePackages = [
    pkgs.gnome-settings-daemon
  ];
  services.gnome.gnome-initial-setup.enable = false;

  services.printing.enable = true;
  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    alsa.support32Bit = true;
    pulse.enable = true;
  };
  systemd.user.services.xdg-user-dirs = {
    description = "Create XDG user directories";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.xdg-user-dirs}/bin/xdg-user-dirs-update";
    };
    wantedBy = [ "default.target" ];
  };

}
