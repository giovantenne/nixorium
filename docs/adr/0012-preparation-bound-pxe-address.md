# ADR-0012: Preparation-bound PXE controller address

Status: accepted

## Context

The controller receives an institutional DHCP address used only while serving
PXE and the signed installation cache. That address was embedded in the
netboot system and treated as an exact configured value by preparation and the
runtime services. An ordinary lease change therefore required editing and
committing the private deployment, rebuilding the controller, and rebuilding
artifacts even though the client closures and netboot runtime were otherwise
unchanged.

Selecting any observed address without a stable boundary would be unsafe on an
interface with aliases or several networks. The runtime address must also reach
the guided installer without introducing client Internet access or weakening
cache signature verification.

## Decision

Treat `masterDhcpIp` as the initial selection hint. At each managed PXE
preparation, prefer that address if it is currently assigned. If it is absent,
accept exactly one usable global IPv4 address other than the declarative static
lab address, excluding IPv4 link-local addresses. Refuse zero or multiple
candidates and preserve the previous manifest.

Bind the selected address to the revisioned preparation manifest. The root
network transition and unprivileged listener consume and revalidate that exact
prepared address rather than the older configured hint. The listener generates
the runtime iPXE script and appends the selected address as a dedicated kernel
parameter. The guided installer strictly parses that parameter and uses the
address only as the explicit Harmonia substituter for the already prepared
client closure. The embedded public cache key remains the trust authority.

Do not configure an address-dependent default substituter in the netboot
system. Its runtime and offline installer sources are already part of the
immutable ramdisk; client installation continues to request the prepared
closure from Harmonia with fallback disabled.

## Consequences

A routine unambiguous lease change needs only `nixorium pxe prepare`; it does
not mutate the deployment or require controller activation. Preparation,
status, doctor, start, and the installer all expose or consume the same recorded
address. Existing schema-version-1 manifests remain readable because their
`controller.dhcpIp` field already identifies the address used by the session.

Ambiguous interfaces deliberately require operator resolution or an updated
configured hint. A lease change after preparation still makes start fail closed
and requires preparation again. The PXE transport remains a trusted local-LAN
boot boundary, while signed Nix paths prevent the selected cache endpoint from
silently substituting untrusted client closures. Physical DHCP/ProxyDHCP
coexistence remains subject to the documented hardware test matrix.
