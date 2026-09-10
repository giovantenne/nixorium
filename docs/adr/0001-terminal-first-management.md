# ADR-0001: Terminal-first management

Status: accepted

## Context

Nixorium must work locally, over SSH, and without external internet. The normal
administrator should not need to understand the underlying Nix, Colmena,
Harmonia, or PXE commands. A graphical desktop application or web service would
add another lifecycle and a larger remote-management surface.

## Decision

Provide a TUI as the primary interactive interface and a non-interactive CLI
over the same application/domain operations. Keep a stable structured output
mode so another frontend can be added later. Retain raw Nix and Colmena
workflows as an advanced interface.

## Consequences

The system remains usable from the controller console and SSH and requires no
browser, cloud account, or network API. Terminal layout, accessibility, and
long-operation feedback require explicit testing. The TUI cannot contain
operational logic or parse human-readable command output.
