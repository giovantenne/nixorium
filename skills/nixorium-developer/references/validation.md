# Upstream validation

Never update `flake.lock` unless input updates are part of the task. The
validation script uses a persistent evaluation cache under
`${XDG_CACHE_HOME:-$HOME/.cache}/nixorium-validation`; override it with
`NIXORIUM_VALIDATION_CACHE_HOME` only when isolation is required.

Use the quick validation during the normal edit-test cycle:

```sh
./scripts/validate.sh
```

This is equivalent to `--quick`. It checks shell syntax, Git whitespace,
client-installer shell tests, skill distribution and discovery, configuration
and settings schemas, the `mkLab` contract, and the packaged Go command with
its unit tests. It does not build NixOS systems or run VM tests.

Add the one affected integration test when changing its behavior:

```sh
./scripts/validate.sh --management-vm
./scripts/validate.sh --client-installer-vm
```

The management VM is required for changes to management operations, adapters,
controller lifecycle, or their TUI/CLI integration. The client-installer VM is
required for changes to enrollment, Disko installation, or installer runtime
behavior. Each targeted mode includes the quick checks.

Run the complete local matrix with:

```sh
./scripts/validate.sh --full
```

It additionally builds every declared `checks` derivation, a representative
client, the controller, netboot ramdisk, Disko package, PXE firmware, command
package, and installer bundle, generates a fresh site deployment, and verifies
offline derivation equivalence. It does not ask `nix flake check` to enumerate
all generated clients: address/hostname generation is covered by `mk-lab`, and
one client exercises their shared module graph. Run it after public API,
template, built-in module, installer bundle, asset-plumbing, input, Disko, or
netboot changes, and before a milestone or release is declared complete. A
successful evaluation does not prove that source patches compile, so affected
host roles require real builds.

GitHub Actions must use the evaluation-only mode:

```sh
./scripts/validate.sh --ci
```

This mode checks syntax and skill distribution, evaluates the schema tests and
one representative client plus the controller, netboot, apps, packages,
Colmena metadata, and deployment status. It also generates and evaluates a
fresh deployment and its installer bundle without building system closures.
It intentionally skips the other generated clients because they share the
same module graph and their address generation is covered by `mk-lab` tests.
The CI mode disables import-from-derivation so evaluation cannot trigger hidden
builds. Keep the full matrix off GitHub-hosted runners; it is a local
prerequisite for changes that affect builds and for release preparation.

Build modes use `--no-link`; the full mode's one installer result link exists
only inside its automatically removed temporary directory. Validation therefore
leaves no persistent result roots. Unrooted results can still remain as reusable
Nix store cache until the system garbage collector removes them. Inspect that
state with `nix-store --gc --print-dead`; never run garbage collection
automatically or assume that validation work authorizes deleting shared store
cache.

For skill changes, validate both skill directories with the skill validator.
The upstream and template copies of `nixorium-maintainer` must be identical,
and template discovery links must resolve to that copy.
