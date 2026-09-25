# Lab operations

## Validation

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
fails, some targets may already have changed. After every attempt, Nixorium
authenticates to the selected hosts and records only those reporting the
reviewed revision and a concrete system path; the private per-repository
history lives under `~/.local/state/nixorium/deployments/`. Inspect the
reported log and host state, make a fresh plan, and retry. The interactive
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
TUI's **Rebuild controller** task uses the same
typed operation with an Enter confirmation after review; the systemd job and
journal survive closing the dashboard. Keep `setup apply` for first-run
compatibility, not as a replacement for routine reviewed controller plans.

The TUI's client distribution task invokes the same plan/apply
operations. Select the intended computers, review the resolved revision and
targets, and enter the exact phrase shown. Do not close the controller terminal
until the final result, authenticated/recorded counts, and log path appear.

```sh
colmena apply --on pc05
colmena apply --on @lab
```

Host keys are accepted on first connection and verified on later connections.
Investigate changed-key failures instead of deleting `known_hosts` entries
blindly.

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

## Client shutdown

Use the reviewed client-only workflow:

```sh
nixorium shutdown plan --on pc05
nixorium shutdown plan --on @lab
nixorium shutdown apply --on @lab --expect REVIEW_TOKEN
```

The controller is never a valid target. Planning checks evaluated client
identity, management access, interactive sessions, PXE/controller-network
state, and concurrent client operations. An active user session remains
eligible after a prominent warning that unsaved work may be lost. Unknown
session state remains blocked unless both plan and apply use
`--acknowledge-unknown-sessions` after explicit review. Unreachable targets are
shown as not sent and are never queued for later.

When the plan includes an active session, the review states explicitly that
`SHUTDOWN` authorizes interrupting it. Apply requires that single word, takes the same lock as
deployment, and repeats inventory, conflict, and session checks immediately
before issuing the fixed operating-system request. Results describe only
`accepted`, `not-sent`, or `unconfirmed`. A successful request is not proof of
physical power state; a lost connection may mean the request took effect, so
do not retry an unconfirmed target blindly. `--yes` is only for deliberate
automation with the exact fresh review token.

## Software and configuration changes

Use [the software guide](software.md) for scopes, package updates, and the
different CLI/TUI effects. Use [the student-home guide](student-home.md) for
editor extensions, desktop preferences, and content that must survive resets.

## Git review and operation logs

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
`x86_64-linux`, UEFI, and wired Ethernet. Ask the operator to keep its physical
console visible, set a temporary password, and read the canonical IPv4 address
and Ed25519 `SHA256:` fingerprint there. In the guided TUI, enter only the
address: Nixorium observes the live key without credentials, displays its
fingerprint, and asks for `MATCH` after a complete physical-console comparison.
Never approve the fingerprint using DNS, a previous boot, or `known_hosts`.

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
only when ready, then use separately confirmed `reboot` and `verify` actions.
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
