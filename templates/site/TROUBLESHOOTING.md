# Nixorium troubleshooting and recovery

This guide is for a controller installed from a generated private deployment.
Commands use the installed `nixorium` binary. Before the first successful
controller apply, run the same command as `nix run .#nixorium -- ...` from the
deployment repository.

Do not delete state, keys, Git changes, network addresses, or Nix store paths
as a first response. Nixorium operations either reconcile observed state or
report when a retry is unsafe.

For a first trial, use the [isolated VM recipe](https://github.com/giovantenne/nixorium/blob/master/docs/evaluation-environment.md).
For day-to-day software, reinstallation, and snapshot procedures, start with
the [administrator guide](https://github.com/giovantenne/nixorium/blob/master/templates/site/README.md).
This page covers diagnosis and recovery when an operation has not reached its
expected state.

## First diagnostics

When preparing information to share, use `nixorium support preview --json`
or `nixorium support export` for an interactive review and private local save.
The [support-report contract](https://github.com/giovantenne/nixorium/blob/master/docs/support-report.md)
describes retained metadata and limitations. There is no upload or automatic
fix. The detailed commands below may display private values and raw errors;
do not attach their output without a separate review.

Start with what blocks operations. `nixorium recovery status` reads only local
state (no Nix evaluation, no network) and lists each interrupted or blocking
item with its next step; the Overview shows the same items when the dashboard
opens. When the laboratory cannot be read at all, the dashboard opens in
**safe mode** with these lists, diagnostics, Git review and the support report.
`nixorium doctor` still reports local checks in that case, with the first
configuration error as `CONFIG-EVAL`.

Run these read-only commands from the deployment repository:

```sh
nixorium recovery status
nixorium status
nixorium doctor
nixorium setup status
nixorium services
nixorium logs
git status --short
```

Use `nixorium doctor --full` only when a real controller build is useful; it is
intentionally slower. Use `nixorium logs show OPERATION_LOG_ID` for the bounded
tail of a listed deployment log. Privileged systemd actions keep their full
output in journald, so use the exact unit named by the failed report.

## Error codes and next steps

When an operation is refused or its result is uncertain, Nixorium names the
situation, why it stops there and the next step. In the TUI the notice reads,
for example:

```text
! The computers were not updated: a previous update of the student computers was interrupted.
  Nixorium does not know how it ended on every computer, so maintenance waits until you review it; classroom controls leave only those computers alone.
  Next: Review the interrupted update (Enter)
```

On a result screen, Enter opens the screen that resolves the situation and `d`
shows the technical message. A failed CLI command ends with `Next (CODE): …`,
and JSON issues carry a `next` object with the same code and texts; the JSON
`message` keeps the technical text. The codes are stable:

| Code | Situation | Next step |
|---|---|---|
| `DEPLOY-PENDING` | A previous update of the student computers was interrupted | Review the interrupted update ([Interrupted client deployment](#interrupted-client-deployment)) |
| `OP-BUSY` | Another Nixorium operation is still running | Wait for it to finish, or open its progress from the Overview ([Another operation is already running](#another-operation-is-already-running)) |
| `USB-RESERVED` | A USB installation was not finished | Resume or close the USB installation ([A USB installation was interrupted](#a-usb-installation-was-interrupted)) |
| `RESET-PENDING` | A reset of the configuration template was interrupted | Finish or undo the reset ([Deployment template reset recovery](https://github.com/giovantenne/nixorium/blob/master/docs/deployment-template-reset.md#backup-and-recovery); also an interrupted Assistant save) |
| `GIT-DIRTY` | The configuration has unsaved changes | Review the changes and save or discard them ([The Git tree is dirty](#the-git-tree-is-dirty)) |
| `REVIEW-EXPIRED` | The review is too old | Create a new review and confirm again |
| `REVIEW-CHANGED` | Something changed after the review | Create a new review and confirm again |
| `PXE-ACTIVE` | Network installation is still active | Finish the network installation, or recover the controller network ([The controller network is inconsistent](#the-controller-network-is-inconsistent)) |
| `SETTINGS-INVALID` | The laboratory settings are not valid | Repair the settings ([Configuration is invalid](#configuration-is-invalid)) |
| `NETWORK-LOCKED` | The laboratory network cannot be changed while computers are configured | Keep the saved network values in Settings and move the computers with the guided change of the laboratory network (Maintenance → Change network addressing); the controller number stays locked, device names and the controller DHCP address can still change |
| `NETWORK-CHANGING` | The laboratory network is being changed | Continue the change (Maintenance → Change network addressing; `nixorium network migrate status`): apply to the controller, update the computers, then finish or undo it (administrator guide, "Changing the laboratory network") |
| `CLIENT-UNCONFIRMED` | A computer did not confirm the request | Check the computer in Computer inventory before trying again |
| `DISK-LOW` | The controller is low on disk space | Free disk space (Maintenance → Free disk space) |
| `CONTROLLER-NOT-APPLIED` | The controller is not running its saved configuration | Apply the saved configuration to the controller ([Controller apply failed](#controller-apply-failed)) |
| `UPDATES-QUEUED` | Some computers will update when they are switched on | Nothing to do (Computers → Queued updates shows them; review again those marked as changed) |
| `UPDATES-NEED-REVIEW` | A queued update needs a new review | Review the queued updates (Computers → Queued updates; `nixorium deploy queue status`): the configuration changed after queueing, or the update kept failing |
| `BACKUP-DUE` | A backup of this laboratory is due | Back up the lab ([Backups and restoration](#backups-and-restoration)) |
| `BACKUP-NONE` | This laboratory has no backup yet | Back up the lab when its setup is done ([Backups and restoration](#backups-and-restoration)) |
| `EVAL-FAILED` | The configuration has an error | Open Diagnostics and fix the first reported error (`nixorium doctor`; do not retry other operations) |
| `PIN-OUTDATED` | The saved Nixorium version is recorded inconsistently | Repeat the update for the configured Nixorium version ([Updating the computers fails with a read-only flake.lock](#updating-the-computers-fails-with-a-read-only-flakelock)) |

## The controller DHCP lease changed

Normal administration and computer updates use the static laboratory address.
PXE preparation separately binds one live non-static controller address to the
session so a routine DHCP lease change does not require a configuration commit.

1. Stop an active installation session with `nixorium pxe stop`.
2. Run `nixorium pxe prepare` again.
3. Confirm with `nixorium status` that the prepared controller address matches
   the live lease, then start PXE normally.

Preparation prefers `masterDhcpIp` while it is assigned. If that hint is stale,
exactly one other usable non-static, non-link-local IPv4 address is accepted
and recorded without changing the deployment. If multiple candidates exist, preparation fails and
retains the previous manifest. Inspect them with
`ip -4 -o addr show dev INTERFACE scope global`; remove the unintended address
or update the configured hint through `nixorium setup configure`, commit it,
and apply the controller before retrying.

## The controller network is inconsistent

`NETWORK-INTERFACE` or `NETWORK-STATIC-IP` means the declared interface or
local address ownership did not match observation (or could not be inspected).
First inspect `nixorium status`, `nixorium doctor`, and `ip -4 -o addr show`.
Compare only with the evaluated inventory, not a remembered interface name.
The static controller address is intentionally absent during active PXE mode.

If the configuration is wrong, propose a reviewed settings change and a
separate controller activation. If PXE was interrupted, use the lifecycle
recovery procedure instead. Stop when ownership is ambiguous or a session is
unfinished; do not remove addresses, stop institutional DHCP, disable the
firewall or start the internal network service to silence a finding.

### Recover the controller network

An interrupted network installation leaves the controller on its temporary
network setup. The Overview then lists **Network installation is still
active**; open it, or **Installation → Network boot (PXE)**, or press Enter on
a refusal that names it. The screen is titled **Recover the controller
network** and has three parts:

- **What Nixorium found**: how the session was left (the temporary network
  setup is still in place but nothing serves installations; the network boot
  service runs without its temporary network setup; or one of its services
  failed), with the state of the network boot service and of the temporary
  network setup.
- **What you can do**: Enter recovers the network (restores the normal
  address and turns network installation off); `x` stops both network
  installation services and checks the normal address.
- **Confirm**: Enter. Recovery changes only the controller's own network and
  nothing on the student computers, so no word is typed (ADR 0026).

The result says whether the controller network is back to normal; `d` shows
the technical text of a failed attempt. If the normal controller address did
not come back, apply the saved configuration to the controller
(**Maintenance → Apply to controller**) and open Diagnostics. From the
command line, `nixorium pxe recover` and `nixorium pxe stop` start their
output with the same sentence and keep their JSON report unchanged.

## Build resources and controller tools

`DISK-FREE` warns below 10 GiB available or when the filesystem probe failed;
it is not an estimate of the space required by the next build. `COMMAND-NIX`,
`COMMAND-GIT`, `COMMAND-SYSTEMCTL`, `COMMAND-SSH` and `COMMAND-NIX-COPY-CLOSURE` report
missing executables in the command's environment. Neither condition starts
a repair. Inspect `df -h /nix` and `command -v nix git systemctl ssh nix-copy-closure`.

Run the packaged command from the deployment's supported controller environment.
A missing tool may require a separately reviewed controller rebuild. For low
space, review retention, snapshots and backups before approving any cleanup.
Stop if storage is failing or the required environment cannot be established.
Do not run automatic garbage collection or delete store paths to make a
diagnostic report pass.

## A PXE client does not appear

First establish whether the client firmware sent a network-boot request or
whether the controller listeners are unavailable.

```sh
nixorium services
nixorium doctor
journalctl -u nixorium-pxe.service -b
```

- Confirm that `nixorium pxe start` reports `active`, and that the client is on
  the configured laboratory interface/VLAN.
- Confirm UEFI network boot is enabled and ahead of the local disk for this
  boot. Nixorium publishes `snponly.efi`; legacy BIOS-only firmware is outside
  the current supported path.
- Check for an institutional DHCP/ProxyDHCP policy, VLAN isolation, or another
  service already using UDP 67/69 or TCP 8080. Do not disable the firewall or
  start a second dnsmasq/HTTP process as a workaround.
- If a prior session was interrupted, recover the controller network
  ([Recover the controller network](#recover-the-controller-network), or
  `nixorium pxe recover`), then prepare and start again. Boot-time recovery also reconciles an unfinished controller
  network transition when its durable PXE session record exists; with no
  recorded transition it is skipped and cannot interfere with controller
  activation.

## The PXE menu appears but boot fails

Stop the session before rebuilding its immutable inputs:

```sh
nixorium pxe stop
nixorium pxe prepare
nixorium doctor
nixorium pxe start
```

Preparation verifies the current Git revision, live DHCP address, cache health,
kernel, initrd, iPXE script, firmware, and every configured client closure. The
dashboard shows the current phase and five recent safe activities; read
`journalctl -u nixorium-prepare-pxe.service -b` for verbose Nix output if it
fails. Do not hand-edit
`/var/lib/nixorium/prepared/prepared.json` or replace its store paths.

## The binary cache is unavailable

```sh
nixorium services
nixorium doctor
journalctl -u harmonia.service -b
nixorium services restart cache
```

The restart action is confirmed with Enter on its review and verifies both the unit and
HTTP endpoint. If the signing key is missing, run `nixorium setup keys`, review
only the public-key change, then `nixorium setup install-secrets`. A public/
private mismatch or a different installed secret fails closed; do not overwrite
either side until its provenance is understood.

## A USB/SSH live client cannot be verified

Use the official NixOS 26.05 ISO for `x86_64-linux`, either Minimal or
Graphical (GNOME), booted in
UEFI mode, preferably on the laboratory cable (Wi-Fi, connected with `nmtui`
in the live environment, serves only the installation itself). Keep its
physical console visible and recheck:

```sh
passwd
systemctl is-active sshd
ip -4 -br address
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

Enter the canonical address in the TUI. Nixorium reads the current Ed25519 host
key without sending the password and shows its complete `SHA256:` fingerprint.
Compare it with the value on the physical console and type `MATCH` only when
they are identical. Do not approve a fingerprint using DNS, an earlier boot,
or `known_hosts`. A changed key is rejected before password authentication and
no operation key is installed. A confirmed fingerprint with a wrong temporary
password also leaves no operation key. Correct the address or restart the live
environment and make a fresh reviewed attempt; do not disable host-key checking
or enable persistent root password access.

If the address cannot be reached, check routing and wireless client isolation,
and confirm that it is not the controller or a configured static client address.
The declared client interface must carry that live address; when it does not,
the TUI offers to save the observed card for that computer and prepares it
again. USB/SSH does not
need PXE, ProxyDHCP, or a controller address transition, but the controller must
reach the live client's SSH server and the client must reach the signed Harmonia
cache. The installed client takes its laboratory address on its wired card
only: connect the cable before the restart, or verification after boot finds
nothing at the static address. Wi-Fi profiles and credentials are not copied
into the installed system. Do not bridge an isolated trial onto an institutional
LAN as a troubleshooting shortcut.

## USB installation refuses the disk or cache

If a connection attempt returns to `artifacts-ready`, only the controller-side
build is ready; the live SSH session has not been verified. The TUI keeps the
connection error visible during the current TUI session, even after refreshing
status, and returns to physical host-key verification before another password
attempt. When reattaching an artifacts-only operation, use **Resume the
installation** to continue with the same prepared artifacts, or **Cancel
safely** to release them. Do not interpret `artifacts-ready` as installation progress or
disable strict SSH checks. Older versions could hide the initial connection
error behind `worker does not own the verified live session`; inspect the live
ISO's `sshd` journal and use an updated CLI/TUI to capture the original error.

The hardware probe excludes the live ISO's boot medium, read-only/removable
media, mounted/active disks, and unsuitable devices. Compare the reviewed
canonical path, model, serial, size, and boot-medium evidence with `lsblk` on
the physical console. Do not detach the boot medium or alter mounts to force a
disk into the eligible list; reboot the supported ISO with only the intended
disposable target attached.

An unreachable cache, wrong signing key, missing target closure, changed
network interface, or changed deployment revision is a hard refusal before
Disko. Check `nixorium services`, `nixorium doctor`, and the operation status;
repair the cache through its managed workflow, then make a fresh plan. Never
add a public substituter or set signature checking/fallback to false on the
client.

If the private operation log says `/mnt is already occupied`, first run
`findmnt -rn --mountpoint /mnt` on the live ISO. A result means an exact mount
must be investigated; no result means `/mnt` is free. Current Nixorium releases
use this exact-mount test. Do not unmount anything merely because the older,
broader target lookup reported the live ISO root filesystem.

## A USB installation was interrupted

The Overview lists **A USB installation was not finished** when the dashboard
opens; open that row, **Installation → USB over SSH**, or press Enter on a
refusal that names it. The screen is titled **Resume or close the USB
installation** and has three parts:

- **What Nixorium found**: the computer (it is still booted from the USB
  stick and answers; it does not answer; Nixorium is no longer connected to
  it; or it was last seen booted from the USB stick) and the disk (the disk
  was not touched; it may have been partly written; or the system is
  installed on it).
- **What you can do**, each named by what it does: **Resume the
  installation** (`a`, connect again; the identity is checked before any
  password), **Check how far it got** (`n`), **Restart the computer** (`b`),
  **Verify installed system** (`v`), **Cancel safely** (`x`) and **Close this
  record** (`c`). Only the ways out that are safe in the observed state are
  offered.
- **Confirm**: **Cancel safely**, and closing a record whose disk was never
  touched, ask only for Enter. Closing after the disk was written asks you to
  type `CLOSE`: the system stays on the disk, but Nixorium does not check that
  the computer starts correctly.

The headline states the outcome in plain words; `d` shows the worker's own
message and the raw fields. From the CLI, `nixorium install usb status`
without `--id` shows the unfinished operation and its ID, and starts with the
same findings and the command for each way out. Then inspect the exact
operation without starting another install:

```sh
nixorium install usb status
nixorium install usb status --id 0123456789abcdef0123456789abcdef --json
nixorium install usb reconcile --id 0123456789abcdef0123456789abcdef
```

If apply may have been dispatched, assume the selected disk may be partially
partitioned. Reconciliation reads the remote receipt and service state; it does
not rerun Disko. After a worker/controller restart, run `install usb start` for
the same host and physically re-enter the same live address, fingerprint, and
password. In the guided TUI, Nixorium instead re-observes the key for physical
comparison and asks the operator to re-enter only the address and password. It
reattaches only when the fingerprint and live boot ID also match, then performs
status reconciliation only. A different boot remains blocked and the recovered
operation key is revoked.

When a failed remote receipt proves that disk mutation did not start, status
revokes the live key and releases the controller reservation automatically.
Use **Cancel safely** if that cleanup remains pending; never move coordination
files by hand. Do not cancel an uncertain or post-mutation failure. Use `reboot`
only after status reports the installation ready, remove the USB first, and
verify the installed revision after disk boot. `close` releases a completed or
deliberately abandoned record; it does not make an uncertain disk safe. If the
live ISO is gone before any apply was dispatched, choose **Close this record**
(Enter), or run `nixorium install usb close --id OPERATION_ID` and answer the
yes/no question. Closing a never-dispatched record discards the operation's
local access credentials and releases its reservation without claiming remote
key revocation. The old record is retained;
a new installation must establish identity and review the disk again.

When the disk may have been written, **Close this record** asks you to type
`CLOSE`: before the restart of an installed computer, and also when the
computer does not come back from the restart (a broken disk, a computer that
starts from the USB stick again) or when the outcome cannot be proved. Closing
frees the controller for other operations and keeps the record as closed
without verification; the live key is revoked when the computer started from
the USB stick still answers, and the controller's copy is always discarded.
Nixorium does not check that computer: check it, or install it again with a
new review. Prefer **Verify now** when the computer is back.

For a reinstall, a different key at the configured static address is expected
only after proving the old machine is the selected physical client. Approve
`ROTATE HOST KEY` during the review; Nixorium replaces just that entry after
the newly installed host passes verification. Never delete the entire
`known_hosts` file or accept a changed key merely because the address matches.

If **Computers → Update computers** finds an unfinished USB installation, it opens
**Finish installation** before asking you to confirm deployment, with what
Nixorium found about that computer and its disk. For an installed computer
awaiting its final check, choose **Verify … and resume**. Turn on that
computer, boot from the installed disk, and check its network cable. Failed
verification keeps the operation protected and offers retry and technical
details. Active or uncertain installations offer **Resume or close the USB
installation** instead; return with Esc after resolving the existing
operation. Do not start
a replacement installation to clear this block. Successful recovery returns to
a fresh deployment review with the same selected computers and requires a new
deployment confirmation. It never starts deployment automatically.

**Update Nixorium** and **Maintenance → Apply to controller** stay available
while a USB installation is reserved, unless its disk is being installed.
Controller rebuild may proceed while a completed USB installation still awaits
reboot or verification, even if the client is offline, and while a reserved
installation never started (the disk was never touched and no reviewed
installation waits for its `ERASE` confirmation). The controller validates the
persisted session, pauses the worker without deleting its state or credentials,
holds the normal operation lock, and resumes the worker on success or failure.
An open installation review, active, failed, or unknown disk work remains
protected: cancel the review safely or wait for the disk work.
Other client operations still respect the reservation, and the result of the
activation still shows **A USB installation was not finished**. Older
controller apply services that block every reserved installation need the
corrected apply service as well as the worker; never remove their coordination
files manually. The [last resort](#last-resort-activate-the-controller-with-nixos)
applies here too.

Complete `install usb verify --id OPERATION_ID` when the installed client is
available. Verification checks the preserved host key, hostname, exact system
closure, and reviewed revision before releasing the reservation. A `read-only
file system` error while publishing known hosts or logs indicates an older
worker sandbox. Apply the corrected worker and filesystem configuration
together, preserving operation state and credentials, then repeat verification.
After an authorized client reboot, verification can also resume if a controller
reboot cleared the ISO credentials: it authenticates the installed host with
the administrator key and the original persisted host pin and revision.
After a reboot requested from the TUI, this same read-only verification is
retried automatically while the installation result remains open; **Verify
now** triggers an immediate retry. The remote helper schedules the reboot only
after preparing a durable acknowledgement, so the expected SSH disconnect is
not mistaken for an uncertain reboot. CLI operation remains explicit: run
`reboot` once, then `verify` after the installed system is reachable.
Do not delete the reservation or relax protection of the entire SSH directory.
Current controller activation preserves the standard `~/.ssh/known_hosts`
path as a link into the dedicated `~/.ssh/nixorium-known-hosts` directory.
Migration refuses conflicting files, and a retry never overwrites a differing
known-hosts backup; retain those files for investigation.

## Another operation is already running

A refused operation names the running one when it can be confirmed, for
example `another Nixorium controller or client operation is already running:
Update computers, started by admin at 10:02`. Wait for it, or open its
progress from the Overview. The description is read from the lock file and
checked against the live process; it is never a reason to remove the lock.
Without a confirmed holder the message says so; do not delete
`/var/lib/nixorium/coordination/operation.lock`.

Classroom controls (Internet access, shut down, restart and the schedule) do
not wait for a running operation: they leave alone only the computers being
updated, or named by an interrupted update.

## Codes shown to the teacher

Classroom controls replace administrative detail with a short code. The
technical cause stays in `journalctl -u nixorium-classroom.service`.

| Code | Meaning | What the administrator does |
|---|---|---|
| `OP-BUSY` | Another Nixorium operation is still running | Let it finish; see [Another operation is already running](#another-operation-is-already-running) |
| `DEPLOY-PENDING` | A previous update of the student computers was interrupted | Review the interrupted update ([Interrupted client deployment](#interrupted-client-deployment)) |
| `USB-RESERVED` | A USB installation was not finished | Resume or close the USB installation ([A USB installation was interrupted](#a-usb-installation-was-interrupted)) |
| `PXE-ACTIVE` | Network installation is still active | Finish installation or run `nixorium pxe recover` |
| `CLASSROOM-LOAD` | The classroom service could not read the committed configuration | Read the service journal; fix and commit the configuration |
| `CLASSROOM-SERVICE` | The classroom service is not running or not answering | `systemctl status nixorium-classroom.service`; apply the controller configuration if it is missing |
| `CLASSROOM-VIEW-OFF` | The classroom view is turned off in this laboratory's settings. If the setting is on, the controller has not yet applied the configuration that writes `/etc/nixorium/classroom-view.json`, or `nixorium` runs from outside the applied system (for example `nix run`) | Turn it on in **Maintenance → Change settings → Classroom view**, save, apply the controller and update the client computers. If it is already on, use **Apply to controller** and run `nixorium` from the controller's own session |
| `DISK-LOW` | Collect from students' desktops found too little free space on the controller to keep a computer's files (it keeps 1 GB free) | Free disk space (Maintenance → Free disk space) and collect again |

Classroom controls read the last **committed** configuration, so uncommitted
edits in the deployment repository do not affect them until they are saved.

## One client is offline or unknown

Run `nixorium hosts`. `unreachable` means the bounded network probe received no
answer; `SSH unavailable` distinguishes a reachable machine without the expected
SSH service. `unknown` deployment state means Nixorium could not authenticate
and prove the active revision—it does not mean success or failure.

Check power, cabling/VLAN, the configured address, and SSH on that client. Treat
a changed host key as a security event; do not delete `known_hosts` entries
blindly. Clients installed before the fixed host-state helper remain `unknown`
until their next normal deployment.

## Computer not reachable

A computer that does not answer at its laboratory address is not necessarily
switched off. The review of an interrupted update, the inventory and a client
update all reach a computer only at its static laboratory address. It may also
be:

- still running an old version that never took the laboratory address;
- connected with a second network card, so the laboratory profile came up on
  the card without the cable;
- the computer of an unfinished USB or network installation, which never
  reached its installed system.

At the computer's console, as the administrator account:

```sh
ip -4 addr                          # is the laboratory address on a card with a link?
nmcli connection show --active      # is the nixorium-lab profile active, and on which device?
nmcli connection up nixorium-lab    # bring the laboratory profile up
systemctl list-jobs                 # is an activation or start-up job still running?
```

The controller uses the same profile since 3.10: the same commands apply on
its console when it lost its laboratory address. Add `ifname DEVICE` to
`nmcli connection up nixorium-lab` to choose the card
that has the cable. Without an interface name, the laboratory profile is active
on the first wired card that comes up; for a computer with several wired cards,
name the right one with a `hostIfaceNames` entry in the private deployment
(`"lab": { "hostIfaceNames": { "pc07": "enp3s0" } }` in `lab-settings.json`),
save it through the reviewed settings change, apply the controller and update
that computer. Finish or close an unfinished USB installation from
**Installation → USB over SSH**, and a network installation from
**Installation → Network boot (PXE)**, before installing that computer again.

## A computer does not start after Switch on

**Switch on** sends a Wake-on-LAN packet; "sent" means only that the packet
left the controller. The controller does not know yet the address of a
computer listed as "address unknown": start it by hand once and open the
classroom view or run **Computer inventory** while it is on. When a computer
with a known address stays off:

- check its BIOS/UEFI: waking from the network must be allowed (often "Wake
  on LAN" or "Power on by PCI-E"); some computers also need "ErP" or "Deep
  sleep" turned off;
- restart it once after the client update that enabled Wake-on-LAN: the
  network card is armed at start-up;
- keep it plugged in: a switched-off power strip or a power cut disarms the
  card until the computer is started by hand;
- check that it is on the same network segment as the controller: the packet
  is a broadcast and does not cross a router.

## A student computer stops before the login screen

The boot shows `Failed to start Restore the configured student workspace
before login` and no login screen appears. The student home is restored at
every boot; when that cannot finish, login stays closed so nobody works in a
half-restored home. The computer still answers SSH from the controller.

A computer that was switched off during the restore repairs itself at the next
boot: before the snapshot of the old home exists, the restore starts again;
after it, the restore completes and publishes that snapshot. Restart it once
before investigating. Read the reason from the controller:

```sh
ssh root@<client-address> 'journalctl -b -u home-reset.service --no-pager | tail -5'
ssh root@<client-address> 'ls -la /var/lib/home-snapshots/.workspace-reset'
```

- `managed account still has running processes (pid …, "…")`: a service ran as
  the student during the restore. Nothing changed in the home and the next
  boot tries again; report the named process if it repeats.
- `automatic recovery of an interrupted reset already failed`, or any other
  message with `administrator recovery is required`: the computer stays
  closed. Keep every file in `.workspace-reset`, especially `previous`, which
  can be the only copy of the student's files, and ask for support with the
  journal output. Never delete the folder, rerun `nixorium-home-reset` or
  disable the student profile to open the login.

## A client was deliberately reinstalled and its SSH key changed

An address match does not prove identity. Save and commit the declared inventory.
Stop PXE first and finish or reconcile
any protected USB installation or interrupted deployment. On the physical
console of the reinstalled computer, run
`ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub` and compare its SHA256
fingerprint with the offered fingerprint in a fresh review:

```sh
nixorium host-key plan --host pc01
nixorium host-key apply --host pc01 --expect REVIEW_TOKEN
```

Replace `REVIEW_TOKEN` with the plan's exact token. Interactive apply requires
the exact confirmation shown in the review. `--yes` is available only for an
explicitly authorized, already physically verified review; it does not bypass
the token or rechecks. The administrator TUI offers **Review changed SSH key** in
the changed-key computer's details. Esc cancels inspection/review without
changes; saving cannot be interrupted.

The operation re-observes the offered key, rechecks inventory and Git revision,
then replaces only that client's recorded address under the normal operation
gate. Unrelated entries/aliases are preserved and the old file is backed up
privately. A changed review is refused. This does not update the client's
system or certify its installed revision: refresh Computers before a fresh
deployment plan. Unexpected key changes require investigation, not rotation.
Never delete the whole trust file or disable host-key verification.

## This computer has no Nixorium identity

Computers of the same kind share one system, and each one keeps its name and
laboratory address from a small identity record,
`/var/lib/nixorium/identity.json`, which holds only its number. If that record
is missing or not valid, the computer starts without its laboratory address,
and `systemctl status nixorium-identity.service` shows a failed unit with the
message "This computer has no Nixorium identity". In the inventory the computer
is offline or unknown, and it cannot be reached from the controller.

Typical causes are a disk restored or copied from another computer, or a
record removed or edited by hand. Without its laboratory address the
controller cannot update the computer, so the fix starts at the computer:

1. If the computer was moved to the shared system by an update, restart it
   and choose the previous generation in the boot menu. That system knows the
   computer's number: it writes the record and starts with the laboratory
   address. Then update the computer from the controller (Computers → Update
   computers).
2. Otherwise, as administrator at the computer, write its number in the
   record and restart. For computer number 3 the file
   `/var/lib/nixorium/identity.json` must contain exactly this line:

   ```text
   {"schemaVersion":1,"number":3}
   ```

3. If neither is possible, reinstall the computer from the controller
   (network installation or USB). The installer writes the record before the
   first start. A reinstall erases the disk.

Do not copy the record from another computer. Two computers with the same
number refuse each other's address and neither works reliably.

## The update says "identity record names another computer"

Before it switches a computer to a shared system, the update checks the
identity record on that computer. If the record names a different computer
number than the one the controller is updating, the update leaves the
computer unchanged and reports "identity record names another computer". It
never overwrites the record, because that could hide a swapped disk or two
computers whose places were exchanged.

1. Decide which computer is which: the label on the case, the inventory entry
   (Computers) and the address the controller used.
2. On the computer that holds the wrong record, correct it by hand as
   administrator. For computer number 3, for example, the file must contain
   exactly this line:

   ```text
   {"schemaVersion":1,"number":3}
   ```

   Save it as `/var/lib/nixorium/identity.json`.
3. Run `nixorium hosts` to refresh the state, then update the computer again.

If you are not sure which computer is which, leave the computer as it is and
check the inventory first: a wrong correction gives the computer another
computer's name and address.

## A deployment failed

A failed update can leave selected machines at different generations.
Nixorium does not claim an automatic rollback across hosts. Unexpected terminal
loss can interrupt the foreground deployment and uses this same recovery path.

1. Read the operation-log ID from the result and run
   `nixorium logs show OPERATION_LOG_ID`.
2. Run `nixorium hosts` to obtain fresh authenticated state for every target.
3. If the result requires recovery or a pending deployment exists, follow
   [Interrupted client deployment](#interrupted-client-deployment) first.
   Otherwise fix the reported build, reachability, or activation problem.
4. Make a fresh `nixorium deploy plan --on ...` against a clean revision.
5. Retry the full build-first `deploy apply` workflow.

After an uncertain apply, matching revisions are observations only and do not
update successful history. Historical success never overrides the live result.

## Interrupted client deployment

The distribution percentage counts phases, not bytes or updated computers.
During build/apply, `l` shows a bounded, terminal-safe tail of the private
update log and the time since its last output. Silence alone is not failure.
SSH connects with a 10-second limit and one attempt; unanswered server-alive
checks use a 10-second interval and a count of three. Phase supervision has
generous upper bounds of six hours for build and two hours for apply. These
are safety ceilings, not progress estimates or proof that a remote job stopped.

If waiting is no longer useful, `s` opens a separate **Stop waiting** review.
Enter on that review confirms termination of the local Nixorium update and SSH processes; Esc keeps waiting.
Nixorium then attempts bounded authenticated client observations. This is not
a remote cancellation or rollback; activation may still be running.

Before dispatching apply, Nixorium durably creates the administrator-owned
mode-0600 `/var/lib/nixorium/coordination/deployment-pending.json`, containing
the repository, reviewed revision, selected names/addresses and start time.
A normal successful finish of the update clears it. A failed/disconnected/timed-out
apply, terminal loss or process death retains it, even across controller reboot.
New managed deployments, installation starts, network installation and other
maintenance operations are refused. Classroom controls (Internet access, shut
down, restart and the schedule) keep working on the other computers and leave
alone only the computers named in the record. Inventory and logs remain
readable. Malformed or unsafe pending evidence also blocks operations, and
classroom controls on every computer, rather than being silently removed.

**Update Nixorium** and **Maintenance → Apply to controller** stay available,
so a newer release can always reach the controller, even when the fix is in
the recovery itself. Activation changes only the controller, not the computers
the record describes; it does not clear the record either. The result still
shows **A previous update of the student computers was interrupted**, and
Enter opens its review. After **Update Nixorium** it also says: "The student
computers were not touched: review the interrupted update before updating
them." Another running operation, an interrupted template reset, a failed
evaluation and an open network installation still stop the activation.

Recovery is reviewed, never automatic: there is no automatic retry or
force-unlock (ADR 0023; confirmation as in ADR 0026). When an update started
from **Computers → Update computers** stops this way, Nixorium opens the review
at once. Otherwise open it from the Overview row **A previous update of the
student computers was interrupted**, or press Enter on a failure notice that
names it (for example on the result of **Update Nixorium**).

**Review the interrupted update** lists every recorded computer, checked over
authenticated SSH (running revision, queued systemd jobs, running activation):

- *finished, running the new version* or *finished, but running another
  version*: nothing to do now;
- *still updating*: wait a minute and press `r` to check again; the review
  cannot be confirmed meanwhile;
- *not answering*: switch the computer on and press `r`, or press `c` to
  continue without the computers that do not answer (`c` again includes them).
  See [Computer not reachable](#computer-not-reachable). The review also says
  when that computer is the one of an unfinished USB installation, or when a
  network installation session is still open.

The review also refuses while another Nixorium window or terminal that started
the update is still open; the window that just ran the update is accepted once
its apply has returned. Press Enter to mark the update as reviewed. Nothing
changes on the computers: the exact reviewed record is archived under
`/var/lib/nixorium/coordination/recovered/` (holding the operation lock, never
overwriting an earlier archive) and other operations are unblocked. The old
update is not declared successful: then use **Computers → Update computers** so
every computer runs the current version. Esc leaves the review without
changing anything.

From the command line:

```sh
nixorium deploy recover plan
nixorium deploy recover apply --expect REVIEW_TOKEN
```

Add `--acknowledge-unreachable` to both to continue without the computers that
do not answer.
`apply` asks `Mark this update as reviewed? [y/N]`; `--yes` skips the question
for scripts. The token binds the reviewed observations; a changed computer or
record requires a fresh plan.

Only when the guided review cannot run, recover by hand:

1. Keep the private operation log and the pending record; do not delete or
   overwrite them, and review them before sharing.
2. Confirm that the Nixorium process that started the update and its SSH
   child processes have exited (a stored PID is only a hint).
3. Inspect every recorded computer, over SSH or at its console, for running
   activation and pending systemd jobs; resolve them first. A matching
   revision or a lost connection is not proof that activation finished.
4. With no operation running, hold `operation.lock` exclusively and move the
   exact record to a new private file under `recovered/`; never delete or
   recreate the lock files, and never bypass them with old binaries or by
   running a manual activation.
5. Check the computers again and authorize a fresh reviewed update.

### Last resort: activate the controller with NixOS

If an older Nixorium still refuses to activate the controller, **Update
Nixorium** has already saved the configuration, and it points at the new
release. Activate it without Nixorium's service, as `admin` in the deployment
folder:

```sh
cd ~/nixorium-deployment
sudo nixos-rebuild switch --flake "$(nixorium config source --repo "$PWD")"
```

`nixorium config source` adds the account passwords that Git does not store;
without it the system would be built without them. This touches only the
controller. The record stays and still blocks client operations until you
review it with the new version. Do not use it while a USB disk installation is
running. Then open Nixorium and run **Maintenance → Apply to controller** once,
so it records the verified activation.

For controller builds, `l` expands managed phase details in both the dedicated
controller screen and framework/package/software update follow-ups. These
phases are not raw Nix output; the full log remains in the controller service
journal. Consult the exact unit named in the result, or inspect current units
with `systemctl list-units 'nixorium-apply-controller*' --all` while it runs.

## The Git tree is dirty

```sh
nixorium git review
git status --short
```

Inspect every staged, unstaged, and untracked path. Nixorium never discards or
stashes changes automatically. Commit an intentional safe allowlist with
`nixorium git commit plan --paths ...` and its reviewed apply command. To undo
a mistaken change instead, use **Review Git changes → x Discard changes** or
`nixorium git discard plan --paths ...` and its reviewed apply (`DISCARD`):
the current content is first saved under
`refs/nixorium/discard-backups/…`, then the selected files return to the last
commit. Untracked files, new files and private keys are never touched. Never add `secret-key`, `admin-ssh`,
or plaintext credentials.

## The last saved change caused trouble

To go back to the configuration saved before your last change, review and
apply the undo (in the dashboard: **Maintenance → Advanced → Review Git
changes**, key `u`), then apply to the controller and update the computers as
usual:

```sh
nixorium git undo plan
nixorium git undo apply --expect REVIEW_TOKEN
```

The undo is saved as one new change; the undone change stays in the history,
no computer changes and nothing is pushed. Unsaved changes stop it with
`GIT-DIRTY` (review and save or discard them first). It also refuses a first
saved version, a merge, a change that is already an undo, a change that
recorded keys or the recovery file (see
[Keys are missing or inconsistent](#keys-are-missing-or-inconsistent)), a
change of the laboratory network (use **Maintenance → Change network
addressing**) and files that cannot go back safely; each refusal names what to
do instead. A change that updated Nixorium or the package base is recovered as
described in [An upstream update failed](#an-upstream-update-failed) and in
UPDATES.md, "Recovery and long-term autonomy". A computer that already runs
the changed configuration returns only when you update it; a computer that
cannot start can be taken back from its own boot menu.

## Configuration is invalid

When `lab-settings.json` no longer validates, for example after an update
removed a field or added a rule, **Maintenance → Change settings** still opens:
it lists each problem, drops obsolete fields and lets you correct the values.
The normal review shows the removed fields and the corrected values, and saving
writes valid settings. Only a file that is not valid JSON must be restored from
Git (`nixorium git discard plan --paths lab-settings.json`) or a backup.

`CONFIG-EVAL` and `NETWORK-SUBNET` normally record successful typed evaluation;
when evaluation itself fails, the support report's status/doctor sections can
be unavailable instead of carrying a failure finding. `DEPLOYMENT-READY` also
covers placeholder values, keys and passwords. Inspect `nixorium status`,
`nixorium config validate` and `nixorium setup status` locally first. Their error
details are private and are deliberately absent from the shared payload.

```sh
nixorium config validate
nixorium setup configure
```

The setup wizard validates the complete candidate before an atomic write and
shows a semantic, secret-redacted review. Use `config plan --file` only with a
complete public JSON candidate; omitted password hashes are loaded from the
local credential file. Change passwords through the native password form, and
apply only the exact returned fingerprint. Do not bypass the Go/schema checks
by editing generated Nix expressions.

## Keys are missing or inconsistent

```sh
nixorium setup keys --verify-only
nixorium setup keys
nixorium setup install-secrets
nixorium setup status
```

Reconciliation creates only missing pairs, enforces mode `0600` on private
files, and never replaces existing key material. Commit only
`keys/cache-public-key` and `keys/admin-ssh.pub`. A missing-private/existing-public or mismatched
pair requires restoring the correct private backup or deliberately rotating the
pair through a separately reviewed maintenance procedure.

## Controller apply failed

`CONTROLLER-BUILD` comes only from an explicitly requested full doctor build,
not support collection. A failed build does not authorize activation or retry;
inspect its local Nix error and validate a corrected candidate before applying.

Read `journalctl -u nixorium-apply-controller.service -b` (or the exact
revision-instanced unit shown by routine `controller apply`). Fix the first
activation error, then make a fresh plan and retry. Completion requires both a
successful switch and a root-owned receipt matching the reviewed Git revision
and active closure; `/run/current-system` equality alone is not success.

Do not create or edit the receipt under `/var/lib/nixorium/controller/`.
The privileged action invalidates stale evidence before retrying and recreates
it only after complete activation.

## Updating the computers fails with a read-only flake.lock

**Symptom.** **Update computers** (or `nixorium deploy apply`) stops before
any computer changes, with a log ending like:

```text
error: opening file "/nix/store/…-source/flake.lock": Read-only file system
colmena build: exit status 1
```

Releases after 3.8.0 stop earlier with `PIN-OUTDATED` and the message
`flake.lock does not match flake.nix (nixorium: flake.nix declares
github:…/vB but flake.lock records github:…/vA)`. `nixorium doctor` reports
the same finding.

**Cause.** **Update Nixorium** in 3.8.0 and earlier saved the new release in
`flake.nix` and locked the right commit in `flake.lock`, but recorded the
release that was configured *before* the update as the lock's reference. Nix
then wants to rewrite `flake.lock`. Applying the controller still works, but
the computers are built from a read-only copy of the configuration, where the
file cannot be rewritten. Every laboratory that updated Nixorium from the menu
is affected; nothing on the computers changed.

**Fix.** With a Nixorium release newer than 3.8.0 running on the controller:

1. Open **Maintenance → Update Nixorium**. The list says the saved version is
   recorded inconsistently; select the configured version (marked
   "Current · select to repair") or a newer release.
2. Review and confirm as for any update. The review shows only the
   `flake.lock` reference changing for the configured version. Nixorium saves
   the files and applies the controller.
3. Run **Update computers** again.

In a terminal the same repair is `nixorium update plan --target <configured
version>`, then `nixorium update apply --target <configured version> --expect
<review-token> --yes`, saving
the two files in **Maintenance → Review Git changes** (`nixorium git review`)
and applying the controller.

Manual alternative (needs Internet access): in the deployment folder run
`nix flake lock`, check with `git diff flake.lock` that only the `original`
reference changed (the locked revision stays the same for a release tag), then
save `flake.lock` in **Maintenance → Review Git changes** and apply the
controller. The same command repairs a `nixpkgs` reference that disagrees with
`flake.nix` after a manual edit.

If the controller still runs 3.8.0, update Nixorium first: that update leaves
the same inconsistency once more, and the newer release then repairs it as
above.

## The update review shows "Manual change"

The review of **Update Nixorium** lists the changes the new release makes to
your own files. "Manual change" means the release could not make one of them
because the file is not as the release expects: you edited that place, or the
file is missing. No file is touched for that change.

1. Read the line under "Manual change" in the review. For a text replacement it
   names the file and says what to do: `Replace "…" with "…".`
2. Open the file named in the review and make that change yourself: replace
   the first text with the second, keeping any other edit you made on purpose.
   If the file already says what the second text says, nothing needs to change.
3. Save it as a normal reviewed change (**Maintenance → Review Git changes**),
   then run **Update Nixorium** again. Assistant can prepare this change as a
   proposal for you to review.

Do not edit `nixorium-migrations.json` to make the message go away: it records
changes the update made and only the update writes it.

If `nixorium-migrations.json` itself is not valid, the update stops with a
message that names the file. Go back to an earlier saved version of the file in
Git. A valid file has this form:

```text
{"schemaVersion":1,"applied":["2026-10-workstation-is-controller"]}
```

If there is no earlier version, remove the file in a reviewed change: the update
then checks each change again, and records those your files already contain.

## The update is refused because of the new release's changes to your files

The new release brings its own program that changes your deployment's files. It
runs on a copy, without network, and **Update Nixorium** accepts only the
changes it can show you in the review. The update is refused, and nothing is
saved, when:

- the new release's migrations fail, do not finish in 2 minutes, or say they
  cannot migrate this deployment (the message gives the reason, for example
  that `nixorium-migrations.json` is not valid);
- they change something they did not declare, or a file they must not touch
  (`flake.nix`, `flake.lock`, keys, credentials, hidden files), or too much;
- the sandbox that runs them cannot start on the controller (the message says
  so).

The current release keeps working. Do not try to work around it:

1. Fix what the message names, if it is on your side (for example restore
   `nixorium-migrations.json` from Git), and update again.
2. Otherwise stay on the current release and collect a support report
   (`nixorium support export`) to send with the message. This is a problem of
   the new release, not of your laboratory.

If the message says **update to an intermediate release first**, the new
release migrates files in a newer way than the release you run knows. Update
to the release that the message or the release notes name as intermediate, then
update again.

If you run a release from before deployment migrations existed, the update
cannot run them. In the deployment folder run
`nix run github:giovantenne/nixorium/<new tag>#nixorium -- deployment migrate --root .`,
review the result with `git diff` (or **Maintenance → Review Git changes**),
save it, and then run **Update Nixorium**.

## An upstream update failed

- A failed `update check` changes nothing. It is optional; when the controller
  is offline, plan an already-known exact release tag directly.
- A failed `update plan` leaves `flake.nix` and `flake.lock` untouched. Resolve
  its typed input, Git, evaluation, readiness, or build issue and plan again.
- A refused `update apply` with a stale token is safe to re-plan.
- After a reported `partial` apply, inspect both files with
  `nixorium git review` and create a fresh plan for the same intended target.
  Do not commit or deploy until the plan validates the pair again.

Update never commits, pushes, activates, starts PXE, or deploys clients.

## Interrupted-operation retry matrix

| Operation | Repeated invocation / power loss behavior |
|---|---|
| Setup configuration | Candidate validation precedes an atomic file replace; rerun the wizard. |
| Key generation | Creates missing files only; mismatches stop for explicit recovery. |
| Secret installation | Reuses identical destinations and refuses different content. |
| Controller apply | Invalidates old completion evidence; retry requires a fresh reviewed revision. |
| PXE preparation | Publishes a new manifest only after success and retains the prior valid one on failure. |
| PXE start/stop | Start rolls back synchronous failure; durable session state enables explicit and boot recovery. |
| Client disk install | Destructive after exact disk/host confirmation; inspect the disk before retrying after interruption. |
| USB/SSH live install | Pre-apply cancel is safe; post-dispatch state is reconciled by operation ID and exact live boot, never replayed automatically. |
| Client deployment | May leave mixed generations; inspect logs/hosts and retry a fresh convergent plan. |
| Upstream update | Plan is read-only; apply rolls back when possible and reports an unsafe partial pair explicitly. |

## A VS Code extension reports permission denied

Typical signs in the student session:

- a command fails with `EACCES: permission denied`, often naming a path under
  `~/.vscode/extensions/`;
- a newly created project opens, but its files cannot be edited or new files
  cannot be added (`ls -l` shows `r--r--r--` files or `r-xr-xr-x` folders).

Most extensions are linked to the read-only Nix store. An extension that
writes into its own folder, or copies its own files (such as a project
template) into the student's projects, then fails, or produces read-only
copies. Such an extension must be marked `writable` in the deployment's
`workspace-catalog.nix`, so the next home reset gives the student a private
copy of it:

```nix
{ id = "vscjava.vscode-maven"; package = "vscode-extensions.vscjava.vscode-maven"; requiredPackages = [ "maven" ]; writable = true; }
```

The template already marks the known cases: the Python and Java debuggers,
Maven for Java (“Maven: New Project”) and Project Manager for Java (“Java:
Create Java Project → No build tools”). For any other extension, add an entry
with its `id`, `package = "vscode-extensions.<id>"` and `writable = true`;
keep its `requiredPackages` and `requiredExtensions` if it already has an
entry. Do not mark every extension writable: copies count against the seed
limit (256 MB, 8192 entries) and some extensions alone exceed it.

1. Edit `workspace-catalog.nix` in the deployment repository and commit it
   (`nixorium git commit plan --paths workspace-catalog.nix`).
2. Review and apply the controller plan, then `deploy plan --on ...` and
   `deploy apply` for the clients.
3. The change reaches each student home at its next reset (the next normal
   boot). Then test the failing command again in a freshly reset home.

Until then, a student can make an already generated project editable and
remove a half-created one:

```sh
chmod -R u+w ~/Desktop/demo
rm -rf ~/Desktop/demo   # only to delete a failed attempt
```

When adding a new extension, try the features that create something (new
project, templates, starting the debugger) in a reset student home before
distributing it to the whole laboratory.

## Backups and restoration

Backup is **optional**: installation and updates do not require it. In
**Maintenance → Back up lab**, choose **Encrypted file** for a fully encrypted
archive in a folder or on a USB drive, or **Private Git repository** for a
private remote repository that then stays up to date automatically. An absent
or stale backup produces an advisory Overview reminder only.

For a **Private Git repository**, push the saved deployment and its encrypted recovery keys
to a **private** GitHub or GitLab repository (including self-hosted GitLab). Create the private repository on your provider first, and configure
independent SSH access on the controller. Verify the Git server's host fingerprint
through a trusted channel before using Nixorium; SSH host verification is never
disabled. This workflow currently accepts SSH URLs, not HTTPS or tokens in URLs.
Keep Git access separate from the lab's SSH key so a replacement controller can
fetch the backup without first needing the keys inside it.

Save configuration edits through **Maintenance → Review Git changes** first.
Back up lab reviews the destination, branch and saved revision, then asks for a
passphrase of at least 12 characters and its repetition; Enter then pushes. Confirm on your
provider that the destination is private: Git transport cannot establish repository
visibility. Only `nixorium-recovery.age` is added by the backup operation. It uses
age/scrypt and contains the original `secret-key`, `admin-ssh`,
`lab-credentials.json` and trusted computer keys from `~/.ssh/nixorium-known-hosts`.
Configuration, assets, public keys and Git history remain readable to repository
members; account hashes are encrypted. Other
ignored files, Git credentials, telemetry state and operation logs are excluded.
Save required custom configuration in Git; do not commit plaintext credentials.
Recognizable secrets and known private-key paths in reachable history block backup,
but automated detection is not a guarantee for arbitrary custom credentials.

The CLI exposes the same review and execution:

```sh
nixorium backup plan --remote git@github.com:SCHOOL/LAB.git
nixorium backup publish --remote git@github.com:SCHOOL/LAB.git --expect REVIEW-TOKEN --yes
```

The remote branch defaults to the deployment's own branch (`master` for
deployments created by the installer); `--branch` chooses another one.

`--yes` confirms the reviewed destination is private and authorizes publication.
Keep the passphrase separately: Nixorium never saves it. `--passphrase-file FILE`
reads it from a private file for unattended use. Changing the passphrase protects
the new recovery file only; older ciphertext remains in Git history.

Success requires the remote branch to report the exact pushed revision. A failed
or uncertain push leaves the encrypted recovery commit locally and is **not** a
verified backup. Fix SSH access or remote divergence and retry; Nixorium never
force-pushes, merges or resolves remote conflicts automatically. If writing the
recovery commit was interrupted, inspect and save `nixorium-recovery.age` through
Review Git changes before retrying.

After the first verified Git backup, every newly saved configuration revision
is pushed **automatically** while the administrator TUI is open: when it starts
and whenever you return to the Overview. The push uses the same checks and
remote verification, never forces, and needs no passphrase because the
encrypted recovery file is unchanged. Unsaved edits are pushed after they are
saved. When keys, account passwords or trusted computers change (for example
after installing a computer), the Overview asks you to run Back up lab again
with the passphrase. **Back up lab → Back up now** pushes immediately; a failed
automatic push is reported once and retried after the next save.

A configured Git backup therefore needs no periodic reminder: the Overview
reports only a pending push or changes that need the passphrase. For encrypted
files, a reminder appears when the configuration, keys or passwords changed
since the last file or it is older than 30 days. No backup state blocks client
installation, updates, local repair or offline operation.

To restore, open **Maintenance → Restore lab**, or start the restore screen without
an existing deployment:

```sh
nixorium backup clone
```

Choose **Backup file** or **Private Git repository**. For a file, Nixorium
looks for `nixorium-backup-*.age` files on connected drives and proposes the
newest; enter its passphrase. **Restore into** proposes a new folder next to
the deployment (`nixorium-restored`); change it only if needed. For Git, enter
the SSH repository URL, branch and the passphrase. Review and confirm. Both restore the deployment
with its private keys and account passwords into the new directory and fill the
controller's trusted computer keys in the same way. The equivalent
noninteractive Git command is:

```sh
nixorium backup clone --remote git@github.com:SCHOOL/LAB.git --branch master --to /home/admin/restored-lab --yes
```

Restore fetches into private staging, decrypts and verifies the original private/
public key pairs, then publishes the new directory without overwriting an existing
one. Private keys have mode `0600`. Trusted computer keys are restored only when
the current store is empty or has only identical entries; conflicting trust blocks restoration.
No fetched Nix is evaluated and no system is activated. Wrong passphrases or key
mismatches leave no published target. A finalization failure explicitly reports a
partial result and the retained directory; keep it and collect support evidence
before activation instead of deleting it or blindly retrying.

On a replacement controller, install the controller as usual, sign in as
`admin`, open Nixorium and choose **Maintenance → Restore lab**. After the
restore, the result
offers **Use this lab on this controller**: the deployment created by the
installer is kept as `nixorium-deployment-before-restore-DATE`, the restored lab
takes its place and its keys are installed on the controller. Nothing is
deleted or activated. Then **Review controller** applies it to the controller
after the usual review. Computers reinstalled after the backup was made show
"SSH key changed": compare the fingerprint on their screen before trusting
them. Do it before **Install computers** on the new controller: the step
refuses a controller that already has the keys of another lab, which the
installation guide creates.

If that happened (the refusal names `/home/admin/.ssh/id_ed25519`), the new
controller's own keys must go before the restored ones can be installed. They
belong to the lab created on this controller, never to the restored one. As
root, remove `/home/admin/.ssh/id_ed25519`, `/home/admin/.ssh/id_ed25519.pub`
and `/var/lib/nixorium/keys/harmonia-secret-key`, then open **Restore lab**
again and choose **Use this lab on this controller**.

Without the TUI, use the restored repository at the configured deployment
path. Keep any newly created deployment aside until recovery is verified. Run
these commands from the restored deployment:

```sh
nixorium setup keys --verify-only
nixorium setup install-secrets
nixorium controller plan
```

Apply the separately reviewed controller plan, then prepare network installation
again. Clients retain trust in the restored original keys and do not need
reinstallation. The passphrase decrypts existing keys; it never generates
replacement keys. Repository access and the passphrase are both needed.

For an **Encrypted file**, the TUI proposes a connected USB drive, or your home
folder when none is connected (any existing absolute folder outside the
deployment can be entered). Enter and repeat a passphrase of at least
12 characters, review the destination, then choose **Create backup**. No Git
remote is needed and nothing is pushed. A detached USB need not stay mounted
for the recorded backup to count. The equivalent CLI and restoration commands are:

```sh
nixorium backup create --to /run/media/admin/USB-DRIVE
nixorium backup verify /run/media/admin/USB-DRIVE/nixorium-backup-20261001-180000.age
nixorium backup restore BACKUP-FILE --to /home/admin/restored-archive
```

An offline archive encrypts the entire repository, including `.git`, ignored keys
and trusted computer keys. Restore lab places it ready to use; `nixorium backup
restore` only extracts it into `deployment/` and `ssh/` within an empty target,
for inspection. Never commit or push plaintext private keys; only their encrypted
recovery file belongs in Git.

Operation logs and authenticated deployment history under the administrator's
private XDG state are useful audit evidence but are not configuration authority.
PXE manifests, GC roots, installed secret copies, and controller receipts under
`/var/lib/nixorium` are derived operational state and can be regenerated from a
valid repository plus the original private keys.

After restoring, verify ownership and mode `0600` for private keys, run
`nixorium setup keys --verify-only`, reinstall verified secrets, apply the
reviewed controller configuration, and prepare PXE artifacts again. Do not
restore stale active-session markers onto a different controller.

## Assistant proposal or save stopped

If the deployment or proposal changed, reopen the review; if HEAD moved, start
a new chat. A failed Nix evaluation remains readable but cannot be saved.
Ask the assistant to correct it and review the new comparison.

An interrupted save retains its backup and shared template-reset journal.
Open the pending condition in Overview, or Recover from the save result, and
follow the existing reviewed recovery. Do not delete the marker or retry the
save as a workaround. A recovered commit still requires separate controller
and client application. See the [Assistant guide](https://github.com/giovantenne/nixorium/blob/master/docs/assistant.md#interrupted-save).
