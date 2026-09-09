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
    [org.gnome.desktop.interface]
    color-scheme='prefer-dark'
    enable-animations=true
    font-name='Liberation Sans 11'
    document-font-name='Liberation Sans 11'
    monospace-font-name='JetBrainsMono Nerd Font Mono 12'
    icon-theme='Yaru-yellow'
    gtk-theme='Adwaita-dark'

    [org.gnome.desktop.wm.preferences]
    button-layout='appmenu:minimize,maximize,close'
    titlebar-font='Liberation Sans Bold 11'

    [org.gnome.shell]
    enabled-extensions=['ding@rastersoft.com', 'dash-to-dock@micxgx.gmail.com']
    favorite-apps=['com.mitchellh.ghostty.desktop', 'chromium-browser.desktop', 'code.desktop', 'io.veyon.desktop', 'org.gnome.Nautilus.desktop', 'org.gnome.TextEditor.desktop']
    welcome-dialog-last-shown-version='9999'

    [org.gnome.shell.extensions.dash-to-dock]
    show-trash=false

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

    [org.gnome.desktop.default-applications.terminal]
    exec='ghostty'
    exec-arg='--'

    [org.gnome.settings-daemon.plugins.media-keys]
    custom-keybindings=['/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom0/', '/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom1/', '/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom2/']

    [org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom0/]
    name='Ghostty'
    command='/run/current-system/sw/bin/ghostty'
    binding='<Super>Return'

    [org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom1/]
    name='Chromium'
    command='/run/current-system/sw/bin/chromium'
    binding='<Super><Shift>Return'

    [org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom2/]
    name='Code'
    command='/run/current-system/sw/bin/code'
    binding='<Primary><Shift>c'

    [org.gnome.desktop.wm.keybindings]
    maximize=['<Super>Up']
    unmaximize=['<Super>Down']

    [org.gnome.mutter.keybindings]
    toggle-tiled-left=['<Super>Left']
    toggle-tiled-right=['<Super>Right']

    [org.gnome.desktop.input-sources]
    sources=[('xkb', '${labSettings.keyboardLayout}')]

    [org.gnome.nautilus.icon-view]
    default-zoom-level='small'
  '';
  services.desktopManager.gnome.extraGSettingsOverridePackages = [
    pkgs.nautilus
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
  programs.firefox.enable = true;

  fonts.packages = [
    pkgs.nerd-fonts.jetbrains-mono
    pkgs.liberation_ttf
  ];
  fonts.fontconfig.defaultFonts = {
    monospace = [ "JetBrainsMono Nerd Font" ];
    sansSerif = [ "Liberation Sans" ];
    serif = [ "Liberation Serif" ];
  };

  systemd.user.services.xdg-user-dirs = {
    description = "Create XDG user directories";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.xdg-user-dirs}/bin/xdg-user-dirs-update";
    };
    wantedBy = [ "default.target" ];
  };

  environment.etc."xdg/ghostty/config".text = ''
    font-size = 14
    background = #2c2525
    foreground = #e6d9db
    cursor-color = #c3b7b8
    selection-background = #403e41
    selection-foreground = #e6d9db
    palette = 0=#72696a
    palette = 1=#fd6883
    palette = 2=#adda78
    palette = 3=#f9cc6c
    palette = 4=#f38d70
    palette = 5=#a8a9eb
    palette = 6=#85dacc
    palette = 7=#e6d9db
    palette = 8=#948a8b
    palette = 9=#ff8297
    palette = 10=#c8e292
    palette = 11=#fcd675
    palette = 12=#f8a788
    palette = 13=#bebffd
    palette = 14=#9bf1e1
    palette = 15=#f1e5e7
    window-padding-x = 8
    window-padding-y = 4
  '';

  environment.etc."chromium/policies/managed/homepage.json".text = builtins.toJSON {
    HomepageLocation = labSettings.homepageUrl;
    HomepageIsNewTabPage = false;
    RestoreOnStartup = 4;
    RestoreOnStartupURLs = [ labSettings.homepageUrl ];
  };

  environment.etc."lab/gnome-user-setup.sh" = {
    text = ''
      #!/usr/bin/env bash
      set -euo pipefail

      case "''${USER:-}" in
        ${labSettings.studentUser}|admin|${labSettings.teacherUser}) ;;
        *) exit 0 ;;
      esac

      sleep 2

      if [ "''${USER:-}" = "${labSettings.studentUser}" ]; then
        gsettings set org.gnome.shell favorite-apps \
          "['com.mitchellh.ghostty.desktop', 'chromium-browser.desktop', 'code.desktop', 'org.gnome.Nautilus.desktop', 'org.gnome.TextEditor.desktop']"
      else
        current_favorites=$(gsettings get org.gnome.shell favorite-apps)
        updated_favorites=$(python3 - "$current_favorites" << 'PY'
      import ast
      import sys

      raw = sys.argv[1].strip()
      if raw.startswith("@as "):
          raw = raw[4:]

      favorites = ast.literal_eval(raw)
      if "io.veyon.desktop" not in favorites:
          if "code.desktop" in favorites:
              favorites.insert(favorites.index("code.desktop") + 1, "io.veyon.desktop")
          else:
              favorites.append("io.veyon.desktop")
      print(repr(favorites))
      PY
        ) || updated_favorites=""
        if [[ -n "$updated_favorites" ]]; then
          gsettings set org.gnome.shell favorite-apps "$updated_favorites"
        fi
      fi

      if gsettings list-schemas | grep -qx "org.gnome.shell.extensions.dash-to-dock"; then
        gsettings set org.gnome.shell.extensions.dash-to-dock show-trash false
      fi

      gsettings set org.gnome.shell welcome-dialog-last-shown-version '9999'
    '';
    mode = "0755";
  };

  environment.gnome.excludePackages = [ pkgs.gnome-console ];

  systemd.user.services.lab-gnome-setup = {
    description = "Lab GNOME favorites and welcome setup";
    wantedBy = [ "graphical-session.target" ];
    after = [ "graphical-session.target" ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.bash}/bin/bash /etc/lab/gnome-user-setup.sh";
    };
    path = [ pkgs.glib pkgs.gsettings-desktop-schemas pkgs.python3 pkgs.gnugrep ];
  };

  environment.sessionVariables = {
    EDITOR = "gnome-text-editor";
    VISUAL = "gnome-text-editor";
  };
}
