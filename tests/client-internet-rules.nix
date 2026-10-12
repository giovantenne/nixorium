# byType selects the rules for clients configured without an interface name.
{ byType ? false }:
import ../lib/client-internet-rules.nix {
  interface = if byType then null else "lab0";
  networkCidr = "10.77.0.0/24";
}
