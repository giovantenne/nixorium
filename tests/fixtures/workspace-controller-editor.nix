{ pkgs, ... }:
{
  # Gives the controller the editor so a client-only declaration is the only
  # possible reason for a missing prerequisite.
  environment.systemPackages = [ pkgs.vscode ];
}
