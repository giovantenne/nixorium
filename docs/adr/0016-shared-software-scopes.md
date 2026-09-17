# ADR 0016: Explicit shared and controller software scopes

Status: Accepted

## Decision

Extend software schema version 1 with two explicit scope kinds:

- `shared`: the controller and all current or subsequently configured clients.
- `controller`: only the controller.

Preserve `all-clients`, `group` and `clients` exactly. No read, save or upstream
upgrade silently broadens an old declaration to include the controller.
One package has one scope. Named groups and explicit identities remain
client-only; controller-only deployment mode does not invent a client.

Apply the managed package module to both roles, resolving attributes against
each host's pinned package set and overlays. Never copy the controller closure
to clients. The offline bundle retains the complete declaration and role
mapping. Downstream modules keep their normal NixOS merge behavior.

`nixoriumSoftware.controller` advertises the controller identity and support
for the new scopes. Without it, the application retains legacy choices and
rejects controller requests. Invalid or overlapping controller/client
identities fail closed. Older upstreams reject the new scope kinds: remove or
explicitly convert them before downgrading; no implicit migration is provided.

The Go application composes the existing site software validator with
`nixoriumValidateControllerSoftwareCandidate` from `mkLab` whenever existing or
candidate declarations include the controller. The latter evaluates a candidate
controller toplevel while preserving private modules, assets and host policy.
This also protects old site templates whose own hook evaluates only clients.
Removal must run the same checks; a missing required hook is an error.

Review reports `affectedController` separately from `affectedClients`. A scope
change affects the union of old and new destinations, including computers that
lose the package. The content-bound review token includes those destinations;
an inventory/group change requires fresh review even if candidate bytes match.

## Workflow boundary

This increment provides the declaration and validation foundation. CLI apply
only saves the declaration. TUI saving also records that file locally, with
clear pending-controller copy. Neither builds, activates or distributes systems.
The combined save/build/activate/verify flow remains subsequent work, as do
batch changes, optional built-in package removal and real package-base updates.
There is no automatic client deployment, input update, data removal or garbage
collection. A changed declaration is not proof of an installed application.

## Verification

Nix tests cover both roles, zero clients, legacy scopes, invalid fields and
controller-candidate rejection. Go tests cover capability fallback, old/new
target review, removal, stale inventories, CLI parsing and pending-state copy.
The management VM exercises shared software with no clients, a rejecting
controller-specific validator and declaration-only saves. Full validation
continues to require representative builds and offline client equivalence.
