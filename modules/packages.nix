{ pkgs, ... }:
{
  environment.systemPackages = with pkgs; [
    wget
    curl
    openssl
    bat
    bash-completion
    docker-compose
    dnsmasq
    eza
    fd
    fzf
    ghostty
    git
    gh
    chromium
    vscode
    gcc
    tig
    tmux
    tree-sitter
    imagemagick
    ghostscript
    hunspell
    hunspellDicts.it_IT
    hunspellDicts.en_US
    libreoffice-qt
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
    nodejs
    opencode
    pi-coding-agent
    php
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
}
