# `interface = null` means the client is configured by type, not by name
# (ADR 0017): the master address alone selects the allowed SSH source.
# `previousMasterIp` is the controller's previous address during a change of
# laboratory network; SSH from it stays accepted until the change finishes.
{ interface, masterIp, previousMasterIp ? null }:
let
  inputMatch = if interface == null then "" else "iifname ${builtins.toJSON interface} ";
  sshFrom = address: "${inputMatch}ip saddr ${address} tcp dport 22 accept";
  # Extra lines keep the layout of the single-address rules byte for byte.
  previousRule = indent:
    if previousMasterIp == null then "" else "\n${indent}${sshFrom previousMasterIp}";
in
{
  allow = ''
    ${sshFrom masterIp}${previousRule ""}
  '';
  guard = ''
    chain input {
      type filter hook input priority -10; policy accept;
      iifname "lo" accept
      ${sshFrom masterIp}${previousRule "  "}
      tcp dport 22 drop
      udp dport 5353 drop
    }
  '';
}
