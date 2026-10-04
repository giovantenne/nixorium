{ interface, masterIp }:
let
  iface = builtins.toJSON interface;
in
{
  allow = ''
    iifname ${iface} ip saddr ${masterIp} tcp dport 22 accept
  '';
  guard = ''
    chain input {
      type filter hook input priority -10; policy accept;
      iifname "lo" accept
      iifname ${iface} ip saddr ${masterIp} tcp dport 22 accept
      tcp dport 22 drop
      udp dport 5353 drop
    }
  '';
}
