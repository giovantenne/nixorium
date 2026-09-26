{ pkgs, lib, labSettings, hostSoftwarePackages, ... }:
let
  has = package: builtins.elem package hostSoftwarePackages;
  hasGhostty = has "ghostty";
  hasChromium = has "chromium";
  hasCode = has "vscode";
  dashToDock = pkgs.gnomeExtensions.dash-to-dock;
  desktopIcons = pkgs.gnomeExtensions.desktop-icons-ng-ding;
  tilingAssistant = pkgs.gnomeExtensions.tiling-assistant;
  extensionSchemas = pkgs.runCommand "nixorium-desktop-extension-schemas" {} ''
    SCHEMA_DIR="$out/share/gsettings-schemas/nixorium-desktop-extension-schemas/glib-2.0/schemas"
    mkdir -p "$SCHEMA_DIR"
    cp ${dashToDock}/share/gnome-shell/extensions/${dashToDock.extensionUuid}/schemas/*.xml "$SCHEMA_DIR/"
    cp ${tilingAssistant}/share/gnome-shell/extensions/${tilingAssistant.extensionUuid}/schemas/*.xml "$SCHEMA_DIR/"
  '';
  desktopBackground = pkgs.writeText "nixorium-desktop.svg" ''
    <svg xmlns="http://www.w3.org/2000/svg" width="3840" height="2160" viewBox="0 0 3840 2160">
      <defs>
        <linearGradient id="base" x2="1" y2="1">
          <stop stop-color="#102339"/><stop offset="1" stop-color="#18202f"/>
        </linearGradient>
        <linearGradient id="ribbon" x2="1" y2="1">
          <stop stop-color="#3584e4" stop-opacity=".30"/>
          <stop offset="1" stop-color="#62a0ea" stop-opacity=".05"/>
        </linearGradient>
      </defs>
      <path fill="url(#base)" d="M0 0h3840v2160H0z"/>
      <path fill="url(#ribbon)" d="M1200 2160 2820 0h520L1720 2160z"/>
      <path fill="#62a0ea" opacity=".06" d="M1880 2160 3500 0h340v240L2400 2160z"/>
    </svg>
  '';
  # One source for fresh-account defaults and the targeted, one-time migration.
  # No dconf locks: staff can adjust these choices after the first login.
  appearanceSettings = {
    "org.gnome.desktop.interface" = {
      color-scheme = "'prefer-dark'";
      gtk-theme = "'Adwaita-dark'";
      icon-theme = "'MoreWaita'";
      accent-color = "'blue'";
      enable-animations = "true";
    } // lib.optionalAttrs (has "liberation_ttf") {
      font-name = "'Liberation Sans 11'";
      document-font-name = "'Liberation Sans 11'";
    } // lib.optionalAttrs (has "nerd-fonts.jetbrains-mono") {
      monospace-font-name = "'JetBrainsMono Nerd Font Mono 12'";
    };
    "org.gnome.desktop.background" = {
      picture-uri = "'file://${desktopBackground}'";
      picture-uri-dark = "'file://${desktopBackground}'";
      picture-options = "'zoom'";
    };
    "org.gnome.shell.extensions.dash-to-dock" = {
      autohide = "false";
      dock-fixed = "true";
      intellihide = "false";
      dock-position = "'BOTTOM'";
      extend-height = "false";
      dash-max-icon-size = "40";
      custom-theme-shrink = "true";
      transparency-mode = "'FIXED'";
      background-opacity = "0.92";
      show-trash = "false";
      show-mounts = "false";
    };
    "org.gnome.shell.extensions.tiling-assistant" = {
      enable-tiling-popup = "true";
      window-gap = "8";
      single-screen-gap = "8";
      show-layout-panel-indicator = "false";
    };
  };
  applyAppearance = lib.concatStringsSep "\n" (lib.mapAttrsToList (schema: values:
    lib.concatStringsSep "\n" (lib.mapAttrsToList (key: value:
      "gsettings set ${lib.escapeShellArg schema} ${lib.escapeShellArg key} ${lib.escapeShellArg value}"
    ) values)
  ) appearanceSettings);
  gvariantList = values: "['${builtins.concatStringsSep "', '" values}']";
  studentFavorites = lib.optionals hasGhostty [ "com.mitchellh.ghostty.desktop" ]
    ++ lib.optionals hasChromium [ "chromium-browser.desktop" ]
    ++ lib.optionals hasCode [ "code.desktop" ]
    ++ [ "org.gnome.Nautilus.desktop" "org.gnome.TextEditor.desktop" ];
  staffFavorites = studentFavorites ++ [ "io.veyon.desktop" ];
  enabledExtensions = [
    "ding@rastersoft.com"
    "dash-to-dock@micxgx.gmail.com"
    "tiling-assistant@leleat-on-github"
  ];
  customBindings = lib.optionals hasGhostty [ "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/terminal/" ]
    ++ lib.optionals hasChromium [ "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/browser/" ]
    ++ lib.optionals hasCode [ "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/code/" ];
in
{
  services.desktopManager.gnome.extraGSettingsOverrides = ''
    [org.gnome.desktop.wm.preferences]
    button-layout='appmenu:minimize,maximize,close'
    ${lib.optionalString (has "liberation_ttf") ''
      titlebar-font='Liberation Sans Bold 11'
    ''}

    [org.gnome.shell]
    enabled-extensions=${gvariantList enabledExtensions}
    favorite-apps=${gvariantList staffFavorites}
    welcome-dialog-last-shown-version='9999'

    ${lib.generators.toINI {} appearanceSettings}
    ${lib.optionalString hasGhostty ''
      [org.gnome.desktop.default-applications.terminal]
      exec='ghostty'
      exec-arg='--'
    ''}

    [org.gnome.settings-daemon.plugins.media-keys]
    custom-keybindings=${gvariantList customBindings}

    ${lib.optionalString hasGhostty ''
      [org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/terminal/]
      name='Ghostty'
      command='/run/current-system/sw/bin/ghostty'
      binding='<Super>Return'
    ''}

    ${lib.optionalString hasChromium ''
      [org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/browser/]
      name='Chromium'
      command='/run/current-system/sw/bin/chromium'
      binding='<Super><Shift>Return'
    ''}

    ${lib.optionalString hasCode ''
      [org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/code/]
      name='Code'
      command='/run/current-system/sw/bin/code'
      binding='<Primary><Shift>c'
    ''}

    [org.gnome.desktop.wm.keybindings]
    maximize=['<Super>Up']
    unmaximize=['<Super>Down']

    [org.gnome.mutter.keybindings]
    toggle-tiled-left=['<Super>Left']
    toggle-tiled-right=['<Super>Right']

    [org.gnome.nautilus.icon-view]
    default-zoom-level='small'
  '';
  services.desktopManager.gnome.extraGSettingsOverridePackages = [
    extensionSchemas
    pkgs.nautilus
    pkgs.gnome-settings-daemon
  ];

  environment.systemPackages = [
    dashToDock
    desktopIcons
    tilingAssistant
    pkgs.morewaita-icon-theme
  ];

  fonts.packages = lib.optionals (has "nerd-fonts.jetbrains-mono") [ pkgs.nerd-fonts.jetbrains-mono ]
    ++ lib.optionals (has "liberation_ttf") [ pkgs.liberation_ttf ];
  fonts.fontconfig.defaultFonts = lib.mkIf (has "liberation_ttf") {
    monospace = lib.optional (has "nerd-fonts.jetbrains-mono") "JetBrainsMono Nerd Font";
    sansSerif = [ "Liberation Sans" ];
    serif = [ "Liberation Serif" ];
  };

  environment.etc."xdg/ghostty/config" = lib.mkIf hasGhostty {
    text = ''
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
  };

  environment.etc."chromium/policies/managed/homepage.json" = lib.mkIf hasChromium {
    text = builtins.toJSON {
      HomepageLocation = labSettings.homepageUrl;
      HomepageIsNewTabPage = false;
      RestoreOnStartup = 4;
      RestoreOnStartupURLs = [ labSettings.homepageUrl ];
    };
  };

  environment.etc."lab/gnome-user-setup.sh" = {
    text = ''
      #!/usr/bin/env bash
      set -euo pipefail

      case "''${USER:-}" in
        ${labSettings.studentUser}) favorites=${lib.escapeShellArg (gvariantList studentFavorites)} ;;
        admin|${labSettings.teacherUser}) favorites=${lib.escapeShellArg (gvariantList staffFavorites)} ;;
        *) exit 0 ;;
      esac

      sleep 2
      gsettings set org.gnome.shell favorite-apps "$favorites"
      ${pkgs.gnome-shell}/bin/gnome-extensions enable "ding@rastersoft.com"
      ${pkgs.gnome-shell}/bin/gnome-extensions enable "dash-to-dock@micxgx.gmail.com"
      ${pkgs.gnome-shell}/bin/gnome-extensions enable "tiling-assistant@leleat-on-github"
      STYLE_STATE="''${XDG_CONFIG_HOME:-$HOME/.config}/nixorium/desktop-style-v1"
      if [ ! -e "$STYLE_STATE" ]; then
        ${applyAppearance}
        mkdir -p "$(dirname "$STYLE_STATE")"
        touch "$STYLE_STATE"
      fi
      gsettings set org.gnome.shell welcome-dialog-last-shown-version '9999'
    '';
    mode = "0755";
  };

  environment.gnome.excludePackages = lib.optionals hasGhostty [ pkgs.gnome-console ];
  environment.sessionVariables = {
    EDITOR = "gnome-text-editor";
    VISUAL = "gnome-text-editor";
  };

  systemd.user.services.lab-gnome-setup = {
    description = "Site GNOME desktop, favorites and welcome setup";
    wantedBy = [ "graphical-session.target" ];
    after = [ "graphical-session.target" ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.bash}/bin/bash /etc/lab/gnome-user-setup.sh";
    };
    path = [ pkgs.glib pkgs.gsettings-desktop-schemas ];
  };
}
