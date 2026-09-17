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
override. Client installation must still verify that the configured interface
exists after reboot, and the future guided client-network flow must explain
role defaults without exposing unnecessary fields during controller bootstrap.

## Verification

Schema tests reject invalid values and unknown hosts. Go tests cover optional
round-trip behavior and preserve old files without adding fields. `mkLab` tests
evaluate controller, client-role, and host-specific networking and firewall
interfaces. Full validation covers real controller/client builds, the generated
site template, installer bundle, and offline derivation equivalence.
