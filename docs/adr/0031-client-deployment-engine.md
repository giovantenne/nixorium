# ADR-0031: Client deployment engine

- Status: accepted
- Date: 2026-10-19
- Replaces: Colmena as the tool that builds and applies client systems
- Amends: [ADR 0023](0023-reviewed-recovery-of-interrupted-operations.md)
  (what the pending record says about each computer)
- Related: [ADR 0027](0027-controller-activation-past-client-records.md),
  [ADR 0030](0030-classroom-controls-beside-maintenance.md)

## Context

Update computers ran `colmena build` and then `colmena apply switch`. Colmena
0.4 evaluates every selected computer separately, and does it again in the
apply: about eight seconds of CPU and 750 MB per computer, twice, before any
copy. Its apply runs `switch-to-configuration` directly over SSH, so a dropped
connection leaves the outcome unknown, and it reports one error for the whole
batch, so one computer that failed or was switched off made the whole update
"unconfirmed" and blocked the laboratory until a review.

## Decision

Nixorium builds and applies client systems itself:

- **Build.** One `nix build --json` of the selected computers' system
  attributes (at most four computers per evaluation process), from the same
  reviewed source as before (`EvaluationSource`, account hashes injected, no
  private file). The store paths are kept for the apply of that revision in
  the same process. The apply never evaluates again; a revision that was not
  built in this update is not applied.
- **Copy.** `nix-copy-closure --to root@<address> --use-substitutes <system>`,
  the command Colmena used, with the management SSH options in
  `NIX_SSHOPTS`. The computer takes what it can from the controller's cache.
  The address is the one that answered the last check: during a change of
  laboratory network, the previous address of a computer not yet moved.
- **Activation.** One fixed SSH command starts a transient unit
  `nixorium-activate.service` (`systemd-run --service-type=exec
  --property=RemainAfterExit=yes --expand-environment=no --no-block`). It sets
  the system profile, runs `switch-to-configuration switch` and writes
  `/var/lib/nixorium/activation/last.json` (activation identifier, system
  path, exit status, times) atomically. The fixed unit name refuses a second
  activation while one runs. The controller polls that record and the unit,
  tolerating the network restarts of an activation, copies the unit's journal
  into the deployment log and unloads the finished unit.
- **Outcomes.** Exit status 0 is activated; 4 is activated with failed units
  (the computer runs the new system; the log names the units); 100 is
  activated with a restart required; any other status, or 200 when the
  profile could not be set, is failed. No result for this activation after 20
  minutes, or a unit that stopped without one, is unknown, never failed.
- **Pending record.** `deployment-pending.json` keeps its version-1 fields and
  gains `activations`: one entry per computer with its activation
  identifier, its state (`planned`, then `started` written durably before the
  activation command, then `finished`) and its result. It is created under the
  classroom lock of ADR 0030 and replaced atomically at every step. When every
  computer finished, or never started, it is removed and the update reports
  the computers that were not updated. Only a computer whose result is
  unknown keeps the record and the operation unconfirmed.
- Copies run four at a time, activations ten at a time.
- **Self-closing records (owner decision D7).** When the engine returns with
  unknown results it marks the record `applyReturned`: the recorder process no
  longer acts on any computer, even while it stays open. The recovery review
  then classifies each computer from the record first: `planned` was never
  touched, `finished` has its result, and a `started` computer is finished
  only when its own `last.json` carries this activation's identifier. When
  every computer is known this way, the record is archived without asking
  (`automatic` in the review); the timer that runs queued updates does this
  every minute while a record waits. A revision observed on the computer is
  never enough on its own, and records of older versions (without
  activations) always wait for the reviewed recovery.

The flake output `colmena` and the `colmena` command stay available for
manual use; Nixorium no longer runs them. (ADR 0033 later removed the flake
output.)

## Consequences

- One evaluation per computer instead of two, and no evaluation in the apply.
- A computer that is off, fails its copy, or finishes its activation with an
  error no longer blocks the laboratory: its outcome is known.
- An activation keeps running when the connection or the controller goes
  away, and its result can be read later. The recovery review of ADR 0023
  can use the record's per-computer states: a `planned` computer was never
  touched.
- The engine is new code instead of a widely used tool; the management and
  network-change VM tests cover the copy, the activation commands, the
  pending record and the previous-address path, and a VM test with a real
  client covers activation itself.
