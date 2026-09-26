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
and settings schemas, and the packaged Go command with its unit tests. An
isolated runtime check also exercises a real Git-backed command with an
otherwise empty `PATH`, proving the installed wrapper supplies its external
dependency fallback. The schema and Go derivations use
`tests/source-checks.nix`, which imports the exact locked `nixpkgs` directly and
avoids constructing the full laboratory graph. It does not evaluate `mkLab`,
build NixOS systems, or run VM tests.

For repeated Go edits, enter the lightweight locked toolchain once with
`nix --extra-experimental-features 'nix-command flakes' develop --file tests/source-checks.nix go-shell`
and run `go test ./...` inside it. Go's incremental cache makes this the tight
loop; finish with the default gate for the reproducible package test.

Add the full `mkLab` contract when Nix schemas, the Flake API, host composition,
module wiring, or software scopes change:

```sh
./scripts/validate.sh --eval
```

This mode includes the quick checks and evaluates the complete `mkLab` test
graph without booting a VM.

For GNOME workstation changes, also run the focused schema/extension check:

```sh
nix --extra-experimental-features 'nix-command flakes' build \
  --file tests/source-checks.nix desktop-profile --no-link
```

It compiles the actual GSettings overrides, checks extension metadata against
GNOME's pinned major version, and validates the login script. Keep it separate
from the quick Go gate: a cold store may need GNOME dependencies. It does not
replace login/hardware verification or an affected-system build.

Add the one affected integration test when changing its behavior:

```sh
./scripts/validate.sh --management-vm
./scripts/validate.sh --client-installer-vm
./scripts/validate.sh --remote-client-installer-vm
```

The management VM is required for changes to management operations that cross
process, filesystem, network, privilege, systemd, or end-to-end terminal
boundaries. Presentation-only refactors with focused state-transition unit
tests do not require it. The client-installer VM is required for changes to
enrollment, Disko installation, or installer runtime behavior. Each targeted
mode includes the quick checks. The remote-client installer VM is required for
USB/SSH live-session identity, signed-cache transfer, remote Disko receipts,
resume, reboot, or post-boot verification. It simulates the live contract and
does not replace a run with the official ISO or physical hardware.

Run the complete local matrix with:

```sh
./scripts/validate.sh --full
```

It additionally builds every declared `checks` derivation, a representative
client, the controller, netboot ramdisk, Disko package, PXE firmware, command
package, PXE installer bundle, and target-independent remote installer bundle,
generates a fresh site deployment, and verifies offline derivation equivalence.
It does not ask `nix flake check` to enumerate
all generated clients: address/hostname generation is covered by `mk-lab`, and
one client exercises their shared module graph. Run it after public API,
template, built-in module, installer bundle, asset-plumbing, input, Disko, or
netboot changes only when the change crosses several built roles or affects
offline equivalence. Always run it before a milestone or release is declared
complete. During development, prefer `--eval`, one targeted VM, and affected
real closure builds. A successful evaluation does not prove that source
patches compile, so affected host roles require real builds.

Never build all generated clients merely to repeat their shared module graph.
Build one representative client plus the controller; schema and `mkLab` tests
cover the complete generated hostname/address inventory. Add another client
build only when a changed host-specific module or role override makes that host
materially different.

The complete mode groups related upstream outputs into one `nix build`
invocation. The generated deployment's management executable is built once and
reused for its CLI scenarios. Keep this batching intact: repeated Flake
evaluation is a material part of the uncached runtime.

The GitHub source/template job must use the evaluation-only mode:

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
builds. A separate CI job builds the same direct-source `nixorium` and isolated
runtime checks used by the fast gate, including the Go unit tests, without
constructing the laboratory graph or building NixOS system closures. Keep the
full matrix off GitHub-hosted runners; it is a local prerequisite for changes
that affect builds and for release preparation.

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

`bash scripts/check-agent-guidance.sh` checks the actual instruction examples
against the CLI parser without executing operations, checks relative links,
and validates skill copies/discovery. The quick gate and management-command CI
run it automatically. Its compiled checker reads prose at runtime to preserve
the code build cache. Review behavioral claims using
[the guidance maintenance map](../../../docs/agent-guidance.md); these checks
cannot prove prose semantics or replace workflow tests.

The contributor-facing decision table and guidance for placing new tests are
maintained in `docs/development-validation.md`.
