# Changelog

All notable changes to Nixorium are documented in this file.

The project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Added guided import for existing cache-signing, administrator SSH, and
  Veyon private keys during first setup. Imports reject symbolic links,
  oversized or broadly readable files, encrypted unattended SSH keys, and any
  existing destination; they derive and fingerprint the public key, preserve
  the source, and never perform implicit rotation.

### Changed

- Made ordinary settings and first-setup saves record their managed files
  locally without exposing Git, commit tokens, hashes, or repository identity.
  Saves use the existing isolated-path safety checks, preserve unrelated work,
  use a fixed internal identity, reject ambiguous same-file changes, and offer
  an in-place retry when writing succeeded but local recording needs recovery.
- Made interactive startup render immediately before repository inspection.
  The dashboard now shows an English opening activity while status and setup
  checks run once in the background, routes to first setup only after those
  checks complete, and provides an in-place retry screen when initialization
  fails. JSON and other non-interactive commands retain synchronous output.
- Unified bare `nixorium setup` and the ordinary dashboard around the same
  first-setup screens. A fresh or incomplete laboratory opens its first pending
  setup step, while every configured laboratory retains a visible Setup and
  readiness intervention. Network, identity, locale, and password work now
  opens the existing validated editors in place; missing keys can be generated,
  verified, and installed through typed setup actions without asking the
  operator to leave the TUI and run another command.
- Changed fresh deployment and standalone-example regional defaults to US
  English for language, formats, desktop keyboard, and console keyboard, with
  `America/New_York` as the visible, editable initial time zone. Existing
  deployment settings remain unchanged; the setup selector now presents the
  principal US time zones before its international suggestions.
- Made dashboard sessions start from fresh task state: reopening Settings or
  Update no longer shows a previous result, cancelling a software removal
  returns to the selected package, and setup refreshes now render their active
  wait. Every managed quit path while PXE is active opens the same consequence
  review, including `Ctrl+C` and exits outside the installation screen; the
  operator can stop and verify installation mode before exit or explicitly
  confirm that it should remain active.
- Added reviewed client-only shutdown through shared CLI/TUI plan/apply
  operations. Nixorium resolves only evaluated client identities, excludes the
  controller, checks management access and interactive sessions, serializes
  against deployments, blocks during PXE/network recovery, and rechecks before
  sending a fixed power-off request. Active sessions remain blocked; unknown
  session state requires explicit acknowledgement. Results distinguish
  accepted, not sent, and unconfirmed requests without inferring physical power
  state or retrying blindly.
- Added guided client-software management through a strict versioned
  `lab-software.json` file. The TUI and CLI share catalog/plan/apply services,
  accept only curated packages resolved from the pinned package set, support
  all-client, evaluated-group, and explicit-client configuration scopes, and
  require a content-bound review phrase before an atomic file replacement.
  Saving a declaration never commits, builds, activates, starts PXE, or deploys
  a client; Git review and distribution remain explicit later operations.
- Reorganized the TUI around first installation and explicit later
  interventions instead of a fleet-health Overview. Restore distinguishes
  reapply from disk-erasing reinstall, setup groups technical checks into five
  operator stages, and Update Nixorium selects only releases fetched through
  the typed application service. Searchable Computers remains an explicit
  advanced check. Added contextual help,
  diagnostics, compact setup/progress, symbol-and-text status, adaptive list
  layouts, and persistent confirmation controls in long reviews. Existing
  application operations, CLI contracts and exact safety confirmations remain
  shared.
- Completed the guided pilot-computer handoff in first setup. The operator now
  selects an immutable configured identity, receives local identity/disk steps,
  checks authenticated active-revision evidence separately from the practical
  desktop check, and may finish after one computer without treating powered-off
  clients as failures. The application layer validates and probes only the
  selected pilot identity. Leaving PXE active requires an explicit consequence
  review and exact confirmation.
- Extended the same explicit-identity boundary to disk-erasing restoration.
  Reinstall now selects one evaluated computer before PXE review, repeats the
  local disk warning with that identity, verifies only the selected computer,
  and supports restoring another computer or ending a partial session.
- Made first-installation and reinstallation sessions resumable across TUI
  processes. Nixorium stores private, atomic, per-repository evidence bound to
  the exact evaluated identity, Git revision and authenticated system path;
  technical and operator practical checks remain distinct. An inventory or
  revision change makes the saved session stale instead of reusing evidence,
  and a new failed observation takes precedence over historical success.

- Added one background-aware official Bubbles spinner to every dashboard wait
  state, so host checks, Git/service loading, settings validation, update
  planning, and other operations without meaningful percentages visibly remain
  active while preserving their plain-language activity label.
- Made the PXE screen stage-aware: it now recommends artifact preparation,
  reviewed PXE start, first-computer boot/install, or network recovery from
  observed state, shows only pertinent controls, and includes the exact
  `/installer/setup.sh` handoff while installation mode is active.
- Replaced appended Settings, cache-service, local-Git-commit, and Nixorium-
  update outcomes with compact success/attention screens and explicit routes to
  the dashboard, Git review, logs, retry, or further editing. First-run Git
  completion returns to the reconciled setup checklist.
- Extended the shared light/dark visual hierarchy and width-adaptive Bubbles
  key help across Computers, Deploy, Controller, Services, Logs, Git, Update,
  and PXE screens; service, Git, and PXE states now use the same semantic color
  roles while retaining explicit text.
- Added live foreground deployment feedback with elapsed time, an official
  Bubbles progress bar, typed build/apply/verification activities, and checked-
  computer counts. Raw Colmena output stays in the private log, and the compact
  result now offers dashboard, log, and fresh-review actions.
- Replaced the dashboard's flat command wall with a paginated, keyboard-
  navigable Bubbles task menu, semantic light/dark status colors, task
  descriptions, and explicit key help while preserving textual state and all
  existing one-letter shortcuts.
- Turned bare first-run setup into a resumable guided handoff: after initial
  configuration and key reconciliation it opens an observed 11-stage progress
  screen whose single primary action routes through reviewed Git commit,
  controller activation, PXE preparation, and the first network installation.
  Reopening setup skips completed configuration stages.
- Made the controller rebuild result actionable: the dashboard refreshes its
  reconciled status, collapses completed activity by default, and offers clear
  dashboard, detail, log, and new-review actions instead of leaving the
  administrator at an ambiguous terminal result.
- Made first-run network suggestions prefer the interface carrying the default
  route and its live DHCP address, explicitly excluding the controller's
  declarative static address; moved the US English locale to the first curated
  locale choice.
- Extended managed live feedback to first-run and routine controller apply:
  both fixed and revision-bound systemd units publish validation, build,
  activation, and verification progress. CLI JSON keeps stdout clean while the
  dashboard shows elapsed time, recent activity, and a Bubbles progress bar.
- Added live PXE preparation feedback to the dashboard using an official
  Bubbles progress bar, elapsed time, typed phases, counters, and five bounded
  recent activities. The systemd job atomically publishes a strict private
  progress record while verbose Nix output remains in journald.
- Introduced a task-oriented dashboard Settings area for Network, Computers,
  Accounts, Regional, Browser, Git, and Veyon edits. It reuses the typed
  Nix-backed plan/fingerprint apply boundary and provides a terminal-only,
  no-echo single-account password path whose review remains redacted.
- Reduced first-run configuration from 19 to 15 task-grouped steps by leaving
  optional Git author identity at its template defaults, and replaced raw time
  zone, locale, regional-format, desktop-keyboard, and console-keymap entry
  with offline searchable Bubbles selectors plus validated custom entry.
- Migrated the terminal frontend to the aligned Bubble Tea, Bubbles, and Lip
  Gloss v2 stack, adding reusable background-aware title, error, and
  width-adaptive key-help components as the foundation for richer setup and
  operation-progress screens.
- Made first-run credential entry recover from short, public-default, and
  mismatched passwords by retrying only the current account, preserving all
  earlier wizard input while keeping terminal and hashing failures fatal.
- Reworked the public README into a concise product overview and seven-step
  guided quick start, moving workstation defaults and `lib.mkLab` extension
  details into a dedicated system reference and linking the existing
  administrator, troubleshooting, hardware, architecture, and contributor
  guides instead of duplicating them inline. Reformatted the generated
  deployment administrator guide around a contents index, dashboard task map,
  short operation-specific sections, and a collapsible complete CLI reference.
- Bound managed PXE sessions to the unambiguous live controller address at
  preparation time and pass it to the offline installer at boot, so ordinary
  DHCP lease changes no longer require rebuilding the controller configuration.
- Made controller-apply reconciliation require an atomic, root-owned success
  receipt bound to both the reviewed Git revision and active NixOS closure, so
  late activation failures cannot be mistaken for completed setup.
- Split local validation into a fast default cycle, targeted VM modes, and an
  explicit full release matrix, with a reusable Nix evaluation cache.
- Documented the complete per-host customization workflow in both READMEs and
  in the Nixorium maintainer skill.
- Made `networkBase` a full IPv4 network address and added a configurable CIDR
  prefix, with static addresses calculated from validated host offsets.
- Split the former monolithic `common.nix` into focused desktop, package,
  power, screensaver, shell, and SSH modules.
- Made student-home reset fail closed when snapshots or cleanup are incomplete,
  and made display-manager startup require a successful reset.
- Replaced README parsing of Nix source with stable `labMeta` evaluations.
- Dedicated `nixorium-maintainer` to private laboratory operations and added a
  separate upstream-only `nixorium-developer` skill.
- Made the development-branch site template consume `master`; release
  preparation replaces it with the matching immutable tag.
- Moved Veyon network-object encoding from module evaluation into the
  `Veyon.conf` build and made CI reject import-from-derivation.

### Added

- A packaged `nixorium` Go command with a task-oriented terminal dashboard,
  human/JSON `status`, actionable `doctor` diagnostics, shared typed domain
  operations, and unit tests. Its first operational screen prepares, reviews,
  starts, stops, and recovers managed PXE installation mode; its deployment
  screen selects, reviews, confirms, and executes the same typed build-first
  client workflow as the CLI.
- Structured client hostname/IP inventory in `labMeta`, comprehensive
  read-only diagnostics for networking, Harmonia, PXE ports, SSH, Colmena and
  disk capacity, an explicit full controller-build check, and a NixOS VM test
  of the installed controller command.
- Explicit `nixorium hosts` text/JSON inventory and a dashboard Computers
  screen with bounded SSH probes and distinct reachable, unreachable, refused,
  and unknown observations, while the default status path remains probe-free.
  Managed generations now embed their private deployment revision and install
  a fixed read-only host-state helper; authenticated bounded SSH observation
  reconciles active system/revision against desired HEAD as current, outdated,
  or unknown without evaluating every client closure during inventory.
- Read-only `nixorium deploy plan --on` reports for one, selected, or all
  clients, bound to a clean Git revision and blocked by readiness, Git, or
  selector errors before any Colmena execution.
- Revision-bound `nixorium deploy apply` execution with exact target
  confirmation, repeated preflight checks, mandatory verbose Colmena build
  before apply, serialized runs, streamed mode-0600 operation logs, explicit
  partial-failure state, authenticated post-apply reconciliation, atomic
  per-host last-successful-verification history, and safe full-workflow retry.
- Routine `nixorium controller plan`/`controller apply` CLI and TUI rebuild
  workflow with exact confirmation, a narrowly revision-instanced systemd unit,
  pinned unprivileged Git builds, pre-activation drift refusal, and active-system
  verification.
- Typed `nixorium services` CLI/JSON and dashboard service management for the
  persistent signed cache and composite on-demand PXE lifecycle, plus an exact
  confirmed cache restart through a fixed capability-free systemd/polkit action
  with post-restart unit and HTTP verification.
- Typed `nixorium logs` list/detail CLI/JSON and dashboard browsing for private
  deployment logs, with basename-only selection, strict owner/mode/type and
  no-follow validation, 50-record/64-KiB bounds, terminal-control sanitization,
  and scrollable tail rendering. A locked atomic mode-0600 newest-1000 record
  adds safe typed summaries for configuration, key, controller, PXE, cache, and
  deployment outcomes without copying raw messages or deleting detailed logs.
- Read-only `nixorium git review` CLI/JSON and dashboard review of bounded
  staged/unstaged patches plus untracked paths, with managed/unexpected
  classification, disabled external diff drivers, settings-password redaction,
  terminal sanitization, and private-key-path refusal before patch capture.
- Optional `nixorium git commit plan`/`apply` CLI/JSON and TUI workflow with an
  explicit path allowlist, isolated HEAD-based proposal index, content-bound
  token, exact confirmation, generated message, file-type/secret/filter/size checks,
  atomic HEAD update, path-only index reconciliation, preserved unrelated
  changes, and no hooks, signing action, remote requirement, or implicit push.
- Reviewed `nixorium update plan`/`apply` CLI/JSON and TUI workflow for explicit
  SemVer releases, with prerelease/downgrade opt-ins, external candidate lock,
  readiness and representative no-link builds, bounded patch review, exact
  confirmation, token-bound two-file apply, and no implicit Git, activation,
  PXE, or deployment actions.
- Explicit read-only `nixorium update check` CLI/JSON release discovery against
  only the configured public GitHub upstream, with Git credentials and prompts
  disabled, strict timeout/output/result bounds, and separately sorted stable
  and prerelease tags while all routine and client paths remain offline-first.
- A task-oriented troubleshooting and recovery guide shipped both upstream and
  in every generated deployment, covering required failure symptoms, safe retry
  semantics, interrupted operations, private-key-aware backups, and restoration;
  quick validation keeps both copies identical.
- A reproducible manual VirtualBox and physical-hardware validation plan with
  isolated/offline topology, evidence records, destructive-disk warnings,
  firmware/NIC/storage coverage, PXE power-loss and DHCP coexistence recovery,
  single/multi-client deployment, and explicit pass/fail matrices.
- Immediate stderr-safe PXE preparation activity and fixed journald follow
  guidance in CLI/JSON, keeping machine stdout clean and verbose systemd-owned
  build output out of the presentation layer.
- A versioned `lab-settings.json` format for new private deployments, strict Go
  and Nix validation, deterministic atomic file writing, and the read-only
  `nixorium config validate` command. Existing `lab-config.nix` deployments
  remain compatible.
- A deterministic, read-only `nixorium setup status` reconciler that reports
  every first-run stage, selects the earliest incomplete one from observed
  settings, credentials, key files, artifacts, and deployment readiness, and
  marks the final installation offer available only with a current PXE
  preparation.
- A secure password-collection backend with terminal-only no-echo input,
  confirmation, default/length checks, best-effort memory wiping, and
  stdin-only salted SHA-512 hashing through the packaged `mkpasswd` tool.
- Idempotent `nixorium setup keys` reconciliation for Harmonia, SSH, and Veyon
  pairs, with create-new writes, private-mode enforcement, cryptographic
  correspondence checks, retry safety, and overwrite refusal.
- A non-secret `config plan` and fingerprint-bound `config apply` protocol
  that validates candidates through the deployment Flake, rejects concurrent
  edits, and atomically updates only `lab-settings.json`.
- A guided `nixorium setup` terminal form with detected network defaults,
  backward navigation, no-echo password hashing, redacted final review,
  explicit acceptance, and Git-aware classification of existing changes.
- A fixed-path, systemd-sandboxed `setup install-secrets` action with
  unit-specific wheel polkit authorization, key-pair re-verification,
  least-privilege destinations, idempotent reuse, and mismatch refusal.
- A confirmed `setup apply` workflow with clean-Git/readiness/key preflights,
  a fixed systemd/polkit action, unprivileged controller build, exact-closure
  activation, durable journald failures, and observed active-generation state.
- A declarative controller-only Harmonia lifecycle with systemd credential
  loading from the non-store installed key, restart/watchdog behavior, a
  stable `nixorium-harmonia.service` alias, health diagnostics, and VM-tested
  missing-key recovery.
- A non-disruptive `nixorium pxe prepare` workflow with clean-Git, live-DHCP,
  and Harmonia readiness gates; a capability-free administrator systemd job;
  pinned firmware and all-client builds; managed GC retention; and an atomic,
  revision-bound manifest of strictly validated immutable Nix store outputs
  used by status and PXE.
- An internal, controller-only PXE network unit with a root-owned
  session-before-mutation record, narrowly bounded `CAP_NET_ADMIN`, exact
  static-address rollback, failure-safe stop behavior, and boot/explicit
  recovery that reconciles recorded state against the live interface.
- A systemd-owned, controller-only PXE listener that strictly reconciles the
  prepared revision, active network session, and live addresses before serving
  ProxyDHCP, TFTP, and HTTP; publishes readiness only after a health check; and
  drops network children to separate unprivileged identities.
- Confirmed `nixorium pxe start`, idempotent `pxe stop`, and explicit `pxe
  recover` operations with exact polkit controls, typed lifecycle status,
  repeated readiness checks, and synchronous rollback after startup failure.
- Guided PXE client enrollment with embedded versioned host inventory,
  hardware and writable-disk display, constrained identity selection,
  best-effort duplicate detection, hostname-and-disk destructive confirmation,
  target revalidation, offline closure and capacity preflight, a precompiled
  offline Disko action, explicit progress/failure results, and confirmed reboot.
- The accepted management-system architecture and ADRs for the terminal-first
  interface, Go/Bubble Tea implementation, structured deployment settings,
  narrow privilege boundary, systemd-owned runtime services, interface-scoped
  firewall, local guided client-enrollment trust model, bounded private
  operation records, bounded read-only Git review, and reviewed local Git
  commits.
- An interactive controller-bootstrap version selector offering `master`, the
  latest GitHub prerelease, and published stable releases while preserving
  `--release` and `NIXORIUM_RELEASE` for unattended installations.
- Semantic validation for IPv4 networks, CIDR capacity, interfaces, users,
  password hashes, URLs, per-host modules, and Veyon pilot hosts.
- A `deploymentStatus` output for placeholders, missing public keys, and public
  default password hashes.
- Configuration-schema tests, targeted GitHub evaluation of one client and
  other representative outputs plus a fresh template, and a separate local
  matrix for builds, netboot, and offline installer equivalence.

### Fixed

- Allowed the fixed controller activation units to update declared user homes
  and `/run/user`, while retaining the reviewed private deployment as an
  explicit read-only mount. This prevents valid NixOS activation scripts from
  failing under `ProtectHome`.

### Security

- Enabled the NixOS firewall on every installed host with role-specific ports
  limited to the configured interface, including controller-only Harmonia/PXE
  rules and no implicit global OpenSSH or Avahi openings.
- Private deployments are evaluated with the local Git Flake fetcher, keeping
  ignored Harmonia, SSH, and Veyon private keys out of Nix source/store copies.
- The controller bootstrap now runs the Disko revision pinned by the generated
  deployment instead of fetching a mutable upstream revision.
- SSH now records keys on first connection and rejects later key changes;
  Colmena uses the same `accept-new` policy.

### Breaking

- Deployment configurations must change `networkBase` from three octets such
  as `10.0.0` to a full network address such as `10.0.0.0` and add
  `networkPrefixLength`. The configuration and `labMeta` schema version is 2.

## [2.0.0-beta.3] - 2026-09-08

### Added

- A Raw GitHub `install.sh` entrypoint that selects a tagged release, prepares
  the private deployment Git repository, and installs the controller from it.

### Changed

- Rewrote the deployment upgrade guide with an explicit release example,
  current Nix Flake commands, complete validation, and the Git merge workflow.
- Completed the project-wide Nixorium rebrand across repositories, Flake
  inputs, installer identifiers, command names, desktop identifiers, and the
  Agent Skill.
- Adopted `nixorium.org` as the canonical website and installer entrypoint,
  with Raw GitHub documented as the bootstrap fallback.

### Breaking

- Private deployments must rename their upstream input to `nixorium` and use
  `github:giovantenne/nixorium/<release>` when moving to this release.
- Installer environment variables now use the `NIXORIUM_` prefix.

## [2.0.0-beta.2] - 2026-09-07

### Added

- A reusable `lib.mkLab` Flake API with typed lab configuration validation.
- A `site` Flake template for private per-lab deployment repositories.
- Extension points for shared, controller, client, host-specific and netboot modules.
- Configurable logo, backgrounds, MIME defaults, VS Code settings and public key paths.
- A standalone netboot installer bundle containing the effective downstream configuration.
- A portable repository-maintainer Agent Skill, preinstalled by the site template for Codex, OpenCode, Claude Code and Pi.

### Changed

- Operational Harmonia and PXE helpers are exposed as Flake apps for downstream repositories.
- Lab-specific configuration can now update the upstream through a pinned Flake input instead of Git merges.
- The upstream screensaver logo is generic and site-specific printer drivers are delegated to deployment modules.

## [2.0.0-beta.1] - 2026-09-04

### Added

- Per-user rootless Docker daemons with declarative subordinate UID/GID ranges.
- A writable `~/.local/npm` global prefix, available in every login session for tools such as Codex and Claude Code.
- An opt-in `veyonNativeHosts` canary for Veyon 4.11's PipeWire/XDG portal Wayland backend.
- Setuid wrappers for Veyon's authentication and Wayland input helpers.

### Changed

- Updated nixpkgs from NixOS 25.11 to 26.05 and refreshed all flake inputs.
- Updated Veyon from the local 4.10.0 derivation to its official 4.11.0 flake.
- Updated the GNOME Remote Desktop reconnect patch for GNOME 50's connection throttler.
- Switched development packages to the current stable Node.js and PHP aliases.
- Migrated the controller sleep policy to NixOS 26.05's structured systemd settings.
- Disabled automatic ZFS root-pool imports because the shared Disko layout uses Btrfs.
- Excluded Docker data, the npm cache, and global npm tools from student home snapshots.

### Security

- Removed every normal user from the root-equivalent `docker` group.

### Breaking

- Docker now uses a per-user rootless socket and storage. Existing rootful images and containers are not migrated.

## [1.0.0] - 2026-09-04

### Added

- Declarative generation of controller and student workstations with Nix Flakes.
- UEFI installation and Btrfs partitioning through Disko.
- Offline client installation through ProxyDHCP, TFTP, HTTP netboot, and a local Harmonia cache.
- Multi-host deployment through Colmena.
- Parameterized lab settings and a public `labMeta` flake output for operational scripts.
- GNOME Wayland student desktop with development tools and classroom defaults.
- Boot-time student home reset with five recoverable Btrfs snapshots.
- Veyon classroom management with pre-generated lab topology and GNOME Remote Desktop integration.

### Security

- Key-only SSH access and immutable declarative users.
- Separate public and private material for SSH, Harmonia, and Veyon.

[Unreleased]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.3...HEAD
[2.0.0-beta.3]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.2...v2.0.0-beta.3
[2.0.0-beta.2]: https://github.com/giovantenne/nixorium/compare/v2.0.0-beta.1...v2.0.0-beta.2
[2.0.0-beta.1]: https://github.com/giovantenne/nixorium/compare/v1.0.0...v2.0.0-beta.1
[1.0.0]: https://github.com/giovantenne/nixorium/releases/tag/v1.0.0
