# Development validation

Nixorium uses a validation pyramid. The normal edit-test loop must stay fast;
expensive system builds and virtual machines are selected only when the changed
boundary can affect them. The complete matrix is a checkpoint, not a default
development command.

## Validation levels

| Command | Purpose | Run it when |
|---|---|---|
| `./scripts/validate.sh` | Fast source gate | After every coherent edit |
| `./scripts/validate.sh --eval` | Complete `mkLab` contract evaluation | Nix schemas, Flake API, host composition, package scopes, or module wiring changed |
| `./scripts/validate.sh --management-vm` | Controller and management integration | Operational adapters, privileged boundaries, controller lifecycle, or end-to-end TUI/CLI flows changed |
| `./scripts/validate.sh --client-installer-vm` | Installer integration | Enrollment, Disko installation, or installer runtime behavior changed |
| `./scripts/validate.sh --ci` | Evaluation-only source and template graph | CI and release-source evaluation |
| `./scripts/validate.sh --full` | Complete build, VM, template, and offline-equivalence checkpoint | Before a milestone or release, and after cross-cutting changes that can affect several built roles |

The default gate checks whitespace, shell syntax, shell regression tests, skill
and troubleshooting-copy coherence, the three Nix data schemas, and the Go
package with its unit tests. Its Nix expression imports the exact `nixpkgs`
revision from `flake.lock` directly. It deliberately avoids evaluating
`defaultLab`, so a warm run remains suitable for frequent use.

It also runs `bash scripts/check-agent-guidance.sh`: a cached Go test binary
uses the real CLI parser on instruction examples and checks links, discovery,
and maintainer-copy equality. It only reads the checkout and never executes
documented operations. The same check runs in the management-command CI job;
the `--ci` source/template job remains evaluation-only. See the
[guidance maintenance map](agent-guidance.md) for the required semantic review.

For a tight Go edit-test loop, enter the lightweight locked toolchain once and
keep the shell open:

```sh
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell
go test ./...
```

This uses Go's incremental build cache between edits. Run the default gate
before considering the change complete; it repeats the tests in the
reproducible package build.

`--eval` adds the complete `checks.x86_64-linux.mk-lab` assertion graph. It is
the normal escalation for Nix behavior that does not require booting a machine.
Presentation-only Go refactors do not require a VM when focused unit tests cover
the changed state transitions. Use the management VM when behavior crosses the
terminal, process, filesystem, network, privilege, or systemd boundary.

The VM modes include the fast gate. They are intentionally not part of
`--eval`: the management VM contains end-to-end terminal flows and the client
installer VM exercises a real installation path, so both have materially
higher fixed cost.

## Selecting the smallest sound gate

- Go domain, application, adapter, or presentation logic: default gate; add
  `--management-vm` only for an affected integration boundary.
- JSON/Nix settings validation: default gate plus `--eval`.
- `lib.mkLab`, built-in module composition, or software scope semantics:
  `--eval`, followed by the affected real host build before completion.
- Controller service or privileged operation: `--management-vm` and the
  controller build.
- Disko or client installer runtime: `--client-installer-vm` and the affected
  installer/client build.
- Netboot, PXE firmware, assets embedded in systems, inputs, or offline bundle
  plumbing: use the complete checkpoint because several outputs must agree.
- Release preparation: always use `--full` after the focused gates are green.

Do not run `--full` repeatedly while editing. First make the default gate green,
then run the single affected evaluator, VM, or build. A failure at a higher
level should be reproduced with the narrowest command that still exercises it.

## Performance model

The default gate uses `tests/source-checks.nix` instead of resolving checks
through the public Flake output. This avoids constructing the full laboratory
graph merely to test schemas or compile Go. Nix still verifies the locked
`nixpkgs` source and `buildGoModule` still runs the Go unit tests.
The package source contains only Go sources, module metadata, `VERSION`, and the
JSON fixtures consumed by Go tests, so documentation or unrelated Nix edits do
not invalidate Go compilation.

The guidance checker also compiles from code-only sources and reads instruction
files at runtime. Editing skills or AGENTS files therefore reruns a small check
without rebuilding the management application or any system closure.

The complete gate submits related upstream outputs to one `nix build`
invocation, allowing one Flake evaluation and normal Nix parallel scheduling.
For the generated site it builds the management executable once and reuses the
result for all CLI checks instead of evaluating `nix run` for every command.

Validation uses a persistent evaluation cache at
`${XDG_CACHE_HOME:-$HOME/.cache}/nixorium-validation`. Set
`NIXORIUM_VALIDATION_CACHE_HOME` only when an isolated cache is required. An
empty cache is expected to be slower because locked sources must first be
materialized. Validation creates no persistent result links and must never run
Nix store garbage collection.

## Maintaining the pyramid

New regression tests belong at the lowest level that proves the invariant:

1. pure Go or shell unit test;
2. direct Nix schema assertion;
3. complete `mkLab` evaluation;
4. one targeted VM or real closure build;
5. full release checkpoint.

Do not move a test upward merely because a higher level can exercise it. When a
new integration scenario requires fixed sleeps or a full desktop closure,
also add a lower-level deterministic test for its state machine or validation
logic whenever possible.
