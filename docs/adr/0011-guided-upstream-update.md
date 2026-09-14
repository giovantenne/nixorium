# ADR-0011: Guided upstream release updates

- Status: accepted
- Date: 2026-09-14

## Context

A private deployment pins Nixorium through its `nixorium` Flake input. Updating
that pin can change the input declaration, lock graph, configuration schema,
controller and client systems, netboot artifacts, and offline installer bundle.
A direct `nix flake update` mutates the checkout before compatibility is known,
while copying the worktree for a proposal risks including ignored private key
material. Tracking a moving branch is also unsuitable as the normal production
upgrade policy.

The controller may use the internet for an explicitly requested update, but
routine management and every client must remain independent of it. Updating a
repository must not imply committing, pushing, activating, or deploying.

## Decision

Use a typed check/plan/apply workflow. Preserve the upstream source identity
already present in one simple `inputs.nixorium.url` string assignment, and
accept only a `v`-prefixed Semantic Version tag as the target reference.
Distinguish stable, prerelease, moving-current-reference, and downgrade states;
require explicit opt-ins for prerelease and downgrade targets. Refuse computed
or ambiguous input declarations instead of rewriting arbitrary Nix syntax.

Release discovery is optional and bounded. It disables Git credential helpers
and prompting, and is the only update command that enumerates the remote. An
explicit target can proceed directly and lets Nix resolve that exact release.

During planning, generate the candidate lock outside the checkout with Nix's
`--output-lock-file`. Evaluate and build against that candidate using an exact
input override, `--reference-lock-file`, and `--no-write-lock-file`. Require
deployment metadata/readiness evaluation and no-link builds of one client, the
controller, netboot ramdisk, PXE firmware, and installer bundle before emitting
a ready plan. Bound the rendered source/lock patch and bind HEAD, target,
original bytes, candidate bytes, and generated confirmation to a SHA-256 token.

Apply repeats the plan, then serializes with other deployment-root writers and
rechecks clean Git, HEAD, and both source digests. Use fsynced sibling temporary
files and replace the lock before the input declaration. Attempt rollback on an
in-process second-write failure and report any two-file inconsistency as partial
and not blindly retry-safe. A later plan detects and can repair a mismatched
source/lock pair.

Leave the two files as uncommitted changes. Reuse the separate Git review and
optional commit workflow if the administrator wants to record them. Never
create branches, commits, merges, pushes, controller activations, PXE
preparations, or client deployments from update apply.

## Consequences

Planning is intentionally expensive, may need controller internet access, and
may populate the shared Nix store. Repeating it during apply normally reuses
cached outputs. The main management interface remains offline-capable, and the
client runtime/network boundary does not change.

The managed path initially supports the generated deployment's simple input
assignment. Advanced computed inputs retain the documented manual update
procedure. Two independent files cannot be replaced in one filesystem rename;
the ordering, rollback attempt, explicit partial state, and mismatch recovery
make that narrow crash window visible instead of pretending atomicity.
