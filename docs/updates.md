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
2. **Update system and packages** shows the current channel and revision.
   Enter resolves/builds a newer revision in that same channel. It does not
   discover or silently select another NixOS release.
3. For a channel change, press **m**, edit the target, and press **Space** to
   accept unverified channel compatibility. Enter validates the proposal.
   A future channel name is only a requested target, not proof it exists or
   is supported. Resolution/evaluation/build failures still block.
4. Review the bounded flake diff and checks. Enter explicitly saves the
   reviewed files and records them in local Git, then activates/verifies the
   controller through the existing managed controller operation.
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

Unlike the ordinary TUI save, CLI apply writes only `flake.nix` and
`flake.lock`. Review/commit those files locally, then use `controller plan`
and `controller apply --expect <reviewed-git-revision>`. Use the existing
`deploy plan --on pc01` / `deploy apply` workflow separately for clients.
No command above pushes a repository.

CLI apply reconstructs and validates a candidate before comparing its token.
If a moving channel advanced since review, apply refuses the changed proposal;
create a fresh plan and review it. The TUI keeps its exact candidate in memory.
Both paths bind before/after bytes, HEAD and policy; changed files or HEAD
invalidate the review. Candidate lock files are temporary and never written
over the deployment during planning.

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
their `mkLab` arguments; these hosts also enter offline validation. Identical
client roles reuse a representative rather than rebuilding the entire fleet.
The plan also builds netboot, PXE firmware and installer outputs
when laboratory mode needs them. Controller-only mode builds no client/PXE
system. `nixoriumOfflineCheck` compares the selected systems' derivation paths
against evaluation from the standalone installer in an isolated offline store.
Neither output should be overridden to bypass failures.

Build success remains explicitly runtime/hardware-unverified. No
`--allow-unverified` flag bypasses input integrity, clean-state checks,
readiness, builds, offline consistency or a stale review token.

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
this automatic recovery promise.

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
