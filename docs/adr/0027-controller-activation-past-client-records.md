# ADR-0027: Controller activation past client records

- Status: accepted
- Date: 2026-10-15
- Amends: [ADR 0023](0023-reviewed-recovery-of-interrupted-operations.md)

## Context

An interrupted client update leaves `deployment-pending.json`, and an
unfinished USB installation leaves `usb-reservation.json`. Both blocked every
operation, including the activation of the controller. **Update Nixorium**
then saved the new release but could not activate it, and **Apply to
controller** was refused. If the old release's recovery of that record had a
bug, the controller could never reach the release that fixes it.

## Decision

The way to the fix is never blocked (owner decision, 2026-10-15).

- Controller activation (`nixorium-apply-controller.service` and
  `nixorium-apply-controller@<revision>.service`, that is **Maintenance →
  Apply to controller** and the activation phase of **Update Nixorium**) is
  allowed while a client update record exists. It changes only the
  controller, not the computers the record describes, and it never clears or
  rewrites the record. Both the command's unit allowlist and the privileged
  job's operation gate apply this exception; every other unit and job keeps
  the refusal.
- The same job may cross a USB reservation whose disk work completed (as
  before) or never started: no review token used, no receipt, no
  dispatch, reboot or verification recorded, and no reviewed installation
  waiting for its `ERASE` confirmation. It pauses the worker, takes the
  operation lock, checks again and resumes the worker afterwards. Disk work
  in progress, an open review and uncertain dispatch still refuse it.
- Activation is still refused by another managed job in flight, an
  interrupted template reset, a failed evaluation and the PXE temporary
  network state.
- Client deployments, installations, network installation, fleet mutations
  and classroom controls stay blocked by both records until their reviewed
  recovery.
- The result of the activation and the Overview still show the record as
  attention. After **Update Nixorium** the result adds that the student
  computers were not touched and still wait for the review.

## Consequences

A controller with a pending record can always be updated, so a bug in a
release's own recovery is fixed by installing the next one. Releases before
3.7.0 still refuse the activation in their own command and service; for them
the troubleshooting guide documents `nixos-rebuild switch` on the prepared
configuration source as the last resort. It touches only the controller and
leaves the record in place.
