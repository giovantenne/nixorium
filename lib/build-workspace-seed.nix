{ lib, pkgs }:
# Build only the supported preference payload from an already resolved profile.
# This does not activate a home or import content from the legacy template.
resolution:
let
  profile = import ./eval-workspace-profile.nix { inherit lib; }
    (builtins.toJSON resolution.effective);
  desktop = profile.desktop or {};
  dock = desktop.dock or {};
  dconfSettings = lib.filterAttrs (_: values: values != {}) {
    "org/gnome/shell" = lib.optionalAttrs (desktop ? favorites) {
      favorite-apps = if desktop.favorites == []
        then lib.gvariant.mkEmptyArray lib.gvariant.type.string
        else desktop.favorites;
    };
    "org/gnome/desktop/interface" =
      lib.optionalAttrs (desktop ? colorScheme) {
        color-scheme = "prefer-${desktop.colorScheme}";
      } // lib.optionalAttrs (desktop ? enableAnimations) {
        enable-animations = desktop.enableAnimations;
      };
    "org/gnome/shell/extensions/dash-to-dock" =
      lib.optionalAttrs (dock ? position) { dock-position = lib.toUpper dock.position; }
      // lib.optionalAttrs (dock ? iconSize) { dash-max-icon-size = lib.gvariant.mkInt32 dock.iconSize; }
      // lib.optionalAttrs (dock ? autoHide) {
        dock-fixed = !dock.autoHide;
        autohide = dock.autoHide;
        intellihide = dock.autoHide;
      }
      // lib.optionalAttrs (dock ? extendHeight) { extend-height = dock.extendHeight; }
      // lib.optionalAttrs (dock ? showTrash) { show-trash = dock.showTrash; }
      // lib.optionalAttrs (dock ? showMounts) { show-mounts = dock.showMounts; };
  };
  dconfSource = pkgs.writeText "nixorium-workspace-dconf.ini"
    (lib.generators.toDconfINI dconfSettings);
  browserSource = pkgs.writeText "nixorium-workspace-mimeapps.list" (lib.generators.toINI {} {
    "Default Applications" = builtins.listToAttrs (map (mime: {
      name = mime;
      value = "${profile.browser.defaultApplication};";
    }) [ "text/html" "x-scheme-handler/http" "x-scheme-handler/https" ]);
  });
  settingsSource = pkgs.writeText "nixorium-workspace-vscode-settings.json" (builtins.toJSON (
    (profile.vscode.settings or {}) // {
      # These are editable session defaults, not global editor policies.
      # Managed extension updates come from the reviewed package-base pin.
      "extensions.autoUpdate" = false;
      "extensions.autoCheckUpdates" = false;
      "update.mode" = "none";
    }
  ));
  packageTools = import ./software-packages.nix { inherit lib pkgs; allowUnfree = true; };
  extensions = map (entry:
    let
      package = packageTools.resolve entry.package;
      info = packageTools.describe entry.package;
    in
    assert entry.package == "vscode-extensions.${entry.id}"
      && info != null && info.availability == "available"
      && info.version == entry.version
      && (package.vscodeExtUniqueId or null) == entry.id
      || throw "workspace seed: extension resolution does not match the package set";
    {
      inherit (entry) id version;
      path = ".vscode/extensions/${entry.id}";
      source = "${package}/share/vscode/extensions/${entry.id}";
    }
  ) resolution.extensions;
  manifest = pkgs.writeText "nixorium-workspace-seed-manifest.json" (builtins.toJSON {
    schemaVersion = 1;
    inherit profile extensions;
  });
in
assert map (entry: entry.id) extensions == (profile.vscode.extensions or [])
  || throw "workspace seed: resolved extensions do not match the effective profile";
pkgs.runCommand "nixorium-workspace-seed" {
  nativeBuildInputs = [ pkgs.dconf pkgs.jq ];
} ''
  mkdir -p "$out/home" "$out/dconf"
  install -m 0444 ${manifest} "$out/manifest.json"
  install -m 0444 ${dconfSource} "$out/dconf/00-workspace"
  ${lib.optionalString (dconfSettings != {}) ''
    mkdir -p "$out/home/.config/dconf"
    dconf compile "$out/home/.config/dconf/user" "$out/dconf"
  ''}
  ${lib.optionalString (profile ? browser.defaultApplication) ''
    install -D -m 0644 ${browserSource} "$out/home/.config/mimeapps.list"
  ''}
  ${lib.optionalString (profile ? vscode) ''
    install -D -m 0644 ${settingsSource} "$out/home/.config/Code/User/settings.json"
    mkdir -p "$out/home/.vscode/extensions"
  ''}
  ${lib.concatMapStringsSep "\n" (entry: ''
    # Inspect the built payload, not just the derivation's claimed identity.
    jq -e --arg id ${lib.escapeShellArg entry.id} --arg version ${lib.escapeShellArg entry.version} \
      '((.publisher + "." + .name) | ascii_downcase) == $id and .version == $version' \
      ${lib.escapeShellArg "${entry.source}/package.json"} > /dev/null
    ln -s ${lib.escapeShellArg entry.source} "$out/home/"${lib.escapeShellArg entry.path}
  '') extensions}
''
