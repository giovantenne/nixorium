# ADR-0023: Reviewed recovery of interrupted operations

- Status: accepted
- Date: 2026-10-01

## Context

An interrupted client update leaves `deployment-pending.json`, and an
interrupted deployment template reset leaves `.git/nixorium-template-reset.json`.
Both deliberately block every operation, including classroom controls, until
an administrator confirms what happened. Until now the only way out was a
manual procedure: holding the operation lock while moving the record, or
completing a Git compare-and-swap by hand. Administrators who are not Git or
Nix experts could not leave that state without help.

## Decision

Both records get a reviewed plan/apply recovery, in the CLI and the TUI.
Neither is automatic, retries the original operation, or declares it
successful.

Interrupted client update (`deploy recover`):

- The plan validates the record, confirms that the process that started the
  update has exited, and observes every recorded computer over authenticated
  SSH with a fixed read-only probe: running revision, queued systemd jobs and
  a running `switch-to-configuration`.
- A computer that is still applying blocks the review. A computer that cannot
  be checked requires an explicit acknowledgement after a console inspection.
- The token binds the record, the observations, the acknowledgement and an
  expiry. Apply re-reads the record, rechecks the reachable computers and,
  under the operation lock, renames the record into `recovered/` without
  overwriting. The confirmation word is `RECOVERED`.

Interrupted template reset (`template-reset recover`):

- The plan classifies the repository from Git state only: already finished,
  only the branch move missing, nothing replaced, or a mixed checkout.
- The first three finish or clear the reset with `RECOVERED`; the branch move
  uses compare-and-swap.
- A mixed checkout is restored with `RESTORE`: the current tracked files are
  kept under a new recovery reference, the original tree is restored and
  committed on top of the current branch, and untracked or ignored files are
  never removed. An untracked file the restore would overwrite stops the
  review.
- Apply holds the operation and repository locks and refuses any change since
  review.

The manual procedures stay documented as a fallback.

## Consequences

Persistent blockers have a way out that a non-expert can follow, visible on
the Overview and in safe mode. The archived records keep the evidence for
investigation. A recovery can still be refused (activity, changed state,
unsafe record); the refusal names the next step.
