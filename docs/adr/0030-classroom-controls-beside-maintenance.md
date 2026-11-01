# ADR-0030: Classroom controls beside maintenance

- Status: accepted
- Date: 2026-10-19
- Amends: [ADR 0023](0023-reviewed-recovery-of-interrupted-operations.md)
  (what an interrupted client update blocks) and
  [ADR 0014](0014-reviewed-client-shutdown.md) (the lock shutdown takes)
- Related: [ADR 0027](0027-controller-activation-past-client-records.md)

## Context

Internet access, shut down, restart and the scheduled shut-down took the same
operation gate as maintenance. While an administrator updated computers (often
more than ten minutes, building included) or while an interrupted update
waited for its review, a teacher could not block Internet during an exam and
the evening schedule left every computer on. The queue of switched-off
computers made this frequent: after the morning switch-on it held the gate
while it updated the computers that came back.

The real hazard is narrow. A power or Internet request must never reach a
computer that may be activating a new system. Every managed activation is
already announced by `deployment-pending.json`, created and synced before
dispatch and removed only when the update returned successfully. The build
phase touches no client.

## Decision

Classroom controls no longer take the operation gate. They take a lease:

- `classroom.lock` (mode 0660, `root:nixorium-operations`, next to
  `operation.lock`) is held shared by a classroom control while it rechecks
  and sends its requests, and exclusively by a client update only while it
  creates and syncs its pending record (bounded wait of 60 seconds; a timeout
  starts nothing and says so). The order is always operation lock, then
  classroom lock.
- Under the shared lock the control reads `deployment-pending.json` and leaves
  alone every computer it names: the review marks them as not available, the
  apply reports them as not sent. Other computers are handled normally, even
  while maintenance holds the operation gate.
- An unreadable record, or a USB installation reservation, still refuses
  classroom controls on every computer: neither can say safely which computer
  is concerned.
- A controller whose configuration has no `classroom.lock` yet writes pending
  records as before; classroom controls there fail closed until the controller
  is applied.

Cleanup of old generations, host-key trust, template reset, Assistant saves,
USB reservations and the deferred-update queue stay on the operation gate. The
fixed SSH command sets of ADR 0014 and of Internet control do not change.

## Consequences

- A teacher can block Internet, switch computers off or restart them during
  maintenance; the scheduled shut-down leaves on only the computers being
  updated and says why.
- An interrupted client update keeps blocking maintenance until its reviewed
  recovery (ADR 0023), but classroom controls refuse only the computers it
  names. The Overview and the situation catalogue say so.
- Classroom controls no longer serialize each other. The client keeps its own
  lock for Internet changes, and every request is preceded by a fresh boot and
  state check, as before.
- A client update may wait up to a minute for classroom requests in flight
  before its apply; this never leaves a pending record behind.
