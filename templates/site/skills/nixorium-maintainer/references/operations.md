# Lab operations

## Overview observations

On supporting pins, numbered Overview rows open pending work: background jobs,
installation recovery, uncommitted configuration, controller application and
low store space. Opening a row does not perform the operation. Client summaries
show the time of the last session observation, not live state. Refresh local
state updates only saved configuration and local observations; open inventory
to check clients. An empty list is not evidence that the fleet is current.

`nixorium recovery status` (TUI: Overview rows, or safe mode when the
laboratory cannot be read) lists persistent blockers — interrupted client
update, unfinished USB installation, interrupted template reset, controller
network recovery, held lock, invalid settings, controller not running its
last applied configuration — each with its next step, from local state only.
Refusals carry stable codes (`Next (CODE): …`, JSON `next`). An interrupted
client update is recovered with `deploy recover plan|apply` (word `RECOVERED`,
unreachable computers acknowledged explicitly) and an interrupted template
reset with `template-reset recover plan|apply` (`RECOVERED`, or `RESTORE` for
a mixed checkout); never edit or delete their records by hand (ADR 0023).
`backup create --to DIR` (TUI: Maintenance → Back up the controller) writes an
age-encrypted file with the repository and its history, the private keys and
the trusted host keys; `backup verify` checks it and `backup restore --to
EMPTY-DIR` extracts it for the controller replacement steps (ADR 0024).
A mistaken uncommitted change is undone with `git discard plan|apply --paths`
(`DISCARD`); the discarded content is kept under
`refs/nixorium/discard-backups/`. Never
put the passphrase or an unencrypted key copy in Git or chat.

A refused operation names the running one when the lock holder can be
confirmed ("Update computers, started by admin at 10:02"); never remove the
lock. The Overview lists an unfinished USB installation; `nixorium install usb
status` without `--id` shows it. Classroom controls read the last committed
configuration and show teachers short codes (`OP-BUSY`, `DEPLOY-PENDING`,
`USB-RESERVED`, `PXE-ACTIVE`, `CLASSROOM-LOAD`, `CLASSROOM-SERVICE`) explained
in the troubleshooting guide; details stay in the classroom service journal.

In the TUI, `r` refreshes observed state on every screen that has a refresh;
it never removes, retries a save or starts a review. After a result, `n` starts
a new review; `s` completes an interrupted local save; PXE recovery is `n`
(Recover network) and a cache restart review is `c`. Every acting key is in
the action bar. Maintenance lists frequent tasks first (settings, diagnostics,
system and Nixorium updates, logs) and groups controller application,
services, Git review and template reset under **Advanced**. Computers →
**Update computers** is the client deployment task.

When preparing PXE on a configured laboratory, the saved-settings summary lets
the operator continue or edit. It proves only form completeness: continuation
still checks keys, configuration, controller and installation files, and does
not replace the separately reviewed start of network installation.

## After saving configuration

On supporting pins, result screens separate local configuration, this
controller and client computers. Saved does not mean applied; an unchanged
declaration is not proof that any running computer is current. Client state
remains unknown until checked, and saves never distribute or reboot clients.

Settings, workspace and template reset offer a separate controller review as
their next action. After verified application at the saved revision, continue
to fresh client selection and the ordinary deployment review/confirmation.
Esc leaves the follow-up without applying. Software and input updates retain
their existing automatic controller follow-up when applicable; client-only
software still skips the controller. Fix partial saves before system actions.
Workspace TUI save includes a confined local commit; CLI workspace apply does
not. Student-home/template/keyboard changes may need the next computer start
after system application; there is no separate workspace personalization switch.

## Validation

On supporting pins, TUI read-only waits show elapsed time and accept Esc back to
their originating view. Ordinary reads have a two-minute limit; candidate build
reviews allow one hour. Cancellation retains the settings draft, ignores late
replies and never rolls back a prior save/activation. A timeout is not permission
to apply: retry the read or inspect Diagnostics. Foreground mutations remain
protected; independent systemd/remote work is not cancelled by closing a read
or quitting an explicitly leave-safe view. Consult the current action bar.

On supporting pins, reopening the administrator TUI finds controller/PXE jobs
that survived its terminal. Use **Overview → v (View progress)** to observe,
and **Tab** to switch jobs. This never starts or resumes work. A running record
without a running unit is interrupted: inspect the displayed journal unit,
then use a fresh ordinary review. A completion record is not verification of
the current configuration. Wait for running work before a conflicting start;
unavailable unit state is not permission to retry. Observation uses local
systemd/progress only, without startup Nix evaluation.

Evaluate before building. For settings changes, use the managed validator;
for other changes, evaluate the affected outputs. Inspect mode, readiness, and
inventory without writing the lock:

```sh
git diff --check
nixorium config validate
nix eval .#labMeta --json --no-write-lock-file
nix eval .#deploymentStatus --json --no-write-lock-file
```

In explicit controller-only mode, validate only the controller and use
`deploymentStatus.controller` when supported. Zero clients is valid: do not
require a client, lab keys, or netboot artifacts. Legacy metadata uses the
older readiness contract; never use controller readiness to authorize clients.

Build changed roles before live application, not on every exploratory edit.
For client-only changes use one actual representative client from `labMeta`;
for shared changes include the controller. Do not build every client unless
their changed host-specific modules produce materially different systems.
Derive names from inventory instead of assuming `pc01` or `pc99`.

When a change affects netboot or inclusion of modules/assets in the installer,
also validate the netboot ramdisk, `pxeFirmware`, and `installerBundle`.
Evaluate the same client through the deployment and the real bundle store
path with `--offline`; their `system.build.toplevel.drvPath` must match.
Do not add this boundary check to a settings-only or controller-only task.

Report evaluations and builds separately. Building may fetch sources and use
substantial storage/time; it does not activate a system or authorize deployment.

## Temporary Internet access

Use **Computers → Internet access** to select clients, choose block/unblock,
review authenticated state and apply. `r` checks every client's current state
read-only and `n` selects those the chosen action would change; the review
checks them again. The controller and clients must first
run a version supporting the client helper. Internet returns on client reboot;
offline clients are never queued. The configured laboratory IPv4 subnet stays
reachable, including SSH and Veyon. Other destinations and established Internet
connections are blocked without changing the gateway.

```sh
nixorium internet plan --on @lab --action block
nixorium internet apply --on @lab --action block --expect TOKEN_FROM_PLAN
nixorium internet plan --on pc01 --action unblock
nixorium internet apply --on pc01 --action unblock --expect TOKEN_FROM_PLAN
```

Use the current plan's exact token. Reboot, changed observations, expiry or an
inventory change require a fresh review. Report unavailable, not-sent and
unconfirmed targets individually; never infer success from an SSH dispatch.
An explicit unblock can recover an inconsistent owned table/service state.
Do not edit routes, enable the unit at boot, flush the ruleset or grant student
network privileges. Allowed lab services, including a lab proxy, remain reachable.

## Installation and deployment

Disk installation is destructive. Resolve the exact host and disk first and
retain the installer's explicit confirmation. Do not install or deploy merely
because builds succeeded.

Use the managed deployment workflow only after authorization:

```sh
nixorium deploy plan --on pc05
nixorium deploy plan --on @lab
nixorium deploy apply --on @lab --expect REVISION_FROM_PLAN
```

Planning requires a ready deployment and clean Git worktree, records HEAD, and
rejects unknown or duplicate clients. Use the exact command and revision shown
by the plan. Apply revalidates the review, requires the one-word `DEPLOY` confirmation, runs a
verbose build before activation, and records a private durable log. If apply
fails, some targets may already have changed or still be activating. On
supporting versions, an uncertain apply retains fleet-wide
`deployment-pending.json` evidence and forbids new mutations, even after reboot.
Authenticated revision observations do not clear it or update successful
history. Follow **Interrupted client deployment** in `TROUBLESHOOTING.md` before
any retry; never delete locks or bypass the gate with raw commands. During
distribution, `l` shows the bounded private output tail; `s` offers a separately
confirmed stop of local supervision, not remote cancellation. The private
per-repository history lives under `~/.local/state/nixorium/deployments/`.
After reviewed recovery, inspect host state and authorize a fresh plan.
Controller follow-ups after software/framework/package updates also support
`l` for managed phase details; full controller output remains in journald.
The interactive
apply confirmation is the single word `DEPLOY`; `--yes` is only for
explicit automation. The raw commands below remain advanced manual operations
and bypass these safeguards.

For routine changes to this controller, use the separate reviewed workflow:

```sh
nixorium controller plan
nixorium controller apply --expect REVISION_FROM_PLAN
```

CLI apply requires the exact confirmation returned by the plan and starts only a
revision-bound systemd instance. It builds the pinned Git source as the
deployment owner, refuses repository drift before activation, and verifies the
active system plus its revision-bound durable success receipt afterward. The
reviewed generation is registered in the persistent NixOS system profile before
activation, keeping boot configuration, rollback history, and GC retention in
sync with the running system. Failed activation still requires inspection;
profile registration alone is not evidence of success. The
TUI's **Maintenance → Apply to controller** task uses the same
typed operation with an Enter confirmation after review; the systemd job and
journal survive closing the dashboard. The review names what changed since the
last verified activation (saved settings, software, student preferences,
Nixorium version, system and packages, other files) from Git alone, says when
that is unknown, and offers no application when the controller is already
current. The plan's `changes` list is not evidence of a built system. Keep `setup apply` for first-run
compatibility, not as a replacement for routine reviewed controller plans.

The TUI's client distribution task invokes the same plan/apply
operations. Before review it checks for an unfinished USB installation. The
guided recovery can verify a completed installation or open its existing
recovery screen. It preserves selected identities, never clears an uncertain
reservation, and returns to fresh planning and confirmation after recovery.
The same recovery is offered when a USB reservation appears after review.
Select the intended computers, review the resolved revision and
targets, and enter the exact phrase shown. Do not close the controller terminal
until the final result, authenticated/recorded counts, and log path appear.

On supporting pins, review probes only selected computers with a brief SSH-port
check. **F2 — Reachable only** creates a new plan for the observed reachable
subset with an open SSH port; review the new revision/targets and confirm again.
Esc during that read retains the original review with its confirmation cleared.
No target is silently skipped, queued or retried. CLI plans show the same
observations and, when available, an explicit selector for another `deploy plan`.
A port check does not authenticate identity or prove whether a computer is off.
Results list each computer; scroll long reports and retain the private log.
Not reached means the outcome is unknown, not unchanged. Recovery-required
results still block retries even if a client reports the reviewed revision.

```sh
colmena apply --on pc05
colmena apply --on @lab
```

Host keys are accepted on first connection and verified on later connections.
Investigate changed-key failures instead of deleting `known_hosts` entries
blindly.

For a deliberately reinstalled declared client, stop PXE and finish/reconcile
protected client work first. Compare `ssh-keygen -lf
/etc/ssh/ssh_host_ed25519_key.pub` on that client's physical console with
`nixorium host-key plan --host pc01`. Only after explicit authorization, use
`nixorium host-key apply --host pc01 --expect REVIEW_TOKEN` with the exact
token and confirmation from the review. The TUI's changed-key client details
offer the same reviewed operation. It rechecks identity, revision and both keys,
preserves other entries and a private backup, and never weakens SSH checking.
Refresh live inventory afterwards; trust rotation is not system verification.
Never use this recovery for an unexplained key change or to bypass protected
USB/deployment recovery.

Use `nixorium hosts` to reconcile deployment state. It first distinguishes
network/SSH availability, then uses the existing root deployment key to run the
fixed read-only `nixorium-host-state` helper. `current` means the authenticated
active generation embeds the desired clean Git revision; `outdated` means the
two revisions differ; `unknown` means Nixorium could not prove either state.
Do not infer success from a Colmena exit status. A client installed before this
helper exists remains `unknown` until its next normal deployment.
The host report also shows the last successful post-apply verification stored
locally. Treat it as history only: current/outdated/unknown always comes from
the live authenticated observation.

The TUI's Software system-state view combines that live client evidence with
the controller's reviewed state. It calls the same controller reconciliation
that requires the active closure and durable activation receipt to match the
current Git revision. Its timestamp identifies one refreshable snapshot; if
the repository revision changes while the snapshot is collected, the view is
partial and must not be treated as verified.

## Classroom controls and client power

On the controller, the configured teacher can run `nixorium` from their normal
home. The restricted dashboard exposes only authenticated computer inventory,
temporary Internet access, and reviewed client shutdown/restart. It deliberately
does not expose or make readable the administrator deployment, settings,
software, Git, PXE/USB installation, deployment, updates, logs, services, or
controller activation. Do not add the teacher to `wheel`, share the deployment
or SSH key, or add students to `nixorium-classroom` to expand this boundary.

Use the reviewed client-only workflow:

```sh
nixorium shutdown plan --on pc05
nixorium shutdown plan --on @lab
nixorium shutdown apply --on @lab --expect REVIEW_TOKEN
nixorium restart plan --on pc05
nixorium restart apply --on pc05 --expect REVIEW_TOKEN
```

The controller is never a valid target. Planning checks evaluated client
identity, management access, interactive sessions, PXE/controller-network
state, and concurrent client operations. An active user session remains
eligible after a prominent warning that unsaved work may be lost. A session
that is logged in without keyboard or mouse input for ten minutes or since it
started, such as an untouched automatic login, is reported as not in use and
does not carry that warning; older client generations report it as active. Unknown
session state remains blocked unless both plan and apply use
`--acknowledge-unknown-sessions` after explicit review. Unreachable targets are
shown as not sent and are never queued for later.

When the plan includes an active session, the review states explicitly that
`SHUTDOWN` or `RESTART` authorizes interrupting it. Apply requires the matching single word, takes the same lock as
deployment, and repeats inventory, conflict, and session checks immediately
before issuing the fixed operating-system request. Results describe only
`accepted`, `not-sent`, or `unconfirmed`. A successful request is not proof of
physical power state or completed restart; a lost connection may mean the request took effect, so
do not retry an unconfirmed target blindly. `--yes` is only for deliberate
automation with the exact fresh review token.

## Software and configuration changes

Use [the software guide](software.md) for scopes, package updates, and the
different CLI/TUI effects. Use [the student-home guide](student-home.md) for
editor extensions, desktop preferences, and content that must survive resets.

## Git review and operation logs

On supporting versions, **Maintenance → Reset deployment template** is an
explicit destructive replacement of copied deployment files, not a framework
update or additive software profile. Use it only when the operator has approved
losing local software, home, asset and module customizations. Choose a preset
from the exact locked upstream and review all paths before `RESET DEPLOYMENT`.
The action preserves committed settings/lock/keys/ignore rules and all untracked
files, creates a backup ref and a local commit, and enables the initial guided
home in configuration only. Apply/deploy/reboot/push remain separate decisions.
Resolve dirty tracked files, collisions and unsupported layouts explicitly;
never force the operation with a hard reset or clean. Keep
`.git/nixorium-template-reset.json` if interrupted and stop normal operations
until recovery. See the upstream
[template reset guide](https://github.com/giovantenne/nixorium/blob/master/docs/deployment-template-reset.md).

For a shareable diagnostic snapshot on supporting pins, use
`nixorium support preview --json`; `nixorium support export` requires an
interactive preview and consent before writing a private local file. It never
uploads, runs doctor full, or remediates. Do not redirect detailed doctor/log
output into an attachment: free text may contain secrets. Retained versions,
revision, collection time and aggregate counts are not anonymous. Missing
sections mean unavailable, not healthy. History counts are user-wide, not
checkout-specific. See the upstream
[support-report contract](https://github.com/giovantenne/nixorium/blob/master/docs/support-report.md).
Authorization to diagnose or export is not permission to send the result to
another person, service or model. Do not automate terminal consent.
The administrator TUI's **Maintenance → Diagnostics → Support report** uses
the same snapshot: arrows/Page Up/Down/Home/End inspect it, Enter saves locally,
and Escape cancels. No export capability is exposed to the teacher dashboard.

Review deployment changes without mutating the index or worktree:

```sh
nixorium git review
```

The report keeps staged, unstaged, and untracked paths distinct and labels
Nixorium-managed settings, software, and public keys separately from unexpected edits. It
never opens untracked file contents, disables external diff/textconv drivers,
bounds tracked patches, redacts settings password hashes, and blocks before
patch capture if a known private-key path appears. The TUI's **Review Git
changes** task uses the same report. Continue to inspect and stage intentionally;
this read-only command never discards, stages, commits, or pushes.

Create an optional local commit only through an explicit path allowlist:

```sh
nixorium git commit plan --paths lab-settings.json,keys/admin-ssh.pub
nixorium git commit apply --paths lab-settings.json,keys/admin-ssh.pub --expect REVIEW_TOKEN
```

Planning uses an isolated HEAD-based index and returns the exact proposed tree,
redacted patch, generated message, token, and confirmation phrase. It rejects
conflicts, private/unknown/unchanged paths, directories, symbolic links,
rename/copy changes, invalid managed settings, Git content transforms,
oversized patches, and recognizable private-key, token, or plaintext-secret
additions. Apply repeats the plan, advances HEAD only from the
reviewed parent, and reconciles only selected index entries. Unrelated changes
remain intact. Hooks, signing helpers, remotes, and push never run. The TUI
offers the same select/plan/confirm flow under **Review Git changes**. Use
`--yes` only for explicit automation with a fresh token.

Browse the private deployment operation logs without copying paths manually:

```sh
nixorium logs
nixorium logs show OPERATION_LOG_ID
```

`logs` returns at most the newest 50 recognized deployment entries and works
outside the deployment checkout. It also shows fixed typed summaries for recent
configuration, key, controller, PXE, cache, and deployment outcomes. The
private atomic summary file retains the newest 1000 records and never deletes
detailed deployment logs. `logs show` accepts only an ID emitted by the list
and displays at most the final 64 KiB. Nixorium refuses symlinks, foreign
owners, non-0700 state directories, and non-0600 files, and neutralizes terminal
control characters before text/TUI rendering. An unsafe entry is reported as
unavailable; do not loosen its permissions merely to make the browser accept
it. The TUI's **View operation logs** task uses the same bounded operations.

## Free disk space

Old NixOS system generations are removed only by the reviewed
`nixorium cleanup plan --on <controller,pcNN,...|@lab>` /
`cleanup apply --expect <token>` flow (TUI: Maintenance → Advanced → Free disk
space; ADR 0022). Each computer keeps its newest 10 generations plus the
running and booted ones; GRUB lists at most 10. The fixed helper
`nixorium-clean-generations` answers `--plan` read-only and `--apply <digest>`
as root; the digest binds the exact generations to remove, so a changed
computer reports `not-sent`. The controller runs through
`nixorium-clean-generations@<digest>.service` under the operation lock;
clients run over root SSH. Results: `cleaned`, `unchanged`, `not-sent`,
`unconfirmed` — inspect an unconfirmed computer before retrying. Never run
`nix-collect-garbage -d` or validation GC instead.

## Binary cache

In laboratory mode, the controller owns Harmonia through systemd after
activation. Controller-only mode intentionally leaves the lab cache inactive; do
not launch a second foreground cache. Check both unit and HTTP readiness with:

```sh
systemctl status nixorium-harmonia.service
nixorium doctor
```

The stable product alias is used for status; control enters through the fixed
action below. Query detailed logs with `journalctl -u harmonia.service`, the
canonical nixpkgs unit name.
The signing key is loaded from `/var/lib/nixorium/keys/harmonia-secret-key` as
an isolated systemd credential and must never be copied into Git or the store.

Use the bounded management workflow for routine observation or recovery:

```sh
nixorium services
nixorium services restart cache
```

Restart requires the exact one-word `RESTART` confirmation, invokes only the fixed
capability-free `nixorium-restart-cache.service` action, and verifies both the
unit and HTTP endpoint afterward. Do not use this path to control PXE units;
their listener and network transition must remain coordinated through
`nixorium pxe`. The TUI's controller services task uses the same typed
operations. `--yes` is only for intentional automation.

## USB/SSH client installation

Use USB/SSH only when the admin explicitly authorizes destructive installation
of one configured client and the pinned deployment exposes `install usb`. The
supported live environment is the official NixOS 26.05 Minimal ISO for
`x86_64-linux`, UEFI, and Ethernet or Wi-Fi connectivity. Ask the operator to
connect Wi-Fi in the live ISO first when needed and keep its physical
console visible, set a temporary password, and read the canonical IPv4 address
and Ed25519 `SHA256:` fingerprint there. In the guided TUI, enter only the
address: Nixorium observes the live key without credentials, displays its
fingerprint, and asks for `MATCH` after a complete physical-console comparison.
Never approve the fingerprint using DNS, a previous boot, or `known_hosts`.

Use the declared client interface and verify that it carries the reviewed live
address. When the live address is on another card, the TUI offers to save that
card as the computer's `hostIfaceNames` override through the reviewed settings
save, cancels the session and prepares again; a new `passwd` is needed. Wi-Fi is not refused solely for being wireless; routing/firewall/AP
policy must allow SSH to the client and signed-cache access to the controller.
Live Wi-Fi profiles are not copied into the installed system. Arrange persistent
connectivity separately or reconnect locally as the administrator after boot,
then verify the configured static address. Never commit wireless secrets or
put them in the Nix store. Older pins may still reject wireless interfaces;
use a reviewed upstream update rather than bypassing their checks.

The ordinary guided TUI owns host selection, secret input, hardware review,
disk selection, and confirmation. For CLI operation, `start` must be run by the
controller administrator in a controlling terminal:

```sh
nixorium install usb prepare --host pc01
nixorium install usb start --host pc01
nixorium install usb status --id 0123456789abcdef0123456789abcdef --json
nixorium install usb reconcile --id 0123456789abcdef0123456789abcdef
```

Preparation is non-destructive and target-independent. The guided TUI observes
the host key before password entry and pins it only after the operator confirms
the physical-console match. Start preserves the manual equivalent for CLI use,
then replaces the password with an operation key,
requires a signed-cache closure, excludes the boot medium, and presents the
exact configured host/disk/revision review. Do not automate `/dev/tty`, expose
the password in arguments/chat/logs, disable signature or host-key checks, or
approve the destructive phrase for the operator.

After apply dispatch, status is independent of the initiating terminal. Use
reconcile after loss or restart; never issue a second apply or assume a silent
client is safe. Controller recovery may require the operator to run `start`
again and physically re-enter the same address, fingerprint, and password. The
guided TUI instead re-observes the fingerprint for comparison and asks the
operator to re-enter only the address and password; it
can reattach only to the same live boot and then observes status without
replaying Disko. A new boot or changed identity remains blocked. Remove the USB
only when ready, then confirm the separate reboot action. The TUI retries the
read-only installed-system verification automatically after disk boot; use
**Verify now** for an immediate retry. The CLI retains separate explicit
`reboot` and `verify` commands.
`cancel` is valid before dispatch and after a confirmed remote failure whose
receipt says disk mutation did not start. Status normally revokes the live key
and releases that reservation automatically; use **Cancel safely** only when
cleanup remains pending. Cancellation remains forbidden for uncertain or
post-mutation failures; `close` does not prove an uncertain disk safe. Approve
a reinstall's `ROTATE HOST KEY` only after the physical host and disk are
independently established; the entry changes after verification.

If the live ISO disappears before apply was ever dispatched, explicitly close
the operation with `install usb close --id OPERATION_ID`. Review `CLOSE`: it
discards only local operation credentials and releases the reservation; remote
key revocation remains unconfirmed. A new install needs fresh identity and disk
review. Consumed apply tokens, any remote receipt, uncertain dispatch, and
reboot evidence prohibit this path. Never remove coordination files manually.

Post-boot verification must succeed before the shared reservation is released.
Controller rebuild is the sole exception to that reservation: after a strict
check of the persisted disk-completion receipt, its service pauses the worker,
acquires and rechecks under the shared lock, and resumes the worker on every
exit. The client may remain offline/unverified; no pending record is discarded.
Active, failed, or unknown disk work still blocks controller activation, and
all other operations continue to respect the reservation.
If an older worker cannot publish known hosts or logs in its sandbox, preserve
the reservation and operation credentials while applying the corrected worker
and service configuration together, then repeat verification. Host trust lives
in `.ssh/nixorium-known-hosts`; the standard `known_hosts` path is a preserved
symlink. Never make the entire `.ssh` writable to the worker or overwrite
conflicting migration/backup evidence to unblock a rebuild.
After an authorized client reboot, verification uses the administrator key and
persisted exact host identity; it can resume even if a controller reboot cleared
the old ISO credentials. This never authorizes replaying install or reboot.

## PXE preparation

Before changing controller addresses or starting the PXE proxy, prepare the
session inputs from a clean, committed deployment:

```sh
nixorium pxe prepare
nixorium status
nixorium doctor
```

The fixed `nixorium-prepare-pxe.service` runs the build as `admin`, prefers
`masterDhcpIp` when it is assigned or selects the only usable non-static,
non-link-local IPv4 address on the configured interface, checks Harmonia over
that address, and builds every client closure plus the kernel, initrd, iPXE
script, and pinned firmware. It retains their closures with
managed Nix garbage-collector roots and atomically records the immutable store
paths for the exact Git revision at
`/var/lib/nixorium/prepared/prepared.json`; no `result-*` links are part of the
normal workflow. The operation is non-disruptive and retry-safe. A changed
unambiguous lease needs only a new preparation; multiple candidates are
refused until the ambiguity is resolved. Use
`journalctl -u nixorium-prepare-pxe.service` for durable build and preflight
failures.

## Updating the upstream input

Use the reviewed workflow for a generated deployment:

Replace `RELEASE_TAG` with the chosen published tag (or the explicitly requested
`master` channel), and `REVIEW_TOKEN` with the token returned by that plan.

```sh
nixorium update check
nixorium update plan --target RELEASE_TAG
nixorium update apply --target RELEASE_TAG --expect REVIEW_TOKEN
```

`update check` is the only remote-enumerating operation. It queries only the
configured public GitHub upstream, with Git prompts/helpers/config overrides
disabled, a 15-second timeout, bounded output, and at most 20 newest stable plus
20 newest prerelease tags. It is read-only and optional; use an explicit target
without enumeration if the required sources are already available. An explicit
target does not guarantee offline validation: resolving/building it may need
controller Internet access.

It preserves upstream identity, validates a candidate lock outside the checkout,
builds representative outputs, and writes only `flake.nix`/`flake.lock` after
reviewed CLI confirmation. Preserve the deployment-owned `nixpkgs` lock node;
a framework update is not a package-base refresh. In supported controller-only
mode, candidate validation builds the controller only. Laboratory mode includes
one representative client and the installation outputs. Prerelease and downgrade
targets require their explicit policy flags. CLI apply never commits, pushes,
activates, starts PXE, or deploys.
The TUI's update task uses the same reviewed proposal, then records it locally
and builds/activates the controller after Enter confirmation. Client distribution
remains separate. Inspect the active deployment's capabilities and the current
review; do not mistake this TUI path for a configuration-only operation.

System/package updates are separate: inspect `nixorium package-base status`,
then `package-base plan` to advance the current channel. A new stable channel
requires `--target nixos-YY.MM --allow-unverified` in both plan and apply;
apply also requires `--expect REVIEW_TOKEN`. These plans preserve every other
lock node, validate controller/client variants and offline installer equivalence,
and refuse ambiguous/legacy source layouts. The TUI Maintenance → Update
system and packages task exposes the same operation and shared save/controller
recovery. Build success does not prove reboot, hardware, Veyon or data migration.
Verify a canary client before explicit fleet distribution; refresh PXE artifacts
before new installations. Do not change `system.stateVersion` or bypass a
failed build. Older private templates need a reviewed adoption, never an
automatic rewrite; see the deployment's UPDATES.md (upstream docs/updates.md).
A framework update may change its own patches/packages despite fixed nixpkgs.
Declare `mkLab.updateValidationHosts` for private host-conditional variants;
explicit host modules, scoped software, interface and Veyon variants are covered.

For an unsupported computed input, create a temporary upgrade branch, change
`inputs.nixorium.url` to the chosen released tag, and update only that input:

```sh
nix flake update nixorium
git diff -- flake.nix flake.lock
```

Review release notes and schema changes. Run the full representative client,
controller, netboot, installer-bundle, and offline-equivalence validation
before merging when in laboratory mode; controller-only deployments require
only their supported controller checks. Inspect the lock diff to ensure the
package base and unrelated inputs were preserved. Updating the pin does not
authorize deployment.

## PXE lifecycle and interrupted operations

Use `nixorium pxe start`, `nixorium pxe stop`, and `nixorium pxe recover`
through their managed boundaries. Start requires the reviewed network transition
and can interrupt static-address connections; stop restores normal networking.
Recovery is for interrupted PXE state, not a general fix for unapplied network
configuration. Automatic boot recovery runs only when the durable PXE session
record exists, so an ordinary controller activation does not start it. Never
start the internal network unit directly.

After a failure, inspect the operation report, `nixorium doctor`, and the named
journal/log before retrying. Do not loosen permissions, delete receipts, rotate
keys, clear host identity records, or manipulate interface addresses to bypass
a refusal. A failed deployment can be partial; a disconnected host is not proof
of shutdown or rollback. Ask for direction when recovery requires a new
destructive action or uncertain target.

## Native Veyon and client access

Every laboratory host uses native PipeWire/Wayland capture. The external VNC
bridge and shared password are removed. The deprecated `veyonNativeHosts`
string list remains accepted and ignored; existing settings need no change. GNOME
needs one local approval of screen sharing and input access; token persistence does not bypass initial consent.
Validate monitoring, control, locking, demo, service restart, logout/login and
reboot/home reset after deployment. The controller needs its own consent when
broadcasting its screen. Client SSH/Veyon ports accept only the controller's
static IPv4 address on the lab interface; other sources and IPv6 are blocked.
Do not open port 5900 or add a shared VNC password to recover a failed session.

Native hosts keep each user's Veyon token and portal permission database in
`/var/lib/nixorium/veyon-session/<user>`, outside the reset home and snapshots.
Portal grants for other applications also persist; normal student files do not.
First enablement uses a fresh permission store and may ask to reapprove grants.
Do not print tokens, copy them into templates, or reuse them on another host.
Revocation or display changes can require fresh approval. Do not delete this
state to diagnose a connection error.
