# ADR-0005: Systemd-owned runtime services

Status: accepted

## Context

Harmonia and PXE currently run in foreground terminals. PXE also has a
temporary networking transition that can be left unclear after interruption.
The TUI must be able to exit while infrastructure remains observable and
recoverable.

## Decision

Make systemd own Harmonia, on-demand PXE listeners, PXE network transition,
recovery, and long-running preparation/apply jobs. Store only the minimum
root-owned active-session data needed for recovery and always reconcile it
against actual unit, listener, and interface state. Use journald for operation
events and detailed logs.

## Consequences

Services gain dependencies, restart behavior, persistent logs, and clean
start/stop semantics. Network cleanup can run on normal stop and boot-time
recovery. Abrupt interruption and power-loss paths still require VM and
physical hardware validation before PXE lifecycle work is complete.

## Implementation status

The controller now enables nixpkgs' native Harmonia cache service and exposes
the stable `nixorium-harmonia.service` alias. The source signing key remains
root-only at `/var/lib/nixorium/keys/harmonia-secret-key`; systemd delivers an
isolated runtime credential to Harmonia. A NixOS VM proves failure without the
key, recovery after verified installation, real cache metadata, service state,
and durable canonical-unit logs. PXE ownership and recovery remain pending.
