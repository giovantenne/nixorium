{ pkgs }:
let
  profile = import ../templates/site/modules/workstation.nix {
    inherit pkgs;
    inherit (pkgs) lib;
    labSettings = {
      studentUser = "student";
      teacherUser = "teacher";
      homepageUrl = "https://example.invalid";
    };
    hostSoftwarePackages = [];
  };
  cfg = profile.services.desktopManager.gnome;
  schemas = pkgs.gnome.nixos-gsettings-overrides.override {
    inherit (cfg) extraGSettingsOverrides extraGSettingsOverridePackages;
  };
  loginScript = pkgs.writeText "desktop-login.sh" profile.environment.etc."lab/gnome-user-setup.sh".text;
in
pkgs.runCommand "nixorium-desktop-profile-check" {
  nativeBuildInputs = [ pkgs.glib pkgs.jq pkgs.bash ];
} ''
  export GSETTINGS_BACKEND=memory
  export GSETTINGS_SCHEMA_DIR=${schemas}/share/gsettings-schemas/nixos-gsettings-overrides/glib-2.0/schemas
  test "$(gsettings get org.gnome.desktop.interface icon-theme)" = "'MoreWaita'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-position)" = "'BOTTOM'"
  test "$(gsettings get org.gnome.shell.extensions.dash-to-dock dock-fixed)" = true
  test "$(gsettings get org.gnome.shell.extensions.tiling-assistant window-gap)" = 8
  ${pkgs.lib.concatMapStringsSep "\n" (extension: ''
    jq -e --arg version '${pkgs.lib.versions.major pkgs.gnome-shell.version}' \
      '."shell-version" | index($version) != null' \
      ${extension}/share/gnome-shell/extensions/${extension.extensionUuid}/metadata.json > /dev/null
  '') [ pkgs.gnomeExtensions.dash-to-dock pkgs.gnomeExtensions.desktop-icons-ng-ding pkgs.gnomeExtensions.tiling-assistant ]}
  bash -n ${loginScript}
  touch "$out"
''
