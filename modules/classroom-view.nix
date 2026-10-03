{ hostName, labSettings, lib, nixoriumPackage, pkgs, ... }:
# Experimental classroom view agent (off by default). The agent runs in the
# graphical session of whoever uses a client; the controller reaches it only
# through its existing SSH access, with the fixed command below. No network
# port is opened.
let
  laboratoryEnabled = (labSettings.deploymentMode or "laboratory") == "laboratory";
  enabled = (labSettings.classroomView or false) && laboratoryEnabled
    && hostName != labSettings.masterHostName;
  connect = pkgs.writeShellApplication {
    name = "nixorium-classroom-connect";
    runtimeInputs = [ pkgs.systemd ];
    text = ''
      exec ${nixoriumPackage}/bin/nixorium-classroom-agent connect
    '';
  };
in
{
  config = lib.mkIf enabled {
    environment.systemPackages = [ connect ];

    systemd.user.services.nixorium-classroom-agent = {
      description = "Nixorium classroom view agent";
      wantedBy = [ "graphical-session.target" ];
      partOf = [ "graphical-session.target" ];
      after = [ "graphical-session.target" ];
      serviceConfig = {
        ExecStart = "${nixoriumPackage}/bin/nixorium-classroom-agent serve";
        Restart = "on-failure";
        RestartSec = 2;
      };
    };
  };
}
