{ pkgs, ... }:
{
  # Run generic Linux binaries that are downloaded outside Nix, such as the JRE
  # a Marketplace VS Code extension bundles or an Electron application.
  # nix-ld provides the dynamic loader and these libraries, found through
  # NIX_LD_LIBRARY_PATH; without it NixOS stops such a program with "Could not
  # start dynamically linked executable". Keep this list to free, common
  # libraries: the first group is the headless set, the rest is what GUI
  # programs usually need. Most are already part of the desktop closure.
  programs.nix-ld.enable = true;
  programs.nix-ld.libraries = [
    pkgs.zlib
    pkgs.stdenv.cc.cc.lib
    pkgs.glib
    pkgs.openssl
    pkgs.libxml2
    pkgs.icu
    pkgs.curl
    pkgs.fontconfig
    pkgs.freetype
    pkgs.libx11
    pkgs.libxext
    pkgs.libxrender
    pkgs.libxtst
    pkgs.libxi
    pkgs.libxcb
    pkgs.gtk3
    pkgs.alsa-lib
    pkgs.nss
    pkgs.nspr
    pkgs.dbus
    pkgs.expat
    pkgs.libdrm
    pkgs.libgbm
    pkgs.libGL
    pkgs.libxkbcommon
    pkgs.pango
    pkgs.cairo
    pkgs.at-spi2-core
    pkgs.cups
  ];
}
