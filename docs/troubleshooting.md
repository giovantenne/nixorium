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

Run these read-only commands from the deployment repository:

```sh
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
  network transition.

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

## One client is offline or unknown

Run `nixorium hosts`. `unreachable` means the bounded network probe received no
answer; `SSH unavailable` distinguishes a reachable machine without the expected
SSH service. `unknown` deployment state means Nixorium could not authenticate
and prove the active revision—it does not mean success or failure.

Check power, cabling/VLAN, the configured address, and SSH on that client. Treat
a changed host key as a security event; do not delete `known_hosts` entries
blindly. Clients installed before the fixed host-state helper remain `unknown`
until their next normal deployment.

## A deployment failed

A Colmena failure can leave selected machines at different generations.
Nixorium does not claim an automatic rollback across hosts. Unexpected terminal
loss can interrupt the foreground deployment and uses this same recovery path.

1. Read the operation-log ID from the result and run
   `nixorium logs show OPERATION_LOG_ID`.
2. Run `nixorium hosts` to obtain fresh authenticated state for every target.
3. Fix the reported build, reachability, or activation problem.
4. Make a fresh `nixorium deploy plan --on ...` against a clean revision.
5. Retry the full build-first `deploy apply` workflow.

Only hosts that report the reviewed revision and a concrete system path are
recorded as successful. Historical success never overrides the live result.

## The Git tree is dirty

```sh
nixorium git review
git status --short
```

Inspect every staged, unstaged, and untracked path. Nixorium never discards or
stashes changes automatically. Commit an intentional safe allowlist with
`nixorium git commit plan --paths ...` and its reviewed apply command, or resolve
the worktree manually. Never add `secret-key`, `admin-ssh`,
`veyon-private-key.pem`, or plaintext credentials.

## Configuration is invalid

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
| Client deployment | May leave mixed generations; inspect logs/hosts and retry a fresh convergent plan. |
| Upstream update | Plan is read-only; apply rolls back when possible and reports an unsafe partial pair explicitly. |

## Backups and restoration

Back up the private deployment repository including its `.git` history and the
three ignored private key files. Store that backup encrypted and separately
from the controller. A Git remote is recommended for the private tracked
configuration, but private keys must never be pushed or committed.

Operation logs and authenticated deployment history under the administrator's
private XDG state are useful audit evidence but are not configuration authority.
PXE manifests, GC roots, installed secret copies, and controller receipts under
`/var/lib/nixorium` are derived operational state and can be regenerated from a
valid repository plus the original private keys.

After restoring, verify ownership and mode `0600` for private keys, run
`nixorium setup keys --verify-only`, reinstall verified secrets, apply the
reviewed controller configuration, and prepare PXE artifacts again. Do not
restore stale active-session markers onto a different controller.
