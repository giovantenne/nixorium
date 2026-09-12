# ADR-0006: Interface-scoped laboratory firewall

Status: accepted

## Context

The shared host module disabled the NixOS firewall while Veyon/VNC policy was
unresolved. This made SSH, desktop discovery, classroom control, Harmonia, and
on-demand PXE listeners depend only on process binding for exposure control.
The controller DHCP network is institution-owned and cannot be represented by
a reliable source subnet in public configuration.

## Decision

Enable the NixOS firewall on every installed host and open product ports only
on the configured laboratory interface. Every host admits SSH, Veyon, and mDNS;
hosts using the GNOME Remote Desktop fallback also admit VNC. Only the
controller admits Harmonia plus ProxyDHCP, TFTP, PXE service, and artifact HTTP
ports. Disable the OpenSSH and Avahi modules' automatic global openings so no
parallel all-interface rule defeats this policy.

PXE ports are present in the static interface policy because the NixOS
firewall does not follow activation of the on-demand listener. When PXE mode is
stopped there is no listening process, so the allowed ports expose no endpoint.
Private deployments may use normal NixOS module overrides for a stricter known
topology; public defaults do not guess an institutional DHCP source range.

## Consequences

The public configuration has a usable firewall without moving site network
policy upstream. A configured interface that carries both laboratory and other
traffic still exposes the documented product services to that link, so physical
deployment review remains necessary. Role-specific evaluation assertions and
the management VM protect the intended port matrix.
