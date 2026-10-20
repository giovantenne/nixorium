# TUI guide and generated gallery

These plain-text screens come from the real Bubble Tea renderer at the 120×30
reference size. Fixtures are synthetic and deterministic: generation does not
inspect a laboratory, contact a computer, query a network, or execute an
operation. Edit the surrounding explanation here; update the marked region
with `scripts/generate-docs.sh --write`.

The Assistant examples cover scoped software and LAN policy, diagnosis from
observed update evidence, and an offline lesson environment restored at boot.
They illustrate proposals; the displayed checks and outcomes are fixture data.
The free Software workflow remains available for ordinary package selection.

To regenerate the localized pricing captures, use an empty output directory and
run the developer-only exporter from a clean source checkout:

```sh
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell \
  --command go run ./cmd/nixorium-demo \
  --source-commit "$(git rev-parse HEAD)" \
  --source-date "$(git show -s --format=%cs HEAD)" \
  --assistant-output /tmp/nixorium-assistant-pricing
```

It exports `assistant.<locale>.<NNN>.ansi`, the typed message in
`assistant.<locale>.txt`, and `assistant-manifest.json` with source identity,
terminal size and the synthetic marker. The website's existing ANSI converter
consumes these files. The pricing example uses six selected PCs; the smaller
navigable dashboard demonstrates the same proposal on two of its four PCs.

## Workflow guide

The first administrator launch shows the operational disclaimer (Enter accepts,
q exits), then the optional telemetry invitation (Enter accepts, n declines,
Esc skips to the menu, q exits). Skipping saves no consent and repeats the
invitation next launch. A saved refusal is remembered.

The overview opens Computers, Installation, Software and Maintenance. Startup
reads saved configuration and local service/Git state; inventory, client probes and expensive
readiness checks run when the selected task needs them. Deferred checks never
imply that installation or deployment is ready.

The controller offers Assistant (`a`) unless `lab.assistant` is explicitly false.
Its account activation, subscription and consent
checks precede content sharing. Ctrl+R reviews candidate changes and their Nix
evaluation; saving records a local commit with a backup. Controller application
and Update computers remain separate reviews. See [Assistant](assistant.md).

Installation offers network boot (PXE) and USB over SSH. PXE preparation builds
the configured client artifacts before reviewing the temporary controller
network change; client identity and disk erasure are confirmed on each client.
USB selects one identity and verifies the live host fingerprint against its
physical console before asking for a password. Its disk review, reboot and
installed-system verification remain separate steps. Interrupted dispatches
require reconciliation, never an automatic retry.

Software changes desired configuration. Package search uses locked inputs;
profiles add missing declarations and preserve existing scopes. After saving,
controller-affecting changes use the controller activation workflow and
client-affecting changes can open a fresh deployment review. Saving alone does
not update running clients. Computers also offers ordinary system distribution
without reinstalling disks, authenticated inventory, Internet access and power
controls. The restricted teacher dashboard exposes only classroom controls.

Client distribution review probes only selected computers with the inventory's
bounded SSH-port check. **F2 — Reachable only**, when offered, creates a fresh
explicit-target plan; it does not start deployment or reuse the old confirmation.
Esc during replanning keeps the original review with its confirmation cleared.
Availability is not authenticated identity or proof that a computer is powered
off. Results list each selected computer with an outcome and guidance; use
Shift+arrows to scroll and **l** for logs. A not-reached result does not imply
unchanged state, and required recovery still blocks retries.

Maintenance contains settings, updates, history and recovery. Update validation
may build candidate systems before presenting its review. The ordinary TUI
then saves and activates the controller; client distribution remains separate.
See the [update guide](updates.md) and [administrator guide](../templates/site/README.md)
for complete procedures and the [architecture](management-architecture.md)
for the operation boundaries.

Settings also offers **Student workspace** on supporting pins. Its desktop,
dock, editor/extensions and browser fields are a draft until a complete review
and explicit save. This workflow saves and records only the profile JSON;
system application, client deployment and boot reset are separate. No additional
personalization switch is required. It is not a home
capture tool and is unavailable in the restricted teacher dashboard.

Arrows or `j`/`k` move, `Enter` invokes the visible action, `Esc` returns or
cancels, `Space` toggles a selection and `F1` opens contextual help. Exact
confirmation is retained for destructive operations. Progress reports phases,
elapsed time and available logs without treating an accepted request as proof
of completion. Renderer tests cover 80×24, 120×30 and 180×45 layouts.

Every working view names the activity, shows elapsed time and states whether it
can be cancelled. Esc cancels read-only loads and proposals, returns to the
originating screen and discards late replies. Settings drafts remain editable;
cancellation does not undo an earlier save or activation. Ordinary reads time
out after two minutes; isolated update/package-base/template candidate build
reviews allow one hour. Downloads and local builds can take several minutes.
A timeout offers retry or Diagnostics, never an automatic apply.

Foreground saves, activation and other mutations stay protected until their
result. The separately reviewed deployment stop still stops local supervision,
not remote work. PXE preparation explicitly allows quitting while its managed
job continues. Cancelling an IPC read closes this client's connection, not a
worker-owned operation. Unsupported keys explain the available action; F1 and
scrolling remain available while waiting.

On reopening, Overview checks controller/PXE units and local progress without
evaluating Nix. **v — View progress** attaches read-only to an existing job;
**Tab** switches jobs. Leaving the view never cancels or resumes work. An
inactive unit with a running record is **interrupted**, not still building:
inspect the named journal unit before a fresh review. A completion record is
not verification of the current configuration. Running units block conflicting
starts; unavailable unit state is not treated as idle. Checks retry every two
seconds after the previous bounded observation finishes.

<!-- BEGIN GENERATED: tui-gallery -->
## Operational disclaimer

```text
Nixorium  /  Welcome  /  Disclaimer

Welcome to Nixorium

Before you manage this laboratory
Nixorium can erase disks, install systems, reset student homes,
change configurations and interrupt active sessions.

! Check every computer, disk and action before confirming.
Keep tested backups. Local snapshots are not backups.
Use Nixorium only on computers and networks you may administer.

Nixorium is provided under the MIT License, without warranty.
You are responsible for testing changes and verifying their results.
Full terms: DISCLAIMER.md and LICENSE in the Nixorium repository.

Enter acknowledges these operational risks and continues.
The next screen asks separately about optional adoption statistics.

▶ Enter Accept & continue
↑/↓ Scroll  ·  q Exit
```

## Optional adoption statistics

```text
Nixorium  /  Welcome  /  Adoption statistics

Help improve Nixorium
Share basic statistics to understand adoption and lab sizes,
and help prioritize development.

Sharing is optional and off by default. Nixorium works fully without it.

Daily: version, system mode, client count band and boot status;
an identifier that changes every month.

No names, files, logs or configuration files are included.
Sent to telemetry.nixorium.org via Cloudflare, which sees your IP.

Disable anytime in Maintenance. See Privacy & retention for details.

Status: Off — no adoption reports are being sent.

e/Enter Share statistics  ·  n No thanks  ·  p Exact report  ·  i Privacy & retention  ·  ↑/↓ Scroll  ·  Esc Menu
q Exit  ·  F1 Help
```

## Unconfigured clients

```text
Nixorium  /  Computers

Manage client computers
Check which computers are on, update them, switch them on or off, and control Internet access.

› [h] Computer inventory
  [d] Update computers
  [x] Power controls
  [i] Internet access

Check reachability and compare observed systems with the intended revision

! Client computers have not been configured yet
  Open Installation to configure clients before installing or managing them.

n Configure clients  ·  ↑/↓ Select  ·  Enter Open  ·  Esc Overview  ·  F1 Help
```

## Channel update notification

```text
Nixorium  /  Overview

Laboratory overview
Choose an area with the arrows or its letter.

› [c] Computers
  [n] Installation
  [w] Software
  [m] Maintenance

Inventory, updating computers, Internet access and power

○ Nixorium update available: v3.1.0
  Chosen channel: stable. u reviews updates; x dismisses this version.

u Review update  ·  x Dismiss update  ·  ↑/↓ Select  ·  Enter Open  ·  r Refresh local state  ·  F1 Help
q Quit
```

## Overview

```text
Nixorium  /  Overview

Laboratory overview
Choose an area with the arrows or its letter.

› [c] Computers
  [n] Installation
  [w] Software
  [m] Maintenance
  [a] Assistant

Inventory, updating computers, Internet access and power

↑/↓ Select  ·  Enter Open  ·  r Refresh local state  ·  F1 Help  ·  q Quit
```

## Optional administrator Assistant

```text
Nixorium  /  Overview

Laboratory overview
Choose an area with the arrows or its letter.

› [c] Computers
  [n] Installation
  [w] Software
  [m] Maintenance
  [a] Assistant

Inventory, updating computers, Internet access and power

↑/↓ Select  ·  Enter Open  ·  r Refresh local state  ·  F1 Help  ·  q Quit
```

## Assistant chat before activation

```text
Assistant                                                                                              Not activated

Try asking, for example:
  Prepare Node.js on pc01–pc06; open port 3000 only to the lab LAN
  pc03 missed the last update: show the evidence and the next check
  Prepare offline Jupyter + pandas lessons; restore the student baseline at boot

















╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│> Describe what your lab should do                                                                                │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
Enter send · Alt+Enter new line · / commands · Ctrl+R review · Esc back
```

## Assistant activation offer

```text
Assistant                                                                                              Not activated

Welcome to Assistant

Activate Assistant to send your request.

› Start a free trial · 10 requests for 30 days, no payment
  I have an activation code
  Subscribe · 360 € + VAT per lab / year

↑↓ choose · Enter confirm · Esc back
```

## Assistant trial page

```text
Assistant                                                                                              Not activated

Activate Assistant

Open the free trial page in your browser, then press Enter to enter the code.

https://assistant.nixorium.org/trial
Enter continue · Esc back
```

## Assistant activation code

```text
Assistant                                                                                              Not activated

Activate Assistant

https://assistant.nixorium.org/trial

Activation code
› Activation code
Paste the code you receive by email.
Enter activate · Esc back
```

## Assistant free trial quota

```text
Assistant · Example laboratory                                                             Free trial · 3 of 10 left

Try asking, for example:
  Prepare Node.js on pc01–pc06; open port 3000 only to the lab LAN
  pc03 missed the last update: show the evidence and the next check
  Prepare offline Jupyter + pandas lessons; restore the student baseline at boot

















╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│> Describe what your lab should do                                                                                │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
Enter send · Alt+Enter new line · / commands · Ctrl+R review · Esc back
```

## Assistant trial conversion

```text
Assistant · Example laboratory                                                                      Free trial ended

Try asking, for example:
  Prepare Node.js on pc01–pc06; open port 3000 only to the lab LAN
  pc03 missed the last update: show the evidence and the next check
  Prepare offline Jupyter + pandas lessons; restore the student baseline at boot
╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ ! Your free trial has ended. Subscribe to continue:                                                              │
│ https://assistant.nixorium.org/u/EjRWeJASNEjRWeJASNEjRWeJASNEjRWeJASNEjRW                                        │
│ This controller stays activated. Type /code to enter a new activation code.                                      │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯












╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│> Describe what your lab should do                                                                                │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
Enter send · Alt+Enter new line · / commands · Ctrl+R review · Esc back
```

## Separate Assistant consent

```text
Assistant                                                                                              Not activated

Before you send

Your messages, configuration files and saved configuration history read by
Assistant are sent to the Nixorium Assistant service and its AI provider.
Assistant may also read system state, recent logs and configuration files
(never home folders, passwords or keys) from declared computers. Known secret patterns and common private
identifiers are filtered;
filtering is best effort and may leave personal or sensitive text.
Student home files are not read. The service does not keep configuration files.
It keeps messages and answers for 30 days to detect abuse, then deletes them.
Whole conversations, including files and diagnostics read, are kept for
90 days only if you report an answer or turn on sharing below.
Each message you send uses one request.

[ ] Share my conversations with Nixorium for 90 days to improve Assistant
Space toggle sharing · Enter continue · Esc back
```

## Optional conversation sharing

```text
Nixorium  /  Assistant

Conversation sharing

Applies to all future conversations of this lab until you turn it off.

[ ] Share my conversations with Nixorium for 90 days to improve Assistant

▶ Enter Toggle sharing
Esc Cancel  ·  F1 Help
```

## Explicit answer report

```text
Nixorium  /  Assistant

Report this answer

Sends only this conversation, once.

This sends the whole conversation to Nixorium, including your messages, configuration and history contents,
diagnostics and check results Assistant read. Known secret patterns are filtered, but filtering may leave personal
or sensitive text. It is kept for 90 days.

› Helpful
  Wrong

Comment (optional, max 500 characters)
› The scope review was helpful.

▶ Enter Send
↑/↓ Rating  ·  Esc Cancel  ·  F1 Help
```

## Assistant candidate ready

```text
Assistant · Your school                                                                            982 requests left

 Prepare Node.js on pc01–pc02; open port 3000 only to the lab LAN

● Reading flake.nix and client scopes
● Reading the lab network and interface
● Resolving Node.js from the pinned package set
● Drafting modules/web-lab.nix
● Scoping the proposal to pc01–pc02
● Preparing the proposal for native review

● Proposal ready: Node.js on pc01–pc02.

  - TCP 3000 accepts only the lab LAN on its declared interface.
  - The controller and other PCs are unchanged.

  Ctrl+R reviews and saves the proposal. Deployment needs a separate review; nothing is installed yet.



╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ Proposal ready · 2 files changed                                                                Ctrl+R to review │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│> Describe what your lab should do                                                                                │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
Enter send · Alt+Enter new line · / commands · Ctrl+R review · Esc back
```

## Assistant proposal review

```text
Nixorium  /  Assistant  /  Review the proposal

What changes
• modules/web-lab.nix added
• flake.nix changed

Computers affected
pc01–pc02

✓ Checked by Nixorium: the configuration is valid
Saving creates a local commit and a backup. Enter on the first choice also updates this controller; you then
choose the computers.
  Save and update the controller
› Save only — update computers later
  Technical details
  Back
↑↓ choose · Enter confirm · Esc back to the chat
```

## Assistant evaluation failure

```text
Nixorium  /  Assistant  /  Review the proposal

What changes
• modules/web-lab.nix added
• flake.nix changed

Computers affected
pc01–pc02

✗ The configuration is not valid
Saving creates a local commit and a backup. Enter on the first choice also updates this controller; you then
choose the computers.
  Save and update the controller
› Save only — update computers later
  Technical details
  Back
↑↓ choose · Enter confirm · Esc back to the chat
```

## Assistant diagnosis from evidence

```text
Assistant · Example laboratory                                                                     982 requests left

 pc03 missed the last update: show the evidence and the next check

● Reading pc03 update evidence

● pc03 is out of date. Its observed revision is 000000000000; desired 0123456789ab. The cause is not established.

  No failure log was supplied. Next: run the read-only updates diagnostic on pc03; check nix-daemon.service and its
  current-boot journal before reviewing a retry. Nothing was changed.













╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│> Describe what your lab should do                                                                                │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
Enter send · Alt+Enter new line · / commands · Ctrl+R review · Esc back
```

## Assistant offline lesson proposal

```text
Assistant · Example laboratory                                                                     982 requests left

 Prepare offline Jupyter + pandas lessons; restore the student baseline at boot

● Lesson proposal: a pinned Python environment with Jupyter + pandas, plus a notebook and CSV in home-files.json.

  Review and save first. Prepare all dependencies and lesson files on the controller before going offline; deploy
  separately. Lessons then need no downloads.

  At boot, the student home returns to the lesson baseline. Student edits are lost; save work elsewhere before
  restarting. No running session was reset.









╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ Proposal ready · 3 files changed                                                                Ctrl+R to review │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│> Describe what your lab should do                                                                                │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
Enter send · Alt+Enter new line · / commands · Ctrl+R review · Esc back
```

## Assistant saved configuration

```text
Nixorium  /  Assistant  /  Review the proposal

Saved in a local commit. Backup: refs/nixorium/assistant-backups/example. Computers were not updated.
You can update them later from Computers → Update computers: try one computer first, check it, then update the
others.

▶ Enter Chat
F1 Help
```

## Pinned package search

```text
Nixorium  /  Software

  [F2] Configured   ▸ [F3] Search     [F4] Suggested

Search for software
Choose a package, then its computers; you review before saving.

› Package name: inkscape_

› Inkscape · 1.4.2
    Create and edit vector graphics

Saved choices reach the client computers with Computers → Update computers.

▶ Enter Choose computers
Type Search  ·  ↑/↓ Results  ·  Tab Change view  ·  Esc Stop typing  ·  F1 Help
```

## Additive software profile review

```text
Nixorium  /  Software  /  Profile review

Add Essential?
Save adds missing packages and keeps existing choices.

Destination  this controller and all clients, including future ones
Packages     4 add · 0 already configured · 1 excluded
Clients      5 included in the added packages

✓ Selected packages passed the configuration checks
› + chromium · add for this controller and all clients, including future ones
  + ghostty · add for this controller and all clients, including future ones
  + nodejs · add for this controller and all clients, including future ones
  + python3Packages.terminaltexteffects · add for this controller and all clients, including future ones

Now          Save and rebuild pc99 (this controller)
Later        Review and update the affected client computers

▶ Enter Add profile
↑/↓ Inspect  ·  Esc Computers  ·  F1 Help
```

## Contextual client deployment selection

```text
Nixorium  /  Computers  /  Update computers

Update computers
Select → Review → Deploy → Verify

Choose where to apply the saved configuration.
Not checked in this session · r checks the computers now

5 of 5 computers selected

› [x] pc01       10.42.0.11 · Not checked
  [x] pc02       10.42.0.12 · Not checked
  [x] pc03       10.42.0.13 · Not checked
  [x] pc04       10.42.0.14 · Not checked
  [x] pc05       10.42.0.15 · Not checked

○ Opened from a saved software change. Review deploys the complete current configuration.

! Software selection saved locally.

▶ Enter Review
Space Select  ·  a All  ·  n Those needing update  ·  r Check computers  ·  Esc Computers  ·  F1 Help
```

## Update computers review

```text
Nixorium  /  Computers  /  Update computers

Update these computers?

Affects  pc01, pc02, pc03, pc04, pc05 · 5 computers
Builds   5 configurations for 5 computers

Reviewed revision  0123456789abcdef0123456789abcdef01234567

Selected computers — latest availability check:
pc01  10.42.0.11  Reachable at last check
pc02  10.42.0.12  Reachable at last check
pc03  10.42.0.13  Reachable at last check
pc04  10.42.0.14  Reachable at last check
pc05  10.42.0.15  Reachable at last check

Enter updates these computers.

! Programs on the computers may restart during the update
  Computers that did not answer are left unchanged. If an update is interrupted, recovery may be needed before the
next one.

▶ Enter Update computers
Esc Selection  ·  F1 Help
```

## Verified deployment result

```text
Nixorium  /  Computers  /  Update computers

✓ 5 computers updated

pc01 — Updated
  Updated and checked: it is running the reviewed version.

pc02 — Updated
  Updated and checked: it is running the reviewed version.

pc03 — Updated
  Updated and checked: it is running the reviewed version.

pc04 — Updated
  Updated and checked: it is running the reviewed version.

pc05 — Updated
  Updated and checked: it is running the reviewed version.

○ All five clients report the reviewed revision.

n New review  ·  l Logs  ·  Enter Computers  ·  F1 Help
```

## Availability before client deployment

```text
Nixorium  /  Computers  /  Update computers

Update these computers?

Affects  pc01, pc02 · 2 computers
Builds   2 configurations for 2 computers

Reviewed revision  0123456789abcdef0123456789abcdef01234567

Selected computers — latest availability check:
pc01  10.42.0.11  Reachable at last check
pc02  10.42.0.12  Not reached at last check

Enter updates these computers.

! Programs on the computers may restart during the update
  Computers that did not answer are left unchanged. If an update is interrupted, recovery may be needed before the
next one.

▶ Enter Update computers
F2 Reachable only  ·  Esc Selection  ·  F1 Help
```

## A new review for reachable computers only

```text
Nixorium  /  Computers  /  Update computers

Update these computers?

Affects  pc01 · 1 computer
Builds   1 configuration for 1 computer

Reviewed revision  0123456789abcdef0123456789abcdef01234567

Selected computers — latest availability check:
pc01  10.42.0.11  Reachable at last check

Enter updates these computers.

! Programs on the computers may restart during the update
  Computers that did not answer are left unchanged. If an update is interrupted, recovery may be needed before the
next one.

▶ Enter Update computers
Esc Selection  ·  F1 Help
```

## Per-computer deployment outcomes

```text
Nixorium  /  Computers  /  Update computers

! Some computers were not reached

1 updated, 1 not reached

pc01 — Updated
  Updated and checked: it is running the reviewed version.

pc02 — Not reached
  Check power and networking. The update outcome is not known; this does not prove the computer is off or
unchanged.

n New review  ·  l Logs  ·  Enter Computers  ·  F1 Help
```

## Unreachable computers do not waive recovery

```text
Nixorium  /  Computers  /  Update computers

! Deployment requires recovery — some computers were not reached

1 not verified, 1 not reached

pc01 — Not verified
  Inspect authenticated computer state and the private log before a fresh review. Activation completion is
unconfirmed; use reviewed recovery, not a retry.

pc02 — Not reached
  Check power and networking. The update outcome is not known; this does not prove the computer is off or
unchanged. Activation completion is unconfirmed; use reviewed recovery, not a retry.

! The computers were not updated: the update was interrupted.
  Nixorium does not know how it ended on every computer, so maintenance waits until you review it; classroom
controls leave only those computers alone.
  Next: Review the interrupted update (Overview)

l Logs  ·  Enter Computers  ·  F1 Help
```

## PXE network-impact review

```text
Nixorium  /  Installation  /  Install computers

Start network installation?

Affects  enp1s0 · controller network

Enter starts network installation mode.

! The controller's laboratory address (10.42.0.99/24) is paused until network installation is finished
  Client computers on this network will be able to start the installer from the network, served by the controller
at 192.168.1.123. The school's DHCP keeps giving out addresses. Finish installation, or a restart, brings the
laboratory address back. Remote connections to the controller may be interrupted meanwhile.

▶ Enter Start PXE
Esc Cancel  ·  F1 Help
```

## Saved PXE settings

```text
Nixorium  /  Installation  /  Network boot  /  Saved settings

Use the saved installation settings?

Laboratory network    10.42.0.0/24
Controller interface  enp1s0
Controller DHCP       192.168.1.123
Client interface      automatic (wired network card)
Client computers      5
Accounts              admin / teacher / student

Continue the guided preparation using these saved values.
Keys, configuration, controller and installation files are checked next.
This summary is not a readiness check. The installation still requires its own review.

▶ Enter Continue
e Edit settings  ·  Esc Back  ·  F1 Help
```

## USB SSH disk review

```text
Nixorium  /  Installation  /  USB over SSH

Step 6 of 6  Check before erasing

Disk to erase        × /dev/nvme0n1 · 137.4 GB
Disk serial / WWN    NVME-DEMO / demo-wwn
Computer             pc01
Address afterwards   10.42.0.11 on enp1s0
PC on the USB stick  192.168.1.141

Technical details
Size               137438953472 bytes
PC fingerprint     SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
Revision           0123456789abcdef0123456789abcdef01234567
System             /nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-pc01-demo
Shift ↑/↓ scroll · ? help

Type exactly
  ERASE
> _

× Everything on this disk will be permanently deleted
  Only this disk is erased. Computer, disk and revision are checked again just before erasing.

▶ Enter Erase and install
Esc Cancel safely  ·  F1 Help
```

## USB SSH verified result

```text
Nixorium  /  Installation  /  USB over SSH

Install one computer from USB

Next: It now appears among the configured computers.

Technical details are hidden; press d to show them.

✓ pc01 is installed and verified.

r Refresh status  ·  d Details  ·  v Verify installed system  ·  Esc Detach  ·  F1 Help
```

## Shutdown with active sessions

```text
Nixorium  /  Computers  /  Power controls

Shut down 2 eligible clients?

Selected        3
Eligible        2
Controller      excluded
Session safety  unknown states protected

! pc02 · In use · will shut down
✓ pc04 · Logged in, not in use · Ready
○ pc05 · Not reachable · not sent

! 1 computer is in use: those sessions will be shut down and unsaved work may be lost.
Access and session state are checked again immediately before requests are sent.
An accepted request does not prove that a computer is physically off.

Press Enter to shut down 2 computers. 1 session in use will be interrupted.

▶ Enter Send requests
u Unknown sessions  ·  Esc Cancel  ·  F1 Help
```

## Restricted teacher dashboard

```text
Nixorium  /  Computers

Classroom controls
Check computers, watch and lock students' screens, send files, control temporary Internet access, or switch
computers on, shut them down or restart them after a review. Administrative configuration is not available here.

› [v] Classroom view
  [h] Computer inventory
  [x] Power controls
  [i] Internet access
  [l] Lock screens
  [s] Send files

See, control and lock the students' screens, show yours, send your desktop

↑/↓ Select  ·  Enter Open  ·  F1 Help  ·  q Quit
```

## Teacher restart review

```text
Nixorium  /  Computers  /  Power controls

Restart 1 eligible client?

Selected        1
Eligible        1
Controller      excluded
Session safety  unknown states protected

✓ pc01 · Ready

! Selected computers will be restarted; unsaved user work may be lost.
Access and session state are checked again immediately before requests are sent.
An accepted request does not prove that the computer completed its restart.

Press Enter to restart 1 computer.

▶ Enter Send requests
u Unknown sessions  ·  Esc Cancel  ·  F1 Help
```

## Settings accounts menu

```text
Nixorium  /  Maintenance  /  Settings  /  Accounts

Accounts
Choose what to change for the laboratory accounts.

› [n] Change account names
  [p] Change account passwords

Teacher and student user names

↑/↓ Select  ·  Enter Open  ·  Esc Settings  ·  F1 Help
```

## Change account names

```text
Nixorium  /  Maintenance  /  Settings  /  Accounts  /  Names

Step 1 of 2  Accounts

Teacher user name
Teacher account name, distinct from administrator and student.

› teacher█

▶ Enter Continue
Shift+Tab Previous  ·  Esc Cancel  ·  F1 Help
```

## Change account passwords

```text
Nixorium  /  Maintenance  /  Settings  /  Accounts  /  Passwords

Change account passwords
Choose the account whose password you want to change.
Your password stays hidden while you type.
A new password takes effect only after you apply the configuration
to this controller and then update the computers.

› [a] Administrator
  Controller administrator account
  [t] Teacher
  Classroom teacher account
  [s] Student
  Classroom student account

▶ Enter Change password
↑/↓ Select  ·  Esc Accounts  ·  F1 Help
```

## Student workspace favorites

```text
Nixorium  /  Settings  /  Student workspace

Student workspace
Favorite applications (ordered)

Explicit selection
  [ ] firefox.desktop
› [1] code.desktop
  [ ] org.gnome.Nautilus.desktop

Shift ↑/↓ changes the order of the selected favorite.

▶ Enter Keep draft
Space Toggle  ·  i Inherit  ·  c Clear  ·  Esc Cancel field  ·  F1 Help
```

## Initial files at 80x24

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files
Captured 2 files · 2 directories · 8.1 KiB
Captured copies are reviewed below; executability starts off.

› Documents/Lessons/lesson.pdf · 8.0 KiB · 0600 · not executable
  Documents/Lessons/run.sh · 96 B · 0600 · not executable
  Documents/Lessons/ · 0700
  Documents/Lessons/Empty/ · 0700
1–4 of 4 captured entries

Student ownership; source owner/group, permissions, ACLs and attributes
are not inherited. Links and special files are rejected.

↑/↓ Select  ·  Space Executable  ·  Enter Keep copies in draft
Esc Cancel  ·  F1 Help
```

## Initial-file path completion

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files
Choose a local controller file or directory.
Import only non-secret educational materials.
Copies are saved in Git, the Nix store, installer and cache.

Source: /demo/materials/Lessons/_
/demo/materials/Lessons/

▶ Enter Continue
Tab Complete path  ·  Esc Cancel  ·  F1 Help
```

## Initial-file copy and executable review

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files
Captured 2 files · 2 directories · 8.1 KiB
Captured copies are reviewed below; executability starts off.

  Documents/Lessons/lesson.pdf · 8.0 KiB · 0600 · not executable
› Documents/Lessons/run.sh · 96 B · 0700 · executable
  Documents/Lessons/ · 0700
  Documents/Lessons/Empty/ · 0700
1–4 of 4 captured entries

Student ownership; source owner/group, permissions, ACLs and attributes
are not inherited. Links and special files are rejected.

▶ Enter Keep copies in draft
↑/↓ Select  ·  Space Executable  ·  Esc Cancel  ·  F1 Help
```

## Initial-file declaration review

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files
Initial files review: READY
Student: student · controller + 2 clients (shared seed)
Declaration changes:
  add Documents/Lessons/lesson.pdf · 8.0 KiB · 0600 · not executable
  add Documents/Lessons/run.sh · 96 B · 0700 · executable
  add Documents/Lessons/Empty/ · 0 B · 0700 directory
Student-owned files 0600; executable files 0700; directories 0700.
Source owner/group, permissions, ACLs, attributes and links are not inherited.
Copies enter Git, the Nix store, installer and cache. Never import secrets.
Save changes local configuration only. Apply systems separately; files restore at next boot.
Complete draft: 2 files · 2 directories · 8.1 KiB
  Documents/Lessons/lesson.pdf · 8.0 KiB · 0600 · not executable
  Documents/Lessons/run.sh · 96 B · 0700 · executable
  Documents/Lessons/ · 0700
Review lines 1–14 of 21

Enter saves and records the manifest and reviewed copies locally.

▶ Enter Save copies
↑/↓ Scroll  ·  Esc Cancel  ·  F1 Help
```

## Initial-file removal and mode-change review

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files
Initial files review: READY
Student: student · controller + 2 clients (shared seed)
Declaration changes:
  remove Documents/Lessons/previous.pdf · 4.0 KiB · 0600 · not executable
  replace Documents/Lessons/lesson.pdf · 4.0 KiB → 8.0 KiB · 0600 · not executable → 0600 · not executable
  permissions Documents/Lessons/run.sh · 0600 · not executable → 0700 · executable
  add Documents/Lessons/Empty/ · 0 B · 0700 directory
Student-owned files 0600; executable files 0700; directories 0700.
Source owner/group, permissions, ACLs, attributes and links are not inherited.
Copies enter Git, the Nix store, installer and cache. Never import secrets.
Save changes local configuration only. Apply systems separately; files restore at next boot.
Complete draft: 2 files · 2 directories · 8.1 KiB
  Documents/Lessons/lesson.pdf · 8.0 KiB · 0600 · not executable
  Documents/Lessons/run.sh · 96 B · 0700 · executable
Review lines 1–14 of 22

Enter saves and records the manifest and reviewed copies locally.

▶ Enter Save copies
↑/↓ Scroll  ·  Esc Cancel  ·  F1 Help
```

## Initial-file protected save

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files

⣾  Rechecking, saving and recording the reviewed initial files

Elapsed: 0s · Cannot be interrupted; wait for the result.

F1 Help
```

## Initial files saved without activation

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files
✓ SAVED

Initial-file manifest and reviewed copies saved and recorded locally.
✓ Configuration — Saved locally
! This controller — Needs applying
○ Client computers — Not updated here; unknown until checked

Current student sessions stay unchanged. After system application,
the saved files are restored at the next boot.

▶ Enter Apply to this controller
Esc Workspace  ·  F1 Help
```

## Initial-file partial save

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files
! PARTIAL

Captured copies saved; the local Git record could not be confirmed. Inspect Git before applying systems.
! Configuration — Save needs recovery
○ This controller — Unknown; not checked here
○ Client computers — Unknown until checked; none updated here
Inspect the local record before applying systems; never replay this save.

Current student sessions stay unchanged. After system application,
the saved files are restored at the next boot.

g Inspect Git state  ·  Esc Workspace  ·  F1 Help
```

## Initial-file source refusal

```text
Nixorium  /  Settings  /  Student workspace  /  Initial files

Initial files
Choose a local controller file or directory.
Import only non-secret educational materials.
Copies are saved in Git, the Nix store, installer and cache.

Source: /demo/materials_

! The folder contains a symbolic link. Choose a clean materials folder without links, keys or credentials; no
copies were added.

▶ Enter Continue
Tab Complete path  ·  Esc Cancel  ·  F1 Help
```

## Student workspace declaration review

```text
Nixorium  /  Settings  /  Student workspace

Student workspace
Student workspace review: READY
Preference changes:
  Desktop / Favorite applications (ordered): Inherit → code.desktop
Preferences take effect at the next computer start after system application.
Saving does not apply systems or reset a home.
Repository: /demo/lab
File: workspace-profile.json
Revision: 0123456789abcdef0123456789abcdef01234567
Student: student
Destinations:
  controller (controller)
  pc01 (client)
  pc02 (client)
Review lines 1–13 of 38

Enter saves and records only the profile JSON; no system apply or reset.

▶ Enter Save JSON
↑/↓ Scroll  ·  Esc Cancel  ·  F1 Help
```

## Student workspace saved and recorded, not deployed

```text
Nixorium  /  Settings  /  Student workspace

Student workspace
✓ SAVED

Profile saved and recorded locally. No computer or student home changed.
✓ Configuration — Saved locally
! This controller — Needs applying
○ Client computers — Not updated here; unknown until checked

Apply to this controller, then review the computers to update.
Preferences take effect at the next computer start after system application.

▶ Enter Apply to this controller
Esc Settings  ·  F1 Help
```

## Separate controller review after a workspace save

```text
Nixorium  /  Maintenance  /  Controller  /  Review

Apply the saved configuration to this controller?

What changes since the last verified activation:
  • Student preferences

Affects   pc99 (this controller only; client computers are not changed)

Press Enter to build, activate, and verify this controller.

Technical details
Revision  0123456789abcdef0123456789abcdef01234567

! Services and networking may restart
  This connection may be interrupted. Nixorium builds, activates and verifies the reviewed configuration; a reboot
is not normally required.

▶ Enter Apply to controller
Esc Cancel  ·  F1 Help
```

## Workspace versions in system update review

```text
Nixorium  /  Maintenance  /  Update system and packages  /  Review

Review update
Builds passed; programs are checked when they run on a computer.
Lines 1-8 of 8
Nixorium: nixos-26.05 -> nixos-26.05
Applications whose version changes:
  Extension example.extension: 1.0 -> 2.0
  Package nodejs: not selected -> 24.0
  Package vscode: 1.0 -> 2.0

Proposed flake.nix/flake.lock changes:
+ reviewed pin

Enter saves the update and applies it to this controller.
Computers are updated afterwards, from Computers → Update computers.

▶ Enter Apply update
↑/↓ Scroll  ·  F4 Details  ·  Esc Cancel  ·  F1 Help
```

## Cancellable read with elapsed time

```text
Nixorium  /  Maintenance  /  Diagnostics  /  Support report

Local support report
Minimized, not anonymous. No upload or remediation.

⣾  Collecting local diagnostics; no build or upload

Elapsed: 0s · Read-only; Esc cancels. Limit: 2m0s.

Esc Cancel read  ·  F1 Help
```

## Read cancelled without changes

```text
Nixorium  /  Maintenance  /  Diagnostics

Diagnostics

› ! Binary cache could not be reached
  Inspect the local cache service before retrying deployment.

! Loading cancelled. Nothing was changed.

▶ Enter Evidence
e Support report  ·  ↑/↓ Select  ·  r Check again  ·  Esc Maintenance  ·  F1 Help
```

## Local support report preview

```text
Nixorium  /  Maintenance  /  Diagnostics  /  Support report

Local support report
Minimized, not anonymous. No upload or remediation.

{
  "schemaVersion": 1,
  "guidanceVersion": 1,
  "operation": "support-report",
  "collectedAt": "2026-09-29T12:00:00Z",
  "commandVersion": "2.0.0",
  "status": {
    "deploymentVersion": "2.0.0",
    "deploymentMode": "laboratory",
    "deploymentReady": false,
    "gitAvailable": true,
    "gitDirty": false,
    "pxeMode": "stopped",
    "preparedPresent": false,

Lines 1–14 / 41 · Versions, revision, time and counts remain visible.

▶ Enter Save locally
↑/↓ Scroll  ·  r Refresh  ·  Esc Cancel  ·  F1 Help
```

## Local support report saved without upload

```text
Nixorium  /  Maintenance  /  Diagnostics  /  Support report

Local support report
Minimized, not anonymous. No upload or remediation.

✓ SAVED
Saved the exact preview locally. Nothing was uploaded.
Local file: "/demo/state/nixorium/support/support-0123456789abcdef.json"
SHA-256: e83aa5986a7231de21c579859bdc0b5a0c3f8caa60a1370050f21840bd25a30f

▶ Enter Diagnostics
r New preview  ·  Esc Back  ·  F1 Help
```

## Deployment template reset review

```text
Nixorium  /  Maintenance  /  Reset template

Reset deployment template
Replace local customizations. Keep settings, keys and exact input pins.

Configured Nixorium version: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
Replace software with: Essential (shared: controller and clients)
Packages: chromium, ghostty, libreoffice
Discard current custom software, home preferences, assets and modules listed below.
Guided home: enabled with the template's initial desktop/dock/browser profile; VS Code favorite when included.
Effect on student homes requires separate system application and reboot.
Backup: a durable local Git ref to the current committed deployment, before replacement.
Only tracked, committed files enter the backup. Untracked/ignored files remain in place.
No input update, activation, client deployment, live home reset, reboot or push.


Preserved tracked files:
  lab-settings.json
  flake.lock
Lines 1–14 / 21
Type RESET DEPLOYMENT:

▶ Enter Back up and reset
↑/↓ Scroll  ·  Esc Cancel  ·  F1 Help
```

## Deployment template reset losses

```text
Nixorium  /  Maintenance  /  Reset template

Reset deployment template
Replace local customizations. Keep settings, keys and exact input pins.

Only tracked, committed files enter the backup. Untracked/ignored files remain in place.
No input update, activation, client deployment, live home reset, reboot or push.


Preserved tracked files:
  lab-settings.json
  flake.lock
  keys/admin-ssh.pub
  .gitignore

Changes (local modules and assets may contain policies that will be lost):
  replace flake.nix
  remove  modules/local.nix
  add     workspace-profile.json
Lines 8–21 / 21
Type RESET DEPLOYMENT:

▶ Enter Back up and reset
↑/↓ Scroll  ·  Esc Cancel  ·  F1 Help
```

## Deployment reset saved without activation

```text
Nixorium  /  Maintenance  /  Reset template

Reset deployment template
Replace local customizations. Keep settings, keys and exact input pins.

✓ SAVED
Saved and committed locally. Apply systems and reboot separately; nothing pushed.

✓ Configuration — Saved locally
! This controller — Needs applying
○ Client computers — Not updated here; unknown until checked

Student-home changes take effect at the next computer start after system application.
Applying the controller is optional here; no client deployment or reboot starts automatically.

Backup: refs/nixorium/template-backups/demo
Local commit: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb

▶ Enter Apply to this controller
Esc Maintenance  ·  F1 Help
```

## Changed client SSH key

```text
Nixorium  /  Computers  /  Inventory

Computer inventory

pc01
× SSH key changed

Do not update this computer until its identity is verified. If it was deliberately reinstalled, compare its
physical-console fingerprint and review this computer's SSH trust.

Address  10.42.0.11

h Review changed SSH key  ·  i Diagnostics  ·  t Technical  ·  Esc Back  ·  F1 Help
```

## Cancellable SSH trust inspection

```text
Nixorium  /  Computers  /  SSH trust

Review this computer's changed SSH key
⣾  Reading the recorded and offered SSH fingerprints

Elapsed: 0s · Read-only; Esc cancels. Limit: 2m0s.

Esc Cancel read  ·  F1 Help
```

## Single-client SSH trust review

```text
Nixorium  /  Computers  /  SSH trust

Review this computer's changed SSH key
Computer: pc01 (10.42.0.11)
Recorded: SHA256:previous-physical-client
Offered: SHA256:reinstalled-physical-client

Compare the offered fingerprint with this deliberately reinstalled computer's physical console. Other entries are
preserved.

Type ROTATE HOST KEY: _

▶ Enter Save reviewed trust
Esc Cancel  ·  F1 Help
```

## SSH trust saved without deployment

```text
Nixorium  /  Computers  /  Inventory

⣾  Checking the configured computers

Elapsed: 0s · Read-only; Esc cancels. Limit: 2m0s.

! The reviewed SSH key of pc01 is saved; checking the computers again.

Esc Cancel read  ·  F1 Help
```

## Overview finds an existing managed job

```text
Nixorium  /  Overview

Laboratory overview
Items in yellow need your attention; press their number to open them.

  [1] Controller configuration — running

› [c] Computers
  [n] Installation
  [w] Software
  [m] Maintenance
  [a] Assistant

Inventory, updating computers, Internet access and power

v View progress  ·  ↑/↓ Select  ·  Enter Open  ·  r Refresh local state  ·  F1 Help  ·  q Quit
```

## Read-only attachment to existing progress

```text
Nixorium  /  Overview  /  Background work

Managed background work
Read-only attachment. Leaving this view does not stop the job.

Controller configuration — running
Managed work is still running; viewing it does not start another operation.

● Building system · Running
1/4 steps complete
Building the reviewed controller system

Journal unit: nixorium-apply-controller@0123456789abcdef0123456789abcdef01234567.service

Tab Other job  ·  Esc Overview  ·  q Quit  ·  F1 Help
```

## Interrupted managed job

```text
Nixorium  /  Overview  /  Background work

Managed background work
Read-only attachment. Leaving this view does not stop the job.

Controller configuration — interrupted
Progress was left running, but no managed unit is running. Inspect the journal before a fresh review.

Last recorded phase: build
Building the reviewed controller system

Journal unit: nixorium-apply-controller@0123456789abcdef0123456789abcdef01234567.service

Tab Other job  ·  Esc Overview  ·  q Quit  ·  F1 Help
```
<!-- END GENERATED: tui-gallery -->
