{ pkgs, usePinnedExtension ? false }:
let
  inherit (pkgs) lib;
  extensionID = "ritwickdey.liveserver";
  fixtureExtension = manifest: pkgs.runCommand "workspace-extension-fixture" {
    version = "1.0.0";
    passthru.vscodeExtUniqueId = extensionID;
  } ''
    mkdir -p "$out/share/vscode/extensions/${extensionID}"
    cp ${pkgs.writeText "package.json" (builtins.toJSON manifest)} \
      "$out/share/vscode/extensions/${extensionID}/package.json"
  '';
  fixtureManifest = { publisher = "ritwickdey"; name = "liveserver"; version = "1.0.0"; };
  extension = if usePinnedExtension then pkgs.vscode-extensions.ritwickdey.liveserver
    else fixtureExtension fixtureManifest;
  # The synthetic editor keeps this artifact check independent of an editor
  # build. Neither fixture claims to test extension activation in VS Code.
  seedPackages = pkgs // {
    vscode = pkgs.hello;
    vscode-extensions = { ritwickdey.liveserver = extension; };
  };
  resolve = import ../lib/resolve-workspace-profile.nix { inherit lib; pkgs = seedPackages; };
  build = import ../lib/build-workspace-seed.nix { inherit lib; pkgs = seedPackages; };
  prepare = profile: resolve {
    profileJSON = builtins.toJSON ({ schemaVersion = 1; } // profile);
    catalog = {
      schemaVersion = 1;
      baseline = { schemaVersion = 1; };
      applications = [
        { id = "code.desktop"; package = "vscode"; }
        { id = "org.gnome.TextEditor.desktop"; package = "gnome-text-editor"; }
        { id = "chromium-browser.desktop"; package = "chromium"; browser = true; }
      ];
      extensions = [ { id = extensionID; package = "vscode-extensions.${extensionID}"; } ];
    };
    controllerName = "pc99";
    clientNames = [];
    hostPackages.pc99 = [ "vscode" "gnome-shell" "gnome-text-editor" "chromium" "gnomeExtensions.dash-to-dock" ];
  };
  resolved = prepare {
    desktop = {
      favorites = [ "org.gnome.TextEditor.desktop" "code.desktop" ];
      colorScheme = "dark";
      enableAnimations = false;
      dock = {
        position = "left";
        iconSize = 48;
        autoHide = false;
        extendHeight = true;
        showTrash = false;
        showMounts = false;
      };
    };
    browser.defaultApplication = "chromium-browser.desktop";
    vscode = {
      extensions = [ extensionID ];
      settings = {
        "editor.fontSize" = 15;
        "editor.tabSize" = 4;
        "editor.insertSpaces" = true;
        "editor.wordWrap" = "bounded";
        "editor.formatOnSave" = false;
        "editor.minimap.enabled" = false;
        "files.autoSave" = "onFocusChange";
      };
    };
  };
  seed = build resolved;
  home = import ../lib/build-workspace-home.nix { inherit lib; pkgs = seedPackages; } {
    resolution = resolved;
    labSettings = { studentGitName = "Student"; studentGitEmail = "student@example.invalid"; };
  };
  lightSeed = build (prepare {
    desktop = { favorites = []; colorScheme = "light"; dock.autoHide = true; };
    vscode.extensions = [];
  });
  emptySeed = build (prepare {});
  schemas = pkgs.runCommand "nixorium-workspace-test-schemas" {
    nativeBuildInputs = [ pkgs.glib ];
  } ''
    mkdir -p "$out"
    cp ${pkgs.glib.getSchemaPath pkgs.gnome-shell}/*.xml "$out/"
    cp ${pkgs.glib.getSchemaPath pkgs.gsettings-desktop-schemas}/*.xml "$out/"
    cp ${pkgs.gnomeExtensions.dash-to-dock}/share/gnome-shell/extensions/${pkgs.gnomeExtensions.dash-to-dock.extensionUuid}/schemas/*.xml "$out/"
    chmod -R u+w "$out"
    glib-compile-schemas --strict "$out"
  '';
  wallpaper = pkgs.writeText "workspace-test-wallpaper.ini" ''
    [org/gnome/desktop/background]
    picture-uri='file:///etc/lab/backgrounds/fixture.jpg'
    picture-uri-dark='file:///etc/lab/backgrounds/fixture.jpg'
  '';
  # Run a deliberately invalid payload through the same build command in a
  # disposable output. Depending on a failing derivation would fail the check
  # before it could assert the expected rejection.
  invalidBuilder = manifest:
    let
      badPkgs = seedPackages // { vscode-extensions.ritwickdey.liveserver = fixtureExtension manifest; };
      badSeed = import ../lib/build-workspace-seed.nix { inherit lib; pkgs = badPkgs; }
        (resolved // { extensions = map (entry: entry // { version = "1.0.0"; }) resolved.extensions; });
    in pkgs.writeText "invalid-workspace-seed-builder.sh" badSeed.buildCommand;
  mismatchedProfile = builtins.tryEval ((build (resolved // { extensions = []; })).drvPath);
in
assert !mismatchedProfile.success;
pkgs.runCommand "nixorium-workspace-seed${lib.optionalString usePinnedExtension "-pinned"}-check" {
  nativeBuildInputs = [ pkgs.glib pkgs.dconf pkgs.dbus pkgs.jq ];
} ''
  test -f ${home}/home/.bashrc
  test -f ${home}/home/.profile
  test -f ${home}/home/.gitconfig
  test -f ${home}/home/.config/user-dirs.dirs
  for DIRECTORY in Desktop Downloads Templates Public Documents Music Pictures Videos .local/share .local/npm; do
    test -d "${home}/home/$DIRECTORY"
  done
  test ! -e ${home}/home/.ssh
  test ! -e ${home}/home/.pi
  cmp ${seed}/manifest.json ${home}/manifest.json
  cmp ${seed}/home/.config/Code/User/settings.json ${home}/home/.config/Code/User/settings.json
  test -L ${home}/home/.vscode/extensions/${extensionID}
  export XDG_CONFIG_HOME="$TMPDIR/student-config"
  export XDG_CACHE_HOME="$TMPDIR/student-cache"
  export GSETTINGS_BACKEND=dconf
  export GIO_EXTRA_MODULES=${lib.getLib pkgs.dconf}/lib/gio/modules
  export XDG_DATA_DIRS=${pkgs.dconf}/share
  export GSETTINGS_SCHEMA_DIR=${schemas}
  export DCONF_PROFILE="$TMPDIR/dconf-profile"
  echo user-db:user > "$DCONF_PROFILE"
  mkdir -p "$XDG_CONFIG_HOME"
  cp -R ${seed}/home/.config/. "$XDG_CONFIG_HOME/"
  chmod -R u+w "$XDG_CONFIG_HOME"

  test "$(gsettings get org.gnome.shell favorite-apps)" = "['org.gnome.TextEditor.desktop', 'code.desktop']"
  test "$(gsettings get org.gnome.desktop.interface color-scheme)" = "'prefer-dark'"
  test "$(gsettings get org.gnome.desktop.interface enable-animations)" = false
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-position)" = "'LEFT'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dash-max-icon-size)" = 48
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-fixed)" = true
  for KEY in autohide intellihide show-trash show-mounts; do
    test "$(gsettings get org.gnome.shell.extensions.dash-to-dock "$KEY")" = false
  done
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock extend-height)" = true
  jq -e '.profile.schemaVersion == 1 and (.extensions | length) == 1' ${seed}/manifest.json
  jq -e '."editor.fontSize" == 15 and ."editor.tabSize" == 4 and ."editor.insertSpaces" == true
    and ."editor.wordWrap" == "bounded" and ."editor.formatOnSave" == false
    and ."editor.minimap.enabled" == false and ."files.autoSave" == "onFocusChange"
    and ."extensions.autoUpdate" == false and ."extensions.autoCheckUpdates" == false
    and ."update.mode" == "none" and length == 10' ${seed}/home/.config/Code/User/settings.json
  test "$(readlink ${seed}/home/.vscode/extensions/${extensionID})" = ${extension}/share/vscode/extensions/${extensionID}
  test "$(jq -r '.extensions[0].version' ${seed}/manifest.json)" = ${lib.escapeShellArg extension.version}
  for MIME in text/html x-scheme-handler/http x-scheme-handler/https; do
    grep -Fx "$MIME=chromium-browser.desktop;" ${seed}/home/.config/mimeapps.list
  done

  # Copied preferences are editable. Changing a disposable session leaves the
  # immutable seed untouched; no login/reset mechanism is exercised here.
  dbus-run-session --config-file=${pkgs.dbus}/share/dbus-1/session.conf \
    -- gsettings set org.gnome.desktop.interface color-scheme prefer-light
  test "$(gsettings get org.gnome.desktop.interface color-scheme)" = "'prefer-light'"
  export XDG_CONFIG_HOME=${seed}/home/.config
  test "$(gsettings get org.gnome.desktop.interface color-scheme)" = "'prefer-dark'"
  export XDG_CONFIG_HOME=${lightSeed}/home/.config
  test "$(gsettings get org.gnome.shell favorite-apps)" = '@as []'
  test "$(gsettings get org.gnome.desktop.interface color-scheme)" = "'prefer-light'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-fixed)" = false
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock autohide)" = true
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock intellihide)" = true
  test -z "$(find ${lightSeed}/home/.vscode/extensions -mindepth 1 -print -quit)"
  test ! -e ${lightSeed}/home/.config/mimeapps.list
  test -z "$(find ${emptySeed}/home -mindepth 1 -print -quit)"
  test ! -s ${emptySeed}/dconf/00-workspace
  test -z "$(find ${seed}/home -type f -perm /111 -print -quit)"

  # Compile wallpaper and workspace in one database rather than overwriting
  # the copied preferences with a wallpaper-only database.
  mkdir -p "$TMPDIR/composed/dconf" "$TMPDIR/keyfiles"
  cp ${seed}/dconf/00-workspace "$TMPDIR/keyfiles/00-workspace"
  cp ${wallpaper} "$TMPDIR/keyfiles/99-wallpaper"
  dconf compile "$TMPDIR/composed/dconf/user" "$TMPDIR/keyfiles"
  export XDG_CONFIG_HOME="$TMPDIR/composed"
  test "$(gsettings get org.gnome.desktop.interface color-scheme)" = "'prefer-dark'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-position)" = "'LEFT'"
  test "$(gsettings get org.gnome.desktop.background picture-uri)" = "'file:///etc/lab/backgrounds/fixture.jpg'"

  for BUILDER in ${invalidBuilder (fixtureManifest // { publisher = "unexpected"; })} \
    ${invalidBuilder (fixtureManifest // { version = "9.9.9"; })}; do
    INVALID_OUTPUT=$(mktemp -d)
    if env out="$INVALID_OUTPUT" ${pkgs.bash}/bin/bash -e "$BUILDER"; then
      echo 'Invalid extension payload was accepted' >&2
      exit 1
    fi
    test ! -e "$INVALID_OUTPUT/home/.vscode/extensions/${extensionID}"
  done
  touch "$out"
''
