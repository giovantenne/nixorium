# ADR-0029: Automatic recovery of interrupted student home resets

- Status: accepted
- Date: 2026-10-19
- Amends: the fail-closed retry rule of the boot-time home reset
- Related: [ADR 0023](0023-reviewed-recovery-of-interrupted-operations.md)

## Context

Every boot restores the student home from the workspace seed before login.
The engine writes `pending.json`, prepares the seed, takes a snapshot of the
old home, sanitizes it and makes it read-only, then empties and restores the
home and publishes the snapshot. Until now any failure after `pending.json`
kept that record, and every later boot refused to run, so login stayed closed
until an administrator moved the evidence by hand.

The rule protected one real risk: a blind retry after the home was emptied
would snapshot the empty home and could rotate the only copy of the student's
files out of the five published snapshots. It also caught cases with no risk
at all. In a laboratory, a daily logrotate run that briefly switched to the
student account failed the process guard before anything changed, and a
computer switched off during the restore stopped after its snapshot was
complete. Both left classrooms with computers that would not start, and there
was no reviewed recovery command, unlike client deployment and template reset.

## Decision

The pending record (schema 2) carries a phase, replaced atomically:

- `prepare`: the original home is untouched. Written first.
- `restore`: written only after the sanitized snapshot `previous` is read-only
  and synced, immediately before the home is emptied.
- `published`: written after `previous` has been durably published as
  `snapshot-1`.

A failure during `prepare` removes that attempt's own work (`previous`,
`prepared`, `keyfiles`, then `pending.json`) and returns the error: the next
boot starts a normal reset.

At the next boot, under the same preflight, lock, storage checks and login
barrier, a pending record is recovered by phase:

- `prepare` (or a schema 1 record without a read-only `previous`): discard the
  attempt and run a normal reset.
- `restore` (or a schema 1 record with a read-only `previous`): prepare the
  seed again, empty and restore the home, and publish `previous`.
- `published`: verify that `snapshot-1` was taken from `previous` (Btrfs
  parent UUID), delete `previous` and record success.

The `restore` and `published` recoveries write `recovery.json` immediately
before their first change to the home or to `previous`; preparing the seed
again before that point may repeat. A pending record with that marker means
the single automatic attempt failed:
the reset fails closed as before. A record for another account, an unknown
phase or schema, malformed JSON, unexpected or untrusted entries, a writable
`previous` in the `restore` phase, or a published snapshot that does not match
`previous` also fail closed.

Publication is idempotent: rotation fills the first free slot instead of
always deleting `snapshot-5`, and a `snapshot-1` already taken from `previous`
is not repeated. `previous` is deleted only after its published copy is
verified.

## Consequences

Transient refusals and power loss no longer need an administrator, and the
student's files stay in a read-only snapshot throughout. The engine never
snapshots a home it has started to empty, because emptying begins only in
the `restore` phase, which always reuses the existing `previous`.

A schema 1 record whose old reset had already published and deleted
`previous` is treated as `prepare`: the restored home is reset again and
published once more, which costs the oldest snapshot but no student data.

Repeated failures still stop at the login barrier with the evidence intact.
A reviewed administrator recovery for those cases remains future work; the
troubleshooting guide describes what to preserve.
