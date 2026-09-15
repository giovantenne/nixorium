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
disk installation. Revision-bound client deployment now has CLI and TUI
plan/apply workflows with mandatory build-first ordering, streamed/private
logs, explicit retry state, and bounded typed log browsing. Updates and richer
recovery are now implemented through bounded release discovery, reviewed
plan/apply, fail-visible two-file recovery, and the shared CLI/TUI operations.
End-to-end documentation and physical validation remain tracked externally.

Important constraints in the current implementation are:

- `masterDhcpIp` is an initial selection hint; managed preparation binds an
  observed unambiguous address to the session and iPXE passes it to the
  installer, while ambiguous multi-address interfaces fail closed;
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
- the controller bootstrap installs an evaluable placeholder deployment and
  the resumable first-run application reconciles configuration after reboot;
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

The frontend uses the aligned v2 Bubble Tea, Bubbles, and Lip Gloss family.
Reusable presentation components query whether the terminal background is
light or dark, add color only as a secondary cue, and render width-aware key
help. Bubbles list/input/progress/viewport primitives may manage interaction
state, but operational state continues to arrive only through typed
application callbacks.

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
nixorium controller         review, rebuild, activate, and verify the controller
nixorium pxe                prepare, start, inspect, stop, or recover PXE mode
nixorium services           inspect services; restart only the signed cache
nixorium logs               list or inspect bounded private operation logs
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
For reachable SSH services, it then uses the existing non-interactive Colmena
root identity to execute one fixed `nixorium-host-state` command, again with at
most eight workers and a per-host timeout. The helper is installed in every
managed generation and returns only the canonical active system path and the
embedded private-deployment revision. The application compares that observed
revision with the repository HEAD and preserves `current`, `outdated`, and
`unknown` as distinct states. It never treats an open port or aggregate Colmena
success as proof of convergence, and an older generation without the helper is
explicitly unknown. Both the CLI/JSON frontend and the TUI **Computers** screen
consume the same report; `doctor` retains the cheaper TCP classification for
its aggregate SSH finding. A separate per-repository, administrator-owned
history records the last post-apply observation that authenticated each host at
the reviewed revision with a concrete system path. Inventory renders that
timestamp and revision, but live observation remains authoritative.

`deploy plan --on` accepts one client, a comma-separated set, or `@lab`, then
resolves only configured client identities into a canonical Colmena selector.
A plan is ready only when `deploymentStatus` is ready, Git is clean, and HEAD
is available; it records that revision and requires build-before-deploy.
Unknown, empty, or duplicate targets fail closed, and planning never invokes
Colmena.

`deploy apply --on ... --expect <revision>` is the corresponding unprivileged
execution boundary. It repeats the plan checks before execution and after the
build, requires exact `DEPLOY <canonical-targets>` confirmation (or explicit
automation-only `--yes`), and constructs fixed argument arrays for verbose
`colmena build` followed by `colmena apply switch`. A per-administrator
non-blocking lock prevents overlapping Nixorium deploys. Output is streamed and
duplicated to a no-follow, mode-0600 operation log under the XDG state
directory. After every apply attempt, the application reuses the bounded
authenticated host-state adapter and atomically updates a mode-0600 deployment
history only for selected hosts whose active revision matches the reviewed
revision. Reports distinguish preflight, build, apply, verify, and complete
phases. Apply success is partial until all targets are verified and recorded;
an apply failure remains failed while still preserving independently verified
hosts. Retrying is convergent: it requires a fresh valid review and rebuilds
before applying again. Direct Colmena remains an advanced compatibility
surface.

`logs` lists at most the newest 50 recognized deployment logs in the current
administrator's XDG state; it does not require a deployment checkout. It also
shows typed safe summaries recorded after important configuration, key, fixed
controller, PXE, cache, and deployment actions. The compact record is locked,
atomically replaced, mode 0600, and explicitly retains only the newest 1000
outcomes; raw report messages are never copied and detailed deployment logs are
never deleted by that retention. `logs show <id>` accepts only the generated
basename grammar and returns at most the final 64 KiB. The adapter opens
directories/files without following symlinks, requires the current UID, exact
0700 directories and 0600 regular files, and neutralizes terminal
control/format characters. Unsafe recognized entries are reported as
unavailable rather than read. CLI text/JSON and the scrollable TUI detail use
the same typed list/show reports. This is a recovery surface, not a database or
a substitute for live host reconciliation.

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

The implemented installation-mode, computer-inventory, deployment, and update
screens follow this structure. Presentation callbacks invoke typed PXE
lifecycle, host-inspection, deployment, and upstream-update services; the TUI
itself contains no command execution, log creation, locking, systemd policy,
network/filesystem mutation, or CLI-output parsing. It renders reconciled state,
runs host probes only when the inventory is opened/refreshed, shows exact
PXE/deployment/update reviews, and requires the operation-specific confirmation
phrase. Long-lived PXE services remain under systemd when the view exits;
foreground Colmena deployment and update apply instead block accidental TUI
exit until their typed final result is available.
Colmena intentionally remains an unprivileged child of the administrator
rather than a system service: it uses that account's SSH authority and streams
direct output into the private operation log. Terminal or process loss may
interrupt it, so recovery is `logs` plus fresh authenticated `hosts` state and
a new revision-bound plan. A user-systemd executor is deferred because reliable
logout survival would also require linger policy and a durable job/result
protocol; it is not introduced merely to move the same process out of view.
The controller screen follows the same boundary: Bubble Tea renders the typed
revision/current-state plan, collects exact `REBUILD <controller>` confirmation,
and invokes the application callback. The systemd-owned rebuild may outlive the
dashboard and retains its build/activation journal.
The services screen likewise receives typed component status and one cache
restart callback. It renders the persistent cache and composite on-demand PXE
lifecycle, requires exact `RESTART CACHE` confirmation, and cannot issue raw
systemd actions. PXE remains linked to its dedicated transactional workflow.
The operation-log screen receives typed bounded list/detail callbacks. Bubble
Tea owns recent-outcome rendering, log selection, and viewport scrolling only;
record construction, persistence, basename validation, no-follow filesystem
access, ownership/mode enforcement, bounds, and terminal-text sanitization stay
in the application/adapter layers.

The dashboard Settings area loads typed managed settings through an application
callback and groups routine edits into Network, Computers, Accounts, Regional,
Browser, Git, and Veyon. Each category reuses the field editor and regional
Bubbles selectors from first-run setup, but validates the complete candidate
through the same Nix-backed plan before showing a redacted semantic review.
Apply is bound to the reviewed source fingerprint and atomically replaces only
`lab-settings.json`; commit, push, rebuild, activation, and deployment remain
explicit later tasks.

Password changes are a separate account selector. Bubble Tea releases the
terminal through its blocking interactive-command boundary, the composition
root runs the existing no-echo confirmed credential collector for exactly one
chosen account, and only the resulting hash returns to the dashboard candidate.
Plaintext never becomes a Bubble Tea message or model field. Recoverable input
errors retry inside that account step; terminal or hashing failures return to
Settings without planning or writing a candidate.

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
and `mkpasswd -m sha-512 --stdin`. Short, public-default, and confirmation
mistakes are typed recoverable input errors: guided setup retries only the
current account and preserves the already collected non-secret candidate.
Terminal I/O, cancellation, and hashing failures remain fatal rather than
looping indefinitely. The password flow stays outside ordinary Bubble Tea text
inputs so immutable Go strings do not extend plaintext lifetime.

Existing deployments that import `lab-config.nix` continue to work unchanged.
The management application treats arbitrary Nix configuration as read-only.
No legacy migration workflow is in product scope because managed deployments
are new; setup optimizes for `lab-settings.json` while preserving upstream
standalone compatibility.

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
Per-host deployment history is observed operational evidence stored outside the
private Git repository, not declarative configuration or a setup stage marker.
On restart, setup re-runs safe inspections and selects the earliest unmet
stage. Going backward changes draft values without undoing applied operations.

The implemented setup slices expose read-only reconciliation through
`nixorium setup status`, guided configuration through `nixorium setup`, and
explicit key reconciliation through `nixorium setup keys`. The wizard proposes
detected network values and groups 15 essential questions into Network,
Laboratory, Accounts, Regional settings, Preferences, and Classroom stages.
Time zone, locale, regional-format locale, desktop keyboard, and console keymap
use offline Bubbles lists with fuzzy filtering, curated common values, and a
validated custom path. Optional Git author identity retains the deployment
template defaults instead of extending first run. The wizard retains entries
across backward navigation, collects default credentials without echo, and
uses the same candidate-plan/apply backend as automation. After acceptance,
bare `setup` continues into
idempotent key reconciliation; `setup configure` limits the run to settings.
Status derives stage state from the managed settings,
required commands, password-hash readiness, verified key correspondence and
private modes, clean Git review state, Nix evaluation, artifacts, and
`deploymentStatus`; it never advances a stage by writing a global completion
flag. The final offer stage becomes complete only when deployment readiness and
the current revision-bound PXE preparation are both observed; its detail routes
the administrator to **Install computers over network** without recording or
implying that any client installation has completed.

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
- restart the binary cache through one fixed action and verify it unprivileged;
- inspect narrowly selected system units and journals.

Systemd owns the long-running processes and root-only state. Polkit grants the
administrator group access only to named Nixorium actions. There is no generic
root command executor and no user-provided executable path or shell fragment.
The deployment path used by privileged actions is configured declaratively on
the controller and validated as a local, administrator-owned Git worktree.

The administrator is already a wheel user, but the narrow interface still
reduces accidental misuse and makes every disruptive operation auditable.
Deployment to clients continues through SSH/Colmena with the existing keys and
host-key policy. Read-only current-generation reconciliation reuses that exact
root trust path and a fixed installed helper; it neither adds a controller
daemon nor grants new client privileges. A future enrollment API, if justified,
is separate from this local privilege interface and must have its own
authentication design.

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
That activation unit permits writes to declared user homes and `/run/user`,
because NixOS activation and user-generation reloads legitimately update both;
`ProtectHome` therefore cannot wrap the switch process. The fixed deployment
path remains mounted explicitly read-only, and the command, target closure,
Git revision, and polkit unit shape remain constrained independently.
Systemd/journald retain preflight, build, and activation output across TUI or
terminal exits. Before switching, the service invalidates any earlier
root-owned activation receipt. Only after `switch-to-configuration` exits zero
and `/run/current-system` resolves to the built closure does it atomically
write `/var/lib/nixorium/controller/applied.json`, bound to the reviewed Git
revision and exact store path. Setup completion requires the evaluated
controller, active symlink, current Git revision, and durable receipt to agree.
This prevents a late activation-script failure from being mistaken for success
when NixOS has already advanced `/run/current-system`.

Routine administration reuses that implementation through `controller plan`
and `controller apply --expect <revision>`. The latter starts only
`nixorium-apply-controller@<40-hex-revision>.service`; the adapter and polkit
policy independently constrain that unit shape. The root service validates the
instance, pins the Git fetcher to the reviewed commit, builds as `admin`, and
rechecks clean HEAD before activating the exact returned closure. The
application then verifies HEAD, `/run/current-system`, and the durable success
receipt. The original
parameterless unit remains for first-run-compatible `setup apply` and now also
pins/rechecks the revision it discovers internally.

Routine cache recovery uses `nixorium-restart-cache.service`. Polkit permits
wheel administrators only to start that fixed capability-free oneshot; it does
not authorize restarting `harmonia.service` or arbitrary units. The action
performs the exact native restart as root, while the application independently
re-observes the product alias and HTTP endpoint before reporting success.

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

`nixorium services` groups those raw units into two operator-facing typed
components: a persistent signed binary cache and the composite on-demand PXE
lifecycle. Inactive PXE units are healthy standby, while inconsistent or failed
listener/network state is degraded. Generic service management exposes only
cache restart; all PXE mutations remain behind prepare/start/stop/recover so
the address transaction cannot be bypassed.

The preparation job is implemented as `nixorium-prepare-pxe.service`. It runs
without root capabilities as the deployment owner, requires a clean and ready
Git source plus a healthy Harmonia service on an unambiguous live controller
address,
builds all netboot outputs and client closures, and atomically records schema
version 1 under `/var/lib/nixorium/prepared/prepared.json`. It prefers the
configured `masterDhcpIp` hint when live, otherwise accepts exactly one usable
non-static, non-link-local IPv4 candidate. The record binds that observed
address, fixed artifact names, and canonical store roots to the full deployment revision.
Revision-scoped indirect GC roots retain those closures; older roots are
removed only after the new manifest has been durably published.
Status/setup reconciliation validates it against current Git, `labMeta`, store
availability, and client ordering. Privileged consumers revalidate the
administrator-owned record rather than treating it as authority.

The internal network unit is also implemented. Before removing the exact
declarative static CIDR, it requires a strictly valid prepared revision whose
network values match the active configuration, the prepared live controller
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
readiness and interface addresses, selects the configured DHCP hint or exactly
one usable non-static, non-link-local candidate, builds necessary client
closures and netboot artifacts, obtains the locked iPXE binary, and verifies cache reachability on
the selected address. The listener generates the runtime iPXE script with that
address in both its HTTP URL and a strictly parsed kernel parameter. The
installer uses it explicitly for signed cache downloads; the netboot system
therefore embeds no address-dependent substituter. Content-addressed build
results are recorded by store path and configuration revision, not merely by
the existence of `result-*` symlinks.

Because preparation can take several minutes, CLI text and JSON modes emit an
immediate activity line on stderr with the fixed journal follow command. The
TUI shows the same activity and detailed-log path while the systemd-owned job
runs. Final reports remain typed and bounded; verbose build output stays in
journald and the job is not moved into presentation.

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
edits. The read-only review adapter consumes NUL-delimited porcelain status,
classifies index/worktree/untracked and Nixorium-managed/unexpected paths, and
captures separate staged and unstaged patches with external diff and textconv
drivers disabled. Each command result and path count is bounded. Untracked
contents are not opened automatically, settings password hashes are redacted,
terminal controls are neutralized, and the presence of a known private-key
path stops patch capture and blocks the report. Private paths are independently
excluded from every diff, and status plus HEAD are rechecked so a concurrent
change discards the review. CLI/JSON and the TUI render the same typed report
and cannot stage, discard, commit, or push.

The optional commit workflow takes only clean repository-relative regular-file
paths that were present in a fresh review. It rejects directories, symbolic
links (including traversal through linked parents), and rename/copy changes
whose two identities cannot be expressed by the selected review row. Planning
rejects conflicts, private paths,
unknown or unchanged selections, invalid managed settings, active Git filters/
encoding/ident transforms, oversized patches, and recognizable private-key,
access-token, or plaintext-secret additions. It creates an isolated temporary
index from HEAD, stages only the allowlist there, and returns the exact proposed
tree, redacted diff, generated message, content-bound token, and confirmation.

Apply repeats the plan and compares the token, then recreates and verifies the
tree. Git plumbing creates a commit object from that tree and atomically updates
HEAD only if the reviewed parent is still current. Only selected entries in the
real index are reconciled to the new HEAD, preserving unrelated staged,
unstaged, and untracked state. Repository hooks, commit signing, remotes, and
push do not run. A partial result explicitly reports the rare case where HEAD
advanced but index reconciliation failed; retry is then unsafe until inspected.
A commit remains optional, requires exact confirmation, and needs no remote.

## Guided upstream update

The normal upgrade surface is a separate typed workflow:

```text
nixorium update check
nixorium update plan --target vMAJOR.MINOR.PATCH[-PRERELEASE]
nixorium update apply --target vMAJOR.MINOR.PATCH[-PRERELEASE] --expect TOKEN
```

`check` is the only release-discovery operation and may contact the configured
public upstream when the controller has internet access. It is bounded,
non-interactive, disables Git credential prompting/helpers, and reports stable
and prerelease tags separately. The concrete adapter derives one HTTPS GitHub
URL from the managed input identity, ignores user/system Git configuration,
stops after 15 seconds and 256 KiB, and returns at most the newest 20 tags per
channel with an explicit truncation flag. An explicit target remains usable
without discovery. Dashboard/status/doctor and all client operations remain
independent of this external request.

Planning requires a clean private deployment at a stable full Git revision and
reads exactly one simple `inputs.nixorium.url` string assignment. The command
preserves the configured upstream identity and accepts only a `v`-prefixed
Semantic Version tag as the new reference; it never accepts an arbitrary source
URL from the command line. Moving references such as `master` are reported as
unpinned. Selecting a prerelease requires an explicit prerelease opt-in, and a
known semantic-version downgrade requires a distinct downgrade opt-in and
stronger confirmation. Complex computed input declarations remain a documented
manual-operation case instead of being rewritten heuristically.

The plan renders candidate `flake.nix` bytes in memory and asks Nix to write a
candidate lock outside the checkout with `flake lock --override-input nixorium
... --output-lock-file ...`. It validates that other top-level inputs remain
owned by the deployment while allowing the selected upstream's transitive lock
closure to change. All subsequent evaluation and builds use the explicit
override plus `--reference-lock-file` and `--no-write-lock-file`; they never
write the deployment lock during review.

Before a plan becomes ready, evaluate `labMeta` and `deploymentStatus`, then
build one configured client, the controller, netboot ramdisk, PXE firmware, and
offline installer bundle with no result links. This work may download/build on
the controller and populate the shared Nix store, but it does not deploy or
introduce client internet access. The report contains current reference and
revision, target/channel, bounded redacted `flake.nix`/`flake.lock` patch,
validation results, a token bound to HEAD plus both original/candidate file
digests, and exact confirmation.

Apply takes the same target and opt-ins, repeats the complete plan so cached
builds are normally reused, and requires the exact token and confirmation. It
then takes the same deployment-root lock used by managed configuration writes,
rechecks HEAD, clean Git state, and both original file digests, writes sibling
fsynced temporary files, replaces `flake.lock` before `flake.nix`, and fsyncs
the directory. In-process failure attempts to restore both originals; a crash
between the two renames is detectable as a source/lock mismatch and recoverable
by planning the same target again. Such an outcome is reported partial and is
not blindly retry-safe.

The successful result is deliberately an uncommitted, reviewable change to only
`flake.nix` and `flake.lock`. The existing Git review/commit workflow can record
it; deployment remains a separate explicit operation. Update never creates or
switches branches, commits, merges, pushes, activates the controller, prepares
PXE artifacts, or deploys clients. The TUI reuses these typed operations and
does not own Nix, network, filesystem, or Git mutation logic. Its **Update
Nixorium** task collects the explicit target and separate prerelease/downgrade
opt-ins, renders all typed candidate checks plus a bounded scrollable patch,
requires the exact plan confirmation, and prevents exit only during the short
two-file apply callback. Candidate planning remains safe to cancel.

Privileged/systemd operations retain detailed output in journald. Foreground
deployments stream output to private mode-0600 files under the administrator's
XDG state, alongside a separate mode-0600 authenticated per-host success
history. Important user-facing action reports are reduced in the application
layer to fixed typed fields—operation, state, bounded subject, and a generated
summary—and appended under a separate lock by atomically replacing a strict
mode-0600 newest-1000 record. Persistence failure is surfaced without changing
the already-observed operation outcome. Bounded typed list/detail operations
make records and deployment logs available to CLI/JSON and the TUI without a
database or arbitrary path reads. A small root-owned state directory contains
only active/recovery state that journald cannot provide; it is not an
alternative configuration database. Logs must never include credential input,
key contents, environment secrets, or command output known to contain secrets.

## Testing strategy

Testing is layered:

- unit tests cover validation, command plans, status aggregation, state
  transitions, redaction, host selection, bounded log reads, and cleanup decisions;
- adapter tests use temporary Git repositories and fake executables with
  recorded argument arrays and controlled output;
- Nix evaluation tests cover package/module exports, strict configuration,
  public metadata schemas, template generation, and unchanged legacy inputs;
- NixOS VM tests cover first-run discovery, systemd ordering, Harmonia health,
  PXE start/stop/recovery, permission boundaries, CLI status, fake-Colmena
  success/failure/retry, and real PTY traversals of the TUI's reviewed PXE and
  deployment flows;
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
2. Add structured settings, setup state reconciliation, secure
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

- A multi-address controller interface can make runtime PXE address selection
  ambiguous; preparation prefers the configured hint and otherwise fails
  closed unless exactly one usable non-static, non-link-local IPv4 candidate
  is present.
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
- [ADR-0007: local guided client enrollment](adr/0007-local-guided-client-enrollment.md)
- [ADR-0008: bounded private operation records](adr/0008-bounded-private-operation-records.md)
- [ADR-0009: bounded read-only Git review](adr/0009-bounded-read-only-git-review.md)
- [ADR-0010: reviewed local Git commits](adr/0010-reviewed-local-git-commits.md)
- [ADR-0011: guided upstream updates](adr/0011-guided-upstream-update.md)
- [ADR-0012: preparation-bound PXE controller address](adr/0012-preparation-bound-pxe-address.md)
