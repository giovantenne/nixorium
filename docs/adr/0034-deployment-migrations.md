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

The release that introduced client classes (ADR 0033) made one such change
mandatory: the workstation profile of deployments created before 3.9.0 finds
the controller by comparing `hostName`, and computers that share a
configuration (ADR 0033) have no single name. The note existed, but nothing
applied it.

## Decision

The release being installed carries the code that changes the deployment's own
files. Update Nixorium runs that code on a copy, checks the difference under
fixed rules, shows it in the review and saves it with the update.

- **Migrations are Go code in the release.** `internal/deploymigrate/registry.go`
  holds `Registry`, a list of `Migration{ID, Description, Apply}`. `ID` starts
  with the year and month (`2026-10-short-slug`), `Description` is one sentence
  for the administrator, and `Apply` receives a `Tree` (the deployment copy)
  and returns an outcome. A migration can change, create or remove files
  through `Tree.Read`, `Tree.Write` and `Tree.Remove`. The helper
  `ReplaceExact` covers the common case of one exact text. The first
  migration, `2026-10-workstation-is-controller`, replaces the `hostName`
  comparison in `modules/workstation.nix` with `args.isController or false`.
- **The release being installed runs them.** The installed release does not
  interpret any migration. It builds the target release's Nixorium package
  from the prepared candidate (the same package the controller needs anyway,
  so usually no extra build), reads the marker file
  `share/nixorium/deployment-migrate-contract` (absent means a release before
  migrations: nothing to run), and runs
  `nixorium deployment migrate --root /work --contract 1 --json` from it. It
  runs only when the update changes the Nixorium revision, not for an update
  of the system and packages alone.
- **Sandbox.** The migrator runs in bubblewrap (`--unshare-all --clearenv`)
  with the Nix store read-only, only the copy writable, no network and a
  120-second limit. The copy is an archive of the saved deployment's last
  commit, so it holds no credentials and no keys outside Git.
- **The stable contract.** This is all the installed release knows about the
  target. Exit status 0: it ran (manual items are allowed). 2: unsupported
  contract; the update says to update to an intermediate release first. 3: the
  migrator refuses this deployment, with the reason on standard error (for
  example an invalid record). Anything else, or a timeout, refuses the update.
  The report is JSON:
  `{"contractVersion":1,"targetVersion":"…","migrations":[{"id","description","state","detail","files"}]}`
  with `state` one of `applied`, `done`, `manual`, `skipped`. A new contract
  version is added next to the old one, and the old one stays for a year of
  releases.
- **The difference is the truth.** The report only labels it. The installed
  release compares the copy with the original and accepts only regular files;
  never `flake.nix`, `flake.lock`, `keys/`, `admin-ssh/`,
  `lab-credentials.json`, `secret-key`, hidden paths or symbolic links; at most
  1 MiB per file, 256 files and 16 MiB in total; only the executable bit counts
  as mode (new files are 0644 or 0755, changed files keep their permissions).
  The record file may be created or changed, not removed. Every changed file
  must be declared by an applied migration (except the record) and every
  declared file must have changed; otherwise the update is refused and nothing
  is saved.
- **Outcomes.** `applied` changed the listed files. `done` means the deployment
  already has the change. `manual` means the files were changed by hand and
  nothing was touched; the review shows the migrator's detail (for
  `ReplaceExact`: the file, the text to replace and the text to put instead).
  A manual migration does not block the update by itself: the update is
  refused only when the candidate, built without that change, does not
  evaluate or build, and the review's error says why.
- **Record.** `nixorium-migrations.json`
  (`{"schemaVersion":1,"applied":[...]}`) is written by the target's migrator,
  append-only; ids from newer releases that this release does not know are
  kept. The installed release only compares it like any other file. A
  migration therefore runs once, even if the old text comes back later. This is
  the idea of Omarchy's migration markers, with the record versioned with the
  configuration and reviewed with it.
- **Review and saving.** The review shows "Migration: <description>" with the
  files under it for applied migrations and "Manual change: <description>" with
  the detail for manual ones. The diff of every changed, created or removed
  file is in F4 Details (and in the text plan), not in the summary: the
  administrator decides on the descriptions, and the diff stays in the Git
  history. The
  review token binds the files, the raw report and the migrator's store path.
  Saving writes the files first, each only if still unchanged since the review,
  then `flake.lock` and `flake.nix`; everything is restored if a later step
  fails. The files are committed in the same Git review as the update (commit
  message "chore: update Nixorium and migrate deployment files"). Undo is the
  same as for any update: with Git.
- **By hand.** `nixorium deployment migrate --root DIR [--contract N] [--json]`
  can be run by an administrator on a checkout of the deployment. This is also
  how to start from a release without the mechanism (see Consequences).

## Consequences

- The administrator sees what a release changes in their own files before
  saving, and the change is in the Git history like the update itself.
- Any kind of change is possible without a new release of the installed
  controller: a release can add, change or remove a file in the deployment
  because the release being installed brings its own code. The installed
  release checks the result, not the method.
- A deployment that skipped releases still receives every migration it has not
  recorded, because entries are never removed. A released entry is never edited
  either: its id is already recorded in deployments.
- **Limit.** A controller that runs a release without the mechanism (3.9.0 or
  older) cannot run it during Update Nixorium. For that one step, in the
  deployment folder run
  `nix run github:giovantenne/nixorium/<new tag>#nixorium -- deployment migrate --root .`,
  review the result with `git diff`, save it in Maintenance, Review Git
  changes, then run Update Nixorium. Editing `modules/workstation.nix` by hand
  stays possible.
- A target release that needs a newer contract than the installed release
  knows is refused with "update to an intermediate release first". To limit
  this, a migrator keeps accepting old contract versions for a year of
  releases.
- Running another release's program is a risk. The sandbox and the file
  difference limit it: the program has no network and no credentials, it can
  change only what the rules accept, and all of it is shown in the review
  before anything is saved. When a migration breaks the rules the update fails;
  the administrator keeps the current release and collects a support report.
- A migration written with `ReplaceExact` is exact text. Each released form of
  a template file that differs needs its own entry; a file the administrator
  changed in that place becomes a "Manual change", which the administrator, or
  Assistant as a normal reviewed proposal, makes by hand.
- Developers must add an entry in the same change as the template change
  (`AGENTS.md`, section "Deployment migrations"), and must test it.
- A corrupt `nixorium-migrations.json` makes the migrator refuse the
  deployment (exit status 3) with a message naming the file; the file is
  restored from Git.

## Alternatives considered

- **Changes listed as data (the first design of this decision).** The release
  listed `id`, `file`, `from` and `to` in `lib/deployment-migrations.nix`, and
  the installed release applied them. Replaced: the installed release would fix
  the kinds of operation for ever, and every new kind (add a file, rename,
  remove) would need a new release of the installed controller and a manual
  step again for everyone behind it.
- **Shell scripts that run on update, as Omarchy does.** Close to what is done
  now, with three differences: the code runs in a sandbox with no network or
  credentials, its effect is checked and shown as a diff in the review before
  anything is saved, and it is undone with Git like any commit. A script that
  changes files without being shown, or without limits, was rejected.
- **Reset the deployment from the template.** Rejected: it discards the local
  changes the laboratory made on purpose. The reset exists, with its own
  review, for the case where the administrator wants it.
- **Make the core tolerate old patterns.** Rejected as a general rule: it is
  not always possible, as in ADR 0033, where the old test cannot work without a
  name, and it keeps old code in the core for ever.
- **A note in the changelog.** Kept for what cannot be done mechanically, and
  for updates from releases before this mechanism.
