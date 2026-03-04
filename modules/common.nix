{ pkgs, lib, hostName, labSettings, ... }:
let
  isMaster = hostName == labSettings.masterHostName;
in
{
  # Enable flakes and nix-command
  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  # UEFI bootloader (GRUB with os-prober for Windows dual-boot detection)
  boot.loader.systemd-boot.enable = false;
  boot.loader.grub.enable = true;
  boot.loader.grub.efiSupport = true;
  boot.loader.grub.device = "nodev";
  boot.loader.grub.useOSProber = true;
  boot.loader.timeout = 5;
  boot.loader.efi.canTouchEfiVariables = true;
  boot.loader.efi.efiSysMountPoint = "/boot";

  # Enable networking
  networking.networkmanager.enable = true;
  networking.firewall.enable = false;

  time.timeZone = "Europe/Rome";

  i18n.defaultLocale = "en_US.UTF-8";

  i18n.extraLocaleSettings = {
    LC_ADDRESS = "it_IT.UTF-8";
    LC_IDENTIFICATION = "it_IT.UTF-8";
    LC_MEASUREMENT = "it_IT.UTF-8";
    LC_MONETARY = "it_IT.UTF-8";
    LC_NAME = "it_IT.UTF-8";
    LC_NUMERIC = "it_IT.UTF-8";
    LC_PAPER = "it_IT.UTF-8";
    LC_TELEPHONE = "it_IT.UTF-8";
    LC_TIME = "it_IT.UTF-8";
  };

  services.xserver.enable = true;

  # GNOME Desktop Environment (Wayland)
  services.displayManager.gdm.enable = true;
  services.desktopManager.gnome.enable = true;

  # Disable GNOME Keyring (no password prompts for Chromium, VSCode, etc.)
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
    custom-keybindings=['/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom0/', '/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom1/']

    [org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom0/]
    name='Ghostty'
    command='/run/current-system/sw/bin/ghostty'
    binding='<Super>Return'

    [org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom1/]
    name='Chromium'
    command='/run/current-system/sw/bin/chromium'
    binding='<Super><Shift>Return'

    [org.gnome.desktop.wm.keybindings]
    maximize=['<Super>Up']
    unmaximize=['<Super>Down']

    [org.gnome.mutter.keybindings]
    toggle-tiled-left=['<Super>Left']
    toggle-tiled-right=['<Super>Right']

    [org.gnome.desktop.input-sources]
    sources=[('xkb', 'it')]

    [org.gnome.nautilus.icon-view]
    default-zoom-level='small'
  '';
  services.desktopManager.gnome.extraGSettingsOverridePackages = [
    pkgs.nautilus
    pkgs.gnome-settings-daemon
  ];

  # Disable GNOME initial setup and welcome
  services.gnome.gnome-initial-setup.enable = false;

  # Disable system sleep/idle actions
  services.logind.settings.Login = {
    HandleLidSwitch = "ignore";
    HandleLidSwitchExternalPower = "ignore";
    HandleLidSwitchDocked = "ignore";
  };

  # Ensure pc99 never enters sleep/suspend/hibernate.
  systemd.sleep.extraConfig = lib.mkIf isMaster ''
    AllowSuspend=no
    AllowHibernation=no
    AllowHybridSleep=no
    AllowSuspendThenHibernate=no
  '';
  systemd.targets.sleep.enable = lib.mkIf isMaster false;
  systemd.targets.suspend.enable = lib.mkIf isMaster false;
  systemd.targets.hibernate.enable = lib.mkIf isMaster false;
  systemd.targets.hybrid-sleep.enable = lib.mkIf isMaster false;

  # VirtualBox guest additions (harmless on bare metal: the kernel module
  # simply fails to load and the service does not start).
  virtualisation.virtualbox.guest.enable = lib.mkDefault true;

  # Keyboard layout
  services.xserver.xkb.layout = "it";
  console.keyMap = "it2";

  services.printing.enable = true;

  # Sound (PipeWire)
  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    alsa.support32Bit = true;
    pulse.enable = true;
  };

  programs.firefox.enable = true;

  # Allow unfree packages
  nixpkgs.config.allowUnfree = true;

  # Fonts.
  fonts.packages = [
    pkgs.nerd-fonts.jetbrains-mono
    pkgs.liberation_ttf
  ];
  fonts.fontconfig.defaultFonts = {
    monospace = [ "JetBrainsMono Nerd Font" ];
    sansSerif = [ "Liberation Sans" ];
    serif = [ "Liberation Serif" ];
  };

  # Docker.
  virtualisation.docker.enable = true;

  # User directories.
  systemd.user.services.xdg-user-dirs = {
    description = "Create XDG user directories";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.xdg-user-dirs}/bin/xdg-user-dirs-update";
    };
    wantedBy = [ "default.target" ];
  };

  # Ghostty defaults for all users (Ristretto color theme).
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

  environment.etc."chromium/policies/managed/homepage.json".text = ''
    {
      "HomepageLocation": "https://www.itismeucci.edu.it",
      "HomepageIsNewTabPage": false,
      "RestoreOnStartup": 4,
      "RestoreOnStartupURLs": ["https://www.itismeucci.edu.it"]
    }
  '';

  programs.neovim.enable = true;

  programs.git = {
    enable = true;
    config = {
      credential.helper = "";
      core.askPass = "";
    };
  };

  # Shell tooling and prompt.
  programs.starship.enable = true;
  # Starship prompt (Ristretto palette: warm oranges, soft pinks, muted tones)
  programs.starship.settings = {
    add_newline = false;
    command_timeout = 200;
    format = "$hostname$directory$git_branch$git_status$character";
    character = {
      error_symbol = "[✗](bold #fd6883)";
      success_symbol = "[❯](bold #f38d70)";
    };
    hostname = {
      ssh_only = false;
      format = "[$hostname](bold #f9cc6c):";
    };
    directory = {
      truncation_length = 2;
      truncation_symbol = "…/";
      style = "#e6d9db";
      repo_root_style = "bold #f38d70";
      repo_root_format = "[$repo_root]($repo_root_style)[$path]($style)[$read_only]($read_only_style) ";
    };
    git_branch = {
      format = "[$branch]($style) ";
      style = "italic #adda78";
    };
    git_status = {
      format = "[$all_status]($style)";
      style = "#85dacc";
      ahead = "⇡\${count} ";
      diverged = "⇕⇡\${ahead_count}⇣\${behind_count} ";
      behind = "⇣\${count} ";
      conflicted = " ";
      up_to_date = " ";
      untracked = "? ";
      modified = " ";
      stashed = "";
      staged = "";
      renamed = "";
      deleted = "";
    };
  };
  programs.bash.completion.enable = true;
  programs.bash.shellAliases = {
    ls = "eza --icons --group-directories-first";
    lsa = "eza --icons -a --group-directories-first";
    lt = "eza --icons -T -L 2 --group-directories-first";
    lta = "eza --icons -a -T -L 2 --group-directories-first";
    ff = "fzf";
  };
  programs.bash.interactiveShellInit = ''
    source ${pkgs.fzf}/share/fzf/key-bindings.bash
    source ${pkgs.fzf}/share/fzf/completion.bash
    source ${pkgs.bash-completion}/share/bash-completion/bash_completion
    source ${pkgs.git}/share/bash-completion/completions/git
  '';
  programs.zoxide.enable = true;
  programs.zoxide.enableBashIntegration = true;

  environment.etc."lab/gnome-user-setup.sh" = {
    source = ../scripts/gnome-user-setup.sh;
    mode = "0755";
  };

  # Screensaver scripts and ASCII art logo
  environment.etc."lab/screensaver.txt".source = ../assets/meucci.txt;
  environment.etc."lab/cmd-screensaver.sh" = {
    source = ../scripts/cmd-screensaver.sh;
    mode = "0755";
  };
  environment.etc."lab/launch-screensaver.sh" = {
    source = ../scripts/launch-screensaver.sh;
    mode = "0755";
  };

  # Ristretto wallpapers for random selection at home-reset
  environment.etc."lab/backgrounds/1-ristretto.jpg".source = ../assets/backgrounds/1-ristretto.jpg;
  environment.etc."lab/backgrounds/2-ristretto.jpg".source = ../assets/backgrounds/2-ristretto.jpg;
  environment.etc."lab/backgrounds/3-ristretto.jpg".source = ../assets/backgrounds/3-ristretto.jpg;

  programs.ssh.extraConfig = ''
    Host localhost 127.0.0.1 10.22.9.* pc*
      StrictHostKeyChecking no
      UserKnownHostsFile /dev/null
  '';

  # Exclude GNOME Console (we use Ghostty)
  environment.gnome.excludePackages = [ pkgs.gnome-console ];

  environment.systemPackages = with pkgs; [
    wget
    curl
    bat
    bash-completion
    docker-compose
    dnsmasq
    eza
    fd
    fzf
    ghostty
    git
    chromium
    vscode
    gcc
    tig
    tmux
    imagemagick
    ghostscript
    tectonic
    mermaid-cli
    jq
    lazygit
    unzip
    python3
    python3Packages.pip
    python3Packages.terminaltexteffects
    python3Packages.virtualenv
    luarocks
    lua-language-server
    jdk21
    maven
    nodejs_20
    opencode
    php83
    ripgrep
    try
    xdg-user-dirs
    (makeDesktopItem {
      name = "io.veyon";
      desktopName = "Veyon Master";
      exec = "${pkgs.veyon}/bin/veyon-master";
      icon = "veyon-master";
      comment = "Monitor and control remote computers";
      categories = [ "Qt" "Education" "Network" "RemoteAccess" ];
    })
    gnomeExtensions.desktop-icons-ng-ding
    gnomeExtensions.dash-to-dock
    yaru-theme
  ];

  # Screensaver monitor script
  environment.etc."lab/screensaver-monitor.sh" = {
    source = ../scripts/screensaver-monitor.sh;
    mode = "0755";
  };

  # Screensaver: watch for GNOME idle (screensaver ActiveChanged signal)
  # and launch the TTE screensaver in a fullscreen Ghostty window.
  systemd.user.services.lab-screensaver = {
    description = "ITIS Meucci TTE screensaver";
    wantedBy = [ "graphical-session.target" ];
    after = [ "graphical-session.target" ];
    serviceConfig = {
      Type = "simple";
      ExecStart = "/etc/lab/screensaver-monitor.sh";
      Restart = "on-failure";
      RestartSec = 5;
    };
    path = [ pkgs.bash pkgs.glib pkgs.gnugrep pkgs.procps pkgs.ghostty pkgs.python3Packages.terminaltexteffects pkgs.ncurses pkgs.systemd ];
  };

  systemd.user.services.lab-gnome-setup = {
    description = "Lab GNOME favorites and welcome setup";
    wantedBy = [ "graphical-session.target" ];
    after = [ "graphical-session.target" ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.bash}/bin/bash /etc/lab/gnome-user-setup.sh";
    };
    path = [ pkgs.glib pkgs.gsettings-desktop-schemas pkgs.python3 ];
  };

  services.openssh = {
    enable = true;
    settings.PasswordAuthentication = false;
  };

  environment.sessionVariables = {
    EDITOR = "gnome-text-editor";
    VISUAL = "gnome-text-editor";
    GIT_ASKPASS = "";
    SSH_ASKPASS_REQUIRE = "never";
  };

  environment.variables.SSH_ASKPASS = lib.mkForce "";

  system.stateVersion = "25.11";

}
