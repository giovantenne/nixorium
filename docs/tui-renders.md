# TUI render gallery

These are plain-text versions of Bubble Tea views at the 120×30 reference size.
The core screens come from deterministic presentation fixtures; release data is
synthetic. No real computer was contacted or changed. ANSI styling and trailing
whitespace are removed here.

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

↑/↓ Select  ·  Enter Open  ·  q Quit  ·  F1 Help
```

No client count or reachability state is loaded at startup. If an already
observed PXE recovery condition exists, it appears above the question.

## Install computers

Selecting **Installation → Install computers** opens the complete Laboratory
settings form directly, reusing the controller's existing time zone and
keyboard rather than asking for them again. `Esc` returns to the overview.
Completing the form validates and saves it without a second review screen, then
shows one continuous progress view:

```text
Nixorium  /  Installation  /  Install computers

Install computers

  ✓ Laboratory settings
  ✓ Save configuration
  ✓ Controller keys
  ● Activate controller
  ○ Prepare clients
  ○ Start PXE

⣾ Building and activating the laboratory controller  elapsed 1m12s

l progress details  •  F1 help
```

Missing controller keys are generated, verified, saved, and installed
automatically. Importing an existing key is deliberately outside this ordinary
flow under **Maintenance → Change settings → Advanced keys**.

After every configured client closure and the immutable netboot artifacts are
prepared, the flow stops at its only confirmation:

```text
Nixorium  /  Installation  /  Install computers

Start network installation?

Affects  enp1s0 · controller network

NOTICE
! Temporarily remove 10.0.0.99/24; remote connections may be interrupted
  Serve ProxyDHCP, TFTP, HTTP and cache via 192.0.2.10.

Type START PXE to continue:
> _

Enter Start PXE  •  Esc Cancel  •  F1 Help
```

There is no controller-side pilot selection. After PXE starts, any configured
computer may boot the installer; identity selection and destructive disk
confirmation happen locally on that computer. Attempting to quit while PXE is
active still requires stopping it or explicitly confirming that it should stay
active.

## Restore choice

```text
Nixorium  /  Restore computers

What kind of restoration is needed?

› Reapply the intended system
  Keeps the disk and deploys the declared configuration again.

  Reinstall from scratch
  Opens network installation; the disk confirmed on the computer is erased.

A failed reapply never becomes a reinstall automatically.

↑/↓ move   enter continue   esc interventions   ? help
```

Choosing reinstall opens the same generic PXE control used by ordinary
installation:

```text
Nixorium  /  Computers  /  Restore  /  Reinstall

Reinstall computers

Installation mode:  ✓ ready
Prepared artifacts: ready
Interface:          enp1s0
Service address:    192.0.2.10

Next: start network installation
  Press s to review the temporary address change and start PXE.

p Prepare  •  s Start PXE  •  Esc Computers  •  q Quit  •  F1 Help
```

There is no controller-side target selection or verification session. Each
computer chooses its configured identity and confirms its disk in the local
installer. Reapply continues to use the separate reviewed deployment flow and
never escalates into reinstall.

## Add or change software

The screen separates desired configuration from discovery. Suggestions and
search results come from pinned inputs and laboratory overlays; no client is
contacted and no input is updated:

```text
Nixorium  /  Software

Selected   [Search packages]   Suggestions
Choose desired software here. Running clients change only when you deploy them.

Search packages
Uses this deployment's locked Nix packages and overlays; inputs are never updated.

Package name  python3Packages.num_

› numpy
    Scientific tools for Python · python3Packages.numpy · 2.4.4
  numpy_1
    Scientific tools for Python · python3Packages.numpy_1 · 1.26.4

This list is desired configuration, not a live installed-software inventory.
Deploy from Computers when you want clients to receive the change.

↑/↓ select  •  enter choose scope  •  r remove  •  / search  •  tab change view  •  esc back
```

Search is debounced and displays activity while Nix evaluates the locked
package set. Results from an older query are ignored. Packages excluded by the
deployment's licensing policy, as well as broken, insecure or
platform-incompatible packages, remain visible with a reason and cannot
advance to scope selection.

Selecting a package asks for configuration scope, not which machines happen to
be powered on for today's distribution:

```text
Nixorium  /  Software  /  Scope

Add GIMP
Choose where this declaration applies. This is not the set of computers deployed today.

› All clients, including future clients
  Group graphics (8 clients)
  Selected configured computers

Powered-on clients required: none
Managed file: lab-software.json

↑/↓ move   space select computer   enter review   esc catalog   ? help
```

The review keeps only the information needed for the decision:

```text
Nixorium  /  Software  /  Review

Add gimp?
gimp

Destination  all clients, including future clients
Clients      24 affected by this declaration

✓ Validated against the pinned package set
Save now     Update lab-software.json locally
Later        Deploy clients to install this change

enter save configuration   esc cancel   F1 help
```

After confirmation with Enter, the declaration and its local history are saved
as one user-facing operation:

```text
✓ Software configuration saved

✓ Software selection saved locally
○ System not prepared
○ No client changed

You can apply this configuration to selected computers now or later.

enter interventions   ? help
```

## Shut down computers

Selection is client-only and performs no room-wide check until the operator
continues:

```text
Nixorium  /  Shut down computers

Which client computers should receive the request?
Computers are checked only after you continue. The controller is never included.

2 of 24 clients selected

› [x] pc01       10.0.0.1
  [x] pc02       10.0.0.2
  [ ] pc03       10.0.0.3
  [ ] pc04       10.0.0.4

No request is queued for a computer that is off or unreachable.

space select  •  a all clients  •  enter check  •  esc back
```

Planning checks authenticated management access, interactive sessions, PXE
state, and conflicting client operations. Active sessions remain eligible with
a data-loss warning; unknown session state becomes eligible only after the
operator presses `u` and reviews a new plan:

```text
Nixorium  /  Shut down computers

Shut down 2 eligible client(s)?

Selected  3
Eligible  2
Controller  excluded
Session safety  unknown states acknowledged

! pc01 · Active user session · will shut down
! pc02 · Session unknown · risk acknowledged
○ pc07 · Not reachable · not sent

! Active user sessions will be shut down; unsaved work may be lost.
Checks run again immediately before requests are sent.
An accepted request does not prove that a computer is physically off.

Type SHUTDOWN to confirm shutdown of active sessions:
> _

enter send requests   u unknown sessions   esc cancel   F1 help
```

The final screen reports only what Nixorium can prove about the request:

```text
! Shutdown requests need attention

Accepted  1    Not sent  1    Unconfirmed  1

✓ pc01       accepted
  the operating system accepted the power-off request
! pc02       unconfirmed
  request result could not be confirmed; inspect the computer before retrying
○ pc07       not-sent
  not reachable

Requests accepted for 1 computer; 1 not sent and 1 unconfirmed. Do not retry blindly.

Network loss alone is not evidence of physical power state.

r new review   l operation history   t technical details   enter interventions   ? help
```

## Computers

```text
Nixorium — Computers

23 / 24 reachable · 23 up to date · 0 update ready
Observation: 17:56:00 · r refresh

  pc01       ✓ Up to date                      pc07
  pc02       ✓ Up to date                      ! Could not be reached
  pc03       ✓ Up to date
  pc04       ✓ Up to date                      Check that the computer is powered on and connected
  pc05       ✓ Up to date                      to the lab network.
  pc06       ✓ Up to date
› pc07       ! Could not be reached            Address  10.0.0.7
  pc08       ✓ Up to date                      Checked  17:56:00
  pc09       ✓ Up to date
  pc10       ✓ Up to date

1–10 of 24 computers

↑/↓ move   enter details   / search   esc back   ? help
```

This screen is reached explicitly through Advanced tools or from a focused
operation. “Could not be reached” does not claim that the computer is broken.

## Distribute the prepared system

```text
Nixorium — Distribute the prepared system

Choose where to apply the saved configuration.
Select → Review → Deploy → Verify

2 of 24 computers selected

> [x] pc01       10.0.0.1
  [x] pc02       10.0.0.2
  [ ] pc03       10.0.0.3
  [ ] pc04       10.0.0.4
  [ ] pc05       10.0.0.5
  [ ] pc06       10.0.0.6
  [ ] pc07       10.0.0.7
  [ ] pc08       10.0.0.8

1–8 of 24

space select  •  a all  •  enter review  •  esc back  •  F1 help
```

## Available Nixorium updates

```text
Nixorium — Update Nixorium

Configured target   v2.2.0
Running interface   2.0.0-beta.3
Source  github:giovantenne/nixorium

Available updates
› master  Development branch
  v2.3.0  Latest stable
  v2.2.1
  v2.2.0  Current

Select master for the latest development revision, or choose a tagged release.
Selection starts validation; it does not change files.
A confirmed update activates this controller; client distribution remains separate.

↑/↓ select  •  enter validate  •  p show prereleases  •  r fetch again  •  esc back
```

If discovery fails, the screen offers retry and back. It never exposes an
editable target as a fallback.

While validation runs, the generic spinner is replaced by authored phases from
the update planner:

```text
Nixorium  /  Maintenance  /  Update Nixorium

Update Nixorium

Target: master

  ✓ Inspect deployment
  ✓ Resolve candidate release
  ✓ Evaluate configuration
  ● Build representative outputs · Running
  ○ Prepare review · Waiting
  ○ Verify unchanged deployment · Waiting

⣾ Building the controller  elapsed 2m11s
Representative output 2/5

NOTICE
○ Deployment files and running systems remain unchanged
  Nix may download and build candidate outputs in the local store. This can
  take several minutes; flake.nix and flake.lock are not written.
```

After validation, the review states that only `flake.nix` and `flake.lock` are
saved before this controller is built, activated, and verified. Enter accepts
the visible review; the TUI no longer asks the administrator to retype
`UPDATE NIXORIUM TO …`. The result shows the original interface version and
asks the administrator to reopen Nixorium; clients remain unchanged. Git review,
commit language, hashes, and push actions are absent from this ordinary flow;
Maintenance retains the explicit repository tools.

## Destructive confirmation

```text
Nixorium  /  Review

Start network installation?

Affects  eth0 · controller network

! Temporarily remove 10.0.0.99/24; remote connections may be interrupted.
Serve ProxyDHCP, TFTP, HTTP and cache via 192.168.1.10. Institutional DHCP
remains authoritative. `nixorium pxe stop` or reboot recovery restores normal addressing.

Type START PXE to continue:
> _

enter confirm   esc cancel   F1 help
```

The client installer retains a separate local confirmation containing both the
selected identity and disk before any disk is erased.

## Semantic progress

```text
Nixorium — Distribute the prepared system

⣾  Updating selected computers  elapsed 0s

✓ Building configurations
✓ Revalidating reviewed configuration
● Updating computers · Running
○ Verifying computers · Waiting

l progress details   F1 help

Detailed Colmena output is being saved in the private deployment log.
Closing is disabled while this foreground deployment is running.
```

The presentation tests bound layouts at 80×24, 120×30, and 180×45, but 120×30
is the review reference rather than a minimum requirement.
