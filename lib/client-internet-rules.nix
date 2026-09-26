{ interface, networkCidr }:
let
  iface = builtins.toJSON interface;
  destinations = ''
    oifname ${iface} ip daddr ${networkCidr} accept
    # Preserve address renewal without permitting ordinary Internet traffic.
    oifname ${iface} udp sport 68 udp dport 67 accept
    oifname ${iface} ip6 daddr ff02::1:2 udp sport 546 udp dport 547 accept
    oifname ${iface} icmpv6 type { nd-router-solicit, nd-neighbor-solicit, nd-neighbor-advert } accept
  '';
in
''
  table inet nixorium_internet {
    chain output {
      type filter hook output priority -10; policy drop;
      oifname "lo" accept
      ${destinations}
    }
    chain forward {
      type filter hook forward priority -10; policy drop;
      oifname ${iface} ip daddr ${networkCidr} accept
    }
  }
''
