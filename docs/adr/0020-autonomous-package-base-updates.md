# ADR 0020: Laboratory-owned system updates

Status: implemented and validated. Supersedes the channel gate in ADR 0019.

## Reanalysis

The direct deployment nixpkgs pin and the framework updater's preservation of
that pin are sound. The template's comparison with a duplicated channel string
is not: it compares upstream claims rather than the effective input, and makes
future channel changes depend on a framework release. A successful build is
evidence of build compatibility, not a hardware or runtime certification.
Veyon sources, Disko and local GNOME patches also have independent lifecycles;
sharing nixpkgs does not imply that every component comes from nixpkgs itself.

## Decision and implementation plan

1. Keep framework updates and system/package updates separate. Framework
   updates preserve the direct nixpkgs node. System updates preserve every
   other lock node, including the framework and its auxiliary sources.
2. Replace the template's hard channel assertion with informative upstream
   support metadata. Inspect the actual direct GitHub nixpkgs input and its
   lock node, including source, ref, revision, hash and follows relationship.
   Existing private files are never silently migrated.
3. Add `package-base status`, `plan`, and `apply`. An ordinary plan advances
   the current channel. A channel change requires an explicit target and
   acknowledgement of unverified compatibility. No name alone certifies a
   revision. Failures in evaluation, builds or input integrity cannot be
   overridden. Changing `system.stateVersion` is not part of an update.
4. Reuse reviewed update storage and controller activation. Candidate locks
   live outside the checkout. Review binds the exact before/after bytes, HEAD,
   update kind and policy. Apply rechecks that binding; CLI apply changes
   declarations only, while the ordinary TUI records the change locally and
   activates/verifies the controller. Clients require explicit distribution.
5. Validate configured roles and host-specific variants. Expose deployment
   validation targets and an offline-equivalence check so private modules and
   packages participate. Preserve controller-only deployments with no clients.
6. Add Maintenance → Update system and packages, with current pin, same-channel
   updates and an explicit channel migration form. Reuse progress, bounded
   review, save recovery and controller activation. Software continues to mean
   package selection; versions are advanced together through the package base.
7. Document migration, recovery, source mirroring and test evidence. Keep
   architecture, administrator guide, template and distributed skills aligned.

## Operator journey

Framework: discover releases → review/build exact candidate → save → activate
controller → observe → distribute explicitly to clients.

System/packages: inspect actual pin → choose current or another channel →
acknowledge an unverified channel → resolve exact candidate → evaluate/build
roles and installation consistency → review → save → activate controller →
reboot if required and perform practical checks → distribute to selected
clients, then the remaining fleet. Refresh PXE preparation before installing
new machines. No unattended distribution, push, reboot or PXE start is implied.

Adding/removing software changes declarations and scopes, not the package-base
revision. Updating a single application independently requires a separately
pinned, deployment-owned package/overlay and its own review; the system updater
must never present a whole-base update as an isolated application update.

## Compatibility and older deployments

Unverified means not certified by this upstream, even when every build passes.
The deployment administrator may validate a new channel independently. Real
module/API or patch incompatibilities still need correction; no flag bypasses
them. Narrow package overrides are preferable to copying the entire framework.

An old template with a direct pin can adopt the new metadata/output contract
through a reviewed local edit preserving site policy. A legacy deployment
without a root pin first needs an explicit migration at its exact effective
revision, followed by direct/offline derivation comparison. The updater refuses
to guess such a migration or rewrite arbitrary Nix expressions.

## Validation gates

- Deterministic tests: input integrity, channel policy, lock isolation, stale
  review, no-op updates, partial-save recovery, CLI flags and TUI reachability.
- Nix evaluation: support metadata, selected roles/variants, controller-only
  mode and offline equivalence.
- Management VM: command execution/storage boundary with deterministic fake
  Nix candidates; no external update or real lab operation.
- Full checkpoint: representative systems, netboot, installer and VM tests.
- Manual evidence: actual updated controller/client boot and services. These
  remain unverified until a named run is recorded; builds cannot close them.

## Implementation and recorded evidence

The seven implementation steps above are present in the working tree:

- `internal/adapters/package_base.go`: effective-input inspection, isolated
  candidate lock and whole-graph non-nixpkgs preservation.
- `internal/app/package_base.go`: explicit channel policy and reviewed plans;
  the shared updater/save boundary rejects changed proposals and stale HEADs,
  including partial-save recovery.
- `cmd/nixorium` and `internal/presentation/package_base.go`: CLI and Maintenance
  entry point, channel form, progress, review and shared controller recovery.
- `lib/mk-lab.nix`: representative role/variant targets, extra
  `updateValidationHosts`, and an isolated offline derivation comparison.
- New template metadata and the complete [update guide](../updates.md), also
  distributed as the deployment's `UPDATES.md`; maintainer instructions agree.

Validation on 2026-09-22:

- `go test ./...` and `go vet ./...`: passed, including graph integrity,
  migration acknowledgement, stale review, no-op/failing builds and save retry.
- `scripts/validate.sh --eval`: passed (quick gate plus complete mkLab contract).
- `scripts/validate.sh --quick`: passed after recovery fixes.
- TUI render/state tests: passed at 80x24, 120x30 and 180x45, light/dark,
  ASCII/ANSI/ANSI256, including migration, review, failure and recovery.
- Both distributed maintainer skills: validator and copy equality passed.
- Real `nixoriumOfflineCheck` build: passed without network access in its
  builder; source metadata and representative outputs evaluated successfully.
- Management VM: passed, including real CLI execution against deterministic
  base candidates, clean-plan behavior, policy rejection, stale-token rejection
  and reviewed two-file apply; the final recovery code was tested separately
  through `checks.x86_64-linux.management-vm` as well.
- `scripts/validate.sh --full`: passed, including the client installation/boot
  VM, management VM, real representative/controller/netboot builds, fresh
  template CLI scenarios and direct/offline derivation equivalence.
- Final same-revision channel-migration edge case: covered by the adapter
  regression suite; `go test ./...` and `go vet ./...` repeated successfully
  after the full checkpoint, followed by a successful final `--quick` gate.
  No second full VM run is claimed for that guard.

No physical controller/client upgrade or future-channel certification is
claimed. Legacy-layout migration is a documented, manually reviewed operation,
not an implemented automatic transformer. Custom mirrored inputs and independent
single-application pins remain deployment-owned manual operations.

## Continuity

Keep source archives/mirrors, lock files, public configuration backups and
required store paths. Support labels are advice rather than remote permission.
An unavailable upstream does not prevent using archived inputs. Unknown future
NixOS APIs may still require local module fixes or a maintained fork.
