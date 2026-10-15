# `interface = null` means the client is configured by type, not by name
# (ADR 0017): destinations are then matched by address only, so the block
# applies to whichever NIC carries the traffic.
# `previousNetworkCidr` is the previous laboratory network during a change of
# laboratory network: the previous controller address and the computers not
# yet moved stay reachable while the block is active.
{ interface, networkCidr, previousNetworkCidr ? null }:
let
  oif = if interface == null then "" else "oifname ${builtins.toJSON interface} ";
  # Extra lines keep the layout of the single-network rules byte for byte.
  previousNetwork = indent:
    if previousNetworkCidr == null then ""
    else "\n${indent}${oif}ip daddr ${previousNetworkCidr} accept";
  destinations = ''
    ${oif}ip daddr ${networkCidr} accept${previousNetwork ""}
    # Preserve address renewal without permitting ordinary Internet traffic.
    ${oif}udp sport 68 udp dport 67 accept
    ${oif}ip6 daddr ff02::1:2 udp sport 546 udp dport 547 accept
    ${oif}icmpv6 type { nd-router-solicit, nd-neighbor-solicit, nd-neighbor-advert } accept
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
      ${oif}ip daddr ${networkCidr} accept${previousNetwork "    "}
    }
  }
''
