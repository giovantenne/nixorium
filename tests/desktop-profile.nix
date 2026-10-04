{ pkgs }:
let
  profileArgs = {
    inherit pkgs;
    inherit (pkgs) lib;
    labSettings = {
      studentUser = "student";
      teacherUser = "teacher";
      homepageUrl = "https://example.invalid";
    };
    hostSoftwarePackages = [];
  };
  profile = import ../templates/site/modules/workstation.nix profileArgs;
  managedProfile = import ../templates/site/modules/workstation.nix (profileArgs // { workspaceRuntimeEnabled = true; });
  roleProfile = hostName: import ../templates/site/modules/workstation.nix (profileArgs // {
    inherit hostName;
    labSettings = profileArgs.labSettings // { masterHostName = "pc99"; };
  });
  controllerScript = (roleProfile "pc99").environment.etc."lab/gnome-user-setup.sh".text;
  clientScript = (roleProfile "pc01").environment.etc."lab/gnome-user-setup.sh".text;
  cfg = profile.services.desktopManager.gnome;
  schemas = pkgs.gnome.nixos-gsettings-overrides.override {
    inherit (cfg) extraGSettingsOverrides extraGSettingsOverridePackages;
  };
  loginScript = pkgs.writeText "desktop-login.sh" profile.environment.etc."lab/gnome-user-setup.sh".text;
  managedScript = pkgs.writeText "managed-desktop-login.sh" managedProfile.environment.etc."lab/gnome-user-setup.sh".text;
in
assert pkgs.lib.hasInfix "APPEARANCE_ROLE=managed-student" managedProfile.environment.etc."lab/gnome-user-setup.sh".text;
# Staff find the Nixorium launcher first in the controller's dock only.
assert pkgs.lib.hasInfix "FAVORITES='['\\''nixorium.desktop'" controllerScript;
assert !(pkgs.lib.hasInfix "nixorium.desktop" clientScript);
pkgs.runCommand "nixorium-desktop-profile-check" {
  nativeBuildInputs = [ pkgs.glib pkgs.jq pkgs.bash ];
} ''
  export GSETTINGS_BACKEND=keyfile
  export XDG_CONFIG_HOME="$TMPDIR/config"
  mkdir -p "$XDG_CONFIG_HOME"
  export GSETTINGS_SCHEMA_DIR=${schemas}/share/gsettings-schemas/nixos-gsettings-overrides/glib-2.0/schemas
  test "$(gsettings get org.gnome.desktop.interface icon-theme)" = "'Yaru-yellow'"
  test -e ${pkgs.yaru-theme}/share/icons/Yaru-yellow/index.theme
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-position)" = "'BOTTOM'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-fixed)" = false
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock autohide)" = true
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock intellihide)" = true
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock intellihide-mode)" = "'ALL_WINDOWS'"
  test "$(gsettings get org.gnome.shell.extensions.tiling-assistant window-gap)" = 8
  source ${loginScript}
  RANDOM_BACKGROUND="file:///etc/lab/backgrounds/random.jpg"
  gsettings set org.gnome.desktop.background picture-uri "'$RANDOM_BACKGROUND'"
  gsettings set org.gnome.desktop.background picture-uri-dark "'$RANDOM_BACKGROUND'"
  apply_student_appearance
  test "$(gsettings get org.gnome.desktop.background picture-uri)" = "'$RANDOM_BACKGROUND'"
  test "$(gsettings get org.gnome.desktop.background picture-uri-dark)" = "'$RANDOM_BACKGROUND'"
  apply_staff_appearance
  test "$(gsettings get org.gnome.desktop.background picture-uri)" != "'$RANDOM_BACKGROUND'"
  test "$(gsettings get org.gnome.desktop.background picture-uri-dark)" != "'$RANDOM_BACKGROUND'"
  source ${managedScript}
  gsettings set org.gnome.shell favorite-apps "['code.desktop']"
  gsettings set org.gnome.desktop.interface color-scheme "'prefer-light'"
  gsettings set org.gnome.shell.extensions.dash-to-dock dock-position "'LEFT'"
  gsettings set org.gnome.shell.extensions.dash-to-dock autohide false
  gsettings set org.gnome.shell.extensions.ding show-home true
  apply_session_defaults managed-student "[]"
  test "$(gsettings get org.gnome.shell favorite-apps)" = "['code.desktop']"
  test "$(gsettings get org.gnome.desktop.interface color-scheme)" = "'prefer-light'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-position)" = "'LEFT'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock autohide)" = false
  test ! -e "$XDG_CONFIG_HOME/nixorium/desktop-style-v1"
  test "$(gsettings get org.gnome.shell.extensions.ding show-home)" = true
  # The same opt-in script must keep the ordinary staff first-login defaults.
  apply_session_defaults staff "['org.gnome.TextEditor.desktop']"
  test "$(gsettings get org.gnome.shell favorite-apps)" = "['org.gnome.TextEditor.desktop']"
  test "$(gsettings get org.gnome.desktop.interface color-scheme)" = "'prefer-dark'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-position)" = "'BOTTOM'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock autohide)" = true
  test -e "$XDG_CONFIG_HOME/nixorium/desktop-style-v1"
  # Staff desktops show only their files; the trash is in the dock, once.
  test "$(gsettings get org.gnome.shell.extensions.ding show-home)" = false
  test "$(gsettings get org.gnome.shell.extensions.ding show-trash)" = false
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock show-trash)" = true
  gsettings set org.gnome.shell.extensions.ding show-home true
  apply_session_defaults staff "['org.gnome.TextEditor.desktop']"
  test "$(gsettings get org.gnome.shell.extensions.ding show-home)" = true
  # Staff keep their own dock: their changes stay, and only applications
  # the laboratory adds later are appended.
  gsettings set org.gnome.shell favorite-apps "['mine.desktop']"
  apply_session_defaults staff "['org.gnome.TextEditor.desktop']"
  test "$(gsettings get org.gnome.shell favorite-apps)" = "['mine.desktop']"
  apply_session_defaults staff "['org.gnome.TextEditor.desktop', 'nixorium.desktop']"
  test "$(gsettings get org.gnome.shell favorite-apps)" = "['mine.desktop', 'nixorium.desktop']"
  gsettings set org.gnome.shell favorite-apps "@as []"
  apply_session_defaults staff "['org.gnome.TextEditor.desktop', 'nixorium.desktop']"
  test "$(gsettings get org.gnome.shell favorite-apps)" = "@as []"
  # Students still get the laboratory's dock at every login.
  apply_session_defaults student "['org.gnome.TextEditor.desktop']"
  test "$(gsettings get org.gnome.shell favorite-apps)" = "['org.gnome.TextEditor.desktop']"
  ${pkgs.lib.concatMapStringsSep "\n" (extension: ''
    jq -e --arg version '${pkgs.lib.versions.major pkgs.gnome-shell.version}' \
      '."shell-version" | index($version) != null' \
      ${extension}/share/gnome-shell/extensions/${extension.extensionUuid}/metadata.json > /dev/null
  '') [ pkgs.gnomeExtensions.dash-to-dock pkgs.gnomeExtensions.desktop-icons-ng-ding pkgs.gnomeExtensions.tiling-assistant ]}
  bash -n ${loginScript}
  bash -n ${managedScript}
  touch "$out"
''
