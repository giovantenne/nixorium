# Nixorium — first installation and occasional maintenance

Status: **V3 APPROVED DIRECTION — IMPLEMENTATION IN PROGRESS**

Date: 2026-09-15

## Corrected product brief

This revision incorporates the owner's clarification and replaces the previous
assumption that Nixorium is opened every day.

**The TUI exists primarily to set up the laboratory.** After setup, it is opened
for a deliberate intervention: restore computers, change software, distribute
the prepared system, update Nixorium from a discovered release, or shut down
selected clients.
It is not a room-monitoring console.

Many computers will normally be powered off. Unreachable is an observation, not
a fault diagnosis and not a reason to put the whole laboratory in an alarm state.

The contracts and layouts in this review are now complete enough to implement
in batches. They do not claim that unshipped services are already available.
The structural entry, restore, pilot-installation, guided software,
deployment, and Nixorium update slices are implemented on the development branch. The
[actual renders](../tui-renders.md) track the implementation rather than a
separate mock-up.

### What changes from the previous proposal

| Previous direction | V3 |
|---|---|
| Health Overview as the primary destination | Initial setup or intervention selection |
| Whole-room availability as the central signal | Availability only for targets of the selected operation |
| Offline treated mainly as attention | Powered-off/unreachable is neutral outside an operation |
| Setup as a conditional side route | Controller → first client → remaining clients is the primary flow |
| Navigation by system area | Navigation by intent: restore, change, distribute, upgrade, shut down |
| Software, changes, and activity always in primary IA | Expose them only when implemented and useful to an intervention |
| Broad recovery roadmap | Computer restoration first; file recovery stays a separate project |

The F/L/D identifiers below belong to V3. The sequence is deliberately centred
on first installation.

## Review order

1. Review [F01 — First installation](flows.md#f01--first-installation-of-the-laboratory)
   before anything else.
2. Review [L01–L09 — Setup and installation](layouts.md): entry, configuration,
   preparation, pilot computer, remaining computers, and completion.
3. Review the later intervention flows F02–F06 and their layouts.
4. Review the open decisions and delivery batches at the end of this document.

Feedback can reference an identifier: “F01: reverse these steps”, “L06: explain
the disk risk here”, or “D03: by restore I mean…”.

## Operating model

~~~text
First time
  Install controller → configure laboratory → prepare system
  → install and verify one client → install others → close the session

Later openings
  What do you want to do?
    Restore one or more computers
    Add or change software
    Distribute the prepared system
    Update Nixorium
    Shut down clients

  Secondary: inventory, settings, diagnostics, activity, and advanced tools
~~~

There is no health dashboard as the primary destination. Inventory supports the
flows; it is not why Nixorium is opened. Git, cache, controller, and PXE remain
available at the point where they are needed.

If an operation is still active or requires recovery, show it before the menu.
If initial setup is incomplete, offer to resume it.

**Do not confuse initial setup with later maintenance.** A repository with
unsaved changes, artifacts that need rebuilding, or powered-off computers must
not send an already configured laboratory back into first installation.
The current setup report is not sufficient on its own to choose this route:
application-owned evidence must distinguish initial provisioning from pending
maintenance work.

## Vocabulary: “image” and versions

The operator may naturally think in terms of preparing and distributing an
image. Product language should be more precise:

- **Laboratory system:** desired software and configuration.
- **Prepare system:** evaluate and build results for the intended targets.
- **Distribute system:** update installed clients without reinstalling disks.
- **Reinstall computer:** PXE path that erases the disk confirmed locally.
- **Nixorium version:** release of the management framework.

“Image” may be explained as familiar shorthand, but must not imply disk cloning
or one identical file for hosts with different configurations. Nixorium builds
NixOS configurations/closures and netboot artifacts.

NixOS release migration is deliberately outside the TUI. Updating Nixorium
changes the pinned framework input only through the existing typed update
service. The UI discovers bounded release candidates from the configured public
upstream; it does not offer arbitrary channels or a free-form release field.

## Powered-off computers

| Context | Unreachable computer | Proposed behaviour |
|---|---|---|
| No operation selected | Normal; reason unknown | No alarm and no implicit fleet scan |
| First installation, client not started yet | Expected | “Waiting for you to continue on the client” |
| Distribution, computer not selected | Irrelevant | Does not affect the result |
| Distribution, computer selected | Blocks work for that target | Ask the operator to power it on or explicitly defer it |
| Operation sent, result cannot be verified | Uncertain outcome | Attention limited to that attempt and target |
| Shutdown requested | May be the intended effect | Do not claim powered off from loss of contact alone |

Do not distinguish powered off from disconnected without evidence.
Green/amber/red describe an operation's outcome or blocker, not the number of
computers currently running.

A deferred target remains “to update when you choose”, not “failed”. There is
no automatic apply when it returns online. Wake-on-LAN and continuous polling
are not implied by this proposal.

Software and installation artifacts must be preparable while every client is
off. Distribution requires checks only for selected targets.

## Priorities and current availability

| Flow | Current implementation | Intervention |
|---|---|---|
| Controller bootstrap, setup, PXE | Guided setup, pilot handoff, durable revision-bound verification, stop/leave review | Physical laboratory validation remains |
| Client verification and deployment | Implemented | Place them in an explicit-target flow that supports partial sessions |
| Restore | Explicit reapply/reinstall split; reinstall selects and verifies one identity at a time with cross-process evidence | Physical laboratory validation remains |
| Guided software changes | Typed catalog/plan/apply implemented for the managed declaration | Physical workflow validation remains |
| Nixorium release update | Typed check/plan/apply exists | Discover releases, select one, validate, review, and apply |
| Batch shutdown | Not implemented | Typed client-only operation after the priorities above |
| File recovery and snapshot browser | Not implemented in the TUI | Outside the main path; separate design work |
| Web UI | Not implemented | Deferred; this usage model reinforces terminal/SSH suitability |

Planned layouts must not appear as working actions until their typed service is
available. During migration, keep every existing operation reachable from
Advanced tools.

## Interaction design

- One primary question or decision per step; group related fields.
- Make the route visible: location, verified work, and what happens next.
- Keep controller configuration separate from client installation progress.
- Explicitly say when the operator must move to a client or reboot the controller.
- Support partial installation sessions; the whole room need not be powered on.
- Configured count, historically observed installation, and currently verified
  count are different facts.
- Remote verification does not certify the graphical classroom experience.
- Preserve every existing safety boundary, including local disk confirmation.
- Opening the TUI, choosing a menu item, or scanning never mutates the system.
- Errors explain what happened, what may have changed, and how to continue.
- A useful result may be “session complete; you can exit”, without returning to
  a monitoring dashboard.

### Keyboard and layout

Use 120×30 as the design reference, without making 80×24 the governing constraint.
Also verify 100×28 and 160×40. Narrow windows use successive pages instead of
side-by-side panels.

Enter opens or advances to a review; Esc goes back without implicit effects;
arrows/j/k move through lists; Tab changes focus between fields and controls;
/ searches where available; ? opens help and F1 works inside fields; q exits
outside fields only when the operation lifecycle permits it.

Show only a few relevant footer controls. Preserve exact confirmations,
password isolation, and foreground-operation exit locks. Technical logs remain
on demand with existing bounds and sanitisation.

Use whitespace, readable headings, one limited accent, and symbol-plus-text
status that survives without colour. Do not bring room-availability graphs into
the setup wizard. Italian wireframes were used to gather the product correction;
this version is English and does not itself approve a localisation system.

## Architecture and safety boundaries

Preserve presentation → application/domain → adapters, shared CLI/TUI services,
and Git as the desired-configuration source of truth.

New application/domain read models are required for:

- initial-setup context versus maintenance of an already configured laboratory;
- installation session and its intended targets;
- observations and blockers relative to the selected operation;
- prepared system, input revision, targets, and artifact validity;
- changes that remain undistributed without turning them into daily alarms;
- discovered Nixorium releases and the currently selected candidate.

Do not infer installation from ping, configured host count, or wizard completion.
A stored checkpoint is evidence to reconcile, not operational authority or a
second configuration source.

Software uses a typed plan/apply service; power still requires one. Preserve
review tokens, evaluated inventory, narrow privileges, locks, pre-effect
rechecks, and final verification. Detailed contracts are in [flows.md](flows.md).

Separate screen models from the root router and use meaningful reusable
components. Correct post-build progress, which can currently appear to jump
back to the initial step. The UI must represent build → revalidation → apply
→ verification without bypassing checks because a result is cached.

## Proposed delivery batches

| Batch | Scope | Required exit |
|---|---|---|
| P0 | Review F01 and L01–L09, then later flows | Explicit approval of affected flows/layouts |
| P1 | End-to-end first installation: entry, fields, controller, PXE, pilot, partial sessions, exit | Resumable comprehensible setup with the same safety rails |
| P2 | Occasional entry and restoration of one/selected clients | No monitoring dashboard and no alarms for uninvolved computers |
| P3 | Software plus prepare/distribute system | Implemented as separate declaration → Git → explicit rollout operations |
| P4 | Nixorium release update | Fetched candidate list, validated proposal, exact confirmation, and honest result |
| P5 | Client shutdown | Client scope, sessions/conflicts, honest per-target outcomes |
| P6 | Site, guide, and actual-version renders | Installation/intervention story rather than monitoring |

Existing prepare/distribute operations can be joined in P2. P3's software editor
deliberately stops before them. Uniform durable jobs require separate technical
review; do not promise reconnection while an operation remains foreground.
A Web UI is not a planned batch and no framework migration is proposed.

## Planned validation

Tests must reflect real usage rather than a permanently powered-on room:

- first installation from newly installed controller to one verified client;
- resume after reboot, invalid password, mismatched keys, and failed build;
- network/disk review, cancellation, and PXE recovery after leaving the page;
- ready controller with zero clients on: no alarm and no false client completion;
- install a subset, conclude voluntarily, and resume another day;
- configured laboratory with dirty Git state: maintenance, not first setup;
- prepare software with every client powered off;
- distribute to four selected clients while the others are off;
- selected unreachable targets: explicit power-on/defer choice and per-target result;
- Nixorium release discovery failure with no editable fallback target;
- Nixorium candidate blocked before effects by dirty Git or failed builds;
- shutdown excludes controller and does not infer physical state from network loss.

Run formatter, Go tests, vet, quick validation, and management VM for frontend
changes. If schema, template, module, bootstrap, or installer changes, use the
project's extended matrix and relevant installer test. Physical hardware remains
a separate requirement. This plan claims no new product validation.

Exploratory review: ask a non-Nix technician to install the first client, then
return a week later to reinstall pc07, add software, update only selected powered-on
clients, and update Nixorium from the fetched release list. Measure understanding of roles,
targets, disk effects, and physical handoffs. Powered-on client count is not a
UX success metric.

## V3 decisions

| ID | Decision | Status |
|---|---|---|
| D01 | First installation is primary; later use is occasional maintenance | Owner direction accepted |
| D02 | Computers are normally off; no fleet-availability alarm | Owner direction accepted |
| D03 | Restore has two explicit paths: reapply configuration or reinstall and erase a locally confirmed disk | Implemented direction |
| D04 | Use a pilot client and support partial installation sessions | Implemented with private, revision-bound cross-process evidence |
| D05 | Guided software is limited to supported packages/configuration while preserving private modules | Implemented |
| D06 | No NixOS-upgrade action; Update Nixorium selects only releases discovered by the typed service | Owner direction accepted |
| D07 | Shutdown targets clients only; session conflicts block by default and unknown sessions require explicit acknowledgement | Planned contract |
| D08 | Keep Bubble Tea; defer Web UI; approve final language and palette separately | Bubble Tea direction implemented |

The owner approved implementation and incremental local commits. The review
does not authorise live laboratory operations, publication, pushing commits, or
presenting an unimplemented service as available.

## Website alignment

After the relevant functions ship, the site should show:
controller → first client → remaining clients → later interventions.
It must distinguish reinstall from non-destructive distribution, and explain
that updating Nixorium updates the management framework. It should show partial installations and normally
powered-off clients without continuous-monitoring language.

The old Overview renders are not the proposed future entry screen.
No website files are modified by this documentation revision.
