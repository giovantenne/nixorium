{ lib, pkgs }:
{ resolution, labSettings }:
let
  preferences = import ./build-workspace-seed.nix { inherit lib pkgs; } resolution;
  gitConfig = pkgs.writeText "student-gitconfig" (lib.generators.toGitINI {
    user.name = labSettings.studentGitName;
    user.email = labSettings.studentGitEmail;
  });
  bashrc = pkgs.writeText "student-bashrc" ''
    # Source global definitions.
    if [ -f /etc/bashrc ]; then
      . /etc/bashrc
    fi
  '';
  profile = pkgs.writeText "student-profile" ''
    # Source the account's interactive shell defaults.
    if [ -f "$HOME/.bashrc" ]; then
      . "$HOME/.bashrc"
    fi
  '';
  # Stable XDG names are part of the neutral scaffold, not captured user data.
  directories = {
    DESKTOP = "Desktop";
    DOWNLOAD = "Downloads";
    TEMPLATES = "Templates";
    PUBLICSHARE = "Public";
    DOCUMENTS = "Documents";
    MUSIC = "Music";
    PICTURES = "Pictures";
    VIDEOS = "Videos";
  };
  userDirectories = pkgs.writeText "student-user-dirs.dirs"
    (lib.concatStringsSep "\n" (lib.mapAttrsToList (key: name:
      ''XDG_${key}_DIR="$HOME/${name}"''
    ) directories) + "\n");
in
pkgs.runCommand "nixorium-workspace-home" {} ''
  cp -a ${preferences}/. "$out"
  find "$out" -type d -exec chmod u+w {} +
  install -m 0644 ${bashrc} "$out/home/.bashrc"
  install -m 0644 ${profile} "$out/home/.profile"
  install -m 0644 ${gitConfig} "$out/home/.gitconfig"
  install -D -m 0644 ${userDirectories} "$out/home/.config/user-dirs.dirs"
  mkdir -p "$out/home/.local/share" "$out/home/.local/npm"
  ${lib.concatMapStringsSep "\n" (name: ''mkdir -p "$out/home/${name}"'') (builtins.attrValues directories)}
''
