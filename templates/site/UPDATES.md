# Updating Nixorium, NixOS and applications

## Ownership and scope

The private deployment owns the desired laboratory configuration and exact
input revisions in `flake.lock`. The installed controller is the management
point, not a remote permission service.

| Operation | Changes | Preserves |
| --- | --- | --- |
| Update Nixorium | Framework source, modules, patches, management program and its auxiliary input graph | Direct deployment nixpkgs lock node |
| Update system and packages | Root nixpkgs revision; channel only when explicitly selected | Nixorium and every other lock node, including transitive private inputs |
| Add/remove software | Package selection and target scope in lab-software.json | Input pins |
| Rebuild/distribute | Installed configuration from the committed deployment | Input pins |

Keeping nixpkgs fixed does **not** guarantee that every installed binary stays
identical during a framework update: Nixorium can change its modules, package
overrides or GNOME patches. Conversely, updating nixpkgs can change
kernel, drivers, desktop, services and ordinary applications together. A
package's displayed upstream version can stay equal while its dependencies
or build change.

`lib.packageBase` describes the upstream reference source/channel and the
revision in the framework source's own lock. It is advice, not an authorization
gate or a certification of every revision in that channel.
`nixoriumPackageBase` exposes effective revision/hash separately.
`nixorium package-base status` inspects the actual declaration, root lock node
and the framework's follows edge, rather than trusting a duplicated string.

## Before any update

- Back up the private deployment, separately protected secrets, and application
  data. A NixOS generation rollback does not restore migrated databases.
- Commit or resolve unrelated work. The updater rejects a dirty deployment.
- Keep an accessible previous generation and recovery access. Do not run
  garbage collection as part of an upgrade.
- Schedule disruption, check active sessions, free disk space and connectivity.
  Planning performs real builds and may download substantial data.
- Read the relevant NixOS/framework release notes. Leave
  `system.stateVersion` unchanged; it is not the selected release.

## TUI: ordinary operator journey

Open Maintenance:

1. **Update Nixorium** discovers the configured upstream's releases. Select
   the release and validate it. This never refreshes the root nixpkgs pin.
   Releases older than the configured release are hidden, including older
   prereleases. When the deployment follows `master` or a commit, which have
   no Semantic Version ordering, the running Nixorium version is the floor
   instead: older releases are hidden and the screen shows the running version.
   A release equal to the running version stays available. The TUI never
   authorizes a downgrade. Development builds without a release version apply
   no floor.
2. **Update system and packages** shows the current channel and revision.
   Enter resolves/builds a newer revision in that same channel. It does not
   discover or silently select another NixOS release.
3. For a channel change, press **m**, edit the target, and press **Space** to
   accept unverified channel compatibility. Enter validates the proposal.
   A future channel name is only a requested target, not proof it exists or
   is supported. Resolution/evaluation/build failures still block.
4. Review the bounded flake diff and checks. Enter explicitly saves the
   reviewed files and records them in local Git, then activates/verifies the
   controller through the existing managed controller operation. The review
   can also list changes the new release makes to your own files (see
   [Changes to your own files](#changes-to-your-own-files)).
5. If saving fails, recover saving first. If controller activation fails after
   saving, the desired configuration is saved but the running controller is
   not verified; retry controller activation, not a new input update.
6. Reopen the TUI after updating its executable. If kernel/boot changes require
   it, arrange a reboot separately and verify services afterward.
7. Verify a selected client before explicitly distributing to the remaining
   computers. No update saves, pushes, reboots, starts PXE or deploys clients
   without the corresponding operator action.
8. Refresh PXE preparation before using the new configuration for installations.
   Previously prepared artifacts can be stale after either kind of update.

A successful controller switch proves the managed service completed and the
active closure matches. It does not prove a subsequent boot, graphical login,
the classroom view, audio/video, printing or every hardware driver.
Record those practical checks and the tested machines.

## Update notifications

After the administrator opens the TUI, a background check reads the configured
Nixorium input and queries its public upstream at most once every 24 hours.
Hourly polling reuses a local cache, including failed attempts, across TUI
restarts. No process runs while the TUI is closed; reopening performs a due
check. Discovery is bounded, does not evaluate Nix, and never applies an update.
The classroom dashboard does not run it.

The Overview notice follows the chosen channel: newer master revisions, newer
stable tags for a stable pin, or newer prerelease tags for a prerelease pin.
It does not switch channels. `u` opens the ordinary fresh update check and review;
`x` dismisses that exact version/revision. Another version can notify again.
Acknowledgement, the advisory cache and dismissal are private local UI state
under `$XDG_STATE_HOME/nixorium/dashboard` (normally `~/.local/state/nixorium/dashboard`),
scoped to the administrator and deployment, outside Git and the Nix store.
Network failures do not interrupt work; the last successful observation may
remain visible until the next successful check.

## CLI: advanced reviewed workflow

From the private deployment:

```sh
nixorium update check
nixorium update plan --target vX.Y.Z
nixorium update apply --target vX.Y.Z --expect <review-token> --yes

nixorium package-base status
nixorium package-base plan
nixorium package-base apply --expect <review-token> --yes
```

Replace placeholders with real reviewed values. For a deliberate channel
migration (the example does not assert release availability):

```sh
nixorium package-base plan --target nixos-26.11 --allow-unverified
nixorium package-base apply --target nixos-26.11 --allow-unverified --expect <review-token> --yes
```

Unlike the ordinary TUI save, CLI apply writes only `flake.nix`,
`flake.lock` and the files the release changes in your own files, with the migration record (see
[Changes to your own files](#changes-to-your-own-files)). Review/commit those files locally, then use `controller plan`
and `controller apply --expect <reviewed-git-revision>`. Use the existing
`deploy plan --on pc01` / `deploy apply` workflow separately for clients.
No command above pushes a repository.

CLI apply reconstructs and validates a candidate before comparing its token.
If a moving channel advanced since review, apply refuses the changed proposal;
create a fresh plan and review it. The TUI keeps its exact candidate in memory.
Both paths bind before/after bytes, HEAD and policy; changed files or HEAD
invalidate the review. Candidate lock files are temporary and never written
over the deployment during planning. The candidate `flake.lock` is computed from the
proposed `flake.nix` and refused unless Nix accepts the pair unchanged; if an
update saved by Nixorium 3.8.0 or older left them out of step (`PIN-OUTDATED`),
plan the configured release again to repair them before updating the computers.

## Validation and remaining gates

For an existing prepared student workspace, both update planners compare its
current and proposed pinned packages, extensions, dependencies and effective
preferences. CLI JSON exposes these as `workspace.current` and
`workspace.proposed`; text and TUI review show a scrollable comparison before
the flake diff. This is not the currently running home state or a search for the
vendor's latest release. Equal version strings can still hide rebuilt
dependencies; review the pin diff too.

The comparison and both validation hooks are bound to the update review. A
candidate that removes existing workspace metadata, changes boot behavior,
the student identity or destinations is blocked pending separate configuration review.
Updating does not change the profile or migrate local home customizations.

To update packaged VS Code extensions, use **Update system and packages** (or
`package-base plan`/`apply`), not repeated profile saves or downloads in student
homes. The package-base pin also controls the editor, desktop and operating
system. After the reviewed update, qualify actual plugin loading on the
controller and a selected client before wider distribution. Build success is
not plugin-runtime certification; managed defaults disable editor/extension
auto-update checks, and the next ordinary boot reset installs the new seed.

System/package planning requires a direct stable
`github:NixOS/nixpkgs/nixos-YY.05` or `nixos-YY.11` input and the explicit
root follows relationship. Custom URLs, unstable channels, channel downgrades,
and ambiguous expressions require a manual reviewed operation.

The system updater compares **every other node** in the candidate lock graph.
Unexpected changes, including transitive dependencies, fail closed. It checks
deployment readiness, builds the controller and representative client variants
(`nixoriumUpdateTargets`): distinct managed-software selections,
interfaces and every explicit host module. Private modules branching on host
identity must declare additional `updateValidationHosts = [ "pc03" ];` in
their `mkLab` arguments, or list that computer under `hostModules`; these hosts
also enter offline validation. Without it, evaluation stops with an error
because a system shared by several computers has no `hostName` or `hostIp`.
Identical client computers share one system (a class), so the update builds it
once rather than once per computer. The plan and the review show, for example,
"Builds  2 configurations for 20 computers".

Before it switches a computer to a shared system, the update writes the
computer's identity record (`/var/lib/nixorium/identity.json`, only its number)
over SSH and checks the number again afterwards. A record that names another
computer is never overwritten: that computer stays unchanged and the result says
"identity record names another computer" (see
[Troubleshooting](troubleshooting.md#the-update-says-identity-record-names-another-computer)).
To go back, choose the previous generation in the computer's boot menu.
In laboratory mode the plan also evaluates the netboot, PXE firmware and
installer outputs without building them: they are built when network
installation is prepared (Installation → Network boot), which keeps the review
short. Controller-only mode builds no client/PXE
system. `nixoriumOfflineCheck` compares the selected systems' derivation paths
against evaluation from the standalone installer in an isolated offline store.
Neither output should be overridden to bypass failures.

Build success remains explicitly runtime/hardware-unverified. No
`--allow-unverified` flag bypasses input integrity, clean-state checks,
readiness, builds, offline consistency or a stale review token.

## Changes to your own files

Some of your deployment's files, such as `modules/workstation.nix`, are copied
from a template when the laboratory is created and then belong to you. When a
release needs a change in one of them, the release being installed brings the
program that makes it, and **Update Nixorium** runs that program as part of the
update. You see the result before you save.

How it works, in short: Update Nixorium makes a copy of your saved deployment
(without passwords or keys), runs the new release's program on the copy in a
closed sandbox (no network, 2 minutes at most), and compares the copy with your
files. It accepts only plain changes to your own files: never `flake.nix`,
`flake.lock`, keys, credentials or hidden files, and at most 256 files of
1 MiB each. It does this only when the update changes the Nixorium release, not for
an update of the system and packages alone. The review shows, for each change:

- `Migration:` the files and, in one sentence, why they change. The change is
  also in the diff, together with any other file the release creates or
  removes.
- `Manual change:` the release could not make this change automatically,
  because you edited that place or the file is missing. Nothing in that file is
  touched. The line under it says what to do, for a text replacement
  `Replace "…" with "…".` Make that change yourself in a normal reviewed
  change, and update again; Assistant can help prepare it as a proposal for you
  to review.

Saving the update writes the changed files before `flake.lock` and `flake.nix`,
and only if each file is still exactly as you reviewed it; otherwise the save is
refused, and files already written are put back if a later step fails. The
changed files are committed with the update in **Maintenance → Review Git
changes** (message "chore: update Nixorium and migrate deployment files"). To
go back, undo the saved change in Git as for any other update.

The file `nixorium-migrations.json` records which changes were applied, so each
one is made only once, even if the old text appears again later. The new
release writes it, and it is committed with the rest. Do not edit it by hand. A
change that your file already contains is recorded as done without touching the
file.

If the new release's program fails, changes something it did not declare, or
needs a newer way of migrating than your release knows, the update is refused
and the current release stays; see
[the update is refused](troubleshooting.md#the-update-is-refused-because-of-the-new-releases-changes-to-your-files)
in the troubleshooting guide.

**Updating from a release without this feature** (3.9.0 or older). Such a
release cannot run the new program during the update. Once, in the deployment
folder, run the new release's program yourself:

```sh
nix run github:giovantenne/nixorium/<new tag>#nixorium -- deployment migrate --root .
```

It prints what it applied and what you must do by hand. Review the result with
`git diff`, save it in **Maintenance → Review Git changes**, and then run
**Update Nixorium**. The release notes of that release say which of your files
it changes, in case you prefer to edit them by hand.

## Updating one application independently

Software selection resolves against the effective pinned package set. To
advance normal packages together, use Update system and packages. There is no
managed “update only this package's version” command: refreshing nixpkgs is a
whole-base operation even when motivated by one application.

For a genuinely independent application version, maintain a narrow, pinned
package/overlay in the deployment and review its source/hash and dependencies.
Use existing `sharedModules` / `hostModules` and local modules/assets; keep
their source paths inside the deployment so the standalone installer includes
them. Build affected roles and run offline equivalence. Independently pinned
packages do not automatically advance with the base updater.

## Recovery and long-term autonomy

A failed plan leaves desired files and running machines unchanged. A partial
save requires inspection/recovery before another update. If a saved update
cannot run, restore the previous desired configuration through a reviewed Git
change and activate the known-good system; use a previous boot generation when
the controller cannot start. Keep data migrations and `stateVersion` outside
this automatic recovery promise. `nixorium git undo` goes back from any other
last saved change (settings, software, workspace) but refuses a change that
updated Nixorium or the package base.

A laboratory can select and validate a newer NixOS base without an upstream
Nixorium release. If NixOS removes an option or a patch no longer applies, that
is a real technical incompatibility: repair a narrow local module/override or
maintain a fork. Autonomy does not make old code compatible with all future
systems.

Archive/mirror the framework, nixpkgs and other pinned source inputs; retain
lock files and required store closures/cache signatures as well as deployment
backups. A lock alone cannot retrieve a vanished upstream. Custom mirrors may
require manual source configuration; the guided updater deliberately accepts
only its supported canonical GitHub layout. Keep mirror adoption separate
from the base-update transaction and validate offline installations again.
