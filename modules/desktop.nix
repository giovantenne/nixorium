{ pkgs, lib, isController, labSettings, labAssets, ... }:
let
  # A GVariant string literal: backslash and single quote are escaped.
  gvariantString = value:
    "'" + lib.replaceStrings [ "\\" "'" ] [ "\\\\" "\\'" ] value + "'";
  backgroundFiles = builtins.listToAttrs (map (background: {
    name = "lab/backgrounds/${builtins.baseNameOf (builtins.unsafeDiscardStringContext (toString background))}";
    value.source = background;
  }) labAssets.backgrounds);
  asUser = import ../lib/as-user.nix { inherit pkgs lib; };
in
{
  imports = [
    { environment.etc = backgroundFiles; }
    ./terminal-fallback.nix
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
  # The text console is derived from the XKB layout, so tty and GDM/GNOME type
  # the same symbols.
  console.useXkbConfig = true;

  services.xserver.enable = true;
  services.displayManager.gdm.enable = true;
  services.desktopManager.gnome.enable = true;

  systemd.services.gdm-monitor-config = lib.mkIf isController {
    description = "Copy admin monitor layout to GDM and teacher";
    wantedBy = [ "display-manager.service" ];
    before = [ "display-manager.service" ];
    after = [ "local-fs.target" ];
    serviceConfig.Type = "oneshot";
    script = ''
      ADMIN_MONITORS="/home/admin/.config/monitors.xml"
      GDM_CONFIG_DIR="/var/lib/gdm/seat0/config"
      GDM_MONITORS="$GDM_CONFIG_DIR/monitors.xml"
      TEACHER_MONITORS="/home/${labSettings.teacherUser}/.config/monitors.xml"

      if [ ! -f "$ADMIN_MONITORS" ] || [ -L "$ADMIN_MONITORS" ]; then
        exit 0
      fi

      # Reads are bounded: a FIFO swapped in must not hold the login screen.
      install -d -m 0700 "$GDM_CONFIG_DIR"
      rm -f "$GDM_MONITORS"
      timeout 30 cp "$ADMIN_MONITORS" "$GDM_MONITORS"
      chmod 0600 "$GDM_MONITORS"
      chown -h "$(stat -c '%u:%g' "$GDM_CONFIG_DIR")" "$GDM_MONITORS"

      # The teacher's copy is written by the teacher from what root reads:
      # the teacher's cat reads the pipe, so the admin's file may stay private.
      timeout 30 cat "$ADMIN_MONITORS" | ${asUser labSettings.teacherUser ''${pkgs.bash}/bin/sh -c ${lib.escapeShellArg ''
        umask 077
        mkdir -p "''${1%/*}"
        rm -f "$1"
        cat > "$1"
      ''} sh "$TEACHER_MONITORS"''}
    '';
    path = [ pkgs.coreutils ];
  };

  services.gnome.gnome-keyring.enable = lib.mkForce false;
  # Without a secret service MySQL Workbench fails every connection ("The
  # name is not activatable"); this makes it skip the keychain. Set here, not
  # only in the site's development module, so laboratories created before
  # 3.8.0 get it too; it has no effect where Workbench is not installed.
  environment.sessionVariables.WB_NO_KEYRING = "1";
  # No GNOME remote desktop server: the classroom view reaches sessions only
  # through the controller's SSH access.
  systemd.user.services.gnome-remote-desktop.enable = false;
  # After 30 idle minutes GNOME blanks and switches off the screen; the session
  # is never locked, so a key or the mouse brings the desktop back without a
  # password. The computer does not sleep: it stays reachable.
  services.desktopManager.gnome.extraGSettingsOverrides = ''
    [org.gnome.desktop.session]
    idle-delay=uint32 1800

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
    sources=[('xkb', ${gvariantString labSettings.keyboardLayout})]
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
