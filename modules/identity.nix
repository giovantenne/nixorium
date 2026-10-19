{ hostName, isController, labSettings, lib, pkgs, nixoriumClass ? null, ... }:
let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  # A system shared by the computers of a class (ADR 0033) reads the number
  # from the record; a per-host system knows it and writes the record.
  shared = nixoriumClass != null && nixoriumClass.shared;
  # The computer's identity record holds only its number: name and
  # addresses follow from it and from the laboratory settings. Written once,
  # by the system that knows its number or by the deployment before it
  # switches a computer to a shared system; a record naming another computer
  # is reported and kept, never overwritten.
  record = "/var/lib/nixorium/identity.json";
  environment = "/run/nixorium/identity.env";
  coreutils = "${pkgs.coreutils}/bin";
  # The record is read in its exact written form only.
  readNumber = ''
    recorded="$(${pkgs.gnused}/bin/sed -n 's/^{"schemaVersion":1,"number":\([1-9][0-9]\{0,3\}\)}$/\1/p' ${record} 2>/dev/null || true)"
  '';
  writeEnvironment = ''
    printf 'NIXORIUM_HOST_NAME=%s\nNIXORIUM_HOST_NUMBER=%s\nNIXORIUM_LAB_ADDRESS=%s\nNIXORIUM_PREVIOUS_ADDRESS=%s\n' \
      "$name" "$number" "$address" "$previous" > ${environment}.tmp
    ${coreutils}/mv -f ${environment}.tmp ${environment}
  '';
  perHost =
    let
      number = lib.toIntBase10 (lib.removePrefix "pc" hostName);
      identity = lib.findFirst (entry: entry.number == number) null (labSettings.clientIdentities or []);
    in ''
      ${coreutils}/mkdir -p /var/lib/nixorium /run/nixorium
      ${readNumber}
      # A missing or damaged record (empty after a power loss, edited) is
      # written again: this system knows its number. A valid record naming
      # another computer is kept and reported.
      if [ -z "$recorded" ]; then
        printf '{"schemaVersion":1,"number":%d}\n' ${toString number} > ${record}.tmp
        ${coreutils}/chmod 0644 ${record}.tmp
        ${coreutils}/mv -f ${record}.tmp ${record}
        ${readNumber}
      fi
      if [ "$recorded" != "${toString number}" ]; then
        echo "nixorium: ${record} names computer number ''${recorded:-unknown}, but this system is ${hostName}; the record was kept" >&2
      fi
      name=${lib.escapeShellArg hostName} number=${toString number}
      address=${lib.escapeShellArg (if identity == null then "" else identity.address)}
      previous=${lib.escapeShellArg (if identity == null then "" else identity.previousAddress or "")}
      ${writeEnvironment}
    '';
  sharedScript = ''
    ${coreutils}/mkdir -p /run/nixorium
    ${coreutils}/rm -f ${environment}
    ${readNumber}
    name= address= previous=
    case "$recorded" in
    ${lib.concatMapStrings (identity: ''
      ${toString identity.number}) name=${identity.name} address=${identity.address} previous=${identity.previousAddress or ""} ;;
    '') labSettings.clientIdentities}
    esac
    if [ -z "$name" ]; then
      echo "nixorium: this computer has no valid identity record (${record}); it gets no laboratory address" >&2
      # Show the broken state at the console instead of an old name.
      ${coreutils}/rm -f /etc/hostname
      printf 'nixorium-client\n' > /etc/hostname
      printf 'nixorium-client\n' > /proc/sys/kernel/hostname
    else
      number=$recorded
      ${writeEnvironment}
      # The computer's name, read by systemd at boot, and the running name;
      # DHCP never changes it (modules/networking.nix).
      ${coreutils}/rm -f /etc/hostname
      printf '%s\n' "$name" > /etc/hostname
      printf '%s\n' "$name" > /proc/sys/kernel/hostname
    fi
  '';
in
{
  config = lib.mkIf (laboratoryEnabled && !isController) {
    system.activationScripts.nixoriumIdentity = {
      # After /etc is set up: a shared system writes /etc/hostname itself.
      deps = [ "specialfs" "etc" ];
      text = if shared then sharedScript else perHost;
    };
    # The visible failure of a shared system without a valid record: no
    # laboratory address, a failed unit and this message.
    systemd.services.nixorium-identity = lib.mkIf shared {
      description = "Check this computer's Nixorium identity";
      wantedBy = [ "multi-user.target" ];
      before = [ "NetworkManager-ensure-profiles.service" ];
      serviceConfig = { Type = "oneshot"; RemainAfterExit = true; };
      script = ''
        test -s ${environment} || {
          echo "This computer has no Nixorium identity (${record}), so it has no laboratory address and the controller cannot reach it. At this computer, start the previous generation from the boot menu or write its number in ${record}, then update it from the controller; or reinstall it." >&2
          exit 1
        }
      '';
    };
  };
}
