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
| `./scripts/validate.sh --remote-client-installer-vm` | USB/SSH live-client boundary | Remote helper, signed cache transfer, live-session identity, disk/reboot receipts, or resume semantics changed |
| `./scripts/validate.sh --ci` | Evaluation-only source and template graph | CI and release-source evaluation |
| `./scripts/validate.sh --full` | Complete build, VM, template, and offline-equivalence checkpoint | Before a milestone or release, and after cross-cutting changes that can affect several built roles |
| `nix develop --file tests/source-checks.nix security-shell --command ./scripts/security-check.sh` | Pinned Go static and known-vulnerability analysis | Security-sensitive Go changes and the dedicated security workflow |

The default gate checks whitespace, shell syntax, shell regression tests,
generated documentation, allowlisted canonical-copy coherence, the Nix data
schemas, and the Go package with its unit tests. It also runs the packaged
command with an otherwise empty `PATH` to prove that required external tools
have a wrapper fallback. Its Nix expression imports the exact `nixpkgs` revision
from `flake.lock` directly. It deliberately avoids evaluating
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

The dedicated security workflow validates the workflow files with `actionlint`
and runs `staticcheck` and `govulncheck` from the same locked `nixpkgs`
revision. `staticcheck` is deterministic for that lock; `govulncheck` consults
the current Go vulnerability database, so a scheduled run can find a newly
published advisory without a source change. GitHub Actions also performs a
manual-build CodeQL analysis of the Go commands and publishes its results
through code scanning. These tools complement review of NixOS, shell,
privilege, network, and credential boundaries; they do not prove that a
deployment is secure.

`--eval` adds the complete `checks.x86_64-linux.mk-lab` assertion graph. It is
the normal escalation for Nix behavior that does not require booting a machine.
Presentation-only Go refactors do not require a VM when focused unit tests cover
the changed state transitions. Use the management VM when behavior crosses the
terminal, process, filesystem, network, privilege, or systemd boundary.

The VM modes include the fast gate. They are intentionally not part of
`--eval`: the management VM contains end-to-end terminal flows, the PXE client
installer VM exercises its local installation path, and the remote-client VM
uses a real signed Harmonia closure across the SSH/systemd/disk boundary. All
have materially higher fixed cost. The remote VM simulates the supported live
ISO contract; it does not certify the official ISO, VirtualBox networking, or
physical firmware and storage.

USB/SSH coverage is deliberately layered. Shell tests own exact live-installer
preflight predicates, Go adapter tests own the pinned SSH command contract, and
worker tests own the ordered receipt, log-publication, credential-revocation,
and reservation-release transition. The management VM owns durable recovery
and the controller systemd sandbox; the remote-client VM owns the signed cache,
independent job, Disko receipt, installation, and booted target. A runtime USB
regression must add a deterministic test at the lowest responsible layer and,
when the failure depended on filesystem, systemd, or network reality, a fixture
in the closest VM. Passing either VM alone does not prove the whole workflow.

There is currently no automated test that drives the public TUI from a
controller VM into an official NixOS Minimal ISO VM. The documented official
ISO acceptance run therefore remains a required separate checkpoint for that
cross-machine user journey; it must not be represented as covered by the
simulated remote-client VM.

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
- USB/SSH worker/helper, credentials, signed transfer, remote Disko receipt, or
  resume/reboot behavior: `--management-vm` for the controller side and
  `--remote-client-installer-vm` for the remote side, plus the controller,
  representative client, and remote-installer bundle builds.
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
`nixpkgs` source, `buildGoModule` still runs the Go unit tests, and the isolated
runtime check exercises a real Git-backed command through the installed wrapper.
The package source contains only Go sources, module metadata, `VERSION`, and the
JSON fixtures consumed by Go tests, so documentation or unrelated Nix edits do
not invalidate Go compilation.

The guidance checker also compiles from code-only sources and reads instruction
files at runtime. Editing skills or AGENTS files therefore reruns a small check
without rebuilding the management application or any system closure.
The documentation generator is a separate code-only command; its check reads
`docs/tui-gallery.md` in a small derivation. Editing narrative or generated
prose does not invalidate the packaged management application.

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

After the automated milestone, follow the documented VirtualBox recipe with
the official Minimal ISO. Keep that result separate from physical-hardware
evidence; neither is replaced by a simulated NixOS VM pass.

Do not move a test upward merely because a higher level can exercise it. When a
new integration scenario requires fixed sleeps or a full desktop closure,
also add a lower-level deterministic test for its state machine or validation
logic whenever possible.
