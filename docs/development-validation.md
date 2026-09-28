# Development validation

Nixorium uses a validation pyramid. The normal edit-test loop must stay fast;
expensive system builds and virtual machines are selected only when the changed
boundary can affect them. The complete matrix is a checkpoint, not a default
development command.

## Validation levels

| Command | Purpose | Run it when |
|---|---|---|
| `./scripts/test-go.sh [packages/flags]` | Incremental Go tests with the locked compiler | During Go edits; defaults to `./...` |
| `./scripts/validate.sh` | Reproducible source gate | After a coherent change |
| `./scripts/validate.sh --eval` | Complete `mkLab` contract evaluation | Nix schemas, Flake API, host composition, package scopes, or module wiring changed |
| `./scripts/validate.sh --management-vm` | Controller and management integration | Operational adapters, privileged boundaries, controller lifecycle, or end-to-end TUI/CLI flows changed |
| `./scripts/validate.sh --client-installer-vm` | Installer integration | Enrollment, Disko installation, or installer runtime behavior changed |
| `./scripts/validate.sh --remote-client-installer-vm` | USB/SSH live-client boundary | Remote helper, signed cache transfer, live-session identity, disk/reboot receipts, or resume semantics changed |
| `./scripts/validate.sh --ci` | Evaluation-only source and template graph | CI and release-source evaluation |
| `./scripts/validate.sh --full` | Complete build, VM, template, and offline-equivalence checkpoint | Required in release-tag CI; local milestone/cross-cutting checks and optional release preflight |
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

For Go edits, use the incremental runner. It accepts ordinary `go test`
arguments and retains Go's build/test cache:

```sh
./scripts/test-go.sh
./scripts/test-go.sh ./internal/presentation/...
./scripts/test-go.sh ./internal/presentation/... -run TestSoftware
```

To avoid even the shell startup cost, enter the locked toolchain once and keep
the shell open:

```sh
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell
go test ./...
```

Run the default gate before considering the change complete; it repeats all
Go tests in the reproducible package build and checks shell, docs and schemas.
Neither development command runs a VM or builds NixOS systems. Do not use
`-count=1` routinely: it deliberately disables Go's test-result cache.

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

The installed targets in both installer VMs disable VirtualBox guest additions
through their shared QEMU-only instrumentation module. Otherwise systemd waits
for the absent `dev-vboxguest.device` before reaching `multi-user.target`, even
though the guest service has a virtualization condition. The tests assert that
the service is absent; production configurations retain their VirtualBox
default. This fixture optimization does not qualify VirtualBox support.

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
- Release preparation: focused local gates, optionally followed by `--full`;
  the release-tag workflow must pass `--full` before publication.

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

The package explicitly runs `go test ./...`, including `internal/` and developer
commands. `buildGoModule` otherwise limits its tests to the installed
`subPackages`, which would silently omit domain, application, adapter and TUI
tests. Git is a test dependency for the real temporary-repository fixtures.

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

CI groups checks, upstream systems and template outputs through
`tests/ci-eval.nix`, replacing repeated evaluations of individual attributes.
Each group shares its NixOS module graphs within one evaluator; separate groups
bound memory use. All declared checks are included, and import-from-derivation
remains disabled. Profile variants are still evaluated independently because
they change the temporary deployment's declarations.

Pull requests and pushes to `master` run Go packaging/tests and `--ci`, retaining
the 15-minute limit per job. Full validation runs only in the release workflow
for `v*.*.*` tags, including prereleases. It validates metadata before expensive
work and checks out the event's exact commit in both validation and publication
jobs. Publication depends on successful `--full`; failed, cancelled or timed-out
validation leaves the tag without a GitHub Release.

The disposable Ubuntu release runner reclaims unused preinstalled SDK space,
enables and checks KVM, and limits Nix to one build at a time with two cores.
Nix build and silence timeouts are disabled, while test-specific deadlines are
preserved. The job uses the [maximum hosted duration of six hours](https://docs.github.com/en/actions/reference/limits).
Removing `timeout-minutes` would not remove that platform limit. The validation
log is retained for 14 days when artifact upload can run; GitHub's job log also
remains available. Release notes and logs live outside the checkout so they do
not change the Flake source being qualified.

Full local release preflight is optional; successful full CI on the tagged
commit is mandatory. If the full matrix exceeds hosted disk, memory or runtime
limits, publication stays blocked until the runner or workload is addressed.
Do not substitute evaluation-only success for full qualification.

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

The internal workspace schema has a shared raw-JSON corpus in
`tests/workspace-validation-cases.json`. Both Go and Nix check rejection and
normalization, including duplicate object keys, exact field names, nulls,
ordered favorites and sorted extension sets. Run the focused checks with
`./scripts/test-go.sh ./internal/domain -run Workspace` and
`nix build --file tests/source-checks.nix workspace-schema --no-link`.
The evaluator takes JSON text, not an attrset: decoding it first would erase
duplicate keys. These tests validate data only, not installed applications,
extension loading, student-home changes, or a public management workflow.

`workspace-resolution` checks the internal catalog/baseline resolver, complete
target coverage (including controller-only), package and extension dependencies,
and unavailable, blocked or mismatched package identities. Synthetic packages
cover failure modes; a separate assertion resolves an extension from the locked
package set. No manifest is read from a derivation during evaluation. Passing
this check proves neither live installed state nor extension loading.

The `mk-lab` graph also checks workspace preparation against generated system
packages, controller-only mode, downstream removals, unchanged representative
system derivations and template compatibility without workspace input.
`workspace-offline` compares system derivations and workspace metadata with
the serialized installer, and `workspace-systems` builds its controller and representative
client. Both build checks belong to the full checkpoint, not the quick loop.

The focused `workspace-seed` and `workspace-seed-pinned` checks build the
internal preference payload. They read compiled dconf values using the pinned
GNOME schemas, verify copied preferences remain editable, check browser/editor
files and extension links, and reject inconsistent extension manifests. The
first uses a synthetic extension; the second checks a real pinned extension
payload. Run both, plus `desktop-profile`, through `tests/source-checks.nix`.
They are full-checkpoint checks, not part of the fast edit loop. They do not
activate homes, exercise the reset service, or prove VS Code loads an extension.

The internal home-reset engine has ordinary Go filesystem tests under
`internal/homereset`. Run them with
`./scripts/test-go.sh ./internal/homereset -race`. The dedicated
`nix build --file tests/source-checks.nix home-reset-filesystem-vm --no-link`
check adds real same-filesystem file/directory bind mounts and nested Btrfs
subvolumes on a disposable VM disk. It also exercises immutable-seed validation,
account ownership, a held login barrier, active student-process rejection,
whole-home preflight, private snapshot sanitization, five-snapshot rotation,
restore ownership, editable preferences and dconf/wallpaper composition.
Injected failures before deletion and during restoration retain durable
evidence, prevent blind retries and preserve recovery data across a VM reboot.
Negative lifecycle cases assert the rejection reason, not just any error.
These are internal engine tests, not proof of systemd ordering, deployment
activation, or legacy-path safety. Mount/lifecycle tests are explicitly disabled
outside that fixture. The VM belongs to the full checkpoint, not the quick loop.

The separate `workspace-reset-service-vm` check imports the internal service
module in two minimal systems. It verifies successful boot ordering and the
packaged helper's fixed root-only entry point, then performs real NixOS
configuration switches while a student-owned process runs. Both a seed update
and a transition from an active legacy reset preserve the process and session
files. Retained failure evidence blocks the display-manager fixture and normal
user sessions on reboot. This check uses a small login consumer, not GNOME;
it does not establish complete `mkLab` or deployment-template integration.
Run it through `tests/source-checks.nix`; it is also in the full checkpoint.

After the automated milestone, follow the documented VirtualBox recipe with
the official Minimal ISO. Keep that result separate from physical-hardware
evidence; neither is replaced by a simulated NixOS VM pass.

Do not move a test upward merely because a higher level can exercise it. When a
new integration scenario requires fixed sleeps or a full desktop closure,
also add a lower-level deterministic test for its state machine or validation
logic whenever possible.

## Isolated client firewall test

After changing client ingress rules, run:

```sh
nix develop --file tests/source-checks.nix network-shell --command scripts/check-client-firewall.sh
```

This uses the production rule generator in unprivileged user/network namespaces.
It refuses the host namespaces, creates only temporary virtual links, and checks
master access, peer denial, IPv6, loopback, external VNC and established peer
connections. No live firewall, gateway or client is changed. Run the mkLab
contract evaluation and affected system builds as well.

The same namespace test covers temporary Internet blocking, existing external
connections, IPv6, management reachability, firewall reload and unblock. The
management VM additionally exercises authenticated CLI dispatch, real systemd,
student denial and reboot restoration. On development hosts without KVM, run
the Internet scenario in the same management VM, using software emulation:

```sh
nix build --file tests/source-checks.nix internet-management-vm-tcg --no-link
```
