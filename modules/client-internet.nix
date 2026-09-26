{ hostName, labSettings, pkgs, lib, ... }:
let
  isClient = (labSettings.deploymentMode or "laboratory") == "laboratory"
    && hostName != labSettings.masterHostName;
  rules = pkgs.writeText "nixorium-internet-block.nft" (import ../lib/client-internet-rules.nix {
    interface = labSettings.ifaceName;
    networkCidr = "${labSettings.networkBase}/${toString labSettings.networkPrefixLength}";
  });
  helper = pkgs.writeShellApplication {
    name = "nixorium-internet";
    runtimeInputs = [ pkgs.coreutils pkgs.gnugrep pkgs.util-linux pkgs.systemd pkgs.nftables ];
    text = builtins.readFile ../scripts/client-internet.sh;
  };
in
{
  environment.systemPackages = lib.optionals isClient [ helper ];
  systemd.services.nixorium-internet-block = lib.mkIf isClient {
    description = "Temporary classroom Internet block";
    after = [ "nftables.service" ];
    wants = [ "nftables.service" ];
    # No wantedBy: only an explicit administrator action starts this unit.
    # Firewall reloads and ordinary activation must retain a current block.
    restartIfChanged = false;
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = "${pkgs.nftables}/bin/nft -f ${rules}";
      ExecStop = "${pkgs.nftables}/bin/nft destroy table inet nixorium_internet";
      CapabilityBoundingSet = [ "CAP_NET_ADMIN" ];
      NoNewPrivileges = true;
      ProtectSystem = "strict";
      ProtectHome = true;
      PrivateTmp = true;
    };
  };
}
