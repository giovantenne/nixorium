{ pkgs, labSettings, homeResetEphemeralPaths, workspaceSeed, workspaceWallpapers, nixoriumPackage, ... }:

{
  # Imported by home-reset.nix. Activation must not reset a live home: restoration waits for normal boot.
  environment.etc."nixorium-workspace-reset.json".text = builtins.toJSON {
    user = labSettings.studentUser;
    seed = workspaceSeed;
    ephemeralPaths = homeResetEphemeralPaths;
    wallpapers = workspaceWallpapers;
    btrfs = "${pkgs.btrfs-progs}/bin/btrfs";
    dconf = "${pkgs.dconf}/bin/dconf";
    systemctl = "${pkgs.systemd}/bin/systemctl";
  };

  systemd.services.home-reset = {
    description = "Restore the configured student workspace before login";
    wantedBy = [ "multi-user.target" ];
    requiredBy = [ "systemd-user-sessions.service" "display-manager.service" ];
    before = [ "systemd-user-sessions.service" "display-manager.service" ];
    after = [ "local-fs.target" "systemd-tmpfiles-setup.service" ];
    # A new profile takes effect at the next normal boot, not during switch.
    restartIfChanged = false;
    stopIfChanged = false;
    unitConfig = {
      "X-OnlyManualStart" = true;
      "X-StopOnRemoval" = false;
      RefuseManualStart = true;
      RequiresMountsFor = [ "/home/${labSettings.studentUser}" "/var/lib/home-snapshots" ];
    };
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${nixoriumPackage}/bin/nixorium-home-reset";
      RemainAfterExit = true;
      Restart = "no";
      TimeoutStartSec = "infinity";
      UMask = "0077";
      NoNewPrivileges = true;
      PrivateTmp = true;
      PrivateDevices = true;
      ProtectSystem = "strict";
      ProtectKernelTunables = true;
      ProtectKernelModules = true;
      ProtectControlGroups = true;
      RestrictAddressFamilies = [ "AF_UNIX" ];
      ReadWritePaths = [ "/home/${labSettings.studentUser}" "/var/lib/home-snapshots" ];
      CapabilityBoundingSet = [ "CAP_CHOWN" "CAP_DAC_OVERRIDE" "CAP_FOWNER" "CAP_FSETID" "CAP_SYS_ADMIN" ];
    };
  };
}
