# AGENTS.md

For public API, built-in module, installer, CI, template, and release work, use
`skills/nixorium-developer/SKILL.md`. The separate
`skills/nixorium-maintainer/SKILL.md` is dedicated to operating private lab
deployments and is the only skill copied into the site template. Discovery
links for Codex, OpenCode, Claude Code, and Pi are versioned with the repository.

Nixorium manages a multi-PC NixOS lab using Nix Flakes,
Disko, and Colmena. A controller PC deploys to student workstations
over a LAN-only deployment network. Clients do not need internet for
installation or system updates, but may have internet during user sessions.

The repository exports `lib.mkLab` for private per-lab deployment Flakes. The
root `lab-config.nix` keeps the standalone example compatible, while new
private deployments store managed site values in `lab-settings.json` and
guided packages and their explicit scopes in `lab-software.json`. Public
keys, assets and local modules also belong in the private repository generated
from `templates/site`.

## Public language

Write all code comments and public documentation in English, including guides,
examples, and agent instructions shipped with the site template. Preserve proper
names and intentional configuration/test data. Private planning documents are
not public documentation and must not be copied into this repository.

## Project Structure

```
.github/workflows/validate.yml # Go build/tests plus evaluation-only source/template CI
.github/workflows/security.yml # CodeQL plus pinned Go vulnerability/static analysis
.github/workflows/full-validation.yml # Parallel full gate: release, nightly and manual
.github/workflows/release.yml # Metadata check; full validation gates GitHub Release publication
install.sh                  # Public entrypoint for controller bootstrap
flake.nix                  # Public Flake API plus backward-compatible example deployment
flake.lock                 # Pinned inputs (nixpkgs nixos-26.05, Disko, Veyon)
VERSION                    # Canonical Semantic Version
CHANGELOG.md               # Curated release notes
LICENSE                    # MIT license
DISCLAIMER.md              # Operational responsibility and risk boundaries
SECURITY.md                # Private reporting and coordinated disclosure policy
lab-config.nix             # Standalone example configuration for this upstream
disko-uefi.nix             # NixOS wrapper for the shared Disko layout
lib/
  disko-layout.nix         # Shared Disko layout function (device + student user)
  eval-lab-config.nix      # Typed schema and validation for lab-config.nix
  eval-lab-settings.nix    # Strict versioned lab-settings.json envelope
  eval-lab-software.nix    # Strict allowlisted lab-software.json evaluator
  eval-workspace-profile.nix # Internal student preference JSON validator
  mk-lab.nix               # Host, netboot, Colmena, app, and installer output constructor
setup.sh                   # Installer script for PXE-booted client PCs
scripts/remote-client-installer.sh # Fixed live-ISO USB/SSH installation helper
pkgs/
  nixorium.nix             # Go management command package
cmd/nixorium/              # Management CLI entrypoint
cmd/nixorium-remote-worker/ # Controller-owned USB/SSH operation worker
cmd/nixorium-demo/         # Developer-only deterministic website demo exporter
internal/                  # Domain, application, adapter, and presentation layers
modules/
  common.nix               # Composition point and shared system defaults
  firewall.nix             # Interface-scoped SSH, Veyon, cache, and PXE policy
  desktop.nix              # Core GNOME session, locale and regional policy
  power.nix                # Idle and controller sleep policy
  ssh.nix                  # SSH client and server policy
  hardware.nix             # Generic hardware detection (replaces per-host hardware-configuration.nix)
  networking.nix           # Hostname + static IP with shared iface name
  management.nix           # Controller-only management command installation
  pxe.nix                  # Transactional PXE address state and boot recovery
  users.nix                # User accounts (admin + teacher + student, veyon-master group)
  cache.nix                # Controller Harmonia service + client cache trust
  filesystems.nix          # Btrfs subvolume mount declarations
  home-reset.nix           # Student home directory templating + boot-time reset
  veyon.nix                # Veyon service, public key, firewall, base config
scripts/
  release.sh               # Validates, tags, and publishes a release
  install-controller.sh    # Live USB bootstrap installer for controller with disk selection
  run-harmonia.sh          # Advanced standalone Harmonia compatibility helper
  run-pxe-proxy.sh         # ProxyDHCP + TFTP + HTTP netboot server (external DHCP compatible)
  lib/lab-meta.sh          # Shared helper: loads labMeta from the flake for shell scripts
  create-home-template.sh  # Builds clean home directory template
  home-reset.sh            # Boot-time snapshot rotation + home reset
  validate.sh              # Tiered upstream validation entry point
  security-check.sh        # Pinned staticcheck and govulncheck entry point
assets/
  empty-*                  # Neutral fallbacks for optional deployment assets
templates/site/            # Private deployment repository template
  software-catalog.nix     # Deployment-owned suggestions for the software UI
  assets/                  # Site branding, MIME and editor defaults
  modules/                 # Workstation, development and home-profile policy
  scripts/                 # Site screensaver implementation
skills/nixorium-developer/ # Public upstream development and release workflow
skills/nixorium-maintainer/ # Private laboratory maintenance workflow
docs/management-architecture.md # Accepted management-system target design
docs/development-validation.md # Validation pyramid and gate-selection guide
docs/troubleshooting.md         # Task-oriented recovery and backup guide
docs/hardware-validation.md     # Manual VM/physical evidence plan
docs/system-reference.md        # Workstation defaults and mkLab extension reference
docs/adr/                   # Product architecture decision records
```

Generated locally during setup and committed in the private deployment repo:
- `keys/cache-public-key`
- `keys/admin-ssh.pub`
- `keys/veyon-public-key.pem`

## Development and validation

Use the smallest gate appropriate to the change:

```sh
# Edit-test loop: cached Go tests; accepts package paths and go test flags
./scripts/test-go.sh

# Reproducible gate: syntax, shell regressions, guidance, schemas, Go package
./scripts/validate.sh --quick

# Add full mkLab evaluation for Nix API or host-composition changes
./scripts/validate.sh --eval

# Persistent, pinned shell for repeated Go edits
nix --extra-experimental-features 'nix-command flakes' develop --file tests/source-checks.nix go-shell
go test ./...

# Dedicated Go security and static analysis with the locked toolchain
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix security-shell \
  --command ./scripts/security-check.sh
```

Use the affected management or client-installer VM only when changing its
integration boundary. Reserve `--full` for cross-cutting system-build changes
and milestone/release completion, not the edit-test loop. Build one
representative client and the controller when needed; add another client only
for materially different host-specific modules. See
[development validation](docs/development-validation.md) for the complete policy.

The packaged Go check must run `go test ./...`; `subPackages` selects installed
commands and must not restrict tests to `cmd/`.

Validation reuses a persistent evaluation cache, creates no persistent result
roots, and must never garbage-collect the shared Nix store automatically.
Run `git diff --check`; there is no automatic repository-wide formatter.

Lab operation commands belong in the
[administrator guide](templates/site/README.md) and the
[maintainer skill](skills/nixorium-maintainer/SKILL.md), not in the upstream
development workflow. Do not run a deployment merely to validate upstream code.

## Keep agent guidance current

Behavior changes must review the affected instructions in the same change.
Use the [guidance maintenance map](docs/agent-guidance.md) to identify owners,
source contracts, and checks. Keep this file focused on upstream invariants;
keep administrator procedures in the deployment skill and its references.
Do not copy TUI labels or confirmation phrases where referring to the current
review report is sufficient.

Update both maintainer skill copies together. Tests validate documented CLI
examples using the real argument parser, relative instruction links, and skill
discovery/distribution. They do not prove that prose describes runtime effects:
review CLI versus TUI behavior and existing workflow tests explicitly.

## Releases

Releases follow Semantic Versioning. `VERSION`, the release tag (`v<version>`),
and the dated `CHANGELOG.md` section must agree. After the release commit has
been pushed to `master`, run `./scripts/release.sh <version>` to create and push
the annotated tag. The GitHub Actions release workflow validates metadata and
calls the full-validation workflow, which runs `--full` on that exact commit as
parallel `NIXORIUM_FULL_SHARD` jobs (every group of
`tests/validation-groups.nix` plus `systems`), before a dependent job publishes
the GitHub Release from the matching changelog section. The same workflow runs
nightly on `master` and on demand, so failures surface before a tag; ordinary
pushes do not run it. Each job has a 240-minute limit; failure or timeout of
any shard must block publication.

## Architecture Notes

- Public controller bootstrap must resolve the selected branch/tag once to a
  full Git revision. Its template, installer, Disko layout, and initial lock
  must use that revision; keep the human-selected channel in the generated
  `flake.nix` for managed updates. Never restore independently moving bootstrap
  downloads; see ADR 0018.
- New deployment templates own the direct `nixpkgs` input and make Nixorium's
  input follow it. `lib.packageBase` describes a reference source/channel;
  `Update Nixorium` must preserve the complete deployment-owned lock node.
  Legacy deployments remain readable and are never migrated implicitly; see
  ADR 0019 and ADR 0020. `package-base` updates own only root nixpkgs, preserve
  every other lock node, and require explicit unverified-channel acceptance.
  Never bypass actual builds or change `system.stateVersion` automatically.
  Keep docs/updates.md and templates/site/UPDATES.md identical. Runtime/hardware
  certification is separate from build and offline-equivalence evidence.
- `lab.deploymentMode` is optional (`laboratory` by default). Explicit
  `controller` mode requires zero clients, leaves lab networking/cache/remote
  control inactive, and permits local controller activation without lab keys.
  `deploymentStatus.controller` is the controller-specific capability; older
  upstreams fall back to strict fleet readiness. Never use this capability to
  authorize PXE or client deployment. Bootstrap capability version 2 collects
  the keyboard first, applies it, then collects the time zone, controller
  identity, and password hashes; it then reviews an initial deployment-owned
  software profile and includes it completely, without package-level
  exclusions, before Git initialization, Nix evaluation, or disk installation.
  Version 1
  keeps the settings-only sequence. Both defer lab networking. Apply
  the selected console keymap before any later input, fail closed when the live
  input layout cannot be verified, document the official NixOS Minimal ISO as
  the supported bootstrap environment, and apply the same keymap to netboot;
  see ADR 0015.

- `flake.nix` exports `lib.mkLab`; host generation and deployment composition live in `lib/mk-lab.nix`.
- Downstream calls pass `deploymentSelf = self`; extension points are `sharedModules`, `controllerModules`, `clientModules`, `hostModules`, `netbootModules`, `assets`, and `publicKeys`.
- Hosts pc01-pcNN are generated programmatically via `builtins.genList` + `mkHost`/`mkColmenaHost`, with the controller defined separately.
- Hostname, static IP, and effective network interface are centralized in
  `lib/mk-lab.nix`. `ifaceName` is the compatibility fallback;
  `controllerIfaceName`, `clientIfaceName`, and `hostIfaceNames` resolve in
  host/role/fallback order. Reject overrides for unknown hosts. `networkBase`
  is a full IPv4 network address and `networkPrefixLength` its CIDR prefix;
  host numbers are validated offsets. Each PC gets DHCP plus its static address
  on its resolved interface.
- The controller has two relevant IPs: `masterIp` (the static network address plus `masterHostNumber`) used by Colmena and the binary cache for day-to-day deploys, and `masterDhcpIp` (the initial institutional DHCP address/hint) used only during PXE/netboot client installation. `nixorium pxe prepare` prefers that hint when it is live, otherwise accepts exactly one usable non-static, non-link-local IPv4 candidate, and binds the observed address plus immutable store paths to the exact deployment Git revision. Managed iPXE passes that prepared address to the offline installer at boot.
- USB/SSH installation supports the official NixOS 26.05 Minimal ISO on
  `x86_64-linux`, UEFI, and reachable Ethernet or Wi-Fi networking. Bind the
  reviewed live address to the declared interface, not its wired/wireless type.
  Wi-Fi credentials are not imported from the live ISO; arrange installed-system
  connectivity separately without putting secrets in Git or the Nix store.
  PXE compatibility over Wi-Fi depends on hardware, firmware and networking.
  Preserve physical-console
  credential-free host-key observation, physical-console fingerprint
  confirmation and pinning before password use, terminal-only masked secret
  input that consumes printable characters before global shortcuts,
  operation-specific keys, signed-cache-only transfer, boot-medium exclusion,
  exact disk/revision revalidation, and separate reboot/verification. The
  controller worker sandbox must retain `AF_NETLINK` for interface address
  observation. Never replay apply or reboot after uncertain dispatch. Recovery
  may reattach only to the exact host, address, fingerprint, and live boot ID;
  a verified pre-apply reattachment must remain safely cancellable.
  CLI/TUI finalization requires explicit successful live bootstrap, never just
  a non-failure state: artifacts-ready can accompany a connection error. Keep
  that error visible across TUI status refreshes and require fresh physical
  key confirmation before a connection retry with the reserved artifacts.
  A confirmed remote failure with no disk mutation must revoke the live key
  and release the controller reservation automatically, with safe cancellation
  retained as a cleanup fallback; uncertain or post-mutation failures remain
  reconciliation-only.
  Explicit `install usb close` may abandon a never-dispatched session after
  the ISO is lost: persist the close intent before discarding only that
  operation's local credentials and releasing its reservation. Report remote
  key revocation as unconfirmed. Consumed review tokens, any remote receipt,
  uncertain dispatch, and reboot evidence forbid this path; never reuse the
  old authorization or accept a replacement identity for recovery.
- Custom settings flow from `lib/mk-lab.nix` via `specialArgs` (`labSettings`, `labAssets`, `hostName`, `hostIp`) to modules that need them.
- `labSettings` is a plain attribute set containing all configurable values: user names (`teacherUser`, `studentUser`), passwords, SSH key, network settings, locale/timezone, homepage URL, git identity, and more.
- Structured settings changes use `config plan` followed by `config apply --expect <fingerprint>`; the plan must pass the deployment's `nixoriumValidateCandidate` hook and must never expose password hashes in its diff.
- Guided software changes use `software catalog`, `software plan`, and
  `software apply`. Optional deployment-owned profiles use `software presets`
  and a single `software preset plan`/`apply` batch; `nixoriumSoftwarePresets`
  is `null` when absent. Resolve every selected package ID from the pinned package set and
  deployment validators; the curated catalog is suggestions, not an allowlist.
  `shared` applies to the controller and present/future clients; `controller`
  applies only to the controller. Existing `all-clients`, groups and explicit
  client scopes never gain controller effects. `nixoriumSoftware.controller`
  advertises the new capability; absent metadata keeps the legacy UI/scopes.
  Review includes both old and new destinations when changing scope, and the
  token binds that impact. Always compose the existing deployment validation
  with the upstream controller-candidate hook when old/new declarations include
  the controller, including removal. Apply may atomically replace only `lab-software.json`
  after token and fingerprint rechecks. A profile is additive initial input,
  never persistent policy: preserve existing declarations and scopes, permit
  explicit exclusions, reject the whole candidate on any invalid package, bind
  review to the normalized profile catalog and resolved package data, and never
  write the catalog, modules, or lock. CLI apply remains declaration-only; the
  ordinary TUI exposes profile, package-exclusion, scope and aggregated-review
  stages, records the whole batch transparently, and immediately invokes the
  typed controller plan/apply boundary when the reviewed scope affects the
  controller.
  It must not push, prepare PXE, deploy clients, or rewrite packages supplied by
  private modules. A successful client-affecting result may open a fresh ordinary
  deployment review with exact affected identities; it never reuses the software
  token or bypasses deployment planning and confirmation. Reconstructed state
  uses the current Git revision, the controller activation receipt plus active
  closure, and authenticated client observations; history alone is not current
  evidence.
- The private site template owns seven expanded software profiles and starts
  new repositories with Essential at `shared` scope. Keep
  `software-presets.json` and initial `lab-software.json` deterministic and
  equal for that default, validate every profile against the locked package
  set in laboratory and controller-only modes, and serialize the optional
  catalog into the offline installer without changing existing deployments.
  Every template profile includes Git and Node/npm; Pi, OpenCode and VS Code
  remain Programming choices. The controller management module and packaged
  command must both carry Git so interactive administration and Nixorium
  operations never depend on the selected deployment profile. Keep npm globals
  user-owned under `~/.local/npm`.
  Student npm globals and Pi/OpenCode state/credentials are ephemeral and
  excluded from pre-reset snapshots; never seed credentials into the shared
  home template.
- Administrative TUI startup must not evaluate Nix outputs, including labMeta:
  older pins can make even metadata evaluate every host. Observe
  local setup/Git/service state only, without claiming readiness. Load evaluated
  inventory before client selection; cancelled or failed reads must not resume
  actions or admit stale identities. Keep full operation validation unchanged.
- Overview pending work uses local Git/service observations and
  previously computed session evidence only. It lists only items that need
  action (no empty-state line, no last-client-check reminder; Computer
  inventory shows dated observations). Keep local refresh free of Nix/client
  probes, and selectable follow-ups read-only until their ordinary review and
  confirmation. Low store space is shown in
  Maintenance beside Free disk space, not on the Overview. The backup reminder
  starts only once laboratory private keys exist.
- Inventory/package discovery must remain independent of host module evaluation.
  Keep workspace prerequisite guards on readiness, validators, configurations,
  Colmena and individual app/package entries; Nix probes package namespaces even
  when reading top-level metadata. Group a controller preflight into one fresh
  evaluation, never a cached authorization. Repeat live revision, activation
  receipt and system checks before/after apply. Update candidate builds share
  one Nix invocation with the exact candidate lock and complete required output
  set; report success only after the whole group succeeds.
  The workspace hook may reuse an identical decoded declaration's complete
  resolution within that evaluator only; always parse/validate candidate JSON
  and retain downstream hooks and fresh evaluation after source or pin changes.
- TUI screens receive typed application callbacks from `cmd/nixorium`; keep command execution, privilege checks, state reconciliation, and other operational logic out of `internal/presentation`.
- Never leave the administrator or teacher stuck: every refusal names the next
  step, persistent blockers appear on the Overview, and recovery is a reviewed
  command, not a manual file edit. See the "Never leave the operator stuck"
  rules in `skills/nixorium-developer/references/tui-design.md`.
- Deployment recovery composes the existing USB worker callbacks, preserves
  exact selected identities, and always returns to fresh planning and explicit
  deployment confirmation. Never clear reservations or infer verified identity
  from a completion label in presentation code.
- The netboot image starts the guided client installer once on tty1 through
  the autologin shell; it remains interactive (identity, `ERASE`, reboot) and
  is never unattended. Both installers use `ERASE` and ask again after a
  mistyped answer; an empty confirmation cancels.
- Client enrollment is local and guided; consume only the immutable versioned installer inventory, treat reachability as a best-effort duplicate warning rather than a reservation, and keep unattended installation disabled without explicit private policy and a documented token model.
- Client deployment expands only evaluated inventory targets, binds execution to the reviewed clean Git revision, builds before apply, runs unprivileged with fixed Colmena argument arrays, and preserves streamed mode-0600 logs plus honest partial-failure/retry reporting. Bound subprocess groups and inherited output pipes, retain SSH liveness checks, and persist fleet-wide pending evidence before apply. An unsuccessful dispatched apply keeps that evidence across exit/reboot, blocks new operations and requires reviewed recovery; an active revision alone never clears it. Authenticated observations still run after local cancellation, but uncertain activation must not enter successful history. Keep the bounded terminal-safe live tail separate from support exports and safe operation summaries.
- Deployment availability is advisory: probe only selected identities with the
  bounded inventory probe; an open SSH port is not authenticated identity or
  proof of power state. A reachable-only choice creates a fresh explicit-target
  plan and confirmation, never edits or silently shrinks an approved execution.
  Per-computer outcomes must retain uncertainty and recovery requirements;
  never infer individual activation success from an aggregate batch result.
- Client shutdown expands only evaluated client identities and never the controller. The session helper (`scripts/session-state.sh`) is a versioned one-word contract: `idle`, `unused` (local graphical session without input for ten minutes or since login, read from the user's GNOME Shell) or `active`, which it must keep for anything it cannot observe; older clients print only `active`/`idle`. Active sessions remain eligible after an explicit data-loss warning; when any are present, the review must state that the one-word `SHUTDOWN` confirmation authorizes their interruption. Preserve explicit acknowledgement for unknown sessions, expiring content-bound review, PXE/recovery conflict check, immediate inventory/session recheck, and the same non-blocking lock used by deployment. Adapters may issue only the fixed `nixorium-session-state` and `systemctl poweroff --no-block` SSH commands. Report accepted/not-sent/unconfirmed requests without inferring physical power state or retrying unconfirmed dispatches.
- Client restart shares the shutdown review and safety boundary but binds the
  distinct action and `RESTART` confirmation; its adapter may additionally
  issue only `systemctl reboot --no-block`. Never infer completion from network
  loss. The teacher dashboard reaches inventory, Internet and client power only
  through the group-private classroom worker running as `admin`; keep the
  deployment mode 0700, exclude the student, reject repository/address/command
  selection over IPC, and never expose administrative TUI callbacks there.
- Operation history records only typed safe summaries for important outcomes in an atomic mode-0600 newest-1000 store; it never copies raw report messages and never deletes detailed deployment logs. Browsing accepts only generated deployment-log basename IDs, caps discovery at 50 results and detail at a 64 KiB tail, validates owner/mode/type with no-follow opens, and sanitizes terminal controls. Keep persistence/filesystem inspection in adapters and list/detail navigation in presentation.
- Support export must use the separate immutable allowlisted snapshot, never
  serialize detailed reports or error strings. Preview and confirmed local
  export share exact bytes. Preserve unavailable/omitted evidence, no-build
  collection, private no-follow create-new publication and honest durability
  reporting. No upload, automatic remediation, raw logs or unreviewed export.
  The administrator TUI uses typed preview/export callbacks, cancels pending
  collection, ignores stale replies and exports only the displayed snapshot.
  Keep the entire payload scrollable and the feature out of classroom mode.
- Reviewed client host-key rotation uses only the existing managed SSH trust
  store, preserves unrelated entries and an atomic backup, and requires physical
  fingerprint verification after deliberate reinstall. Recheck inventory, offered
  key and recorded content under the fleet gate; refuse controller/ambiguous
  targets, active PXE and pending client work. Never relax SSH checking or rotate
  automatically after a changed-key error.
- The USB worker may write only the dedicated `.ssh/nixorium-known-hosts`
  directory for atomic host-trust updates; the enclosing `.ssh`, keys, and
  configuration remain read-only. Activation preserves existing entries and
  links the standard `known_hosts` path to that directory; a differing regular
  `known_hosts` is kept as a private `known_hosts-legacy-*.bak` copy there
  (never merged) so controller apply cannot get stuck. Never replace
  conflicting migration/backup evidence or remove a reservation to bypass
  failed post-boot verification. Already-private log ancestors need no chmod.
- A completed USB disk installation may await reboot/verification while the
  controller rebuilds. Only the controller apply job may cross that reservation:
  check its strictly decoded, identity-bound completion receipt, pause the
  worker, acquire the shared lock, and recheck before continuing. Resume the
  worker on every exit and retain its private runtime directory across stops.
  Never infer completion from a state label alone, clear the pending record,
  or apply this exception to active/failed/unknown disk work or client operations.
- Git review is read-only and typed: preserve the staged/unstaged/untracked
  distinction, managed-versus-unexpected classification, bounded patch output,
  password-hash redaction, no automatic untracked-file reads, and refusal before
  diff capture when a known private-key path appears. Never enable external diff
  drivers or let presentation mutate the index/worktree.
- Optional Git commit uses an explicit clean relative-path allowlist, isolated
  HEAD-based temporary index, content-bound review token, generated message,
  and exact confirmation. Preserve secret/content-transform checks, atomic
  compare-and-update of HEAD, path-only index reconciliation, unrelated index/
  worktree state, disabled hooks/signing, and the no-remote/no-push boundary.
- Upstream update planning must preserve the configured source identity, accept
  only the exact `master` branch or an explicit SemVer tag, generate the
  candidate lock outside the checkout,
  and validate representative outputs against that exact lock. Explicit
  controller mode requires zero clients, explicit controller readiness, and a
  candidate controller build; it must not require or build client/PXE outputs.
  Laboratory mode and legacy metadata retain strict fleet readiness plus the
  controller/client/netboot/firmware/installer build set. Reject unknown modes
  and inconsistent inventories. The CLI applies only the
  token-bound `flake.nix`/`flake.lock` proposal under the deployment-root lock;
  the TUI records it transparently and then invokes typed controller plan/apply.
  Never imply a branch, push, PXE action, or client deployment.
  Remote enumeration belongs only to explicit `update check`, must use the
  configured public upstream with bounded time/output/results and no Git
  prompting, credential helpers, or user/system Git configuration.
  The TUI must reuse this typed plan/apply boundary, with presentation limited
  to target/policy input, bounded review scrolling, exact confirmation, and
  typed result rendering.
- Routine controller rebuild uses `controller plan`/`controller apply`, binds the privileged systemd instance to a full reviewed Git revision, builds a pinned Git source as the deployment owner, refuses repository drift before activation, and writes a root-owned success record only after both `switch-to-configuration` and `/run/current-system` verification succeed. Reconciliation must require that record to match both the reviewed revision and evaluated closure; the active symlink alone is not completion evidence. Keep `setup apply` as the first-run-compatible path through the same fixed service implementation.
- Both controller-apply systemd units must keep `restartIfChanged = false` so
  their own `switch-to-configuration` cannot terminate the in-flight reviewed
  job when the target changes its `ExecStart` store path. The job must survive
  through active-system verification and receipt creation.
- `labMeta` is a public flake output containing the small set of non-sensitive operational values that tools need (controller IPs, network prefix, iface name, structured client hostname/IP inventory, ports, usernames). `deploymentStatus` separately reports whether placeholders, public default passwords, or public keys still block deployment. `nixoriumSoftware` is the typed non-secret catalog/scope/declaration contract; optional `nixoriumSoftwarePresets` is the versioned deployment-owned profile catalog. Scripts and documentation commands must consume these outputs instead of parsing Nix source files textually.
- `lib/eval-lab-settings.nix` validates the versioned JSON envelope and delegates its `lab` object to `lib/eval-lab-config.nix`, whose private `lib.evalModules` schema remains the final type/semantic authority. Keep its semantic checks aligned with `internal/domain/settings.go` through `tests/lab-settings-validation-cases.json`. No custom NixOS options are added to host configurations.
- Reject controller DHCP addresses inside the static lab prefix in both
  validators; the placeholder remains preparation-only. Field guidance and
  address previews do not replace complete candidate validation.
- `lib/eval-workspace-profile.nix` accepts raw JSON text so duplicate keys are
  rejected before they can disappear in `builtins.fromJSON`. Keep it aligned
  with `internal/domain/workspace.go` through `tests/workspace-validation-cases.json`.
  This internal schema alone does not enable a profile, resolve software or
  change a home; do not describe schema validation as deployment validation.
- `lib/resolve-workspace-profile.nix` is the internal pure catalog/prerequisite
  resolver. Its input inventory must include the controller and every client;
  supplied host package attributes describe declarations, not live installation
  evidence. Preserve explicit dependencies, pinned identity/version checks and
  list-replacement semantics when composing the deployment baseline. It does
  not install software or qualify plugin loading.
- Optional `mkLab.workspaceProfileJSON` (raw JSON text) and `workspaceCatalog`
  expose `nixoriumWorkspace` preparation metadata and a candidate hook.
  `mkLab` checks prerequisites against each generated host's actual declarative
  system packages, including downstream overrides, evaluating one representative
  per class of hosts with the same role, interface and managed software; a host
  with `hostModules` is its own class. Keep this memory bounded independently of
  the inventory size. Serialize both inputs into
  the offline installer. `state = "prepared"` is not activation.
  A supplied profile always configures the managed boot reset; there is no
  public runtime opt-in argument. Internal module selection is derived from
  profile presence. The template ships an active `workspace-profile.json`
  matching Essential without VS Code; leave it unchanged for default behavior.
  Never silently replace a missing/invalid profile with the example. Expose the
  seed path without claiming it is built, deployed or active. Preserve offline
  profile/catalog equivalence and the profile-free standalone composition.
  Bootstrap and template reset copy `workspace-profile.<preset>.example.json`
  to `workspace-profile.json` when the template ships one for the selected
  software profile; only Programming does. Additive software-profile changes
  never rewrite a saved workspace profile. Keep its
  extension list equal to the legacy list in `modules/home-profile.nix`, and
  every extension prerequisite inside the Programming package list.
  `vscode.extraSettings` carries reviewed free-form editor defaults; keep its
  refused keys identical in the Go and Nix decoders through the shared corpus.
  Extension IDs stay lowercase while packaged identities compare without case.
  The extension catalog adds prerequisites but is not an allowlist; any
  extension of the pinned set is selectable and linked. Catalog `writable`
  entries are copied into the seed instead: debug adapters create files in
  their own folder, and a directory of per-file links breaks the editor's
  attribution of running code to its extension. The reset validator accepts
  exactly one link to the payload or a copy with an identical manifest.
  `vscode.marketplace` pins (publisher, name, stable version, SRI hash,
  optional linux-x64) join the package set through
  `lib/workspace-marketplace.nix`; keep the URL derived by the nixpkgs fetcher
  and never accept one in the profile. `workspace marketplace` and the TUI
  `m`/`u` actions are the only gallery clients: fixed endpoint, no redirects,
  bounded answers, prefetch under the fetcher's name so builds stay offline,
  and no deployment write. Every pin must be selected.
  `nixoriumResolveWorkspaceCandidate` returns the same metadata for raw candidate
  JSON without writing it. Review consumers must also compose the deployment's
  `nixoriumValidateWorkspaceCandidate` hook and bind/recheck source, pin and base
  identity; resolved metadata by itself is not save authorization.
- Deployment template reset is a separate administrator workflow, not additive
  software selection or workspace declaration save. It uses the exact locked
  upstream template, preserves settings/lock/keys/ignore rules and all untracked
  files, evaluates protected effective identity, enables the guided home, and
  requires `RESET DEPLOYMENT` before a backup and local commit. No implicit
  update, apply, deploy, reboot or push. Keep interrupted-reset evidence and
  block operational preflight until reviewed recovery. See
  `docs/deployment-template-reset.md` and the template's `DEPLOYMENT-RESET.md`.
- `internal/app/workspace.go` owns the declaration-only workspace review/save
  contract. Bind review to the normalized candidate, complete resolved metadata,
  base-file presence/content/mode, Git source, revision and pin. Always regenerate
  review before saving, including unchanged proposals; stale tokens must not
  become successful no-ops. The adapter must recheck the snapshot under the
  deployment-root lock and replace only `workspace-profile.json`. Report saved,
  conflict and unconfirmed durability distinctly, never inferred activation.
  `workspace plan`/`apply` use this boundary; interactive confirmation or explicit
  `--yes` authorizes only the JSON save. Keep candidate reads bounded and
  non-blocking for special files. The administrative TUI uses typed
  load/plan/save callbacks for its draft editor, with a source-bound review and
  explicit save confirmation. Preserve inherit versus empty values, ordered
  favorites, cancellation and late-message checks. The ordinary TUI save also
  records exactly the just-written profile through WorkspaceSaveManager;
  CLI Apply stays file-only. Refuse pre-existing profile edits, bind the commit
  to the reviewed HEAD and exact candidate blob, preserve unrelated index paths,
  and use the existing no-hooks/no-signing/no-remote commit boundary. A partial
  write/record requires inspection, never token replay or inferred completion.
  Do not apply systems or reset homes from the save callback.
  The initial editor load reads the current declaration,
  resolves it, and rejects concurrent changes. Never load the example implicitly.
  Git review classifies the profile as managed; a separately authorized exact-path
  commit validates its schema without implying Nix validation or activation.
  Update planning compares existing workspace metadata on current/candidate
  pins and preserves local validation hooks. Bind the complete comparison to
  the update token; do not silently change reset behavior, identity or targets.
  Legacy absence stays outside this comparison. Extension versions come from
  the package base, not independent home downloads or vendor-latest promises.
- Save results distinguish local configuration, controller verification at the
  saved revision, and unobserved client state. Settings, workspace and template
  reset offer a separate controller review, never automatic activation; later
  client selection requires fresh inventory, planning and confirmation. Preserve
  existing automatic controller follow-up for Software/Update only. Never infer
  activation from an unchanged declaration or reuse stale success from a prior save.
- `lib/build-workspace-seed.nix` builds the internal preference payload from a
  resolved workspace. Keep its dconf source separate from the compiled user
  database so reset-time wallpaper selection can be composed without losing
  preferences. Check built extension manifests against resolved identities and
  versions. `build-workspace-home.nix` adds a neutral shell/Git/XDG scaffold,
  never writable template content. A built seed is not home activation or
  proof of plugin loading; neither builder changes the legacy payload.
- `internal/homereset` contains a confined removal primitive and an internal
  boot-reset engine. Preserve complete preflight, no-follow traversal,
  mount/subvolume checks, immutable seed validation, account/process checks,
  and a held login barrier. The private recovery snapshot is sanitized before
  publication; five read-only managed snapshots live separately from legacy
  snapshots. Durable pending evidence must block retries after failure and
  reboot. Never delete it automatically or treat a previous success receipt as
  completion of a pending attempt. The fixed root-only `nixorium-home-reset`
  helper accepts no target arguments and reads only its store-backed system
  configuration. The isolated `modules/workspace-reset.nix` service retains
  the `home-reset` unit name, never restarts on switch, and gates both the
  display manager and normal user sessions. Preserve manual-start refusal,
  no automatic retry, and `X-OnlyManualStart` so first-time installation during
  a rebuild also waits for boot. A supplied workspace profile selects the
  module. The profile-free service also avoids rebuild-time restart and must refuse
  managed pending evidence, so disabling the profile cannot bypass recovery.
  Keep student ownership repair/template writes and login preference migration
  out of the managed path, while retaining staff and profile-free content behavior.
- VirtualBox guest additions are enabled by default via `mkDefault` in `common.nix` (harmless on bare metal).
- Hardware detection uses `modules/hardware.nix` with `not-detected.nix` for automatic driver loading. No per-host hardware-configuration.nix files are needed.
- UEFI boot is required on all machines. Disk partitioning uses an EFI System Partition (`/boot`) plus Btrfs subvolumes.
- Netboot uses the systemd-owned `nixorium-pxe.service` with `dnsmasq` in ProxyDHCP mode so institutional DHCP remains authoritative for leases. `scripts/run-pxe-proxy.sh` remains an advanced foreground compatibility helper. `mkLab` builds a standalone installer source containing the effective downstream configuration and only local Flake inputs for offline evaluation.
- `labOverlay` composes Veyon's official overlay with local PipeWire and RGB32 rendering fixes. It is applied in each host's module list and in `colmena.meta.nixpkgs`.
- Docker is rootless for every normal user. Never add users back to the root-equivalent `docker` group; each account has declarative subordinate UID/GID ranges.
- Global npm packages use `~/.local/npm` through `NPM_CONFIG_PREFIX`. Do not install npm tools with `sudo` or into the Nix store.
- Veyon classroom management is configured in `modules/veyon.nix`: runs `veyon-service` in the graphical user session, deploys the public key, generates a `Veyon.conf` with all client PCs pre-mapped, and opens port 11100. Every lab host uses native PipeWire capture. The deprecated `veyonNativeHosts` string list remains accepted and ignored for 2.x compatibility; never use it to select capture behavior or restore the external bridge or shared VNC password. The private key is not managed by Nix (see Security).
- `modules/firewall.nix` enables the firewall everywhere, disables implicit
  all-interface SSH/Avahi openings, and uses nftables. Client TCP 22/11100
  admits only the static controller IPv4 address on the lab interface, with
  a guard before connection tracking preventing IPv6/old-connection bypass.
  Controller services stay interface-scoped; only it opens Harmonia/PXE.
  Preserve independently owned runtime tables on firewall reload.
- The experimental classroom view (`lab.classroomView`, default false) adds
  only a client user-session agent on a private Unix socket. The controller
  reaches it solely through its existing SSH access with the fixed
  `nixorium-classroom-connect` command; never open a network port for it,
  accept addresses or commands from the protocol, or follow links when
  locating the session socket. Keep `internal/classroomview` messages
  versioned, size-bounded and strict about unknown fields. By owner decision
  the agent captures through Mutter's ScreenCast/RemoteDesktop D-Bus
  interfaces without a consent dialog; GNOME's sharing indicator must stay
  visible whenever it captures (the classroom extension only removes the
  stop action), and capture must stop when nobody requests frames. The
  controller side lives in the classroom worker: it serves the page on
  loopback only behind a one-time token from the classroom socket, refuses
  foreign Host headers, keeps a strict CSP with no external resources, takes
  computer names only from the evaluated inventory, and closes agent channels
  when the page stops polling. Remote input is accepted only as bounded,
  validated JSON from the page's own Origin for a computer open in the large
  view, and every path that ends control must release pressed keys and
  buttons. Page actions on selected computers (Internet, power) call the
  worker's own typed plan/apply handling in process: the review token never
  reaches the page, each review is applied at most once, and restart or
  shutdown with active or unknown sessions requires the typed word. The
  teacher's lock is the classroom extension's full-screen cover with a modal
  grab, requested by the agent on the session bus; logout and restart always
  end it. It is a classroom aid, not a security boundary: the session's own
  user can reach the agent and the bus. Lock plan/apply (`LockManager`) has
  an expiring content-bound review like Internet control but does not take
  the administrative operation lock, since it changes no system state.
  Files sent to a desktop (`files.begin/chunk/end`) are validated as a plain
  relative tree within 2000 entries and 500 MB, written by the agent as the
  student into a private staging folder with no-follow exclusive creation and
  explicit 0755/0644 modes, and moved to the XDG desktop folder only when
  complete, never replacing an existing name; an interrupted sending leaves
  nothing behind. The classroom worker prepares files in its private
  temporary folder under position-based names only, hashes each file, and
  binds the share review to paths, sizes and digests rather than to the
  transfer identifier; the page may upload raw pieces only to its own
  `/api/share/chunk` endpoint. The
  "initial consent" rule below applies to Veyon only.
- Temporary Internet control uses typed `internet plan`/`apply` callbacks,
  client-only evaluated identities, an expiring review and the shared fleet
  lock. Recheck authenticated boot ID and state before every fixed helper
  request; verify afterward and report partial/unconfirmed outcomes honestly.
  Never queue offline targets or reuse authorization across reboot. The
  root-only helper controls only `nixorium-internet-block.service` and its
  owned nftables table. Preserve lab IPv4, DHCP, IPv6 neighbor discovery,
  ordinary firewall reloads and reboot-to-enabled semantics. No gateway,
  persistent enablement, arbitrary remote commands or student privileges.
- Native Veyon hosts persist per-user tokens and portal grants under
  `/var/lib/nixorium/veyon-session`; never copy these into templates, snapshots,
  Git, or other machines. GNOME initial consent stays explicit for Veyon. The user-only
  state link is required because Veyon reconstructs server environment from the
  login session; a service-only XDG_STATE_HOME does not propagate.
- `Veyon.conf` is a build-time derivation: evaluation must never read a derivation output to encode its network objects. GitHub CI disables import-from-derivation to enforce this boundary.
- The `veyon-master` group (declared in `modules/veyon.nix`) controls access to the Veyon private key. Users `admin` and the teacher user are members (configured in `modules/users.nix`).
- `modules/common.nix` is only the composition point for core desktop, firewall,
  power, and SSH modules. Packages, shell preferences, development tools,
  screensaver behavior, and application policy belong in the deployment.
- Site desktop favorites and shortcuts live in `templates/site/modules/workstation.nix`;
  student template content lives in `templates/site/modules/home-profile.nix`.
  Keep application entries conditional on effective software scope, not in core
  desktop policy. The site template's Desktop Icons NG, the compact intelligently hiding Dash
  to Dock and Tiling Assistant are baseline workstation behavior, independent
  of the selected profile. Keep Yaru-yellow icon/Adwaita appearance deployment-owned,
  validate extension metadata and GSettings, and preserve the targeted one-time
  migration rather than resetting unrelated user preferences.

## Nix Code Style

### Formatting
- **2-space indentation**, no tabs
- No block comments (`/* ... */`); use single-line `#` comments with a space after `#`
- Place comments on the line above the code they describe
- No trailing commas in lists or attribute sets (Nix does not use them)

### Module Signatures
Use only the arguments the module actually needs:
```nix
{ ... }:            # When no module args are used
{ config, pkgs, lib, ... }:   # When config/pkgs/lib are needed
{ labSettings, ... }:          # When consuming specialArgs
```

### Attribute Sets
- Dot-path notation for one-liners: `boot.loader.grub.enable = true;`
- Nested set notation for grouped settings:
  ```nix
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    pulse.enable = true;
  };
  ```
- Mixing both styles within a file is acceptable and expected.

### Lists
- Short lists on one line: `[ "nix-command" "flakes" ]`
- Long lists with one item per line:
  ```nix
  environment.systemPackages = with pkgs; [
    wget
    curl
    bat
  ];
  ```

### `with` Usage
- Use `with pkgs;` **only** for `environment.systemPackages` (the long package list)
- Everywhere else, use explicit `pkgs.packageName` references
- Always use store-qualified paths for executables: `"${pkgs.bash}/bin/bash"`

### `inherit` Usage
- One binding per `inherit` statement, each on its own line:
  ```nix
  inherit name;
  inherit system;
  ```

### `let...in` Blocks
- Place between the function signature and the attribute set body
- Only use when there are repeated values or complex expressions to extract

### String Handling
- Multi-line strings: `''...''` (Nix indented strings)
- Interpolation: `${...}` inside strings
- Explicit `toString` for int-to-string conversion: `toString n`

### Naming Conventions
| Scope            | Convention       | Examples                                    |
|------------------|------------------|---------------------------------------------|
| Nix variables    | camelCase        | `masterIp`, `mkHost`, `labSettings`         |
| File names       | kebab-case       | `home-reset.nix`, `run-harmonia.sh`         |
| Shell variables  | UPPER_SNAKE_CASE | `PC_NUMBER`, `TEMPLATE_DIR`                 |
| NixOS options    | Standard dotted  | `services.openssh.enable`                   |
| Helper functions | `mk` prefix      | `mkHost`, `mkColmenaHost`                   |

### Imports
- Flake uses root-relative: `./modules/networking.nix`, `./modules/cache.nix`
- Script references from modules: `../scripts/create-home-template.sh`

## Shell Script Style

All shell scripts must follow these conventions:

```bash
#!/usr/bin/env bash
set -euo pipefail
```

- Shebang: always `#!/usr/bin/env bash` (not `/bin/bash`)
- Safety: always `set -euo pipefail` as the first non-comment line
- Variables: `UPPER_SNAKE_CASE`, always double-quoted (`"$VAR"`, `"${VAR}"`)
- Positional args: assign to named variables immediately (`TEMPLATE_DIR="$1"`)
- Errors: print to stderr with `>&2` (`echo "Error: ..." >&2`)
- Cleanup: use `trap '...' EXIT` for temporary file cleanup
- Validation: check argument count and format before proceeding

## Security

- **Never commit** `secret-key` or `admin-ssh` (both in the deployment `.gitignore`)
- **Never commit** `veyon-private-key.pem` (in `.gitignore`). Install it through
  `nixorium setup install-secrets`; the fixed destination is
  `/etc/veyon/keys/private/teacher/key`, mode `0640`, group `veyon-master`.
- `nixorium git review` must refuse known private-key paths before reading any
  patch content; do not weaken this boundary when adding the optional commit
  workflow
- `nixorium git commit apply` must never invoke broad `git add`, normal commit
  hooks, signing helpers, a remote, or push; commit only the exact reviewed tree
  and reconcile only selected index paths
- `nixorium workspace marketplace` (and the TUI Marketplace actions) may
  contact only the fixed public gallery and the pinned download URL, only
  when invoked, and write only to the Nix store
- `nixorium update` may use controller internet only when explicitly invoked;
  it must never accept an arbitrary replacement source URL, expose ignored
  private files to Nix, or introduce client-side network requirements
- `keys/cache-public-key`, `keys/admin-ssh.pub`, and `keys/veyon-public-key.pem` are public and may be committed
- `nixorium setup keys` uses create-new semantics and refuses public-only or mismatched pairs; never bypass that refusal by overwriting an existing key
- `nixorium setup install-secrets` may start only `nixorium-install-secrets.service`; its deployment path is declarative and destinations are fixed
- `nixorium setup apply` requires a clean reviewed Git deployment and may start
  only `nixorium-apply-controller.service`; never add arbitrary target/path
  parameters or evaluate ignored private files through a `path:` Flake URL
- Legacy deployment locks are opened read-only and locked exclusively so
  service home sandboxes remain read-only. Preserve ownership/symlink checks
  and reject active legacy operations; never delete a lock to unblock work.
- `nixorium pxe prepare` may start only the fixed administrator-owned
  `nixorium-prepare-pxe.service`; keep its clean-Git, live-DHCP, healthy-cache,
  canonical-store-path, and managed-GC-root checks intact. Evaluate all shared
  artifacts and all configured client derivations together, then build the
  resolved derivation paths without repeating Flake evaluation. Bind validation
  and builds to the same Git revision, reject repository drift before publishing,
  and keep temporary result roots until permanent roots are installed
- `nixorium-pxe-network.service` is an internal root boundary with only
  `CAP_NET_ADMIN`; preserve its root-owned session-before-mutation ordering,
  exact static-address restoration, and boot-time recovery semantics. The
  automatic recovery unit must be conditioned on the durable PXE session so a
  session-free controller activation never contends for its own operation lock
- `nixorium-pxe.service` must validate the prepared revision, root-owned active
  session, live DHCP address, and absent static CIDR before binding; preserve
  its systemd readiness protocol and unprivileged listener identities
- public PXE lifecycle control must retain the exact verb/unit allowlist,
  require readiness before confirmed start, deny direct network-unit start,
  and synchronously stop listener/network units after a failed start
- `nixorium-remote-install.service` is the only USB/SSH controller worker. Keep
  its fixed deployment path, administrator ownership, mode-0600 private socket,
  `/run`-only credentials, strict durable records, and content-bound IPC. Its
  persistent reservation and the fleet lock must remain atomic with PXE and
  deployment; malformed/orphan state fails closed and post-dispatch recovery
  observes receipts without issuing a second mutation.
- keep product firewall openings interface- and role-scoped; do not restore
  global `allowed*Ports` or module `openFirewall` shortcuts
- Keep the student outside the `networkmanager` group and preserve the explicit
  polkit denial for `org.freedesktop.NetworkManager.*`; teacher and admin retain
  network management while student sessions only consume system connectivity.
- Passwords in `users.nix` are hashed (SHA-512 crypt); never store plaintext
- SSH password auth is disabled; key-based only
- `users.mutableUsers = false` enforces declarative user management

## Host Configuration

Hostname + static IP are generated in `lib/mk-lab.nix` from the IPv4 network,
CIDR prefix, and host offset. Interface resolution uses host, role, then shared
fallback settings in `lib/mk-lab.nix`; `modules/networking.nix` applies the
resolved interface.

Managed site settings live in a new deployment's `lab-settings.json`, and
managed software declarations in `lab-software.json`; legacy `lab-config.nix` deployments remain supported and
read-only until explicitly migrated.
Additional behavior belongs in downstream extension modules, never in copies of upstream modules.
Shell scripts must load operational settings from `labMeta` via `scripts/lib/lab-meta.sh`.
