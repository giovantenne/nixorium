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
fails, some targets may already have changed; inspect the reported log and
host state, make a fresh plan, and retry. `--yes` is only for explicit
automation. The raw commands below remain advanced manual operations and bypass
these safeguards:

The default TUI's **Deploy updates** screen invokes the same plan/apply
operations. Select the intended computers, review the resolved revision and
targets, and enter the exact phrase shown. Do not close the controller terminal
until the final result and log path appear.

```sh
colmena apply --on pc05
colmena apply --on @lab
```

Host keys are accepted on first connection and verified on later connections.
Investigate changed-key failures instead of deleting `known_hosts` entries
blindly.

## Binary cache

After `nixorium setup apply`, the controller owns Harmonia through systemd; do
not launch a second foreground cache. Check both unit and HTTP readiness with:

```sh
systemctl status nixorium-harmonia.service
nixorium doctor
```

The stable product alias is used for status and service control. Query detailed
logs with `journalctl -u harmonia.service`, the canonical nixpkgs unit name.
The signing key is loaded from `/var/lib/nixorium/keys/harmonia-secret-key` as
an isolated systemd credential and must never be copied into Git or the store.

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
