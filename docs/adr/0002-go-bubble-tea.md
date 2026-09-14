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

The computer inventory follows the same boundary: Bubble Tea triggers a typed
host-report callback only when the administrator opens or refreshes the screen.
The application preserves inventory semantics, while the adapter bounds and
classifies network probes. CLI text, JSON, TUI, and doctor do not parse or
reimplement one another's output. The richer inventory still follows this
boundary: managed NixOS generations embed the private deployment revision and
install a fixed read-only state helper; an authenticated, bounded SSH adapter
observes it, the application reconciles it against desired HEAD, and Bubble Tea
only renders the typed current/outdated/unknown result.

The deployment screen also receives only typed plan/apply callbacks. Bubble Tea
owns ordered target selection, review rendering, exact confirmation input,
activity state, and the final report view. The composition root and adapters
retain revision/readiness checks, operation locking, private logging, and fixed
Colmena execution. Post-apply host authentication, per-host history recording,
and complete/partial classification likewise remain application and adapter
responsibilities; Bubble Tea only renders their typed counts and timestamps.
Because Colmena is a foreground child rather than a systemd-owned service, the
TUI refuses an accidental quit until it receives the operation's final typed
result.

Routine controller rebuild also enters Bubble Tea only as typed plan/apply
callbacks. Presentation owns review and exact confirmation; revision checks,
the systemd action, activation verification, and journal ownership remain in
the application, adapter, and controller module layers. Unlike foreground
Colmena, the systemd-owned rebuild may safely survive dashboard exit.

Service management follows the same typed callback rule. Bubble Tea displays
the cache and composite PXE state, collects exact cache-restart confirmation,
and renders the verified action report. It cannot select an arbitrary unit or
verb, and it directs PXE mutations to the dedicated lifecycle operation.

Operation-log browsing also enters presentation only as typed bounded list and
detail reports. Bubble Tea owns selection and viewport scrolling. The adapter
owns generated-ID validation, no-follow file access, current-user ownership and
strict-mode checks, newest-record and byte limits, and terminal-control
sanitization; neither CLI nor TUI accepts an arbitrary path.
