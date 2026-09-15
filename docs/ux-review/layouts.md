# V3 layouts for review

**PROPOSED WIREFRAMES — NOT APPLICATION RENDERS.**

[Plan](README.md) · [Flows](flows.md) ·
[Current implementation renders](../tui-renders.md)

All data is synthetic. L12 and L17 describe planned application operations and
must not be presented as available features until their typed services ship.
L15–L16 use the existing Nixorium update service.

Reference size: 120×30. Wireframes omit unused rows and colour, so they are not
rendering tests. Narrow windows open detail on a successive page; wider windows
show more useful rows, not more metrics.

Legend: `›` focus, `✓` verified, `!` attention within the current flow,
`×` error, `●` running, `○` expected/not yet observed.
Focus and status retain symbols and text in addition to colour.

# First installation

## L01 — Entry on the installed controller

Flow F01. Application evidence selects the entry; client reachability does not.

~~~text
   Nixorium
   Set up your laboratory

   This controller is installed.
   Next, configure the network, users, and computers,
   then install the first client.

   You can leave and resume this process.
   Important changes will always be reviewed before they are applied.

   › Start setup

   Enter continue   ? how it works   q quit
~~~

Resume variant:

~~~text
   Nixorium
   Resume laboratory setup

   ✓ Laboratory settings saved
   ✓ Controller configured
   › Prepare the client system

   Last verified: today at 10:14
   Current state will be checked before continuing.

   Enter resume   t all steps   ? help   q quit
~~~

Required PXE recovery takes priority:

~~~text
   ! Controller network recovery required
   A previous installation session did not close correctly.

   › Restore normal networking
     Show technical details
~~~

Computer/Software/Activity must not look equally important during first setup.
Advanced routes remain available through an explicit exit/help path, while
network recovery can never be hidden.

## L02 — Initial route map

Flow F01. Orientation, not eleven internal checks competing for attention.

~~~text
   First setup                                   Step 1 of 5

   1  Laboratory settings                  ● In progress
   2  Controller                           ○ To configure
   3  Client system                        ○ To prepare
   4  First computer                       ○ To install
   5  Other computers                      ○ Whenever you are ready

   Configure the laboratory
   We will collect only the information needed to build the system.

   › Continue

   Enter continue   t step details   ? help   q quit
~~~

This five-step view is a UX grouping, not a replacement for the domain's
observed setup facts. `t` reveals technical checks inside the current group.
“Other computers” does not prevent completing a session.

## L03 — Settings grouped by subject

Flow F01. Network example: one decision and why it is needed.

~~~text
   First setup / Laboratory settings             Network

   Controller connection

   Network interface
   › enp2s0 — default route, 192.168.1.42

   Nixorium will use this interface to prepare and reach clients.
   Institutional addressing remains under the existing DHCP server.

   Enter select   Tab fields   Esc back   F1 help
~~~

Inventory example:

~~~text
   First setup / Laboratory settings             Computers

   How many clients should be configured?   24_

   Nixorium will create identities pc01–pc24.
   The computers do not need to be powered on now.

   Controller: pc99
   The controller is not part of the client group.

   Enter continue   Esc back   F1 help
~~~

Locale, keyboard, and time zone use searchable offline choices. Validation
errors stay beside the field without discarding valid answers. Passwords retain
the protected collector; no secret or hash appears in review or logs.

## L04 — Review laboratory settings

Flow F01. Secrets are redacted; saving and machine changes remain separate.

~~~text
   First setup / Review settings

   Laboratory       Computer laboratory
   Network          enp2s0 · 10.0.0.0/24
   Controller       pc99 · 10.0.0.99
   Clients          pc01–pc24
   Language         Italian · keyboard it
   Accounts         admin · teacher · student
   Passwords        set, values hidden

   Confirming will save only managed configuration files.
   No controller or client will be updated in this step.

   › Review and save settings
     Edit a value
     Technical details

   ↑↓ move   Enter review action   Esc back   F1 help
~~~

The next screen preserves complete validation, redaction, fingerprint, and the
existing exact local-commit confirmation. Repository drift requires a new review.

## L05 — Build and apply the controller

Flow F01. Saved configuration, build, and activation are not one “Next” action.

~~~text
   First setup / Controller                      Step 2 of 5

   ✓ Settings saved in revision 8f2c91a
   ○ Controller not yet updated

   The controller must build and activate this revision.
   Services and networking may restart; the connection may be interrupted.

   After a reboot or disconnection, reopen Nixorium.
   It will verify current state before continuing.

   › Review controller update
     Show revision and checks

   Enter review   Esc back   ? help   q quit
~~~

Existing disruptive review:

~~~text
   First setup / Update controller?

   Target             pc99 · this controller
   Revision           8f2c91a
   Effect             build, activate, and verify
   Possible impact    services and networking may restart

   Type REBUILD pc99 to continue:
   > _

   Enter confirm   Esc cancel   F1 help
~~~

Only current verified receipts support “activated and verified”. If a further
reboot is required, say so explicitly.

## L06 — Prepare the client system

Flow F01. Every client may remain powered off.

~~~text
   First setup / Client system                   Step 3 of 5

   Laboratory revision    8f2c91a
   Configured clients     pc01–pc24

   You can prepare systems and installation with every client powered off.
   The controller will build the required results and check cache and network.

   › Prepare system and installation
     Show what will be built

   Enter start   Esc back   ? help   q quit
~~~

Semantic progress:

~~~text
   Preparing the system

   ✓ Configuration evaluated
   ● Building client systems
   ○ Preparing installation files
   ○ Checking cache and network

   Powered-on clients required: none
   Technical output is being saved in the private log.

   l available details   ? help
~~~

On failure, say that no client disk changed and whether partial artifacts will
be ignored or revalidated.

## L07 — Open installation and move to the pilot client

Flow F01. The network transition retains the `START PXE` review and confirmation.

~~~text
   First setup / First computer                  Step 4 of 5

   ✓ System and installation files ready
   ○ Network installation not open

   Choose a pilot computer
   › pc01

   After review, the controller will temporarily open network installation.
   You will then continue physically at pc01.

   Enter select   / search identity   Esc back   ? help
~~~

After confirmation:

~~~text
   First setup / Install pc01

   ! Network installation is active
   The controller is serving the prepared system on the laboratory network.

   Continue at pc01
   1. Power it on and choose UEFI network boot.
   2. Run /installer/setup.sh.
   3. Choose pc01 and inspect the target disk.
   4. Confirm installation locally on the client.

   ! The disk selected on the client will be erased.
   Nixorium has not yet verified an installed system.

   › I completed the steps on pc01 — check it
     Stop installation and restore networking
     Instructions and common problems

   ↑↓ move   Enter continue   ? help   q quit
~~~

The TUI shows no invented installer percentage. Leaving does not stop PXE; quit
must explain the consequence and offer an explicit supported choice.

## L08 — Verify the pilot and perform a practical check

Flow F01. Technical and human checks are separate evidence.

~~~text
   First setup / Verify pc01

   ✓ Management authentication succeeded
   ✓ Active revision: 8f2c91a
   ✓ Installed system booted from disk

   Technical verification succeeded.

   At the computer, check:
   - expected login and desktop session;
   - required software and peripherals;
   - network and classroom tools.

   › The practical check succeeded
     Report a problem
     Defer the practical check

   ↑↓ move   Enter continue   t technical details   ? help
~~~

The first action never starts rollout. If practical evidence is persisted, bind
it to identity and revision and invalidate it when the tested system changes.

## L09 — Install more clients or finish

Flow F01. The room need not be powered on or completed in one session.

~~~text
   First setup / Other computers                 Step 5 of 5

   ✓ Controller ready
   ✓ System 8f2c91a prepared
   ✓ pc01 installed and verified in this session

   The other 23 clients may be installed now or later.
   Powered-off computers are not errors.

   › Install another computer
     Install a group now
     Stop installation and finish
     Leave installation temporarily open…

   ↑↓ move   Enter continue   ? help
~~~

After verified stop:

~~~text
   ✓ Installation session complete

   Controller ready
   pc01 verified on revision 8f2c91a
   23 identities not verified in this session

   Normal controller networking was restored.
   You can exit and install other clients in another session.

   › Exit Nixorium
     Open interventions
     Technical summary
~~~

“Not verified in this session” does not mean “never installed”. Later openings
show interventions and do not restart setup merely because clients are off.

# Later openings

## L10 — Intervention-oriented entry

Flows F02–F06. No global scan and no health dashboard.

~~~text
   Nixorium · Computer laboratory

   What do you want to do?

   › Restore one or more computers
     Add or change software
     Distribute the prepared system
     Update Nixorium
     Shut down clients…

     Advanced tools and diagnostics

   No client check was run at startup.
   Powered-off computers are normal and matter only when you select them.

   ↑↓ move   Enter open   ? help   q quit
~~~

A real active/recovery operation appears above the menu:

~~~text
   ! Complete controller network recovery
   The previous PXE session requires reconciliation.
   › Open recovery
~~~

Local changes can annotate the relevant action (“Software — draft to review”)
without becoming a global alarm that blocks an unrelated safe intervention.

## L11 — Restore: clarify the intent

Flow F02. The first two paths use existing capabilities but need coordination.

~~~text
   Restore computers

   What kind of restoration is needed?

   › Reapply the intended system
     Keeps the disk and deploys the declared configuration again.

     Reinstall from scratch
     Opens network installation and erases the disk confirmed on the client.

     Advanced recovery
     Recent files and local generations have different paths and limits.

   ↑↓ move   Enter continue   Esc interventions   ? help
~~~

Selection is identity-based. Other clients are not checked. Reapply failure
never changes the selection to reinstall.

~~~text
   ! Reinstallation will erase the disk selected on the client.
   Snapshots and local files on that disk may be lost.
   Final disk confirmation happens physically at the computer.

   › Continue to preparation
     Return to reapply
~~~

## L12 — Software and prepared system

Flow F03. **IMPLEMENTED.** These renders describe the shipped typed workflow.

~~~text
   Change software

   Supported client software
   Resolved from the laboratory's pinned package set.

   › GIMP
       Edit bitmap images · gimp
     VLC                 ✓ all clients, including future clients
       Play audio and video files · vlc

   Configuration can be prepared while every client is powered off.
   Declared does not mean committed, built, or distributed.

   Enter scope   r remove   / search   Esc interventions   ? help
~~~

Add:

~~~text
   Add software

   Search  / vlc_

   › VLC · media player
     Package identifier: vlc

   Catalog: currently pinned inputs
   Searching will not update inputs or Nixorium.

   Enter select   Esc cancel   F1 help
~~~

Draft review:

~~~text
   Add VLC

   Configuration scope    all clients, including future clients
   Managed file           [path returned by the service]
   Powered-on clients     none required

   ✓ Proposal validated
   ○ Revision not saved
   ○ System not prepared
   ○ No client changed

   Only lab-software.json will be replaced atomically.
   No commit, build, activation, PXE action, or deployment is included.

   Type SAVE SOFTWARE abcdef012345 to continue:
   > _
~~~

After save: Prepare system, Pilot one client, Distribute to selected clients, or
Exit. Each has its own review; commit, build, and deploy are never one confirmation.

## L13 — Distribution selection for this intervention

Flow F04. Unselected machines are not errors.

~~~text
   Distribute the system

   Prepared revision  3ad947e

   Choose clients for this session
   [x] pc01   ✓ Reachable and verifiable
   [x] pc02   ✓ Reachable and verifiable
   [ ] pc03   ○ Not checked
   [x] pc07   ! Not reachable now
   [ ] pc08   ○ Not checked

   3 selected · 2 ready · 1 needs a decision

   › Check pc07 again
     Defer pc07 and review 2 targets
     Cancel

   ↑↓ move   Space select   Enter continue   Esc back   ? help
~~~

Never silently “exclude offline”. A reachable-client suggestion must be frozen
and reviewed, not track machines that appear during deployment.

Final review:

~~~text
   Distribute the system?

   Revision       3ad947e
   Targets        pc01, pc02
   Deferred       pc07
   Not involved   the other 21 clients

   Target services may restart.
   The selected computers will be built, updated, and verified.

   Type DEPLOY pc01,pc02 to continue:
   > _

   Enter confirm   Esc cancel   F1 help
~~~

The real phrase remains service-generated. This wireframe explains its meaning
but does not unilaterally change the existing token format.

## L14 — Distribution result

Flow F04. The denominator is the reviewed set, not the whole room.

~~~text
   Distribution complete

   ✓ 2 of 2 selected clients verified on revision 3ad947e
     pc01, pc02

   Deferred
     pc07 — not included in the operation

   The other 21 clients were not checked or changed.

   › Exit Nixorium
     Prepare another distribution
     Technical details and logs
~~~

Partial result:

~~~text
   ! Distribution needs attention

   ✓ pc01 verified on revision 3ad947e
   ! pc02 not verified after apply

   Nixorium cannot confirm pc02's active system.
   It may have changed; inspect it before trying again.

   › View pc02
     Prepare a fresh review
     Open log
~~~

No automatic retry or rollback claim. Leaving does not erase evidence or start
work on other targets.

## L15 — Select a discovered Nixorium release

Flow F05. Uses `UpdateCheckReport`; no editable target field.

~~~text
   Update Nixorium

   Current release     v2.1.0
   Source              github:giovantenne/nixorium

   Available stable releases
   › v2.3.0             Latest
     v2.2.1
     v2.2.0
     v2.1.0             Current

   Selecting a release only starts validation.
   No file, controller, or client has changed.

   Enter validate   p show prereleases   r fetch again   Esc back
~~~

Discovery failure:

~~~text
   ! Releases could not be fetched

   Nixorium could not contact the configured public upstream.
   No candidate can be selected and no file changed.

   › Try again
     Technical details
     Back to interventions
~~~

The UI never replaces a failed discovery with a free-form tag.

## L16 — Validate and apply a Nixorium release

Flow F05. The existing application plan owns candidate checks and confirmation.

~~~text
   Update Nixorium / Review

   Current             v2.1.0
   Selected            v2.3.0 · stable
   Scope               flake.nix and flake.lock only

   ✓ Candidate input resolved
   ✓ Representative controller built
   ✓ Representative client built

   This does not commit, activate the controller, start installation,
   or distribute the system to clients.

   Type UPDATE v2.3.0 to continue:
   > _

   Enter confirm   t checks/diff   Esc cancel   F1 help
~~~

After success, offer Git review as the primary next action. Applying the new
controller configuration and distributing client systems remain separate
reviewed operations.

## L17 — Shut down clients

Flow F06. **PLANNED CONTRACT: hidden until the service exists.**

~~~text
   Shut down clients

   Select computers to receive the request

   [x] pc01   ✓ Management available    no session detected
   [x] pc02   ! Session unknown
   [ ] pc03   ○ Not checked
   [ ] pc07   ○ Not reachable

   2 selected · controller pc99 always excluded

   ! Unsaved work may be lost.
   Unreachable clients will not be shut down automatically when they return.

   › Review 2 targets
     Change selection
     Cancel

   Enter continue   Space select   Esc interventions   ? help
~~~

Review with application-generated phrase and token:

~~~text
   Shut down 2 clients?

   Targets       pc01, pc02
   Controller    excluded
   Sessions      pc02 unknown
   Conflicts     no incompatible operation detected

   Checks will run again before sending.
   Cancellation after sending is not guaranteed.

   Type SHUTDOWN pc01,pc02 to continue:
   > _

   Enter confirm   Esc cancel   F1 help
~~~

Outcome qualifies the request, not physical power:

~~~text
   Shutdown requests

   pc01   ✓ Request accepted · now unreachable
   pc02   ! Request accepted · final state unconfirmed

   Losing network contact does not prove a computer is powered off.
   No other client was involved.

   › Exit
     Show details
~~~

## V3 layout review checklist

- [ ] L01–L02: is first configuration/installation clearly the primary purpose?
- [ ] L03–L05: are settings, save, and controller activation distinct?
- [ ] L06: is preparation with every client powered off clear?
- [ ] L07: is the physical client handoff and disk risk unmistakable?
- [ ] L08: are technical and practical verification distinct?
- [ ] L09: can the operator naturally finish after one client and resume later?
- [ ] L10: do later openings feel like interventions rather than monitoring?
- [ ] L11: are reapply and reinstall understandable without Nix terminology?
- [ ] L12: are configured, prepared, and distributed software distinct?
- [ ] L13–L14: can selected clients succeed while the rest remain off?
- [ ] L15–L16: is selection limited to fetched Nixorium releases and are later effects explicit?
- [ ] L17: is shutdown careful enough, and worth its implementation priority?

Every box remains intentionally unapproved. Approving a wireframe does not
authorise new application operations, commits, or publication.
