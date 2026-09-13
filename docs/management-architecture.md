# Nixorium management architecture

Status: accepted design for incremental implementation

This document defines the target management architecture and the boundaries
that implementation must preserve. It describes the intended end state; items
not yet implemented are tracked in the external project status rather than
being implied complete here.

## Baseline and implementation state

Nixorium already has a sound declarative core:

- `lib.mkLab` turns a typed site configuration into NixOS hosts, Colmena
  metadata, netboot outputs, helper applications, and an offline installer.
- `labMeta` is a versioned, non-secret operational interface. Shell helpers
  consume it through Nix evaluation instead of parsing Nix source.
- `deploymentStatus` reports configuration blockers without making the
  standalone public example unevaluable.
- a private deployment owns identities, network values, password hashes,
  public keys, assets, and site modules while the public repository owns
  reusable behavior;
- the installer bundle preserves the deployment's locked inputs and produces
  the same client derivation without client internet access.

At the start of this design, the administrator edited Nix, generated three key
pairs, invoked builds, ran Harmonia and PXE in two terminals, temporarily
changed an address with `ip`, chose a client identity by numeric argument, and
invoked Colmena directly. The management command, structured settings,
first-run reconciliation, secure key handling, reviewed controller apply, and
managed Harmonia service are now implemented. PXE preparation is systemd-owned
and records immutable artifacts/client closures. Transactional PXE networking
and systemd-owned listeners are also implemented. The public lifecycle now
performs confirmed start, idempotent stop, explicit recovery, and typed state
reconciliation through both CLI and the first task-oriented TUI screen. Local
guided client enrollment now consumes immutable inventory and enforces reviewed
disk installation; deployment, updates, and richer recovery remain incremental
work tracked externally.

Important constraints in the current implementation are:

- the controller DHCP address is embedded in the netboot closure and generated
  iPXE script, so a lease change requires targeted artifact rebuilding;
- PXE listeners are owned by a controller-only systemd service; preparation is
  a fixed administrator-owned systemd action and Harmonia is controller-only
  and systemd-owned, while the old foreground helper remains an advanced
  compatibility app;
- the internal PXE network transition now has persistent session state and
  boot/explicit recovery and is ordered before the listener; CLI and TUI call
  the same confirmed start/stop/recover application operations;
- the client installer shows hardware, constrains host selection to embedded
  inventory, revalidates and exactly confirms the target disk, and reports
  progress/result; duplicate probing is intentionally best-effort and does not
  coordinate simultaneous installers;
- the controller bootstrap installs an evaluable placeholder deployment, but
  there is no first-run application after reboot;
- the firewall admits product ports only on the configured laboratory
  interface, with controller-only rules for Harmonia and PXE;

## Architectural goals and invariants

The management system has three layers:

```text
terminal UI and human-readable CLI rendering
                    |
typed application/domain operations
                    |
Nix, Git, systemd, networking, Colmena, and filesystem adapters
```

The TUI never owns operational logic and never scrapes the human-readable CLI.
CLI commands and the TUI call the same Go application services. Domain results
are typed values that may be rendered as terminal text or stable versioned
JSON. Long-running operations emit structured progress events.

The following invariants apply to every milestone:

1. The private Git deployment remains the declarative source of truth.
2. Runtime observations come from the operating system, not only marker files.
3. Public code never absorbs site identities, keys, network values, or policy.
4. Client installation and normal deployment require no client internet.
5. Destructive and disruptive actions require an explicit preview and
   confirmation; read-only commands do not require root.
6. Commands are executed with argument arrays, never by interpolating input
   into a shell program.
7. Operations are idempotent or report precisely why a repeated invocation is
   unsafe. Interrupted operations are reconciled from actual state.
8. Plaintext passwords and private keys never enter Git, Nix expressions,
   store paths, logs, process arguments, or world-readable temporary files.

## Management application

The application is a Go executable named `nixorium`. Go provides a small
deployable binary, explicit process execution, straightforward unit testing,
and good Nix packaging. Bubble Tea is used only in the presentation package.
The domain and adapter packages have no Bubble Tea dependency.

The initial package layout is:

```text
cmd/nixorium/           command parsing and renderer selection
internal/domain/        statuses, findings, plans, state transitions
internal/app/           use cases and orchestration interfaces
internal/adapters/      exec, filesystem, Git, Nix, systemd, network adapters
internal/presentation/  text, JSON, and Bubble Tea renderers
modules/management.nix  executable, services, policy, and first-run discovery
```

The first increment installs the executable only on the controller. Repository
discovery prefers an explicit `--repo`, then `NIXORIUM_REPO`, the current
directory when it contains `flake.nix`, and finally
`~/nixorium-deployment`.

Adapters receive validated values such as a deployment root, host selector, or
service name. They construct fixed executable/argument arrays and return
captured structured results. Domain tests use fakes; integration tests exercise
the real adapters in isolated repositories or NixOS VMs.

Every non-interactive command supports `--json`. JSON responses have a top
level schema version, operation, status, findings or data, and optional next
actions. Exit status is zero for a successfully completed operation, including
a status report containing warnings; it is nonzero for invalid invocation,
failed operation, or a doctor report containing errors. Exact exit semantics
are documented with each command before stabilization.

## Command model

The compact command surface is:

```text
nixorium                    open the TUI
nixorium setup              resume first-run setup
nixorium status             summarize controller and deployment state
nixorium doctor             run actionable diagnostics
nixorium config             review or change managed settings
nixorium hosts              inspect configured machines
nixorium deploy             build and deploy selected machines
nixorium pxe                prepare, start, inspect, stop, or recover PXE mode
nixorium services           inspect relevant controller services
nixorium update             prepare a reviewable upstream release update
```

Advanced output names and raw tool commands remain documented and usable. The
management layer wraps rather than replaces Nix and Colmena.

`status` is deliberately cheap and read-only. It reports configuration
readiness, Git state, controller identity, configured clients, Harmonia/PXE
unit state, PXE recovery need, and artifact presence. Network probes and builds
are opt-in or belong to `doctor`, so opening the dashboard is predictable.

`hosts` is the explicit observed client-inventory operation. The application
layer preserves configured ordering and emits typed reachability and SSH
availability rather than a boolean that conflates connection refusal, timeout,
and probe failure. The local adapter limits concurrent TCP/22 probes to eight.
Both the CLI/JSON frontend and the TUI **Computers** screen consume the same
report; `doctor` uses the same classification for its aggregate SSH finding.

The first deployment slice is also read-only: `deploy plan --on` accepts one
client, a comma-separated set, or `@lab`, then resolves only configured client
identities into a canonical Colmena selector. A plan is ready only when
`deploymentStatus` is ready, Git is clean, and HEAD is available; it records
that revision and requires build-before-deploy. Unknown, empty, or duplicate
targets fail closed. Execution, progress/log streaming, and confirmation bound
to this plan are later slices; planning never invokes Colmena.

`doctor` returns ordered findings with `OK`, `WARNING`, or `ERROR`, a stable
finding identifier, evidence safe to display, and a remediation. Expensive
checks are grouped behind `doctor --full`; its first such check performs a real
controller build. Automatic remediation is a
separate confirmed operation, not a side effect of diagnosis.

## TUI information architecture

The default screen prioritizes tasks rather than implementation names:

```text
Nixorium

Laboratory
  Configuration        ready / action required
  Controller services  healthy / degraded
  Computers            reachable / configured
  Installation mode    stopped / preparing / active / recovery required
  Deployment            current / changes pending / unknown

Actions
  Finish laboratory setup
  Install computers over network
  View computers
  Deploy configuration
  Diagnose a problem
  Change settings
  Update Nixorium
  Advanced services and logs
```

Each action has a review screen before mutation. Long operations show the
current stage, elapsed time, recent events, and a route to detailed logs.
Failures state what failed, what was left intact, whether retry is safe, and
the next action. ASCII text conveys critical state; color and Unicode are
enhancements only. The layout targets ordinary 80-column terminals and SSH.

The implemented installation-mode and computer-inventory screens are the first
operational slices of this structure. Presentation callbacks invoke typed PXE
lifecycle and host-inspection services; the TUI itself contains no command
execution, systemd policy, network mutation, or CLI-output parsing. It renders
reconciled state, runs host probes only when the inventory is opened/refreshed,
shows the exact address transition before PXE start, requires `START PXE`, and
leaves long-lived services under systemd when the view exits.

## Configuration ownership and editing

New deployments will opt into a deterministic `lab-settings.json` file:

```json
{
  "schemaVersion": 1,
  "lab": {
    "masterDhcpIp": "192.0.2.10",
    "networkBase": "10.0.0.0",
    "networkPrefixLength": 24
  }
}
```

The complete `lab` object contains the same typed fields currently accepted by
`lib.mkLab`. Nix reads it with `builtins.fromJSON`; the existing Nix module
schema remains the final authority and rejects unknown values. A small
management schema validates input before writing and is kept in conformance
with Nix evaluation tests. Deterministic pretty-printed JSON provides stable
Git diffs and is the only file the management application edits.

The application first evaluates a complete candidate through the deployment
Flake's candidate hook, including controller configuration instantiation. It
then shows an ordered semantic diff whose password values are always redacted.
Acceptance carries the reviewed SHA-256 fingerprint of the source settings.
The writer locks the deployment directory, compares the source at the start
and immediately before commit, writes a sibling mode-`0600` temporary file,
fsyncs it, and atomically renames it after rejecting symlinks. Existing
unrelated changes are preserved; stale or concurrent managed-file edits become
explicit conflicts.

Plaintext passwords are read without terminal echo, sent to a local hashing
process over standard input, retained in memory only as long as needed, and
cleared where practical. Only salted SHA-512 password hashes enter the private
configuration. Private Harmonia, SSH, and Veyon keys stay outside the Git
worktree in root- or user-owned locations. Their public counterparts remain in
the deployment and may be committed.

The credential backend implements this boundary with terminal-only confirmed
input, explicit rejection of short/default values, best-effort slice wiping,
and `mkpasswd -m sha-512 --stdin`. It remains an application service until the
candidate-review workflow can apply the resulting hash without an unreviewed
configuration write.

Existing deployments that import `lab-config.nix` continue to work unchanged.
The management application treats arbitrary Nix configuration as read-only and
offers an explicit migration that evaluates current `labMeta` plus the typed
configuration export, writes `lab-settings.json`, and shows the required small
Flake diff. It never rewrites arbitrary Nix. Migration is accepted only after
old and new `labMeta`, `deploymentStatus`, and representative derivations are
equivalent.

## First-run state machine

Setup is a resumable reconciliation, not a linear script or a single
`configured` flag. Its stages are:

```text
inspect environment
  -> collect network settings
  -> collect lab identity and locale
  -> collect and hash credentials
  -> reconcile key material
  -> validate candidate configuration
  -> review and accept Git changes
  -> apply controller configuration
  -> prepare installation artifacts
  -> verify readiness
  -> offer first client installation
```

The private deployment records only non-secret intent and completed review
decisions. Runtime completion is inferred from configuration, public/private
key correspondence, system generations, service state, and artifact metadata.
On restart, setup re-runs safe inspections and selects the earliest unmet
stage. Going backward changes draft values without undoing applied operations.

The implemented setup slices expose read-only reconciliation through
`nixorium setup status`, guided configuration through `nixorium setup`, and
explicit key reconciliation through `nixorium setup keys`. The wizard proposes
detected network values, retains entries across backward navigation, collects
default credentials without echo, and uses the same candidate-plan/apply
backend as automation. After acceptance, bare `setup` continues into
idempotent key reconciliation; `setup configure` limits the run to settings.
Status derives stage state from the managed settings,
required commands, password-hash readiness, verified key correspondence and
private modes, clean Git review state, Nix evaluation, artifacts, and
`deploymentStatus`; it never advances a stage by writing a global completion
flag.

Key creation uses create-new semantics. Existing keys are verified and reused;
they are never overwritten. Regeneration is a separately named recovery action
that describes affected clients and requires confirmation. Controller rebuild
and artifact preparation are restartable because output paths are content
addressed; their logs are recorded by systemd or the operation event stream.

First-run discoverability is provided by a controller-only desktop entry and a
GNOME autostart notification/launcher conditioned on incomplete readiness. It
runs as the logged-in administrator and does not use shell profile hooks or
automatic root execution. The exact desktop mechanism is verified in a NixOS
VM before it becomes the default.

## Privilege model

The TUI and most of the CLI run as the administrator account. They may read
public Flake outputs, inspect Git, edit the administrator-owned deployment,
build in the Nix store, probe hosts, and invoke Colmena as that account.

Root operations are exposed as fixed controller services/actions:

- install or verify private key material at fixed destinations;
- apply the controller NixOS configuration;
- start, stop, and recover PXE networking and services;
- inspect narrowly selected system units and journals.

Systemd owns the long-running processes and root-only state. Polkit grants the
administrator group access only to named Nixorium actions. There is no generic
root command executor and no user-provided executable path or shell fragment.
The deployment path used by privileged actions is configured declaratively on
the controller and validated as a local, administrator-owned Git worktree.

The administrator is already a wheel user, but the narrow interface still
reduces accidental misuse and makes every disruptive operation auditable.
Deployment to clients continues through SSH/Colmena with the existing keys and
host-key policy. A future enrollment API, if justified, is separate from this
local privilege interface and must have its own authentication design.

The first implemented privileged action is
`nixorium-install-secrets.service`, reached only through `nixorium setup
install-secrets` (or bare guided setup). It reads the declaratively fixed,
administrator-owned deployment, re-verifies all three pairs, and copies them
only to fixed SSH, Veyon, and Harmonia destinations. Its polkit rule permits
wheel members to start that exact unit only. The systemd sandbox makes the
deployment read-only and exposes write access only to the three pre-created
destination directories; differing existing keys and symlinks are fatal.

The second privileged action is `nixorium-apply-controller.service`, reached
through `nixorium setup apply`. The CLI first requires every pre-apply setup
stage to be observably complete and obtains exact interactive confirmation (or
an explicit automation-only `--yes`). The service accepts no path, target, or
command parameters: it uses only the declarative deployment path and the
controller identity evaluated from `labMeta`.

The service rejects dirty Git worktrees, invalid or unready configurations,
key mismatches, and installed-secret drift. Evaluation and the build run as
the administrator through a `git+file` Flake URL, deliberately excluding
ignored private keys from the Nix source/store. Root receives only the single
resulting store closure and executes its `switch-to-configuration switch`.
Systemd/journald retain preflight, build, and activation output across TUI or
terminal exits. Setup completion is reconciled by comparing the evaluated
controller to `/run/current-system`, not by setting a flag.

## Managed services

The controller architecture defines:

- `nixorium-harmonia.service` (implemented as an alias of the native
  `harmonia.service`), a persistent service with the installed non-store key
  delivered through `LoadCredential`, socket activation, restart/watchdog
  behavior, journald logs, and status/HTTP readiness checks. Harmonia accepts
  one bind address, while the firewall limits its port to the configured
  laboratory interface on the controller;
- `nixorium-pxe.service`, an on-demand service for ProxyDHCP/TFTP/HTTP whose
  runtime directory and generated configuration are owned by systemd. It
  validates the prepared revision and active network session before binding,
  reports readiness only after an HTTP health probe, and runs HTTP and dnsmasq
  under separate unprivileged identities after the narrow bind setup;
- `nixorium-pxe-network.service`, a root oneshot that applies and reverts the
  temporary address transition idempotently;
- preparation/apply jobs as transient or oneshot units so their logs survive a
  TUI exit.

The preparation job is implemented as `nixorium-prepare-pxe.service`. It runs
without root capabilities as the deployment owner, requires a clean and ready
Git source plus the configured live DHCP address and healthy Harmonia service,
builds all netboot outputs and client closures, and atomically records schema
version 1 under `/var/lib/nixorium/prepared/prepared.json`. The record binds
fixed artifact names and canonical store roots to the full deployment revision.
Revision-scoped indirect GC roots retain those closures; older roots are
removed only after the new manifest has been durably published.
Status/setup reconciliation validates it against current Git, `labMeta`, store
availability, and client ordering. Privileged consumers revalidate the
administrator-owned record rather than treating it as authority.

The internal network unit is also implemented. Before removing the exact
declarative static CIDR, it requires a strictly valid prepared revision whose
network values match the active configuration, the configured live DHCP
address, and the expected static address. It durably
writes a root-owned mode-0600 session containing the original observed
addresses and prepared artifacts, then removes only that static CIDR. Stop and
`nixorium-pxe-recover.service` restore only the recorded address, preserve
unrelated addresses, verify live state, and archive the result. Malformed or
unowned session records fail closed. The listener is ordered after both this
boundary and Harmonia. Public start repeats the unprivileged preflight after
confirmation, starts only the listener unit, and verifies both service and
address state. A failure triggers synchronous listener/network cleanup. Public
stop and recover use only fixed unit/action pairs and verify the restored
static address before success.

PXE ordering requires the cache, prepared artifacts, and network transition
before starting the proxy. Stopping PXE stops network services first and then
restores normal addressing. Firewall openings are scoped to the configured
interface and by host role. The NixOS firewall policy is static, but stopped
on-demand PXE services leave no listening endpoint. The management application
never keeps infrastructure alive by remaining open.

## PXE lifecycle and recovery

The lifecycle is:

```text
stopped -> inspecting -> preparing -> ready -> starting -> active
   ^                                               |
   +--------- stopping <- active/degraded <--------+
                    |
                 recovering
```

Preparation is non-disruptive. The implemented action verifies deployment
readiness, interface and DHCP address, refuses a changed lease until the
managed setting is reviewed and committed, builds necessary client closures
and netboot artifacts, obtains the locked iPXE binary, and verifies cache
reachability. Content-addressed build results are recorded by store path and
configuration revision, not merely by the existence of `result-*` symlinks.

Starting creates a root-owned session record under `/var/lib/nixorium/pxe/`
containing a schema version, original observed addresses, desired transition,
artifact store paths, and timestamps. The network unit then reconciles the
actual interface, followed by PXE service startup. A session is `active` only
when both actual unit/network state and health checks agree.

Stopping is idempotent. It stops listeners, restores the declarative static
address if absent, verifies the resulting interface, and archives a concise
operation result. Ctrl-C closes only the interactive view unless the operator
explicitly chooses to stop installation mode. On controller reboot, normal
declarative networking returns; a boot-time recovery unit detects an unfinished
session, verifies reality, performs any missing cleanup, and marks it recovered.
On the next invocation, stale records are never trusted over actual addresses,
listeners, processes, and systemd unit state.

The first implementation does not add a controller/client protocol. Passive
information from dnsmasq journald and neighbor state may be displayed as
untrusted observations. Reliable host reservation or duplicate-assignment
prevention requires an authenticated enrollment protocol and is deferred until
authenticated client identity can be established without embedding a reusable
secret in the public netboot closure.

## Client enrollment

The client-side application reads the configured host list from the versioned
`labMeta` document embedded in the immutable installer bundle, shows firmware,
system, CPU, memory, NIC/MAC, disks, and the exact target disk, and lets the
operator choose a host identity. It requires an unmistakable final
confirmation containing both hostname and the canonical disk path before Disko
runs, revalidates that target immediately before mutation, reports explicit
progress and outcome, and treats reboot as a separate confirmed action. An
unattended mode is disabled by default and requires an explicit deployment
policy plus invocation token.

The bundle contains a precompiled Disko no-dependency script generated from the
shared layout and parameterized only by the validated device basename. The
netboot system contains the exact runtime tools derived from that layout, so
disk setup does not evaluate, fetch, or compile tooling in the client ramdisk.
The selected client's prepared system path and closure size are also resolved
offline before disk selection. The installer refuses a target smaller than the
closure plus 2 GiB, then installs that exact path with substitution fallback
disabled.

Without a trusted controller protocol, the installer probes the chosen static
address where routing permits and refuses a responding identity, but it cannot
claim that a silent address is reserved or free. A later protocol must bind to
the installation network, authenticate the controller, prevent unauthenticated
clients from reserving arbitrary identities indefinitely, and retain a manual
recovery path.

## Git workflow and operation records

Configuration changes are prepared in the existing working tree. The
application refuses to conflate its generated patch with overlapping existing
edits, shows the diff, and can discard only its own staged draft before
acceptance. A commit is optional and requires explicit confirmation; push is
never implicit and no remote is required.

System operations log structured key/value events to journald with an operation
identifier. A small root-owned state directory contains only active/recovery
state that journald cannot provide. It is not an alternative configuration
database. Logs redact credential input, private paths where useful, key
contents, environment secrets, and command output known to contain secrets.

## Testing strategy

Testing is layered:

- unit tests cover validation, command plans, status aggregation, state
  transitions, migration, redaction, host selection, and cleanup decisions;
- adapter tests use temporary Git repositories and fake executables with
  recorded argument arrays and controlled output;
- Nix evaluation tests cover package/module exports, strict configuration,
  public metadata schemas, template generation, and unchanged legacy inputs;
- NixOS VM tests cover first-run discovery, systemd ordering, Harmonia health,
  PXE start/stop/recovery, permission boundaries, CLI status, and a real PTY
  traversal of the TUI's reviewed PXE start flow;
- offline equivalence continues comparing the direct and bundled client
  derivations;
- a QEMU PXE scenario is added when deterministic ProxyDHCP behavior can be
  isolated;
- physical validation separately covers firmware varieties, NICs, disk wipe,
  DHCP coexistence, power loss, and multi-client deployment.

Every implementation increment runs the smallest relevant tests plus the
repository validation matrix required by `skills/nixorium-developer`.
Evaluation alone is not evidence that affected packages or host roles build.

## Incremental delivery

1. Package the Go command with typed read-only `status` and `doctor`, JSON
   output, adapters, tests, and legacy deployment compatibility.
2. Add structured settings, migration, setup state reconciliation, secure
   credential hashing and idempotent key handling.
3. Add controller services, PXE preparation, transactional networking, and
   recovery tests.
4. Replace numeric client setup with guided enrollment and explicit destructive
   review; decide whether an authenticated controller protocol is justified.
5. Add host inventory, deployment, service/log, and Git review workflows.
6. Add guided upstream update and richer recovery.
7. Complete administrator documentation, VM/PXE automation, migration testing,
   and physical-lab validation.

Each step preserves the public `mkLab` outputs. Advanced compatibility helpers
may remain exported after the normal documented workflow moves to managed
operations.

## Risks and open questions

- DHCP leases embedded in netboot artifacts can make a prepared session stale;
  preparation must compare observed and configured addresses every time.
- Address changes can interrupt controller connectivity; systemd cleanup and
  reboot reconciliation need VM and hardware testing before the workflow is
  called safe.
- A single configured interface can carry both laboratory and institutional
  traffic. Public defaults scope services to that interface but cannot safely
  guess a narrower institutional DHCP source range; physical deployments must
  review whether downstream source-specific overrides are appropriate.
- Git cannot safely include private keys or plaintext credentials; key and
  password flows need adversarial tests for files, process arguments, logs,
  and Nix store references.
- Go module vendoring and Nix packaging must remain reproducible and must not
  add network use to installed management commands.
- Hardware diversity may invalidate assumptions about interface ownership,
  UEFI PXE, disk naming, and `snponly.efi`; physical validation remains distinct
  from VM validation.
- Reliable duplicate enrollment is unresolved without a controller protocol.
  The first client UX must describe the actual guarantee rather than simulate
  coordination.

The following decisions are recorded separately:

- [ADR-0001: terminal-first management](adr/0001-terminal-first-management.md)
- [ADR-0002: Go and Bubble Tea](adr/0002-go-bubble-tea.md)
- [ADR-0003: structured deployment settings](adr/0003-structured-deployment-settings.md)
- [ADR-0004: narrow privileged actions](adr/0004-narrow-privileged-actions.md)
- [ADR-0005: systemd-owned runtime services](adr/0005-systemd-owned-runtime-services.md)
- [ADR-0006: interface-scoped laboratory firewall](adr/0006-interface-scoped-firewall.md)
