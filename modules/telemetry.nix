{ lib, hostName, labSettings, nixoriumPackage, pkgs, ... }:
let
  count = labSettings.pcCount;
  band = if count == 0 then "0" else if count <= 5 then "1-5"
    else if count <= 15 then "6-15" else if count <= 30 then "16-30"
    else if count <= 60 then "31-60" else "61+";
in
{
  config = lib.mkIf (hostName == labSettings.masterHostName) {
    # This describes the installed controller generation, not an unevaluated
    # checkout. No identity or consent is stored in the Nix store.
    environment.etc."nixorium/telemetry.json".text = builtins.toJSON {
      schemaVersion = 1;
      version = nixoriumPackage.version;
      deploymentMode = labSettings.deploymentMode;
      configuredClients = band;
    };
    systemd.tmpfiles.rules = [ "d /var/lib/nixorium/telemetry 0700 admin users -" ];
    systemd.services.nixorium-telemetry = {
      description = "Optional Nixorium adoption report";
      after = [ "network.target" ];
      restartIfChanged = false;
      environment.SSL_CERT_FILE = "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt";
      serviceConfig = {
        Type = "oneshot";
        User = "admin";
        Group = "users";
        ExecStart = "${nixoriumPackage}/bin/nixorium telemetry send";
        TimeoutStartSec = 15;
        UMask = "0077";
        NoNewPrivileges = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictSUIDSGID = true;
        CapabilityBoundingSet = "";
        RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];
        ReadWritePaths = [ "/var/lib/nixorium/telemetry" ];
        InaccessiblePaths = [ "-/var/lib/nixorium/keys" ];
      };
    };
    systemd.timers.nixorium-telemetry = {
      description = "Daily optional Nixorium adoption report";
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnCalendar = "daily";
        RandomizedDelaySec = "1h";
        Persistent = true;
      };
    };
  };
}
