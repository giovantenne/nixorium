# ADR 0019: Deployment-owned package-base pin

Status: Accepted

The hard channel gate and deferred updater described below are superseded by
[ADR 0020](0020-autonomous-package-base-updates.md). Pin ownership and framework
update isolation remain in effect.

## Context

Nixorium previously owned the only `nixpkgs` input. A private deployment could
select Nixorium releases but had no independent, explicit package-base pin for
future `Check for updates`. Rebuilding an unchanged lock does not search for
new package versions, while updating Nixorium must not silently update the
kernel, libraries, services, or applications.

## Decision

New site templates declare a direct `nixpkgs` input on the supported
`nixos-26.05` channel. `inputs.nixorium.inputs.nixpkgs.follows = "nixpkgs"`
makes the framework, Disko, Veyon, controller, and clients consume that one
deployment-owned source.

Nixorium exports `lib.packageBase` with its supported source and channel. The
template asserts that contract before constructing any deployment and exposes
`nixoriumPackageBase` with schema version, source, channel, and locked revision
for future machine-facing update operations.

`Update Nixorium` remains a framework update. If a deployment has a direct
root `nixpkgs` node, its complete lock node must be byte-semantically unchanged
after candidate-lock normalization. Removal, source/revision/hash changes, or a
non-direct root input fail before candidate builds. Legacy deployments without
a direct pin retain their existing behavior and are not rewritten
automatically.

## Migration and future updates

Existing deployments are compatible and may remain legacy. A later guided
migration must add the direct input initially at the exact effective revision,
verify overlays/private inputs and direct/offline derivation equivalence, then
commit the two declarative files. It must not guess from arbitrary Nix source.

The package-base updater belongs to UX5-M3. It may advance only the managed
root pin within the compatible channel, must review system-wide impact, build
configured roles, apply and verify the controller first, and leave clients
unchanged until explicit distribution. This ADR does not implement that
operation or claim compatibility with an untested newer revision.

## Verification

Go tests prove framework source rewriting preserves the direct input and
`follows` declaration, and that candidate locks cannot change/remove the
deployment-owned node. A targeted offline template evaluation checks that all
consumers follow the root pin and that `nixoriumPackageBase` reports it. No VM
or system build is required for this contract-only increment.
