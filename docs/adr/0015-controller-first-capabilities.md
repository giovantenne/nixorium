# ADR 0015: Controller-first capabilities

Status: Accepted

## Context

The controller must be useful before a laboratory is configured. Previously,
controller activation required the same DHCP hint and keys as client
deployment. Moving account prompts into the live installer cannot by itself
remove this dependency. Guided software currently applies only to clients.

## Decision

Add optional `lab.deploymentMode`: `laboratory` (the default) or `controller`.
The additive field uses settings schema version 1. Opening/saving an old
deployment must not insert it. Older upstreams reject the new field: upgrade
before opting in; remove it before downgrading to an older reader.

Controller mode requires zero clients; laboratory mode requires at least one.
Do not infer mode from keys, placeholders, evidence or reachability. Reserved
network settings remain structurally validated but do not configure a static
address in controller mode. NetworkManager handles local connectivity. Lab
cache, remote-control services and lab firewall openings remain inactive.
There are no generated client hosts or Colmena client targets.

`deploymentStatus.ready` keeps its strict fleet meaning and is always false in
controller mode. Add `deploymentStatus.controller` with `ready`, `issues`, and
`requiresKeys`. Controller mode requires non-default account credentials but
not lab keys or the DHCP hint. Laboratory mode retains all current checks.
Old upstreams without this capability fall back to strict fleet readiness.

Application preflight and the fixed privileged service both consume this
capability. The service evaluates the exact reviewed Git revision and skips
key checks only for explicit `requiresKeys = false`. Validation, secret-source
filtering, clean worktree/revision checks, verified activation and root-owned
success evidence remain mandatory. False readiness with no explanatory issues
still blocks activation. PXE and client deployment never use the controller
capability as authority.

Guided Nixorium update planning also consumes the explicit mode. Controller
mode requires zero client inventory and explicit controller readiness, then
builds only the candidate controller system. It does not evaluate client,
netboot, firmware, or installer outputs that cannot be used in that mode.
Laboratory mode and legacy metadata retain strict fleet readiness and the full
representative build set. Unknown modes, inconsistent inventory, and a target
that omits controller readiness fail closed. This changes validation scope only:
CLI update apply still writes only the reviewed `flake.nix` and `flake.lock`
proposal. The ordinary TUI follows its transparent save with the existing
typed controller plan/apply boundary.

## Bootstrap and workflow contracts

Bootstrap capability `lib.controllerBootstrapVersion = 1` collects teacher and
student identities, timezone, keyboard, and all three passwords before disk
installation. It persists controller mode with US internal locales and defers
client networking and keys. Installers for older revisions remain on their
legacy workflow rather than invoking an unsupported command.

The operator TUI exposes five top-level tasks. Client installation owns the
resumable laboratory configuration and never blocks using the controller.
The remaining long-lived constraints are:

- Persist real installation evidence, never a fabricated controller-service
  receipt.
- Introduce shared software while preserving old client-only scopes. Compose
  save/build/activate/verify in an application workflow; keep the low-level
  software file writer narrow rather than adding hidden activation to it.
- Define and validate a reviewed package-base pin on the supported NixOS
  channel. Rebuilding an unchanged lock is not a software update.
- Configure the client network on demand, resolving interface differences
  between host roles. A mode change does not authorize PXE or disk erasure.
- Reuse/import keys without implicit overwriting or rotation.
- Keep software, client installation, client updates, shutdown and advanced
  tools usable without an obligatory lab wizard or GNOME notification.

## Verification

Nix/Go tests cover mode/count invariants and legacy defaults; host evaluation
covers inventory, networking, services, firewall and credential readiness.
Application tests cover independent readiness and strict legacy fallback.
Management VM coverage exercises the privileged helper without lab keys while
fleet operations stay blocked. Update adapter and VM coverage verify both the
legacy laboratory build set and the controller-only build set. Representative
system builds and offline equivalence remain required for this foundation.
