{ hostName, labSettings, nixoriumPackage, lib, pkgs, ... }:
let
  isController = hostName == labSettings.masterHostName;
in
{
  environment.systemPackages = lib.mkIf isController [
    nixoriumPackage
    pkgs.colmena
  ];
}
