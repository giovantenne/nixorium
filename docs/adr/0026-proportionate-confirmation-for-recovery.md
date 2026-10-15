# ADR-0026: Proportionate confirmation for recovery

- Status: accepted
- Date: 2026-10-15
- Amends: [ADR 0023](0023-reviewed-recovery-of-interrupted-operations.md)

## Context

ADR 0023 gave every interrupted operation a reviewed recovery, and each one
asked for a typed word. On a real laboratory the interrupted client update
review took six steps from the failure to the fix: leave the failed screen,
find the Overview row, read a technical refusal, acknowledge the computer that
did not answer, type `RECOVERED`, and go back to retry. The typed word protected
nothing: confirming that review changes nothing on any computer. It only moves
the record into `recovered/` and unblocks other operations, and the review
itself already enforces every check that matters.

A typed word is useful when it makes the operator stop before something that
cannot be taken back. Asking for one where nothing can be lost teaches people
to type confirmation words without reading.

## Decision

The confirmation is proportionate to what the action can change.

- Enter confirms a recovery that changes nothing on any computer and only
  archives or clears a record: the review of an interrupted client update, and
  finishing or clearing an interrupted template reset when every file already
  matches. The CLI asks a yes/no question; `--yes` keeps working for scripts.
- A typed word stays for actions that may be destructive or whose effect is
  uncertain: `STOP WAITING` (stopping local supervision of a running update),
  closing a USB installation after a possible disk change, the template reset
  `RESTORE`, `DEPLOY`, and every destructive action (`ERASE`, discard, cleanup,
  shutdown and the like).

The checks of ADR 0023 do not change: there is no automatic unlock; every
recorded computer is checked over SSH; a computer that is still applying blocks
the review; a computer that cannot be checked needs an explicit choice
("continue without the computers that do not answer"); the token binds the
observations and expires; apply rechecks the record and the computers; the
record is archived, never deleted; and the old update is never declared
successful.

The review is also offered where the failure happens: a client update that
leaves its record opens the review at once, and a failure caused by a
catalogued situation offers its screen on Enter. The process that ran the
update is not refused as "still running" once its apply has returned; any
other live Nixorium process is.

## Consequences

Recovering from an interrupted client update takes one decision instead of a
word to type, and the screens say in plain words what was checked and what
confirming does. The typed words that remain are rare enough to keep their
meaning. The client update review uses this policy from 3.7.0; the template
reset finish/clear branches follow in the same release line, while `RESTORE`
keeps its word.
