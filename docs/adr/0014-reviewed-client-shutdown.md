# ADR-0014: Reviewed client shutdown

- Status: accepted
- Date: 2026-09-15
- Amended: 2026-09-19

## Context

An administrator occasionally needs to power off one, several, or all client
computers. The laboratory is not continuously monitored and many clients are
normally off, so absence from the network is neither an error nor proof of
physical power state. A broad remote-shell feature would also bypass evaluated
inventory, session protection, concurrent-operation rules, and the existing
application boundary shared by CLI and TUI.

The controller must never be included in a client batch. A user may have
unsaved work, management access may disappear while a request is in flight,
and the operating system accepting a request is not the same fact as a machine
being physically off.

## Decision

Provide one typed client-only plan/apply operation shared by CLI and TUI. Plan
resolves only evaluated client identities, observes authenticated management
access and interactive session state, checks PXE/controller-network conflicts,
and returns an expiring content-bound review token and the one-word
`SHUTDOWN` confirmation. When an active session is present, the confirmation
prompt states explicitly that this word authorizes its interruption.
Unreachable clients remain visible but ineligible and no request is
queued for later.

Install a fixed `nixorium-session-state` helper in managed host generations.
Its output is limited to `active` or `idle`; errors or invalid output become
unknown. An active session remains eligible after the review presents an
explicit warning that unsaved work may be lost. Unknown session state is
blocked by default and becomes eligible only in a newly reviewed
`acknowledge-unknown` plan.

Apply validates token and expiry, takes the same per-administrator lock as
deployment, reevaluates inventory, rechecks PXE state and sessions, and then
passes only eligible evaluated host metadata to the adapter. The adapter
constructs fixed non-interactive SSH argument arrays for the session helper and
`systemctl poweroff --no-block`. No presentation value can supply a host
address, executable, argument, or shell fragment.

Report each target as `accepted`, `not-sent`, or `unconfirmed`. Accepted means
only that the remote command returned successfully. Connection loss during
dispatch is unconfirmed and must not trigger an automatic retry. Network
absence after dispatch is never translated into “powered off.” Store only a
bounded typed summary in operation history.

## Consequences

The ordinary workflow supports a partly occupied room without scanning or
alarming on unrelated computers. It preserves the controller boundary,
warns clearly before interrupting observed interactive users, and serializes
with deployment while remaining available without a privileged controller
service.

Older client generations without the session helper are unknown and require
explicit risk acknowledgement; their next normal deployment installs the
helper. The workflow cannot prove ACPI completion, physical power state, or
whether an unconfirmed request took effect. Wake-on-LAN, scheduling, and
automatic retry remain outside this decision.
