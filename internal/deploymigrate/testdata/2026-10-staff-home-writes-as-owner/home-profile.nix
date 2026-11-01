{ pkgs, lib, labSettings, labAssets, hostSoftwarePackages, ... }:
let
  # Staff homes persist. The student home is restored at every boot from the
  # workspace profile (workspace-profile.json), not from this module.
  has = package: builtins.elem package hostSoftwarePackages;
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
in
{
  system.activationScripts.siteHomeProfile = {
    deps = [ "createHomeTemplates" ];
    text = ''
      ${lib.optionalString (has "chromium") ''
        install -D -o admin -g users -m 0644 ${labAssets.mimeApps} "${adminHome}/.config/mimeapps.list"
      ''}
      ${lib.optionalString (!has "chromium") ''
        rm -f "${adminHome}/.config/mimeapps.list"
      ''}

      ${lib.optionalString (has "vscode") ''
        ${prepareStaffDirectories}
        ${installForStaff ".config/Code/User/settings.json" labAssets.vscodeSettings}
        ${installForStaff ".vscode/argv.json" vscodeArgv}
      ''}
      ${lib.optionalString (!has "vscode") ''
        ${removeForStaff ".config/Code/User/settings.json"}
        ${removeForStaff ".vscode/argv.json"}
      ''}
      ${lib.optionalString (has "nodejs") ''
        ${builtins.concatStringsSep "\n        " (map (profile:
          ''install -d -o ${profile.user} -g users -m 0755 "${profile.home}/.local/npm"''
        ) staffHomes)}
      ''}
    '';
  };

  system.activationScripts.nixoriumUserHomeOwnership.deps = lib.mkAfter [ "siteHomeProfile" ];
}
