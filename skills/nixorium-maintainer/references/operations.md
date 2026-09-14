# Lab operations

## Validation

For every configuration change:

```sh
git diff --check
nix run .#nixorium -- config validate
nix eval .#labMeta --json --no-write-lock-file
nix eval .#deploymentStatus --json --no-write-lock-file
```

Build every affected role. A representative client and controller validation
is:

```sh
CONTROLLER_NAME="$(nix eval .#labMeta.controller.name --raw --no-write-lock-file)"
nix build .#nixosConfigurations.pc01.config.system.build.toplevel --no-write-lock-file --no-link
nix build ".#nixosConfigurations.${CONTROLLER_NAME}.config.system.build.toplevel" --no-write-lock-file --no-link
```

For netboot, assets, or extension-module plumbing, also build the netboot
ramdisk, `pxeFirmware`, and `installerBundle`. Evaluate one client through both
the deployment and the real installer-bundle store path with `--offline`; the
two `system.build.toplevel.drvPath` values must match.

## Installation and deployment

Disk installation is destructive. Resolve the exact host and disk first and
retain the installer's explicit confirmation. Do not install or deploy merely
because builds succeeded.

Use Colmena only after authorization:

```sh
nixorium deploy plan --on pc05
nixorium deploy plan --on @lab
nixorium deploy apply --on @lab --expect REVISION_FROM_PLAN
```

Planning requires a ready deployment and clean Git worktree, records HEAD, and
rejects unknown or duplicate clients. Use the exact command and revision shown
by the plan. Apply revalidates the review, requires `DEPLOY <targets>`, runs a
verbose build before activation, and records a private durable log. If apply
fails, some targets may already have changed. After every attempt, Nixorium
authenticates to the selected hosts and records only those reporting the
reviewed revision and a concrete system path; the private per-repository
history lives under `~/.local/state/nixorium/deployments/`. Inspect the
reported log and host state, make a fresh plan, and retry. `--yes` is only for
explicit automation. The raw commands below remain advanced manual operations
and bypass these safeguards:

For routine changes to this controller, use the separate reviewed workflow:

```sh
nixorium controller plan
nixorium controller apply --expect REVISION_FROM_PLAN
```

Apply requires exact `REBUILD <controller>` confirmation and starts only a
revision-bound systemd instance. It builds the pinned Git source as the
deployment owner, refuses repository drift before activation, and verifies the
active system afterward. The TUI's **Rebuild controller** task uses the same
typed operation; the systemd job and journal survive closing the dashboard.

The default TUI's **Deploy updates** screen invokes the same plan/apply
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

Review deployment changes without mutating the index or worktree:

```sh
nixorium git review
```

The report keeps staged, unstaged, and untracked paths distinct and labels
Nixorium-managed settings/public keys separately from unexpected edits. It
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

After `nixorium setup apply`, the controller owns Harmonia through systemd; do
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

Restart requires exact `RESTART CACHE` confirmation, invokes only the fixed
capability-free `nixorium-restart-cache.service` action, and verifies both the
unit and HTTP endpoint afterward. Do not use this path to control PXE units;
their listener and network transition must remain coordinated through
`nixorium pxe`. The TUI's **Manage services** task uses the same typed
operations. `--yes` is only for intentional automation.

## PXE preparation

Before changing controller addresses or starting the PXE proxy, prepare the
session inputs from a clean, committed deployment:

```sh
nixorium pxe prepare
nixorium status
nixorium doctor
```

The fixed `nixorium-prepare-pxe.service` runs the build as `admin`, verifies
that `masterDhcpIp` is currently assigned to the configured interface, checks
Harmonia over that address, and builds every client closure plus the kernel,
initrd, iPXE script, and pinned firmware. It retains their closures with
managed Nix garbage-collector roots and atomically records the immutable store
paths for the exact Git revision at
`/var/lib/nixorium/prepared/prepared.json`; no `result-*` links are part of the
normal workflow. The operation is non-disruptive and retry-safe. If the lease
changed, update and commit the setting, apply the controller, then prepare
again. Use `journalctl -u nixorium-prepare-pxe.service` for durable build and
preflight failures.

## Updating the upstream input

Create a temporary upgrade branch, change `inputs.nixorium.url` to the chosen
released tag, and update only that input:

```sh
nix flake update nixorium
git diff -- flake.nix flake.lock
```

Review release notes and schema changes. Run the full representative client,
controller, netboot, installer-bundle, and offline-equivalence validation
before merging. Updating the pin does not authorize deployment.
