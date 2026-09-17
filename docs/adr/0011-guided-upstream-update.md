# ADR-0011: Guided upstream release updates

- Status: accepted
- Date: 2026-09-14

## Context

A private deployment pins Nixorium through its `nixorium` Flake input. Updating
that pin can change the input declaration, lock graph, configuration schema,
controller and client systems, netboot artifacts, and offline installer bundle.
A direct `nix flake update` mutates the checkout before compatibility is known,
while copying the worktree for a proposal risks including ignored private key
material. Tracking a moving branch is unsuitable as the default production
upgrade policy, but maintainers need an explicit way to validate and adopt the
latest development revision.

The controller may use the internet for an explicitly requested update, but
routine management and every client must remain independent of it. Updating a
repository must not imply committing, pushing, activating, or deploying.

## Decision

Use a typed check/plan/apply workflow. Preserve the upstream source identity
already present in one simple `inputs.nixorium.url` string assignment, and
accept only the exact `master` branch or a `v`-prefixed Semantic Version tag as
the target reference. Label `master` as a moving Development target rather than
presenting it as a release.
Distinguish stable, prerelease, moving-current-reference, and downgrade states;
require explicit opt-ins for prerelease and downgrade targets. Refuse computed
or ambiguous input declarations instead of rewriting arbitrary Nix syntax.

Release discovery is optional and bounded. It disables Git credential helpers
and prompting plus user/system Git configuration, and is the only update
command that enumerates the remote. It queries the configured GitHub identity
over HTTPS, stops after 15 seconds, accepts at most 256 KiB of references,
queries only `refs/heads/master` and `refs/tags/v*`, and returns `master` plus at
most the newest 20 stable and 20 prerelease SemVer tags. An explicit target can
proceed directly and lets Nix resolve that exact branch or release.

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

The explicit CLI `update apply` operation leaves the two files as uncommitted
changes so automation retains the plan/apply boundary. The ordinary TUI wraps
that operation in a bounded application save which records exactly
`flake.nix` and `flake.lock` in the local deployment history with Nixorium's
fixed internal identity. Git tokens, hashes, staging, and commit terminology
are not part of that ordinary interaction. It never creates branches, merges,
pushes, controller activations, PXE preparations, or client deployments.

If the two writes succeed but local recording does not, the TUI reports a
recoverable partial save. Retry records the files only when they still match
the exact reviewed proposal and no unrelated deployment path changed.

CLI and TUI reuse the same typed plan and apply operations. The TUI owns only
explicit target/policy entry, bounded patch presentation, exact confirmation,
and result rendering; its application callback owns the transparent local
save. Presentation code never reconstructs commands or performs Nix,
filesystem, or repository operations itself. It prevents accidental exit while
the mutating save callback is in progress.

## Consequences

Planning is intentionally expensive, may need controller internet access, and
may populate the shared Nix store. Repeating it during apply normally reuses
cached outputs. The main management interface remains offline-capable, and the
client runtime/network boundary does not change. Selecting `master` deliberately
trades release immutability for the newest upstream development revision, while
retaining the same validation and review boundary.

The managed path initially supports the generated deployment's simple input
assignment. Advanced computed inputs retain the documented manual update
procedure. Two independent files cannot be replaced in one filesystem rename;
the ordering, rollback attempt, explicit partial state, and mismatch recovery
make that narrow crash window visible instead of pretending atomicity.
