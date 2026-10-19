# ADR-0028: Guided change of the laboratory network

- Status: accepted
- Date: 2026-10-15
- Builds on: [ADR 0017](0017-role-aware-network-interfaces.md),
  [ADR 0026](0026-proportionate-confirmation-for-recovery.md)

## Context

The laboratory network (`networkBase`, `networkPrefixLength`) sets every
static address: the controller's, each client's, the client firewall's only
accepted SSH source and the cache substituter. Applied in the wrong order a
new network cuts the controller off: clients updated first accept only a
controller address the controller does not have yet, and a controller changed
first cannot reach clients that are still on the old network. Settings
therefore refuse the change while computers are configured (`NETWORK-LOCKED`).
Schools still renumber networks, and reinstalling every computer is not an
answer.

## Decision

A change of laboratory network is a guided sequence of the existing reviewed
operations, with a settings record carrying its state:

- **Start** saves the new network together with `lab.networkTransition`, the
  previous network. While the record exists the configuration is dual: the
  controller carries both addresses, every client keeps its previous address
  next to the new one, trusts both controller addresses and tries the new
  cache first. Then **Apply to controller**, then **Update computers**, each
  with its own review confirmed with Enter; computers
  that are off can be queued.
- During the change every per-computer probe tries the new address first and
  the previous one second and reports which one answered. Update computers
  reaches a computer still on the previous network over that address and
  checks it afterwards on whichever answers.
- **Finish** is offered only when every client runs the saved configuration
  and answers on its new address. It removes the record; then the controller
  is applied (previous address dropped) and the computers updated (single
  controller address). Network installation must be prepared again.
- **Undo** is possible at any time before finish. It restores the previous
  network and removes the record; the computers are updated first, then the
  controller drops the address on the network being left.
- The trusted SSH key of each client is recorded for its new address from the
  key already trusted for its previous address, through the managed known
  hosts store, before anything contacts the new address. A different key
  already recorded there is never replaced.
- Each step is reviewed (every computer's old and new address, what follows)
  and saved with Enter: saving changes no computer. The CLI uses the same
  manager (`nixorium network migrate plan|start|status|finish|undo`) with
  `--expect` tokens. Starting requires every client to answer or to be
  acknowledged as off, no blocking situation, network installation stopped and
  no unsaved configuration.
- A change in progress is the situation `NETWORK-CHANGING` on the Overview,
  with the next step.

## Invariants

- The configuration is dual only while the record exists; without it every
  host is byte-identical to the single-network configuration.
- Nothing moves automatically: each step is an explicit save followed by the
  ordinary reviewed controller and computer operations.
- Undo is always possible before finish, and safe in any order of its two
  applications: every client of the change still has its previous address.
- Host key checking is never weakened.
- The controller number stays locked; the client count does not change during
  a change.

## Consequences

A laboratory can move to a new network without reinstalling a computer and
without a moment in which a computer loses the controller. A computer that
stays off through the whole change is updated over its previous address when it
answers, as long as the change is not finished. An address a client no longer
needs disappears at its next start.
