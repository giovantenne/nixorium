# V3 flows — installation and occasional interventions

**APPROVED IMPLEMENTATION REFERENCE. Individual availability is recorded in the plan.**

[Plan](README.md) · [Layouts](layouts.md)

The V3 numbering replaces the earlier proposal. F01 is the central flow; F02–F06
are reasons to reopen the TUI after provisioning. This document defines product
behavior; the plan and render gallery distinguish shipped slices from planned
services.

## F01 — First installation of the laboratory

Layouts L01–L09. Priority P1. This journey crosses the controller and clients;
the controller TUI does not perform every step itself.

~~~text
NixOS live USB on controller
  → bootstrap: release, disk, destructive confirmation
  → reboot into installed controller
  → guided laboratory configuration
  → credentials/keys → validation and review → saved revision
  → controller review and apply → verification
  → build client systems and installation artifacts
  → network review and PXE confirmation
  → network-boot pilot computer
  → select and confirm identity/disk locally on client
  → install, disk boot, controller verification
  → practical check → more clients now or later
  → stop PXE and verify network recovery → summary and exit
~~~

### 1. Bootstrap controller before the installed TUI

The existing installer starts from a NixOS live USB. It exposes release, target
disk, and erasure before installation, then asks for a reboot. The proposal does
not assume that the installed TUI is already present in the live environment.

The UX must hand off clearly to the next step without requiring the operator to
reconstruct which repository to enter or which role the computer has.
Changing bootstrap is a separately validated scope, not a theme change.

**Interruption:** distinguish failure before writing from a partial installation.
Do not promise disk rollback if none exists.

### 2. Start on the installed controller

Show “Configure laboratory” or “Resume configuration”, with verified work and
the next step. Do not lead with a wall of services. Once initial setup is
complete, enter intervention selection instead.

Do not choose this route from dirty Git, artifact freshness, or client
reachability alone; all may change during maintenance. Application-owned
evidence must distinguish initial provisioning from pending later work.

### 3. Required data and credentials

Group network → identity/inventory → users → locale/keyboard → essential desktop
choices. Suggest only safely inferred values and explain why each is needed.

- Ambiguous interface/address: require a choice; do not guess a network.
- Input error: preserve valid prior answers and focus the relevant field.
- Password: protected input with no echo, presentation-model plaintext, or logs.
- Existing keys: verify correspondence; do not overwrite inconsistent material.
- Optional advanced settings: available separately, not a setup prerequisite.
- Initial software: show the actual baseline. Guided customisation belongs to F03
  and must not block the first setup batch.

### 4. Review and activate controller

Present a human summary with redacted secrets and on-demand technical evidence.
Writing configuration, creating a reviewed commit, and applying the controller
remain separate effects with existing confirmations.

Explain that activation can interrupt network access and how to find the
controller again. After interruption, reconcile service state and verified
receipts; screen closure is not success evidence.

### 5. Prepare client systems

Show revision, configured targets, and actual phases: evaluation, build,
artifact preparation, cache/network checks. Every client may remain powered off.

- Space/input/build failure: say that no client disk was changed and whether
  partial artifacts will be ignored or revalidated.
- Do not estimate duration without measured evidence.
- Different hosts may produce different results.
- Revision/input drift invalidates artifacts until revalidated.
- Progress is typed; technical output remains in bounded private logs.

### 6. Open network installation

Review interface, addresses, and effects. Preserve START PXE, readiness checks,
narrow privileges, and transactional recovery.

Say “Network-boot the computer you want to install.” A powered-off client is
waiting for the operator, not failing. Keep stop and network recovery reachable.

### 7. Pilot client and physical handoff

The controller explains the steps; client identity and disk remain local
installer decisions. Controller review does not authorise erasing a remote disk.

- Identity comes from immutable inventory; hardware/disks are observed locally.
- Exact local identity-and-disk confirmation; appearing on the network never
  triggers destructive installation.
- Existing duplicate probes remain best effort; do not promise global identity
  reservation without a coordination protocol.
- Without installer telemetry say “Complete these steps on the client”; show no
  invented remote percentage.
- After disk boot, verify authenticated active configuration. Ping, PXE menu
  arrival, or wizard closure does not prove installation.
- A practical desktop/login/software check remains separate from revision verification.

### 8. Remaining clients without requiring a full-room session

A verified pilot lets the operator continue. Each installer retains local disk
confirmation. The session may end and remaining machines may be installed later.

Distinguish configured, historically observed as installed, currently verified,
and deferred. Missing current observation does not prove never installed.
Infrastructure may be ready while only some clients are installed.

Any stored session state records identity/revision/evidence to reconcile, not
authoritative completion written by the UI. Powering off a previously verified
client does not erase its historical progress.

### 9. Complete and exit

“Stop installation and complete session” uses the existing service and verifies
normal network restoration. Keep “leave installation temporarily open” distinct
and explain the consequences.

Summarise controller readiness, clients verified this session, deferred/unknown
clients, and real failures. Do not claim all machines installed merely because
the current domain exposes an installation-offer stage.

**F01 acceptance:** a non-Nix technician always knows which machine to use,
which disk is at risk, when to power on the client, how to resume after an error,
and how to finish after only one client. Zero powered-on clients during
preparation is normal.

## F02 — Restore one or more computers

Layouts L10–L11. Priority P2. “Restore” requires an explicit choice.

~~~text
Choose Restore computers → select identities → choose restoration type
  ├─ Reapply intended system → deploy review → build/apply/verify
  └─ Reinstall from scratch → data warning → PXE → local client steps
~~~

- Reapply: for manageable clients; does not format the disk or promise removal
  of every local/user-data change.
- Reinstall: erases the locally selected disk, including snapshots on that disk.
  Required data must be copied elsewhere first.
- Unreachable: do not diagnose failure. Ask to power it on for reapply or follow
  network boot for reinstall.
- Never turn a failed reapply into automatic reinstall.
- Multiple selection: bind scope to identities, exclude controller, show complete
  review, and preserve each client's local destructive confirmation.
- Unselected or powered-off computers do not appear as failures.
- Lost-file or local-generation recovery is separate advanced work; a snapshot
  browser is not automatically included in this batch.

**Acceptance:** the operator knows whether configuration, disk, or personal data
is affected before any effect.

## F03 — Add/change software and prepare the system

Layout L12. Priority P3. New software use case over shared Go services.

~~~text
Choose Change software → find supported package → choose configuration scope
  → review diff → save validated draft → explicit review and commit
  → prepare system → distribute now, pilot one client, or exit
~~~

Every client may be off. Configuration scope (all clients, explicit group,
single host) is different from machines updated in this session.

Final service contract:

- `SoftwareCatalogReport` lists only package identifiers resolved from the
  deployment's pinned package set, with label, summary, availability, and
  origin (`framework`, `managed`, or `private-module`). Catalog lookup is
  read-only and never updates an input;
- `SoftwareChangeRequest` contains package identifier, desired presence, and a
  typed configuration scope: all clients (including later generated clients),
  an evaluated group, or explicit evaluated client identities;
- `SoftwareChangePlanReport` contains the normalised request, managed-file diff,
  base fingerprint, affected configuration identities, validation issues, and
  review token. It rejects packages unavailable in pinned inputs, unknown
  scopes, and files not owned by the software service;
- `SoftwareChangeApplyReport` atomically writes only the managed declarative
  software file after rechecking its fingerprint and token. It does not commit,
  build, activate, or deploy;
- private modules are visible as externally managed entries but never rewritten.
  Removal of an externally managed package links to technical guidance;
- drift after review blocks and requires a fresh plan. Unrelated worktree and
  index changes remain untouched;
- catalog unavailability gives retry and technical guidance, never a raw-name
  input that can become arbitrary Nix code;
- configured, committed, prepared, and distributed are separate evidence and
  separate confirmations. “All clients” states that later generated clients
  inherit the declaration;
- software changes and Nixorium-release changes remain separate. Presentation
  never executes shell or agent-produced commands.

The managed file location and Nix schema are an implementation decision for P3,
but must be exposed by the adapter/application report rather than duplicated in
the TUI. CLI commands will use the same catalog/plan/apply services.

AI may assist advanced customisation outside this supported workflow, but is
not required for it and receives no implicit mutation authority.

**Acceptance:** prepare the system and exit with all clients off, without
confusing preparation with distribution.

## F04 — Distribute the system to selected clients

Layouts L13–L14. Priority P2/P3. Reuses deploy plan/apply; it is not disk cloning.

~~~text
Choose system/revision → select targets → check only relevant clients
  → power on or defer unreachable selected targets → review final set
  → exact confirmation → build → revalidate → apply → verify
  → per-target result → exit or make a new review for other clients
~~~

- Checking starts explicitly inside the flow, not at application launch.
- “Currently reachable” may suggest a selection but it must be frozen and
  reviewed; never dynamically follow machines that appear online.
- A selected unreachable target blocks that target: power it on or defer it.
  Never exclude it silently.
- Results distinguish executed, deferred, and unconfirmed targets.
  Unselected clients are not deployment failures.
- No queued deployment when a client later returns.
- A configuration intended for all but deployed to a subset leaves planned
  work, not a continuous alarm.
- Recommend a pilot for material changes. Technical and practical checks are
  separate and bound to revision/identity. Direct advanced deploy remains available.
- Preserve build-before-apply, post-build recheck, and authenticated verification.
- Build failure means no apply. Apply failure may leave mixed state; retry starts
  from a fresh plan, never inferred rollback.
- Foreground operations still block exit until a real durable/reconnectable
  service exists.

**Acceptance:** updating four selected clients while twenty are off may finish
as “4/4 selected clients verified”, never “24/24 updated”.

## F05 — Update Nixorium from available releases

Layouts L15–L16. Priority P4. This flow uses the existing typed
`UpdateManager.Check`, `Plan`, and `ApplyPlan` boundary.

~~~text
Choose Update Nixorium → fetch bounded releases from configured upstream
  → select stable release, or explicitly reveal prereleases
  → validate candidate and representative builds → review exact two-file diff
  → type service-generated confirmation → atomically update flake files
  → review Git changes → explicitly commit/apply/distribute in later operations
~~~

Final contract:

- `UpdateCheckReport` owns upstream identity, current ref, stable/prerelease
  candidates, truncation, and discovery issues;
- presentation never accepts a free-form target and never constructs a remote;
- stable releases are the default list; prereleases require an explicit UI
  choice that is passed as `allowPrerelease` to planning;
- downgrades are not offered by the TUI. If an older discovered tag is selected,
  application planning rejects it before effects;
- refresh performs another explicit bounded discovery; failure keeps no stale
  candidate actionable and explains that no file changed;
- `UpdatePlanReport` remains bound to repository state, exact candidate,
  representative builds, review token, diff, and confirmation phrase;
- apply changes only `flake.nix` and `flake.lock` atomically. It never commits,
  pushes, activates the controller, starts PXE, or deploys clients;
- after success the next sensible action is Git review. Controller activation
  and client distribution remain separately reviewed operations;
- NixOS release migration is not a TUI action.

**Acceptance:** every selectable release came from the configured upstream,
discovery and validation are visibly distinct, and success never implies that
the controller or any client was updated.

## F06 — Shut down selected clients

Layout L17. Priority P5. Planned typed plan/apply operation; it stays hidden
from the executable menu until the service exists.

~~~text
Choose Shut down clients → explicit selection → check conflicts/sessions
  → review and exact confirmation → send request → per-target outcome
~~~

- “All” means clients, never controller. No free-form shell command.
- Already unreachable means unknown, not “already off”, and no deferred request.
- Service blocks targets in incompatible deployment/installation operations.
- Active/unknown sessions require an approved policy; explain unsaved-work risk.
- Bind review to identity, action, token, expiry; recheck before effects.
- Do not promise cancellation or physically-confirmed shutdown after send.
- Outcomes: request accepted, not sent, or not confirmed. Network loss alone
  does not prove power state.
- No Wake-on-LAN, schedules, or continuous monitoring is implied.

Final service contract:

- `ShutdownPlanRequest` contains explicit evaluated client identities and a
  session policy (`require-idle` by default or `acknowledge-unknown`);
- `ShutdownPlanReport` contains the frozen target set, controller exclusion,
  per-target reachability/session/conflict observations, expiry, review token,
  and generated confirmation phrase;
- unreachable targets are ineligible, remain visible as `not sent`, and are
  never queued for later;
- active deployment, installation, or controller-network recovery is a hard
  conflict. An observed active user session is blocked by default; unknown
  session state needs explicit acknowledgement in the reviewed plan;
- `ShutdownApplyReport` records per target only `accepted`, `not-sent`, or
  `unconfirmed`, plus typed issues. It never claims physical power state;
- the application rechecks token, expiry, inventory membership, controller
  exclusion, repository-independent operation conflicts, and session policy
  immediately before dispatch;
- the adapter receives a fixed shutdown operation for an evaluated hostname.
  Presentation cannot provide a command, host address, or shell fragment;
- operation history stores bounded typed outcomes, while technical SSH output
  remains in private logs.

**Acceptance:** the action does not require the whole room to be on, never
targets the controller, and never treats network absence as a fault.

## Cross-cutting — errors, evidence, and resume

Show errors inside the relevant intervention with scope and consequence. PXE
recovery, conflicting work, and controller failures may precede intervention
selection; uninvolved powered-off clients do not.

Help cannot confirm mutations. Logs retain sanitisation, size bounds, and
permissions. Events from an old attempt cannot update a new one. Checkpoints
and history do not replace current observations or review tokens.

Installation-session checkpoints are private per-repository state. Technical
evidence is bound to the exact client identity, deployment revision and
authenticated system path; the practical check is a separate operator claim.
Changing the revision or evaluated inventory makes the checkpoint stale. A
fresh failed observation must take precedence in the UI without deleting the
older historical record.

Completing an intervention provides a clear exit. Nixorium should not ask the
operator to keep it open to monitor the room.
