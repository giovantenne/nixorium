# Updating Nixorium, NixOS and applications

## Ownership and scope

The private deployment owns the desired laboratory configuration and exact
input revisions in `flake.lock`. The installed controller is the management
point, not a remote permission service.

Legacy sites without a direct root nixpkgs pin still inherit their effective
base from the framework. They need the reviewed pin migration below **before**
relying on core-only update isolation; a framework update alone does not migrate
their private flake.

| Operation | Changes | Preserves |
| --- | --- | --- |
| Update Nixorium | Framework source, modules, patches, management program and its auxiliary input graph | Direct deployment nixpkgs lock node |
| Update system and packages | Root nixpkgs revision; channel only when explicitly selected | Nixorium and every other lock node, including transitive private inputs |
| Add/remove software | Package selection and target scope in lab-software.json | Input pins |
| Rebuild/distribute | Installed configuration from the committed deployment | Input pins |

Keeping nixpkgs fixed does **not** guarantee that every installed binary stays
identical during a framework update: Nixorium can change its modules, package
overrides, Veyon input or GNOME patches. Conversely, updating nixpkgs can change
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
Veyon screen sharing/control, audio/video, printing or every hardware driver.
Record those practical checks and the tested machines.

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

System/package planning requires a direct stable
`github:NixOS/nixpkgs/nixos-YY.05` or `nixos-YY.11` input and the explicit
root follows relationship. Custom URLs, unstable channels, channel downgrades,
ambiguous expressions and legacy layouts require a manual reviewed operation.

The system updater compares **every other node** in the candidate lock graph.
Unexpected changes, including transitive dependencies, fail closed. It checks
deployment readiness, builds the controller and representative client variants
(`nixoriumUpdateTargets`): distinct managed-software selections, Veyon modes,
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
readiness, builds, offline consistency or a stale review token. The legacy
framework update path retains its controller/representative-client and
installation-artifact checks; it is not the new variant-aware base validation.

## Existing deployments: one-time, reviewed adoption

Updating a framework input does not overwrite private template files.

For a deployment already owning root nixpkgs:

1. Adopt a Nixorium revision containing this feature while keeping nixpkgs
   unchanged. Ensure the installed management executable is that version.
2. Remove only the old `packageBaseCompatible` channel-comparison binding and
   its assertion from the private flake. Preserve local policy, modules,
   assets and validation hooks. Do not remove real module/schema assertions.
3. Keep `inputs.nixpkgs.url` and
   `inputs.nixorium.inputs.nixpkgs.follows = "nixpkgs";`.
   Update optional package-base metadata following the new template.
4. Expose the `mkLab` outputs unchanged (normally `deployment // { ... }`
   already does this). Validate the effective lock and both update outputs,
   run offline equivalence and compare systems before/after this adoption.
5. Review and commit the migration before planning a base update.

For a legacy deployment without root nixpkgs, **first migrate without upgrading
the effective package base**. Record the current framework nixpkgs source,
revision and hash by resolving its lock graph (including follows paths).
Add the direct declaration and follows edge, generate a temporary candidate
lock pinned to that exact revision, preserve the appropriate channel in the
original source metadata, and inspect every changed node. Nix may rename nodes;
do not replace the whole lock blindly. Compare direct/offline derivation paths,
build the configured systems, review and commit. Only then request a newer
base. This structural migration is intentionally manual: arbitrary private
Nix expressions cannot safely be rewritten by a textual updater.

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
