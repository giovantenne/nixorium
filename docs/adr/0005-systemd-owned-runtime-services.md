# ADR-0005: Systemd-owned runtime services

Status: accepted

## Context

At the time of this decision, Harmonia and PXE ran in foreground terminals.
PXE also had a temporary networking transition that could be left unclear
after interruption. The TUI must be able to exit while infrastructure remains
observable and recoverable.

## Decision

Make systemd own Harmonia, on-demand PXE listeners, PXE network transition,
recovery, and long-running preparation/apply jobs. Store only the minimum
root-owned active-session data needed for recovery and always reconcile it
against actual unit, listener, and interface state. Use journald for operation
events and detailed logs.

## Consequences

Services gain dependencies, restart behavior, persistent logs, and clean
start/stop semantics. Network cleanup can run on normal stop and boot-time
recovery. Abrupt interruption and power-loss paths are covered in the NixOS
VM; physical hardware validation remains required before claiming hardware
coverage.

## Implementation status

The controller now enables nixpkgs' native Harmonia cache service and exposes
the stable `nixorium-harmonia.service` alias. The source signing key remains
root-only at `/var/lib/nixorium/keys/harmonia-secret-key`; systemd delivers an
isolated runtime credential to Harmonia. A NixOS VM proves failure without the
key, recovery after verified installation, real cache metadata, service state,
and durable canonical-unit logs. PXE preparation now runs as `admin` inside a
fixed oneshot unit, verifies the live DHCP address and cache health, and
atomically records canonical build outputs for the exact Git revision. The
existing compatibility proxy consumes that record defensively. The internal
`nixorium-pxe-network.service` now writes a root-owned session before removing
the exact configured static CIDR; stop and the boot-enabled recovery unit
restore only that recorded address and archive the reconciled outcome. The
unit retains only `CAP_NET_ADMIN` and validates the live interface over stored
state before every mutation. `nixorium-pxe.service` now owns the ProxyDHCP,
TFTP, and HTTP listeners, validates both records and live address state before
binding, and reports readiness only after the HTTP endpoint responds. Its
ephemeral runtime files are systemd-owned; HTTP runs as `nobody` and dnsmasq
drops to a dedicated system identity. The public lifecycle repeats readiness
checks around exact confirmation, exposes typed active/degraded/recovery state,
and controls only fixed start/stop/recover unit pairs through polkit. VM tests
prove idempotent start/stop, synchronous rollback after listener failure,
explicit recovery, and boot recovery after a controller crash.
