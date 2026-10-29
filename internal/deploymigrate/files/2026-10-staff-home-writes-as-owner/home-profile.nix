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
  # Activation runs as root, but every file in a staff home is written as its
  # owner: a symbolic link left in the home then cannot point the write at a
  # system file.
  asUser = user: command: ''
    ${pkgs.util-linux}/bin/setpriv --reuid=${lib.escapeShellArg user} --regid=users --init-groups -- ${command} \
      || echo "Nixorium: could not update a file in the home of ${user}" >&2'';
  installForStaff = path: source: builtins.concatStringsSep "\n" (map (profile:
    asUser profile.user ''${pkgs.coreutils}/bin/install -D -m 0644 ${source} "${profile.home}/${path}"''
  ) staffHomes);
  removeForStaff = path: builtins.concatStringsSep "\n" (map (profile:
    asUser profile.user ''${pkgs.coreutils}/bin/rm -f "${profile.home}/${path}"''
  ) staffHomes);
  prepareStaffDirectories = builtins.concatStringsSep "\n" (map (profile:
    asUser profile.user ''${pkgs.coreutils}/bin/install -d -m 0700 "${profile.home}/.config"''
    + "\n"
    + asUser profile.user ''${pkgs.coreutils}/bin/install -d -m 0755 "${profile.home}/.config/Code/User" "${profile.home}/.vscode/extensions"''
  ) staffHomes);
  vscodeArgv = pkgs.writeText "nixorium-site-vscode-argv.json" (builtins.toJSON {
    password-store = "basic";
    enable-crash-reporter = false;
  });
in
{
  system.activationScripts.siteHomeProfile = {
    # After the core repairs ownership of the managed folders in staff homes.
    deps = [ "createHomeTemplates" "nixoriumUserHomeOwnership" ];
    text = ''
      ${lib.optionalString (has "chromium") ''
        ${asUser "admin" ''${pkgs.coreutils}/bin/install -D -m 0644 ${labAssets.mimeApps} "${adminHome}/.config/mimeapps.list"''}
      ''}
      ${lib.optionalString (!has "chromium") ''
        ${asUser "admin" ''${pkgs.coreutils}/bin/rm -f "${adminHome}/.config/mimeapps.list"''}
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
        ${builtins.concatStringsSep "\n" (map (profile:
          asUser profile.user ''${pkgs.coreutils}/bin/install -d -m 0755 "${profile.home}/.local/npm"''
        ) staffHomes)}
      ''}
    '';
  };
}
