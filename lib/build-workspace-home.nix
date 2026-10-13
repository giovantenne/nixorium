{ lib, pkgs }:
{ resolution, labSettings, homeFilesProfile ? { schemaVersion = 1; }, homeFilesRoot ? null }:
let
  preferences = import ./build-workspace-seed.nix { inherit lib pkgs; } resolution;
  additional = import ./build-home-files.nix { inherit lib pkgs; } {
    profile = homeFilesProfile;
    root = homeFilesRoot;
  };
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
pkgs.runCommand "nixorium-workspace-home" { nativeBuildInputs = [ pkgs.jq ]; } ''
  cp -a ${preferences}/. "$out"
  find "$out" -type d -exec chmod u+w {} +
  install -m 0644 ${bashrc} "$out/home/.bashrc"
  install -m 0644 ${profile} "$out/home/.profile"
  install -m 0644 ${gitConfig} "$out/home/.gitconfig"
  install -D -m 0644 ${userDirectories} "$out/home/.config/user-dirs.dirs"
  mkdir -p "$out/home/.local/share" "$out/home/.local/npm"
  ${lib.concatMapStringsSep "\n" (name: ''mkdir -p "$out/home/${name}"'') (builtins.attrValues directories)}
  # Schema validation reserves generated paths. Refuse a collision as a final
  # build-time guard, including downstream changes to the scaffold.
  while IFS= read -r -d "" file; do
    relative="''${file#${additional}/home/}"
    if [ -e "$out/home/$relative" ] || [ -L "$out/home/$relative" ]; then
      echo "home files: destination collides with the generated seed" >&2
      exit 1
    fi
  done < <(find ${additional}/home -type f -print0)
  cp -r --no-preserve=mode,ownership ${additional}/home/. "$out/home/"
  ${lib.concatMapStringsSep "\n" (entry: ''
    chmod ${if entry.executable or false then "0700" else "0600"} \
      "$out/home/"${lib.escapeShellArg entry.destination}
  '') (homeFilesProfile.files or [])}
  ${lib.optionalString ((homeFilesProfile.files or []) != []) ''
    jq --slurpfile files ${additional}/manifest.json '. + {homeFiles:$files[0]}' \
      "$out/manifest.json" > manifest.json
    install -m 0600 manifest.json "$out/manifest.json"
  ''}
''
