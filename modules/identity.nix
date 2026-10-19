{ hostName, isController, labSettings, lib, pkgs, ... }:
let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  number = lib.toIntBase10 (lib.removePrefix "pc" hostName);
  # The computer's identity record holds only its number: name and
  # addresses follow from it and from the laboratory settings. Written once,
  # by the system that knows its number; a record naming another computer is
  # reported and kept, never overwritten.
  record = "/var/lib/nixorium/identity.json";
in
{
  config = lib.mkIf (laboratoryEnabled && !isController) {
    system.activationScripts.nixoriumIdentity = {
      deps = [ "specialfs" ];
      text = ''
        ${pkgs.coreutils}/bin/mkdir -p /var/lib/nixorium /run/nixorium
        if [ ! -e ${record} ]; then
          printf '{"schemaVersion":1,"number":%d}\n' ${toString number} > ${record}.tmp
          ${pkgs.coreutils}/bin/chmod 0644 ${record}.tmp
          ${pkgs.coreutils}/bin/mv -f ${record}.tmp ${record}
        fi
        recorded="$(${pkgs.gnused}/bin/sed -n 's/.*"number":[[:space:]]*\([0-9][0-9]*\).*/\1/p' ${record} 2>/dev/null || true)"
        if [ "$recorded" != "${toString number}" ]; then
          echo "nixorium: ${record} names computer number ''${recorded:-unknown}, but this system is ${hostName}; the record was kept" >&2
        fi
        printf 'NIXORIUM_HOST_NAME=%s\nNIXORIUM_HOST_NUMBER=%s\n' ${lib.escapeShellArg hostName} "''${recorded:-${toString number}}" > /run/nixorium/identity.env.tmp
        ${pkgs.coreutils}/bin/mv -f /run/nixorium/identity.env.tmp /run/nixorium/identity.env
      '';
    };
  };
}
