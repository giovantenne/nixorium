{ pkgs, lib, labSettings, labAssets, hostSoftwarePackages, ... }:
let
  has = package: builtins.elem package hostSoftwarePackages;
  studentTemplate = "/var/lib/home-template/${labSettings.studentUser}";
  adminHome = "/home/admin";
  vscodeArgv = pkgs.writeText "nixorium-site-vscode-argv.json" (builtins.toJSON {
    password-store = "basic";
    enable-crash-reporter = false;
  });
  vscodeExtensions = [
    { pkg = pkgs.vscode-extensions.ritwickdey.liveserver; dir = "ritwickdey.liveserver"; }
    { pkg = pkgs.vscode-extensions.vscjava.vscode-java-pack; dir = "vscjava.vscode-java-pack"; }
    { pkg = pkgs.vscode-extensions.redhat.java; dir = "redhat.java"; }
    { pkg = pkgs.vscode-extensions.vscjava.vscode-java-debug; dir = "vscjava.vscode-java-debug"; }
    { pkg = pkgs.vscode-extensions.vscjava.vscode-java-test; dir = "vscjava.vscode-java-test"; }
    { pkg = pkgs.vscode-extensions.vscjava.vscode-maven; dir = "vscjava.vscode-maven"; }
    { pkg = pkgs.vscode-extensions.vscjava.vscode-java-dependency; dir = "vscjava.vscode-java-dependency"; }
  ];
in
{
  system.activationScripts.siteHomeProfile = {
    deps = [ "createHomeTemplates" ];
    text = ''
      ${lib.optionalString (has "chromium") ''
        install -D -m 0644 ${labAssets.mimeApps} "${studentTemplate}/.config/mimeapps.list"
        install -D -o admin -g users -m 0644 ${labAssets.mimeApps} "${adminHome}/.config/mimeapps.list"
      ''}
      ${lib.optionalString (!has "chromium") ''
        rm -f "${adminHome}/.config/mimeapps.list"
      ''}

      ${lib.optionalString (has "vscode") ''
        install -D -m 0644 ${labAssets.vscodeSettings} "${studentTemplate}/.config/Code/User/settings.json"
        install -D -o admin -g users -m 0644 ${labAssets.vscodeSettings} "${adminHome}/.config/Code/User/settings.json"
        install -d -m 0755 "${studentTemplate}/.vscode/extensions"
        ${builtins.concatStringsSep "\n        " (map (extension:
          ''cp -a "${extension.pkg}/share/vscode/extensions/${extension.dir}" "${studentTemplate}/.vscode/extensions/"''
        ) vscodeExtensions)}
        chmod -R u+w "${studentTemplate}/.vscode/extensions"
        ${pkgs.jq}/bin/jq 'del(.announcement)' \
          "${studentTemplate}/.vscode/extensions/ritwickdey.liveserver/package.json" \
          > "${studentTemplate}/.vscode/extensions/ritwickdey.liveserver/package.json.tmp"
        mv "${studentTemplate}/.vscode/extensions/ritwickdey.liveserver/package.json.tmp" \
          "${studentTemplate}/.vscode/extensions/ritwickdey.liveserver/package.json"
        install -D -m 0644 ${vscodeArgv} "${studentTemplate}/.vscode/argv.json"
        install -D -o admin -g users -m 0644 "${studentTemplate}/.vscode/argv.json" "${adminHome}/.vscode/argv.json"
      ''}
      ${lib.optionalString (!has "vscode") ''
        rm -f "${adminHome}/.config/Code/User/settings.json" "${adminHome}/.vscode/argv.json"
      ''}
      ${lib.optionalString (has "nodejs") ''
        install -d -m 0755 "${studentTemplate}/.local/npm"
        install -d -o admin -g users -m 0755 "${adminHome}/.local/npm"
      ''}

      chown -R ${labSettings.studentUser}:users "${studentTemplate}"
    '';
  };
}
