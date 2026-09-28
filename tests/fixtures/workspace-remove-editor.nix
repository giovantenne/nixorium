{ lib, ... }:
{
  # A downstream override must not bypass profile prerequisites even when
  # lab-software.json still declares the editor.
  environment.systemPackages = lib.mkForce [];
}
