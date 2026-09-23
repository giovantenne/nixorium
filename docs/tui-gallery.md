# Generated TUI gallery

These plain-text screens come from the real Bubble Tea renderer at the 120×30
reference size. Fixtures are synthetic and deterministic: generation does not
inspect a laboratory, contact a computer, query a network, or execute an
operation. Edit the surrounding explanation here; update the marked region
with `scripts/generate-docs.sh --write`.

<!-- BEGIN GENERATED: tui-gallery -->
## Overview

```text
Nixorium  /  Overview

Laboratory overview
Choose an area. Observed state is loaded only when the selected task needs it.

› Computers
    Inventory, distribute, restore or shut down client computers
  Installation
    Configure the lab, prepare netboot and guide computer installation
  Software
    Review configured choices or search this lab's pinned packages
  Maintenance
    Settings, controller updates, services, revisions, logs and diagnostics

↑/↓ Select  ·  Enter Open  ·  ? Help  ·  q Quit
```

## Pinned package search

```text
Nixorium  /  Software

Software

Selected   [Search packages]   Suggestions
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

Space Select  ·  a All  ·  Enter Review  ·  Esc Computers  ·  ? Help
```

## Client deployment review

```text
Nixorium  /  Computers  /  Distribute

Distribute the system?

Affects  @lab · 5 computer(s)

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

r New review  ·  l Logs  ·  Enter Computers  ·  ? Help
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

## Shutdown with active sessions

```text
Nixorium  /  Computers  /  Shut down

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
<!-- END GENERATED: tui-gallery -->
