# Lab operations

## Validation

For every configuration change:

```sh
git diff --check
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
ramdisk and `installerBundle`. Evaluate one client through both the deployment
and the real installer-bundle store path with `--offline`; the two
`system.build.toplevel.drvPath` values must match.

## Installation and deployment

Disk installation is destructive. Resolve the exact host and disk first and
retain the installer's explicit confirmation. Do not install or deploy merely
because builds succeeded.

Use Colmena only after authorization:

```sh
colmena apply --on pc05
colmena apply --on @lab
```

Host keys are accepted on first connection and verified on later connections.
Investigate changed-key failures instead of deleting `known_hosts` entries
blindly.

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
