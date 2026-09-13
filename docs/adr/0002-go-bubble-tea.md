# ADR-0002: Go and Bubble Tea

Status: accepted

## Context

The management application needs a single packageable executable, safe process
execution, typed domain results, forms, state-machine tests, and reliable use
over SSH. A collection of interactive shell scripts would make state,
validation, and failure recovery difficult to test.

## Decision

Implement the application in Go. Use Bubble Tea for the terminal presentation
layer only. Domain, application, and adapter packages must not import Bubble
Tea, allowing both CLI/JSON and TUI frontends to use the same operations.

## Consequences

Go provides straightforward cross-package tests and Nix packaging. Module
dependencies must be pinned and vendored or otherwise built reproducibly by
Nix. Bubble Tea adds terminal behavior that needs integration testing, but it
does not become an operational dependency of the backend.

The first operational TUI screen implements this boundary with injected typed
callbacks for PXE preparation and lifecycle operations. Unit tests drive its
state machine, including Bubble Tea's distinct space-key event, and the NixOS
management VM drives the packaged application through a real PTY. Command
execution and privilege decisions remain in the shared application and adapter
layers.
