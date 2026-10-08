{ pkgs, ... }:
{
  environment.systemPackages = [ pkgs.cowsay ];
}
