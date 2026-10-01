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
next step: the TUI adds a `Next:` line to the notice, a failed CLI command
ends with `Next (CODE): …`, and JSON issues carry a `next` object with the
same code. The codes are stable:

| Code | Situation | Next step |
|---|---|---|
| `OP-BUSY` | Another operation holds the lock | Wait or open its progress; see [Another operation is already running](#another-operation-is-already-running) |
| `DEPLOY-PENDING` | An interrupted client update blocks operations | [Interrupted client deployment](#interrupted-client-deployment) |
| `USB-RESERVED` | A USB installation is unfinished | [A USB installation was interrupted](#a-usb-installation-was-interrupted) |
| `RESET-PENDING` | A deployment template reset was interrupted | [Deployment template reset recovery](https://github.com/giovantenne/nixorium/blob/master/docs/deployment-template-reset.md#backup-and-recovery) |
| `GIT-DIRTY` | The configuration has uncommitted changes | [The Git tree is dirty](#the-git-tree-is-dirty) |
| `REVIEW-EXPIRED` | The review is too old | Create a new review and confirm again |
| `REVIEW-CHANGED` | Computers or files changed after the review | Create a new review and confirm again |
| `PXE-ACTIVE` | Network installation is active or needs recovery | [The controller network is inconsistent](#the-controller-network-is-inconsistent) |
| `SETTINGS-INVALID` | The laboratory settings no longer validate | [Configuration is invalid](#configuration-is-invalid) |
| `CLIENT-UNCONFIRMED` | A request to a computer could not be confirmed | Check the computer in Computer inventory before retrying |
| `DISK-LOW` | The Nix store is low on space | Maintenance → Free disk space |
| `CONTROLLER-NOT-APPLIED` | The controller does not run the saved configuration | [Controller apply failed](#controller-apply-failed) |
| `BACKUP-DUE` | No recent backup, or keys or settings changed since it | [Backups and restoration](#backups-and-restoration) |
| `EVAL-FAILED` | The configuration does not evaluate | Run `nixorium doctor`; fix the first reported error; do not retry other operations |

## The controller DHCP lease changed

Normal administration and Colmena deployment use the static laboratory address.
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

## Build resources and controller tools

`DISK-FREE` warns below 10 GiB available or when the filesystem probe failed;
it is not an estimate of the space required by the next build. `COMMAND-NIX`,
`COMMAND-GIT`, `COMMAND-SYSTEMCTL`, `COMMAND-SSH` and `COMMAND-COLMENA` report
missing executables in the command's environment. Neither condition starts
a repair. Inspect `df -h /nix` and `command -v nix git systemctl ssh colmena`.

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
- If a prior session was interrupted, run `nixorium pxe recover`, then prepare
  and start again. Boot-time recovery also reconciles an unfinished controller
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

The restart action requires exact confirmation and verifies both the unit and
HTTP endpoint. If the signing key is missing, run `nixorium setup keys`, review
only the public-key change, then `nixorium setup install-secrets`. A public/
private mismatch or a different installed secret fails closed; do not overwrite
either side until its provenance is understood.

## A USB/SSH live client cannot be verified

Use only the official NixOS 26.05 Minimal ISO for `x86_64-linux`, booted in
UEFI mode with Ethernet or Wi-Fi connectivity. Connect Wi-Fi with `nmtui` in
the live environment first. Keep its physical console visible and recheck:

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
The declared client interface must carry that live address. USB/SSH does not
need PXE, ProxyDHCP, or a controller address transition, but the controller must
reach the live client's SSH server and the client must reach the signed Harmonia
cache. Wi-Fi profiles and credentials are not copied into the installed system:
arrange connectivity separately or reconnect locally as the administrator after
boot before verifying the installed client. Keep secrets out of Git and the
Nix store. Do not bridge an isolated trial onto an institutional
LAN as a troubleshooting shortcut.

## USB installation refuses the disk or cache

If a connection attempt returns to `artifacts-ready`, only the controller-side
build is ready; the live SSH session has not been verified. The TUI keeps the
connection error visible during the current TUI session, even after refreshing
status, and returns to physical host-key verification before another password
attempt. When reattaching an artifacts-only operation, use **Connect live
client** to continue with the same prepared artifacts, or **Cancel safely** to
release them. Do not interpret `artifacts-ready` as installation progress or
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

The Overview lists an unfinished USB installation when the dashboard opens;
open that row or **Installation → USB over SSH** to continue. From the CLI,
`nixorium install usb status` without `--id` shows the unfinished operation and
its ID. Then inspect the exact operation without starting another install:

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
live ISO is gone before any apply was dispatched, explicitly use
`nixorium install usb close --id OPERATION_ID` and review the `CLOSE` prompt.
This discards the operation's local access credentials and releases its
reservation without claiming remote key revocation. The old record is retained;
a new installation must establish identity and review the disk again. Any
consumed apply token, remote receipt, uncertain dispatch, or reboot evidence
blocks this abandonment path. If apply was dispatched and completion cannot
be proved, retain the reservation for reconciliation.

For a reinstall, a different key at the configured static address is expected
only after proving the old machine is the selected physical client. Approve
`ROTATE HOST KEY` during the review; Nixorium replaces just that entry after
the newly installed host passes verification. Never delete the entire
`known_hosts` file or accept a changed key merely because the address matches.

If **Computers → Distribute** finds an unfinished USB installation, it opens
**Finish installation** before asking you to confirm deployment. For an
installed computer awaiting its final check, choose **Verify … and resume**.
Turn on that computer, boot from the installed disk, and check its network
cable. Failed verification keeps the operation protected and offers retry and
technical details. Active or uncertain installations offer **Open installation**
instead; return with Esc after resolving the existing operation. Do not start
a replacement installation to clear this block. Successful recovery returns to
a fresh deployment review with the same selected computers and requires a new
deployment confirmation. It never starts deployment automatically.

Controller rebuild may proceed while a completed USB installation still awaits
reboot or verification, even if the client is offline. The controller validates
the persisted disk-completion receipt, pauses the worker without deleting its
state or credentials, holds the normal operation lock, and resumes the worker
on success or failure. Active, failed, or unknown disk work remains protected;
other client operations still respect the reservation. Older controller apply
services that block every reserved installation need the corrected apply
service as well as the worker; never remove their coordination files manually.

Complete `install usb verify --id OPERATION_ID` when the installed client is
available. Verification checks the preserved host key, hostname, exact system
closure, and reviewed revision before releasing the reservation. A `read-only
file system` error while publishing known hosts or logs indicates an older
worker sandbox. Apply the corrected worker and filesystem configuration
together, preserving operation state and credentials, then repeat verification.
After an authorized client reboot, verification can also resume if a controller
reboot cleared the ISO credentials: it authenticates the installed host with
the administrator key and the original persisted host pin and revision.
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

## Codes shown to the teacher

Classroom controls replace administrative detail with a short code. The
technical cause stays in `journalctl -u nixorium-classroom.service`.

| Code | Meaning | What the administrator does |
|---|---|---|
| `OP-BUSY` | Another Nixorium operation is running | Let it finish; see [Another operation is already running](#another-operation-is-already-running) |
| `DEPLOY-PENDING` | An interrupted client update blocks operations | Follow [Interrupted client deployment](#interrupted-client-deployment) |
| `USB-RESERVED` | A USB installation is unfinished | Finish, verify or close it ([A USB installation was interrupted](#a-usb-installation-was-interrupted)) |
| `PXE-ACTIVE` | Network installation is running or needs recovery | Finish installation or run `nixorium pxe recover` |
| `CLASSROOM-LOAD` | The classroom service could not read the committed configuration | Read the service journal; fix and commit the configuration |
| `CLASSROOM-SERVICE` | The classroom service is not running or not answering | `systemctl status nixorium-classroom.service`; apply the controller configuration if it is missing |

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

## A deployment failed

A Colmena failure can leave selected machines at different generations.
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
Colmena log and the time since its last output. Silence alone is not failure.
SSH connects with a 10-second limit and one attempt; unanswered server-alive
checks use a 10-second interval and a count of three. Phase supervision has
generous upper bounds of six hours for build and two hours for apply. These
are safety ceilings, not progress estimates or proof that a remote job stopped.

If waiting is no longer useful, `s` opens a separate **Stop waiting** review.
Only typing `STOP WAITING` confirms termination of local Colmena/SSH processes.
Nixorium then attempts bounded authenticated client observations. This is not
a remote cancellation or rollback; activation may still be running.

Before dispatching apply, Nixorium durably creates the administrator-owned
mode-0600 `/var/lib/nixorium/coordination/deployment-pending.json`, containing
the repository, reviewed revision, selected names/addresses and start time.
A normal successful Colmena return clears it. A failed/disconnected/timed-out
apply, terminal loss or process death retains it, even across controller reboot.
New managed deployments, controller applies, installation starts and fleet
mutations are refused. Inventory and logs remain readable. Malformed or unsafe
pending evidence also blocks operations rather than being silently removed.

Recovery is reviewed, never automatic: there is no automatic retry or
force-unlock. Use the guided recovery (ADR 0023): the Overview row
**An interrupted client update blocks other operations**, or

```sh
nixorium deploy recover plan
nixorium deploy recover apply --expect REVIEW_TOKEN
```

The review confirms that the Nixorium process that started the update has
exited, checks every recorded computer over authenticated SSH (running
revision, queued systemd jobs, running activation) and refuses while any of
them is still applying. Computers that cannot be checked must be inspected at
their console and acknowledged explicitly (`u` in the TUI,
`--acknowledge-unreachable` in the CLI). Typing `RECOVERED` archives the exact
reviewed record under `/var/lib/nixorium/coordination/recovered/`, holding the
operation lock and never overwriting an earlier archive. It does not declare
the old update successful: create a fresh deployment review afterwards.

The manual procedure below remains the fallback when the guided review cannot
run:

1. Read the private operation log and pending record locally. Do not post either
   without reviewing sensitive values. Do not overwrite or delete the evidence.
2. Confirm the original controller process and its Colmena/SSH children have
   exited. A stored PID is only a hint, not proof of identity or completion.
3. Inspect **every recorded client**, through authenticated SSH or its physical
   console, for still-running activation processes and pending systemd jobs.
   Resolve any unfinished activation first. A matching `/run/current-system`,
   an inventory “current” label, or loss of connectivity is insufficient.
   If a client cannot be inspected, keep the reservation and do not retry.
4. With explicit administrator approval and no operation still running, hold
   the existing fleet `operation.lock` exclusively and archive the exact pending
   record to a new private, non-overwriting recovery file. Preserve the record
   and log for investigation; sync the directory before releasing the lock.
   Never delete/recreate `operation.lock` or `deploy.lock` to bypass ownership.
   This step acknowledges reviewed recovery; it does not declare the old apply
   successful. Do not run old Nixorium binaries or raw Colmena to bypass it.
5. Inspect live client state again, correct the underlying problem and authorize
   a fresh reviewed deployment. Reboots, if needed, are separate disruptive
   actions and require their own approval.

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
`veyon-private-key.pem`, or plaintext credentials.

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
complete JSON candidate containing password hashes—not plaintext passwords—and
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
`keys/cache-public-key`, `keys/admin-ssh.pub`, and
`keys/veyon-public-key.pem`. A missing-private/existing-public or mismatched
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

## Backups and restoration

Create backups with **Maintenance → Back up the controller** or:

```sh
nixorium backup create --to /run/media/admin/USB-DRIVE
nixorium backup verify /run/media/admin/USB-DRIVE/nixorium-backup-20261001-180000.age
```

A backup is one file encrypted with a passphrase (age, scrypt). It contains
the deployment repository with its `.git` history, the three ignored private
key files and the trusted computer keys (`~/.ssh/nixorium-known-hosts`). Build
results linked into the Nix store are not included. Keep the file and its
passphrase away from the controller and from each other: without the
passphrase the backup cannot be read, and anyone with both can manage the
laboratory. The Overview and `nixorium doctor` remind you when no backup is
recorded, the last one is older than 30 days, or the private keys or
laboratory settings changed since it. `--passphrase-file` reads the passphrase
from a private file for unattended use.

To replace a failed controller: install the new controller from the NixOS
Minimal ISO with the same controller number and laboratory network, sign in as
`admin`, then

```sh
nixorium backup restore BACKUP-FILE --to ~/restored
mv ~/nixorium-deployment ~/nixorium-deployment.new-install
mv ~/restored/deployment ~/nixorium-deployment
cp -a ~/restored/ssh/nixorium-known-hosts/. ~/.ssh/nixorium-known-hosts/
cd ~/nixorium-deployment
nixorium setup keys --verify-only
nixorium setup install-secrets
nixorium controller plan
```

Apply the reviewed controller plan, then prepare network installation again.
The installed computers keep trusting the restored keys, so they are managed
again without reinstallation. Restore refuses a non-empty target and checks
every file against the backup manifest first.

A Git remote is still useful for the private tracked configuration, but
private keys must never be pushed or committed.

Operation logs and authenticated deployment history under the administrator's
private XDG state are useful audit evidence but are not configuration authority.
PXE manifests, GC roots, installed secret copies, and controller receipts under
`/var/lib/nixorium` are derived operational state and can be regenerated from a
valid repository plus the original private keys.

After restoring, verify ownership and mode `0600` for private keys, run
`nixorium setup keys --verify-only`, reinstall verified secrets, apply the
reviewed controller configuration, and prepare PXE artifacts again. Do not
restore stale active-session markers onto a different controller.
