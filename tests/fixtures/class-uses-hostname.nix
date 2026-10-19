# A client module that needs the computer's own name: in a configuration
# shared by several computers it must fail clearly (ADR 0033).
{ hostName, ... }:
{
  environment.etc."who".text = hostName;
}
