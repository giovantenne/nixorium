# ADR-0013: Guided software declarations

- Status: accepted, amended for pinned package search
- Date: 2026-09-15

## Context

Laboratory administrators need to add or remove common client software without
understanding Nix expressions. Existing private NixOS modules remain necessary
for advanced configuration, but exposing arbitrary package or module text in a
TUI would turn presentation into a code generator and bypass the deployment's
pinned package set, evaluation, and review boundaries.

Choosing software configuration is also different from deploying it. Every
client may be powered off while the desired configuration is edited. A saved
declaration is recorded in local configuration history, but does not prove that
it was built or activated on any computer.

## Decision

Store guided declarations in a dedicated versioned `lab-software.json` file.
Its strict Nix and Go schemas allow one declaration per package and exactly
three typed scopes: all clients including future generated clients, one named
evaluated group, or explicit evaluated client identities. Unknown fields,
duplicate identities, unknown groups/clients, and unsupported schema versions
fail closed.

Accept an optional deployment-owned catalog as suggestions, not as an
allowlist. The generated private template keeps it in `software-catalog.nix` so
administrators can evolve local recommendations without an upstream release.
Search and exact resolution run through deployment outputs backed by its locked
nixpkgs input, laboratory overlays, and effective package policy. The built-in
Nixorium systems permit unfree packages, so deployment search and validation
apply that same policy instead of reporting those packages as blocked. Requests
cross the adapter as JSON data and attribute paths are split and resolved
structurally; user input is never interpolated into Nix or shell code. Results
identify broken, insecure, policy-blocked, and platform-incompatible
derivations instead of silently widening package policy.

Apply declarations as an additional NixOS module on clients only. Do not add
them to the controller. Package declarations supplied by framework or private
modules compose normally and are never rewritten or removed by this workflow.
The offline installer bundle carries the normalized managed file and group
definition so it evaluates the same client configurations.

Use shared typed catalog/search/plan/apply application services for CLI and
TUI. Dotted attribute paths support nested package sets. Plan resolves additions
again from the locked inputs, normalizes the request, resolves affected
identities, evaluates the complete candidate through the private deployment's
`nixoriumValidateSoftwareCandidate` hook, and binds the source fingerprint and
candidate bytes to a review token. Apply repeats planning and validation,
rechecks the token and source bytes, locks the deployment root, and atomically
replaces only `lab-software.json` through a no-follow regular-file boundary.

The TUI selects catalog entries and evaluated scopes and saves the reversible
declaration with Enter after one explicit review. The generated review token
remains an internal binding; the advanced CLI retains its explicit token and
confirmation contract. After that single review, the application atomically writes and records only
`lab-software.json`; Git remains an internal storage detail. A pre-existing
change to that file blocks the operation, while a failed record after a
successful write can be retried without applying the declaration twice. The
flow does not build, activate the controller, prepare PXE, or distribute
clients. Those remain separate reviewed operations.

## Consequences

The common path is usable without Nix knowledge and remains deterministic and
offline after inputs are present. Suggested entries remain intentionally small,
while name search can select other derivations already available to the
deployment. Packages requiring service configuration or module options still
belong in private modules; adding a derivation promises only a system package.

The guided flow leaves `lab-software.json` clean in the local repository and
does not expose history mechanics to the operator. No client is contacted by
catalog, plan, or save. A later explicit distribution selects the powered-on
clients for that intervention and retains its existing revision, build, and
verification safety rails. Advanced and automation callers can still use the
lower-level plan/apply and explicit history commands independently.
