{ nixoriumClass, lib, ... }:
{
  # pc02 shares pc01's class; prerequisite checks must not evaluate it again.
  # As its own class (updateValidationHosts) it is evaluated, and throws.
  environment.systemPackages = lib.mkIf (nixoriumClass.representative == "pc02")
    (throw "an identical client was evaluated again");
}
