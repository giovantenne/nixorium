# TUI render gallery

These are plain-text versions of Bubble Tea views at the 120×30 reference size.
The core screens come from deterministic presentation fixtures; release data is
synthetic. No real computer was contacted or changed. ANSI styling and trailing
whitespace are removed here.

## Intervention entry

```text
Nixorium  /  Computer laboratory

What do you want to do?
Choose an intervention. Computers are checked only when the selected task needs them.

› Restore computers
    Reapply the intended system or reinstall from scratch

  Add or change software
    Open the supported workflow and current advanced path

  Distribute the prepared system
    Update only the computers selected for this intervention

  Install or reinstall computers
    Prepare and control network installation

  Update Nixorium
    Choose from releases fetched from the configured upstream

  Advanced tools
    Inventory, settings, revisions, services, logs and diagnostics

↑/↓ select  •  enter open  •  ? help  •  q quit
```

No client count or reachability state is loaded at startup. If an already
observed PXE recovery condition exists, it appears above the question.

## First setup

```text
Nixorium — First setup

Step 2 of 5
You can leave safely and resume later with `nixorium setup`.

  ✓ Laboratory settings · Complete
› ● Controller · In progress
  ○ Client system · To prepare
  ○ First computer · To install
  ○ Other computers · Whenever you are ready

! Next step
  Apply controller configuration
  Review and activate the controller configuration

enter continue  •  t technical steps  •  esc interventions  •  q quit  •  F1 help
```

The five steps are operator-facing groups. `t` reveals the existing eleven
observed technical stages without making them compete for primary attention.

## Choose and install the pilot computer

```text
Nixorium — First setup / First computer

Installation mode:  ! active
Prepared artifacts: ready
Interface:          enp1s0
Service address:    192.0.2.10

Pilot computer
  pc01

! Continue at pc01
  1. Power it on and choose UEFI network boot.
  2. In the downloaded installer, run /installer/setup.sh.
  3. Choose pc01 and inspect the target disk.
  4. Confirm installation locally, then boot from the installed disk.

! The disk selected on the computer will be erased.
Nixorium has not yet verified an authenticated installed system.
No remote progress is shown because the installer does not provide telemetry.

v check pilot  •  x stop installation  •  esc change pilot  •  q leave PXE active
```

The pilot comes from the evaluated inventory. The application validates and
probes only this identity; powered-off computers outside the selected operation
are not contacted or labelled. Selecting the pilot on the controller does not
select a disk and does not authorise installation on the client.

## Verify the pilot and finish a partial session

```text
Nixorium — First setup / First computer

Pilot computer
  pc01

✓ Up to date
✓ Technical verification succeeded
Authenticated management reports the saved revision as active.

Check at the computer
  • Log in and open the expected desktop session.
  • Check required software, network and classroom peripherals.
  • Confirm that the computer started from its installed disk.

enter practical check passed  •  v check again  •  x stop installation
```

After the practical check, the operator may install another computer or stop
PXE. The completion summary says which identities were verified **in this
session** and counts the rest as unverified, not failed. Attempting to quit while
PXE is active first reviews the consequences and requires the exact phrase
`LEAVE PXE ACTIVE`.

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

Choosing reinstall does not jump directly to a generic PXE console:

```text
Nixorium — Restore / Reinstall from scratch

Installation mode:  ✓ ready
Prepared artifacts: ready

Choose a computer to reinstall
› pc01         10.0.0.1
  pc02         10.0.0.2
  pc03         10.0.0.3

The identity comes from the saved inventory. Disk selection and erasure are confirmed locally.

↑/↓ move  •  enter select  •  esc back  •  q quit
```

After selection the screen repeats the identity-specific disk warning before
PXE review. Once the local reinstall and installed-disk boot are complete, `v`
checks only that computer. Reapply continues to use the separate reviewed
deployment flow and never escalates into reinstall.

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

## Available Nixorium releases

```text
Nixorium — Update Nixorium

Current release  v2.2.0
Source  github:giovantenne/nixorium

Available releases
› v2.3.0  Latest stable
  v2.2.1
  v2.2.0  Current

Selecting a release starts validation; it does not change files.
Controller activation and client distribution remain separate operations.

↑/↓ select  •  enter validate  •  p show prereleases  •  r fetch again  •  esc back
```

If discovery fails, the screen offers retry and back. It never exposes an
editable target as a fallback.

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
