# TUI guide and generated gallery

These plain-text screens come from the real Bubble Tea renderer at the 120×30
reference size. Fixtures are synthetic and deterministic: generation does not
inspect a laboratory, contact a computer, query a network, or execute an
operation. Edit the surrounding explanation here; update the marked region
with `scripts/generate-docs.sh --write`.

## Workflow guide

The overview opens Computers, Installation, Software and Maintenance. Startup
reads configuration, inventory and service state; client probes and expensive
readiness checks run when the selected task needs them. Deferred checks never
imply that installation or deployment is ready.

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

Maintenance contains settings, updates, history and recovery. Update validation
may build candidate systems before presenting its review. The ordinary TUI
then saves and activates the controller; client distribution remains separate.
See the [update guide](updates.md) and [administrator guide](../templates/site/README.md)
for complete procedures and the [architecture](management-architecture.md)
for the operation boundaries.

Settings also offers **Student workspace** on supporting pins. Its desktop,
dock, editor/extensions and browser fields are a draft until a complete review
and explicit save. This workflow writes only the profile JSON; Git commit,
runtime opt-in, system deployment and boot reset are separate. It is not a home
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

<!-- BEGIN GENERATED: tui-gallery -->
## Overview

```text
Nixorium  /  Overview

Laboratory overview
Choose an area. Observed state is loaded only when the selected task needs it.

› [c] Computers
  [n] Installation
  [w] Software
  [a] Maintenance

Inventory, system deployment, Internet access and shutdown

↑/↓ Select  ·  Enter Open  ·  F1 Help  ·  q Quit
```

## Pinned package search

```text
Nixorium  /  Software

Software

[F2] Selected   [F3] Search packages   [F4] Suggestions
Choose desired software here. Running clients change only when you deploy them.

Search packages
Uses this deployment's locked Nix packages and overlays; inputs are never updated.

Package name  inkscape_

› Inkscape
    Create and edit vector graphics · inkscape · 1.4.2

This is desired configuration; deploy from Computers to update clients.

Type Search  ·  ↑/↓ Results  ·  Tab Change view  ·  Esc Stop typing  ·  F1 Help
```

## Additive software profile review

```text
Nixorium  /  Software  /  Profile review

Add Essential?
One local change adds every missing declaration shown below.

Destination  this controller and all current or future clients
Packages     5 add · 0 keep scope · 1 excluded
Clients      5 affected by new declarations

✓ Validated together against the pinned package set
› + chromium · add for this controller and all current or future clients
  + ghostty · add for this controller and all current or future clients
  + nodejs · add for this controller and all current or future clients
  + opencode · add for this controller and all current or future clients
  + pi-coding-agent · add for this controller and all current or future clients

Now          Save one update to lab-software.json
Later        Build the controller and deploy affected clients through their normal reviews

↑/↓ Inspect  ·  Enter Add profile  ·  Esc Scope  ·  F1 Help
```

## Contextual client deployment selection

```text
Nixorium  /  Computers  /  Distribute

Distribute the prepared system
Select → Review → Deploy → Verify

Choose where to apply the saved configuration.

5 of 5 computers selected

› [x] pc01       10.42.0.11
  [x] pc02       10.42.0.12
  [x] pc03       10.42.0.13
  [x] pc04       10.42.0.14
  [x] pc05       10.42.0.15

NOTICE
○ Opened from a saved software change. Review deploys the complete current configuration.
! Software selection saved locally.

Space Select  ·  a All  ·  Enter Review  ·  Esc Computers  ·  F1 Help
```

## Client deployment review

```text
Nixorium  /  Computers  /  Distribute

Distribute the system?

Affects  pc01,pc02,pc03,pc04,pc05 · 5 computer(s)

Reviewed revision  0123456789abcdef0123456789abcdef01234567

Type DEPLOY to continue:
> _

NOTICE
! Target services may restart; unreachable computers may remain unchanged
  Every selected configuration is built first. A failed apply may leave mixed target state; a fresh full retry is
safe.

Enter Deploy  ·  Esc Selection  ·  F1 Help
```

## Verified deployment result

```text
Nixorium  /  Computers  /  Distribute

✓ Deployment completed and verified

State: completed   Phase: complete
Build complete: true   Apply complete: true
Authenticated: 5/5   Recorded: 5
Detailed log: /demo/state/deploy-lab.log

NOTICE
○ All five clients report the reviewed revision.

r New review  ·  l Logs  ·  Enter Computers  ·  F1 Help
```

## PXE network-impact review

```text
Nixorium  /  Installation  /  Install computers

Start network installation?

Affects  enp1s0 · controller network

Type START to continue:
> _

NOTICE
! Temporarily remove 10.42.0.99/24; remote connections may be interrupted
  Serve ProxyDHCP, TFTP, HTTP and cache via 192.168.1.123. Institutional DHCP remains authoritative; stop or
reboot recovery restores normal addressing.

Enter Start PXE  ·  Esc Cancel  ·  F1 Help
```

## USB SSH disk review

```text
Nixorium  /  Installation  /  USB over SSH

Install one computer from USB over SSH
Logical identity:    pc01
Physical session:   192.168.1.141 · SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
Installed address:  10.42.0.11 on enp1s0
Disk to erase:      /dev/nvme0n1 · 137438953472 bytes
Disk serial / WWN:  NVME-DEMO / demo-wwn
Revision:           0123456789abcdef0123456789abcdef01234567
System closure:     /nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-pc01-demo
Signed cache:       http://10.42.0.99:5000
Host-key rotation:  false

Type exactly
  ERASE /dev/nvme0n1 FOR pc01
> _

NOTICE
× This permanently erases only the reviewed disk
  Type the exact confirmation below. The worker rechecks identity, cache, revision and disk before mutation.

Enter Erase and install  ·  Esc Cancel safely  ·  F1 Help
```

## USB SSH verified result

```text
Nixorium  /  Installation  /  USB over SSH

Install one computer from USB over SSH
State:               verified
Operation ID:        0123456789abcdef0123456789abcdef
Phase:               post-boot-verify
Disk may be changed: true
Installed:           true
Reboot requested:    true
Boot verified:       true
Dispatch uncertain:  false
Cleanup unconfirmed: false

NOTICE
✓ verified pc01 at 10.42.0.11 with the reviewed revision and system closure

r Refresh status  ·  v Verify installed system  ·  Esc Detach  ·  F1 Help
```

## Shutdown with active sessions

```text
Nixorium  /  Computers  /  Power controls

Shut down 2 eligible client(s)?

Selected  3
Eligible  2
Controller  excluded
Session safety  unknown states protected

! pc02 · Active user session · will shut down
✓ pc04 · Ready
○ pc05 · Not reachable · not sent

! Active user sessions will be shut down; unsaved work may be lost.
Access and session state are checked again immediately before requests are sent.
An accepted request does not prove that a computer is physically off.

Type SHUTDOWN to confirm shutdown of active sessions:
> _

Enter Send requests  ·  u Unknown sessions  ·  Esc Cancel  ·  F1 Help
```

## Restricted teacher dashboard

```text
Nixorium  /  Computers

Classroom controls
Check computers, control temporary Internet access, or review a shutdown or restart. Administrative configuration
is not available here.

› [h] Computer inventory
  [x] Power controls
  [i] Internet access

Check reachability and compare observed systems with the intended revision

↑/↓ Select  ·  Enter Open  ·  F1 Help  ·  q Quit
```

## Teacher restart review

```text
Nixorium  /  Computers  /  Power controls

Restart 1 eligible client(s)?

Selected  1
Eligible  1
Controller  excluded
Session safety  unknown states protected

✓ pc01 · Ready

! Selected computers will be restarted; unsaved user work may be lost.
Access and session state are checked again immediately before requests are sent.
An accepted request does not prove that the computer completed its restart.

Type RESTART to continue:
> _

Enter Send requests  ·  u Unknown sessions  ·  Esc Cancel  ·  F1 Help
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

Space Toggle  ·  i Inherit  ·  c Clear  ·  Enter Keep draft  ·  Esc Cancel field  ·  F1 Help
```

## Student workspace declaration review

```text
Nixorium  /  Settings  /  Student workspace

Student workspace
Student workspace review: READY
Repository: /demo/lab
File: workspace-profile.json
Revision: 0123456789abcdef0123456789abcdef01234567
Student: student
Runtime opt-in: false (not changed by saving)
Destinations:
  controller (controller)
  pc01 (client)
  pc02 (client)
Current declaration: absent (legacy mode)
Proposed declaration:
{
Review lines 1–13 of 31

Only the profile JSON is saved; no commit, deploy or reset.
Type SAVE: _

↑/↓ Scroll  ·  Enter Save JSON  ·  Esc Cancel  ·  F1 Help
```

## Student workspace saved, not deployed

```text
Nixorium  /  Settings  /  Student workspace

Student workspace
✓ SAVED

Profile saved. No computer or student home changed.

Next: separate Git review/commit, then reviewed system deployment.
With runtime opt-in enabled, preferences apply at the next boot reset.

g Git review  ·  Esc Settings  ·  F1 Help
```

## Workspace versions in system update review

```text
Nixorium  /  Maintenance  /  Update system and packages  /  Review

Review update and student workspace
Builds passed; runtime and plugin loading remain unverified.
Workspace and diff lines 1-14 of 41
Student workspace: current pin -> proposed pin (not live versions)
This is a system/package update, not an isolated extension update.
Builds do not certify plugin loading or the latest vendor release.
Extension example.extension: 1.0 -> 2.0
Package nodejs: not selected -> 24.0
Package vscode: 1.0 -> 2.0
Current pin: student student, runtime opt-in false
  controller (controller)
  pc01 (client)
  pc02 (client)
  example.extension requires packages [], extensions []
Current pin effective preferences:
{
  "schemaVersion": 1,

Enter saves/records the pin and activates this controller.
No client deploy or home reset. Verify runtime on one client.

↑/↓ Scroll  ·  F4 Details  ·  Enter Apply update  ·  Esc Cancel  ·  F1 Help
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

NOTICE
! Cancelled; nothing was changed by this read.

e Support report  ·  ↑/↓ Select  ·  Enter Evidence  ·  r Check again  ·  Esc Maintenance  ·  F1 Help
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

↑/↓ Scroll  ·  Enter Save locally  ·  r Refresh  ·  Esc Cancel  ·  F1 Help
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

Enter Diagnostics  ·  r New preview  ·  Esc Back  ·  F1 Help
```

## Deployment template reset review

```text
Nixorium  /  Maintenance  /  Reset template

Reset deployment template
Replace local customizations. Keep settings, keys and exact input pins.

Pinned upstream: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
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

↑/↓ Scroll  ·  Enter Back up and reset  ·  Esc Cancel  ·  F1 Help
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

↑/↓ Scroll  ·  Enter Back up and reset  ·  Esc Cancel  ·  F1 Help
```

## Deployment reset saved without activation

```text
Nixorium  /  Maintenance  /  Reset template

Reset deployment template
Replace local customizations. Keep settings, keys and exact input pins.

✓ SAVED
Saved and committed locally. Apply systems and reboot separately; nothing pushed.

Backup: refs/nixorium/template-backups/demo
Local commit: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb

Enter Maintenance  ·  Esc Back  ·  F1 Help
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

h Review changed SSH key  ·  d Deploy  ·  i Diagnostics  ·  t Technical  ·  Esc Back  ·  F1 Help
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

Enter Save reviewed trust  ·  Esc Cancel  ·  F1 Help
```

## SSH trust saved without deployment

```text
Nixorium  /  Computers  /  SSH trust

Review this computer's changed SSH key
Reviewed trust saved for pc01 only. Refresh Computers before reviewing deployment.

Enter Computer details  ·  Esc Back  ·  F1 Help
```
<!-- END GENERATED: tui-gallery -->
