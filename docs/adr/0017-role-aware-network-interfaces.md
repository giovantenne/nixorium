# ADR 0017: Role-aware network interfaces

Status: Accepted

## Context

The original configuration used one `ifaceName` for the controller, every
client, PXE lifecycle, firewall, and static network. First setup detected the
controller's default-route interface and wrote that shared field. Real
controller and client hardware can expose different predictable names, so a
correct controller detection could silently create unusable client systems.

## Decision

Keep `ifaceName` as the required compatibility fallback. Add optional
`controllerIfaceName`, `clientIfaceName`, and `hostIfaceNames`. Resolve the
effective value in this order:

1. exact host override;
2. controller or client role override;
3. compatibility fallback.

All values use the existing Linux interface-name validation. Per-host keys must
name a configured controller or client. Existing files omit the new fields on
round-trip and retain byte-level behavior apart from unrelated formatting.

`mkLab` passes a host-specific `labSettings.ifaceName` to built-in and extension
modules, so networking and firewall configuration use the same resolved value.
Controller management and PXE therefore use the controller interface. Client
systems use their role or host interface. The offline installer receives the
same typed configuration and resolution logic.

`labMeta.network.ifaceName` remains the effective controller interface for
older management consumers. The controller, each client, and the network
record also expose their effective or role-default interface. The Go decoder
accepts older metadata where those additive fields are absent.

Default-route detection now proposes `controllerIfaceName` instead of changing
the fallback used by clients. This is a proposal only; the normal candidate
validation and reviewed atomic write remain mandatory.

## Consequences

The contract relies on predictable NixOS interface names; it does not infer a
client's hardware identity remotely. A heterogeneous client can use a per-host
override. Client installation verifies an explicitly configured interface before
erasing; the addendum below covers clients without one.

## Addendum: clients without an explicit name

Detecting the controller's card cannot name the clients' cards, and the
template fallback `enp0s3` is VirtualBox's. On real laboratories the PXE client
installer stopped with "network interface not found", and an installed client
would have put its lab address on a card that does not exist.

A client is now configured by name only when the administrator chose one: a
`hostIfaceNames` entry or `clientIfaceName`. The controller always uses its
configured name. `mkLab` records this as `ifaceExplicit` (per host in
`labMeta.clients.hosts`, and in the host's `labSettings`), and the first-setup
form no longer copies the controller's card into `clientIfaceName`.

For other clients:

- `modules/networking.nix` declares one NetworkManager profile for any wired
  Ethernet device with DHCP plus the static lab address. NetworkManager already
  manages clients, so no second backend is added; wireless, bridge, veth and
  other virtual devices are different NetworkManager types and never match. The
  address is applied at once and kept without a DHCP server. Automatic DHCP-only
  profiles are disabled so nothing competes. A profile without an interface
  name is active on one device at a time: with several wired cards the first to
  come up carries it, which is not deterministic across boots, so such PCs need
  `hostIfaceNames`.
- The temporary Internet block and the client SSH source rule match addresses
  only (`ip daddr <lab network>`, `ip saddr <controller>`), keeping their
  semantics without `oifname`/`iifname`.
- The PXE installer and the USB/SSH preparation skip the name check and use the
  card that carries the live address.

Explicit names keep today's behaviour everywhere. Existing deployments that
saved a client name are unaffected; deployments that relied on the fallback now
get the name-independent behaviour.

## Verification

Schema tests reject invalid values and unknown hosts. Go tests cover optional
round-trip behavior and preserve old files without adding fields. `mkLab` tests
evaluate controller, client-role, and host-specific networking and firewall
interfaces. Full validation covers real controller/client builds, the generated
site template, installer bundle, and offline derivation equivalence.
