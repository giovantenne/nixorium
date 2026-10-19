# ADR-0026: Proportionate confirmation for recovery

- Status: accepted (amended 2026-10-19)
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
  archives or clears a record: the review of an interrupted client update,
  finishing or clearing an interrupted template reset when every file already
  matches, cancelling a USB installation safely or closing its record when
  the disk was never touched, and recovering the controller network after an
  interrupted network installation, which changes only the controller's own
  addressing. The CLI asks a yes/no question where it asked a typed word;
  `--yes` keeps working for scripts. `pxe stop` and `pxe recover` keep asking
  nothing.
- A typed word stays for actions that may be destructive or whose effect is
  uncertain: `STOP WAITING` (stopping local supervision of a running update),
  closing a USB installation after a possible disk change, the template reset
  `RESTORE`, `DEPLOY`, and every destructive action (`ERASE`, discard, cleanup,
  shutdown and the like). (The amendment of 2026-10-19 below moves `STOP
  WAITING` and `DEPLOY` to Enter and narrows the typed shutdown.)

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
meaning. The client update review, the template reset finish/clear branches, the
safe USB cancel and close, and the controller network recovery use this
policy from 3.7.0, while `RESTORE` and `CLOSE` after a disk change keep their
words. Every recovery screen has the same three parts: what Nixorium found,
what you can do, and the confirmation.

## Amendment (2026-10-19)

The rule above applies to every review, not only to recovery. A typed
confirmation word stays only where something is destroyed or cannot be undone.
Every other review is confirmed with Enter on the review screen that already
shows it in full, and with a `Continue? [y/N]` question in the interactive CLI.

Words kept:

- `ERASE`, before a disk is erased (the PXE installer `setup.sh`, the USB
  installer and the remote installer helper);
- `RESET DEPLOYMENT` and `RESTORE`, for the template reset and its mixed-state
  recovery;
- `DISCARD`, for unsaved changes;
- `ROTATE HOST KEY`, when a computer's SSH key is replaced;
- `CLEAN`, for the removal of old system generations;
- `CLOSE`, for a USB installation record only after the disk may have been
  written;
- `MATCH`, to say that two fingerprints compared on the physical console are
  identical;
- `SHUTDOWN` and `RESTART` in the CLI, only when the review includes an
  eligible computer whose session is active or unknown, because student work
  may be lost. When every computer is idle, logged in without use, or off, the
  CLI asks `Continue? [y/N]`. The administrator TUI confirms power actions with
  Enter, as decided earlier, and states how many sessions in use are
  interrupted.

Words that became Enter (TUI) and `[y/N]` (CLI): `DEPLOY` (Update computers),
`SAVE` and `SAVE SOFTWARE` (settings, software, workspace, initial files),
`ADD PROFILE`, `REMOVE`, `UPDATE`, `MIGRATE` and `DOWNGRADE` (Update Nixorium
and the package base), `REBUILD` and `APPLY` (Apply to controller), `COMMIT`,
`UNDO`, `START` and `LEAVE` (network installation mode), `RESTART` for the
binary cache, `STOP WAITING`, `RECOVERED`, `REBOOT` and `PUSH`.
`STOP WAITING` no longer needs a word because the review names what stopping
leaves behind and recovery after an interrupted update is itself reviewed.

Nothing else changes. Each review keeps its token, expiry, recheck and refusal;
`--yes` and `--expect <token>` behave as before; the screen says in one plain
sentence what Enter does; Esc cancels. Plans for the changed actions leave
their `confirmation` field empty, and the JSON field is omitted because it is
`omitempty`; plans for the kept words and for power actions with sessions in
use still carry theirs.
