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

Keep Bubble Tea, the official Bubbles components, and Lip Gloss on aligned
major versions. The presentation layer may compose Bubbles for input,
selection, progress, viewport, and help behavior, and Lip Gloss for visual
hierarchy. Critical state and actions must remain explicit in text without
depending on color or Unicode.

## Consequences

Go provides straightforward cross-package tests and Nix packaging. Module
dependencies must be pinned and vendored or otherwise built reproducibly by
Nix. Bubble Tea adds terminal behavior that needs integration testing, but it
does not become an operational dependency of the backend.

The v2 presentation requests the terminal background color and selects
light/dark styles explicitly. Shared Bubbles help/key components adapt to the
available width; future list, input, progress, spinner, table, and viewport
usage must reuse this presentation-only foundation rather than introducing
backend dependencies or parsing rendered output.

The dashboard home composes an official Bubbles list into a keyboard-navigable
task menu. Arrow keys and Enter provide the primary path while stable
one-letter shortcuts remain available. Lip Gloss separates title, section,
muted description, success, attention, and failure roles for both light and
dark terminals. These roles supplement explicit text labels; color and Unicode
are never the only carrier of state or action meaning.

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

Foreground execution emits a small in-process `DeploymentProgress` event at
application-owned build, revalidation, apply, verification, and completion
boundaries. Bubble Tea retains at most five authored activities and renders a
four-stage progress bar plus verification counts. It never parses or displays
the untrusted Colmena byte stream, which continues to flow only to the private
mode-0600 log. The compact terminal result links back to the dashboard, the
bounded log browser, or a fresh revision-bound review.

This foreground model is retained for the current product scope. Moving the
operation into a system service would detach it from the administrator's SSH
authority and widen the privilege boundary. A transient user service would
still require linger/session policy plus a second durable job/result protocol
before it could honestly survive logout, while losing direct streamed output
and cancellation semantics. Terminal or process loss can therefore interrupt
Colmena. The private operation log and fresh authenticated host reconciliation
are the explicit recovery surface; operators must make a new plan before
retrying. Revisit a user-owned background executor only if real-lab evidence
shows that survivable unattended deployment is a product requirement.

Routine controller rebuild also enters Bubble Tea only as typed plan/apply
callbacks. Presentation owns review and exact confirmation; revision checks,
the systemd action, activation verification, and journal ownership remain in
the application, adapter, and controller module layers. Unlike foreground
Colmena, the systemd-owned rebuild may safely survive dashboard exit. Its
terminal result refreshes the ordinary typed status callback, keeps completed
activity collapsed until requested, and exposes explicit navigation; this
presentation refresh does not become activation evidence.

Service management follows the same typed callback rule. Bubble Tea displays
the cache and composite PXE state, collects exact cache-restart confirmation,
and renders the verified action report. It cannot select an arbitrary unit or
verb, and it directs PXE mutations to the dedicated lifecycle operation.

Operation-log browsing also enters presentation only as typed bounded list and
detail reports. Bubble Tea owns recent-outcome rendering, log selection, and
viewport scrolling. The application reduces supported final reports to fixed
safe summary fields without raw messages; the adapter serializes concurrent
writers and atomically retains the newest 1000 records. It also owns
generated-ID validation, no-follow file access, current-user ownership and
strict-mode checks, newest-log and byte limits, and terminal-control
sanitization; neither CLI nor TUI accepts an arbitrary path.

Git review/commit and upstream update screens preserve the same rule. Bubble
Tea selects typed reviewed paths or release policy, renders bounded patches,
collects the exact generated confirmation, and displays typed results. Git,
filesystem, Nix, release discovery, operation recording, and every follow-up
action remain in application/adapters. Only explicit `update check` enumerates
the public remote; opening the dashboard never does.

Systemd-owned PXE preparation and controller apply can outlive their initiating
terminal. The CLI writes immediate typed activity plus the fixed journal route
to stderr so JSON stdout remains machine-clean. Each service atomically
publishes a private, bounded, versioned progress record containing its fixed
phases, counters, and at most five authored activities. A strict
adapter/domain callback polls that record while the TUI renders elapsed time
and an official Bubbles progress bar. Bubble Tea never reads the journal or
chooses a filesystem path, stale pre-run records are ignored, detailed build
output remains in journald, and the progress record is not success authority;
the separate controller activation receipt and PXE manifest retain that role.

Bare first-run setup reuses the same dashboard state machine after its terminal
configuration and credential boundary. A typed setup-status callback supplies
the observed current stage; presentation maps that stage only to existing typed
Git, controller, and PXE callbacks. It neither writes a completion flag nor
duplicates operational completion decisions. Returning from a workflow reloads
the setup report so a new process can safely resume the same path.
