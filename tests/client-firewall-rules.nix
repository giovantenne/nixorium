let
  rules = import ../lib/client-firewall-rules.nix {
    interface = "lab0";
    masterIp = "10.77.0.99";
  };
in
''
  table inet nixorium_client_access {
    ${rules.guard}
  }
  table inet base_firewall {
    chain input {
      type filter hook input priority 0; policy drop;
      iifname "lo" accept
      ct state established,related accept
      meta l4proto ipv6-icmp accept
      ${rules.allow}
    }
  }
''
