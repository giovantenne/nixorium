# ADR-0034: Deployment migrations

- Status: accepted
- Date: 2026-10-19
- Related: [ADR 0019](0019-deployment-owned-package-base.md),
  [ADR 0020](0020-autonomous-package-base-updates.md),
  [ADR 0033](0033-client-classes.md)

## Context

A private deployment is created from the site template: its files
(`modules/workstation.nix`, `software-catalog.nix`, `flake.nix` and others) are
copied once and then belong to the laboratory, who may change them. A later
release sometimes needs a change in those files. Until now the only answer was
a note in the changelog asking the administrator to edit a file by hand, which
is easy to miss and error-prone for a teacher or school technician, and an
update that did not take the note into account could stop with an evaluation
error.

The release that introduced client classes (ADR 0033) made one such change mandatory: the workstation profile of
deployments created before 3.9.0 finds the controller by comparing `hostName`,
and computers that share a configuration (ADR 0033) have no single name. The
note existed, but nothing applied it.

## Decision

A release lists, as data, the exact changes it needs in a deployment's own
files, and Update Nixorium applies them as part of the reviewed update.

- **The list.** `lib/deployment-migrations.nix` is a list exported as
  `lib.deploymentMigrations`. Each entry has `id` (date-prefixed, for example
  `2026-10-short-slug`), `file` (path relative to the deployment), `description`
  (one sentence for the administrator), `from` (exact text) and `to`
  (replacement text).
- **Evaluation.** The controller runs the code of the release already
  installed. It evaluates the list of the *target* release from the prepared
  candidate (`inputs.nixorium.lib.deploymentMigrations`), applies each
  migration to a staged copy of the committed deployment, and evaluates and
  builds the candidate with the migrated files.
- **Outcomes.** Each migration is:
  - `applied`: `from` appears exactly once. It is replaced, and the review
    shows the description and the change in the diff.
  - `done`: `from` is absent and `to` is already present. Nothing changes.
  - `manual`: the file was changed (`from` absent or present several times) or
    is missing or not tracked. Nothing is touched. The review shows "Manual
    change" with the text to replace and the text to put instead, and the
    administrator makes that change as an ordinary reviewed change. A manual
    migration does not block the update by itself: the update is refused only
    when the candidate, built without that change, does not evaluate or build,
    and then the review's error says why.
- **Record.** Applied and done ids are written to the tracked file
  `nixorium-migrations.json` (`{"schemaVersion":1,"applied":[...]}`), created
  or updated in the same save. A migration therefore runs once, even if the old
  text comes back later. This is the idea of Omarchy's migration markers, with
  the record versioned with the configuration and reviewed with it.
- **Saving.** Saving the update writes the migrated files first, each only if
  it is still exactly as reviewed (otherwise the save is refused, and files
  already written are restored if a later step fails), then `flake.lock` and
  `flake.nix`. The files are committed in the same Git review as the update
  (commit message "chore: update Nixorium and migrate deployment files"). The
  review token binds the migrated files. Undo is the same as for any update:
  with Git.
- **Protected files.** A migration may never name `flake.nix`, `flake.lock`,
  `lab-credentials.json`, `secret-key`, anything under `keys/` or `admin-ssh/`,
  hidden paths, paths with `..`, absolute paths, or the record file. A list has
  at most 64 migrations, each text at most 64 KiB, each file at most 1 MiB.
  Larger or invalid lists stop the update before anything is written.
- **First migration.** `2026-10-workstation-is-controller` replaces the
  `hostName` comparison in `modules/workstation.nix` with
  `args.isController or false`.

Why data and not scripts: every change to the deployment must be visible in the
update review, reproducible and undoable with Git. New declarative kinds (add a
file, rename a file) can be added when a release needs them.

## Consequences

- The administrator sees what a release changes in their own files before
  saving, and the change is in the Git history like the update itself.
- A deployment that skipped releases still receives every migration it has not
  recorded, because entries are never removed. A released entry is never edited
  either: its id is already recorded in deployments.
- **Limit.** Migrations run only when the installed controller already has a
  release with the mechanism, from this release on. An update from 3.9.0 or
  older to this release cannot apply them, because the old code does not read
  the list. The manual note for `modules/workstation.nix` in the changelog stays
  for that one step.
- A migration is exact text. Each released form of a template file that
  differs needs its own entry; a file the administrator changed in that place
  becomes a "Manual change", which the administrator, or Assistant as a normal
  reviewed proposal, makes by hand. When the missing change makes the
  deployment fail to evaluate, the review reports that evaluation error as for
  any other update.
- Developers must add an entry in the same change as the template change
  (`AGENTS.md`, section "Deployment migrations"), and must test it.
- A corrupt `nixorium-migrations.json` stops the update with a message naming
  the file; the file is restored from Git.

## Alternatives considered

- **Shell scripts that run on update, as Omarchy does.** Rejected: a script
  changes files without being shown in the review, depends on the state it
  finds, and cannot be reproduced or checked before saving. The administrators
  of a school laboratory cannot be expected to read it, and undoing it is not
  the same as undoing a commit.
- **Reset the deployment from the template.** Rejected: it discards the local
  changes the laboratory made on purpose. The reset exists, with its own
  review, for the case where the administrator wants it.
- **Make the core tolerate old patterns.** Rejected as a general rule: it is
  not always possible, as in ADR 0033, where the old test cannot work without a
  name, and it keeps old code in the core for ever.
- **A note in the changelog.** Kept for what cannot be done mechanically, and
  for updates from releases before this mechanism.
