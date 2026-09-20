{ pkgs, lib, labSettings, labAssets, hostSoftwarePackages, ... }:
let
  has = package: builtins.elem package hostSoftwarePackages;
  studentTemplate = "/var/lib/home-template/${labSettings.studentUser}";
  adminHome = "/home/admin";
  teacherHome = "/home/${labSettings.teacherUser}";
  staffHomes = [
    { user = "admin"; home = adminHome; }
    { user = labSettings.teacherUser; home = teacherHome; }
  ];
  installForStaff = path: source: builtins.concatStringsSep "\n" (map (profile:
    ''install -D -o ${profile.user} -g users -m 0644 ${source} "${profile.home}/${path}"''
  ) staffHomes);
  removeForStaff = path: builtins.concatStringsSep "\n" (map (profile:
    ''rm -f "${profile.home}/${path}"''
  ) staffHomes);
  prepareStaffDirectories = builtins.concatStringsSep "\n" (map (profile: ''
    install -d -o ${profile.user} -g users -m 0700 "${profile.home}/.config"
    install -d -o ${profile.user} -g users -m 0755 "${profile.home}/.config/Code/User"
    install -d -o ${profile.user} -g users -m 0755 "${profile.home}/.vscode/extensions"
  '') staffHomes);
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
        ${prepareStaffDirectories}
        install -D -m 0644 ${labAssets.vscodeSettings} "${studentTemplate}/.config/Code/User/settings.json"
        ${installForStaff ".config/Code/User/settings.json" labAssets.vscodeSettings}
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
        ${installForStaff ".vscode/argv.json" vscodeArgv}
      ''}
      ${lib.optionalString (!has "vscode") ''
        ${removeForStaff ".config/Code/User/settings.json"}
        ${removeForStaff ".vscode/argv.json"}
      ''}
      ${lib.optionalString (has "nodejs") ''
        install -d -m 0755 "${studentTemplate}/.local/npm"
        ${builtins.concatStringsSep "\n        " (map (profile:
          ''install -d -o ${profile.user} -g users -m 0755 "${profile.home}/.local/npm"''
        ) staffHomes)}
      ''}

      chown -R ${labSettings.studentUser}:users "${studentTemplate}"
    '';
  };

  system.activationScripts.nixoriumUserHomeOwnership.deps = lib.mkAfter [ "siteHomeProfile" ];
}
