# ADR-0013: Guided software declarations

- Status: accepted
- Date: 2026-09-15

## Context

Laboratory administrators need to add or remove common client software without
understanding Nix expressions. Existing private NixOS modules remain necessary
for advanced configuration, but exposing arbitrary package or module text in a
TUI would turn presentation into a code generator and bypass the deployment's
pinned package set, evaluation, and review boundaries.

Choosing software configuration is also different from deploying it. Every
client may be powered off while the desired configuration is edited. A saved
declaration does not prove that it was committed, built, or activated on any
computer.

## Decision

Store guided declarations in a dedicated versioned `lab-software.json` file.
Its strict Nix and Go schemas allow one declaration per package and exactly
three typed scopes: all clients including future generated clients, one named
evaluated group, or explicit evaluated client identities. Unknown fields,
duplicate identities, unknown groups/clients, and unsupported schema versions
fail closed.

Expose a small curated catalog from `lib/software-catalog.nix`. Include only
entries that resolve to available derivations in the deployment's pinned
package set. The schema evaluator also checks the same allowlist, so direct
file edits cannot use a package that merely happens to exist in nixpkgs.

Apply declarations as an additional NixOS module on clients only. Do not add
them to the controller. Package declarations supplied by framework or private
modules compose normally and are never rewritten or removed by this workflow.
The offline installer bundle carries the normalized managed file and group
definition so it evaluates the same client configurations.

Use shared typed catalog/plan/apply application services for CLI and TUI. Plan
normalizes the request, resolves affected identities, evaluates the complete
candidate through the private deployment's
`nixoriumValidateSoftwareCandidate` hook, and binds the source fingerprint and
candidate bytes to a review token. Apply repeats planning and validation,
rechecks the token and source bytes, locks the deployment root, and atomically
replaces only `lab-software.json` through a no-follow regular-file boundary.

The TUI selects catalog entries and evaluated scopes and requires the exact
generated `SAVE SOFTWARE …` phrase. It contains no Nix or shell construction.
After apply it promotes Git review. It does not commit, build, activate the
controller, prepare PXE, or distribute clients. Those remain separate reviewed
operations.

## Consequences

The common path is usable without Nix knowledge and remains deterministic and
offline after inputs are present. Catalog expansion is an upstream code change
that must be reviewed and validated; it is intentionally not a free-form
package search. Advanced packages and per-package configuration continue to use
private modules.

Changing `lab-software.json` makes the deployment source dirty until the
operator reviews and commits it. No client is contacted by catalog, plan, or
apply. A later explicit distribution selects the powered-on clients for that
intervention and retains its existing revision, build, and verification safety
rails.
