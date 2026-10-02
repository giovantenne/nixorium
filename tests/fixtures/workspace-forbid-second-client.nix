{ hostName, lib, ... }:
{
  # pc02 shares pc01's class; prerequisite checks must not evaluate it again.
  environment.systemPackages = lib.mkIf (hostName == "pc02")
    (throw "an identical client was evaluated again");
}
