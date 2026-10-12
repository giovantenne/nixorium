# `interface = null` means the client is configured by type, not by name
# (ADR 0017): the master address alone selects the allowed SSH source.
{ interface, masterIp }:
let
  inputMatch = if interface == null then "" else "iifname ${builtins.toJSON interface} ";
in
{
  allow = ''
    ${inputMatch}ip saddr ${masterIp} tcp dport 22 accept
  '';
  guard = ''
    chain input {
      type filter hook input priority -10; policy accept;
      iifname "lo" accept
      ${inputMatch}ip saddr ${masterIp} tcp dport 22 accept
      tcp dport 22 drop
      udp dport 5353 drop
    }
  '';
}
