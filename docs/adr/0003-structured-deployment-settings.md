# ADR-0003: Structured deployment settings

Status: accepted

## Context

Existing deployments express site settings as a Nix attribute set. Editing
arbitrary Nix safely from a management application would require a full parser
and would still risk rewriting operator-owned expressions. Configuration must
remain declarative, inspectable, strictly typed, and Git-friendly.

## Decision

New managed deployments use deterministic, versioned `lab-settings.json`.
Their Flake reads its `lab` object with `builtins.fromJSON` and passes the
result to the existing typed `lib.mkLab` boundary. The management application
only writes this owned file, atomically and after schema validation. Existing
`lab-config.nix` deployments remain supported and read-only until the operator
accepts an explicit equivalence-checked migration.

Password input is never stored in plaintext. Salted hashes may remain in the
private declarative deployment as NixOS account inputs. Private signing, SSH,
and Veyon keys remain outside Git and the Nix store; only public keys are
referenced by the deployment.

## Consequences

Configuration has deterministic diffs and a clear migration/version boundary.
The Go and Nix validators must have conformance tests. Advanced deployments may
continue constructing an attribute set in Nix, but the management application
will not attempt to mutate arbitrary expressions.
