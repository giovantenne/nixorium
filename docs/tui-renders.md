# TUI render gallery

This hand-maintained tour explains the decisions and transitions in the
management interface. See the [generated TUI gallery](tui-gallery.md) for
byte-checked 120×30 screens produced by the real Bubble Tea renderer. Its data
is synthetic; no real computer is contacted or changed.

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

↑/↓ Select  ·  Enter Open  ·  q Quit  ·  F1 Help
```

Startup reads saved settings, evaluated inventory and current service state.
It does not probe clients, reconcile keys or evaluate system/PXE closures.
Current PXE activity or recovery remains visible; deferring the full checks
never implies that an installation is ready.

## Install computers

**Installation** directly offers `[p] Network boot (PXE)` and `[u] USB over SSH`.
PXE opens its current state without changing settings or networking. Enter
configures/prepares when needed, reviews the start when ready, finishes an
active session, or recovers an interrupted one. There is no separate advanced
PXE entry. Refresh failures keep operational actions unavailable until status
can be checked again.

The guided preparation reuses the current time zone and keyboard; Esc returns
to Installation. It validates and saves the lab form, then prepares the shared
prerequisites:

```text
Nixorium  /  Installation  /  Install computers

Install computers

  ✓ Laboratory settings
  ✓ Save configuration
  ✓ Controller keys
  ● Activate controller
  ○ Choose installation method

⣾ Building and activating the laboratory controller  elapsed 1m12s

l progress details  •  F1 help
```

Missing controller keys are generated, verified, saved, and installed
automatically. Importing an existing key is deliberately outside this ordinary
flow under **Maintenance → Change settings → Controller keys**. The chosen method remains visible throughout the workflow. Choosing USB does
not prepare every PXE closure or change controller networking.

On the PXE branch, after every configured client closure and the immutable
netboot artifacts are prepared, the flow stops at its network confirmation:

```text
Nixorium  /  Installation  /  Install computers

Start network installation?

Affects  enp1s0 · controller network

NOTICE
! Temporarily remove 10.0.0.99/24; remote connections may be interrupted
  Serve ProxyDHCP, TFTP, HTTP and cache via 192.0.2.10.

Type START to continue:
> _

Enter Start PXE  •  Esc Cancel  •  F1 Help
```

There is no controller-side pilot selection. After PXE starts, any configured
computer may boot the installer; identity selection and destructive disk
confirmation happen locally on that computer. Attempting to quit while PXE is
active still requires stopping it or explicitly confirming that it should stay
active.

Choosing **USB over SSH** instead keeps PXE stopped and selects one configured
identity on the controller. The form asks only for the live address, reads the
Ed25519 host key without credentials, and shows its fingerprint for comparison
with the physical Minimal-ISO console. After the operator types `MATCH`, it
reads the temporary password without echo. Hardware inspection leads to a
content-bound review:

```text
Nixorium  /  Installation  /  USB over SSH

Install one computer from USB over SSH
Logical identity:    pc01
Physical session:    192.168.1.141 · SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
Installed address:   10.42.0.11 on enp1s0
Disk to erase:       /dev/nvme0n1 · 137438953472 bytes
Revision:            0123456789abcdef0123456789abcdef01234567
Signed cache:        http://10.42.0.99:5000

Type exactly
  ERASE /dev/nvme0n1 FOR pc01
> _

Enter Erase and install  ·  Esc Cancel safely  ·  F1 Help
```

After dispatch, leaving the view does not cancel the systemd-owned operation.
The result retains its operation ID, reports whether the disk may have changed,
and exposes only state-valid actions: refresh/reconcile, a separately confirmed
reboot, installed-system verification, or close. An interrupted dispatch is
shown as requiring reconciliation and is never offered as an automatic retry.
The generated gallery includes the full disk-review and verified-result frames.

## One route for each operation

Use **Computers → Distribute the prepared system** to reapply the declared
configuration while keeping the disk. Use **Installation → Network boot (PXE) / USB over SSH**
to reinstall through PXE or USB over SSH, with the method-specific disk review.
There is no separate Restore submenu duplicating these choices. A failed
deployment never becomes a reinstall automatically.

Task menus show one action per row with its shortcut in a fixed column and
the focused action's description below. `Esc` returns to the parent area.
Settings categories and account-password choices also display direct shortcuts;
`k` always moves up, while `y` opens controller keys from Settings. `F1` opens
contextual help even in text fields. Data collections retain arrows, search
where available, and `Space` for multiple selection.

## Add or change software

The screen separates desired configuration from discovery. Suggestions and
search results come from pinned inputs and laboratory overlays; no client is
contacted and no input is updated:

```text
Nixorium  /  Software

[F2] Selected   [F3] Search packages   [F4] Suggestions
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

↑/↓ move   space select computer   enter review   esc catalog   F1 help
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

enter interventions   F1 help
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

r new review   l operation history   t technical details   enter interventions   F1 help
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

↑/↓ move   enter details   / search   esc back   F1 help
```

This screen is reached through Computers → Computer inventory or from a focused
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
an update phrase. The result shows the original interface version and
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

Type START to continue:
> _

enter confirm   esc cancel   F1 help
```

The client installer retains a separate local `ERASE` confirmation after
showing the selected identity and disk, then verifies the disk again before erasure.

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
