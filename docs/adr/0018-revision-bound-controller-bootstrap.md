# ADR 0018: Revision-bound controller bootstrap

Status: Accepted

## Context

The public bootstrap selected `master` or a release tag, then independently
resolved the deployment template, controller installer, Disko layout, and
deployment lock. A moving branch could advance between those requests. The
installed system could therefore contain individually valid files that never
belonged to one upstream revision.

The configured channel must remain visible in `flake.nix`, because managed
updates distinguish `master`, stable releases, and prereleases.

## Decision

Resolve the selected GitHub ref once through the commits API and require a full
40-character object ID. Use that object ID for:

- the site template source;
- the downloaded controller installer;
- the downloaded Disko layout;
- the initial `nixorium` lock override.

Keep the selected channel or tag in the generated `flake.nix`. The exact lock
revision makes the initial deployment immutable; the declared ref preserves the
operator's update channel. Pass the already downloaded layout to current
installers as a local regular file and also pass its immutable URL for
compatibility with already published installers. A separate
`NIXORIUM_INSTALLER_REF` is accepted only when it equals the selected ref, so it
cannot split the trust boundary.

The lower-level installer accepts a local layout. Its compatibility URL is
still explicit; automatic URL derivation is permitted only from a full
revision-pinned GitHub flake reference, never from an implicit `master`.

## Consequences

Bootstrap now requires the commits API in addition to raw GitHub and Nix cache
access. Resolution, capability inspection, or malformed API output fails before
disk installation. A tag owner could still move a tag before resolution, but
every artifact used after resolution remains bound to the one returned object
ID. Current capability-aware installers collect credentials and regional
settings before Nix work; legacy releases retain their published post-install
flow.

## Verification

The controller bootstrap contract test uses no network. It supplies a mocked
GitHub response, verifies every raw download and the site template use the same
object ID, verifies the lower-level installer receives that revision for its
lock override, checks the declared channel remains `master`, and proves a
mismatched installer ref fails before any network call. Tiered validation runs
this test in every mode.
