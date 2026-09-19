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

## Project Structure

```
.github/workflows/validate.yml # Go build/tests plus evaluation-only source/template CI
.github/workflows/release.yml # Revalidates release metadata and publishes GitHub Releases
install.sh                  # Public entrypoint for controller bootstrap
flake.nix                  # Public Flake API plus backward-compatible example deployment
flake.lock                 # Pinned inputs (nixpkgs nixos-26.05, Disko, Veyon)
VERSION                    # Canonical Semantic Version
CHANGELOG.md               # Curated release notes
LICENSE                    # MIT license
lab-config.nix             # Standalone example configuration for this upstream
disko-uefi.nix             # NixOS wrapper for the shared Disko layout
lib/
  disko-layout.nix         # Shared Disko layout function (device + student user)
  eval-lab-config.nix      # Typed schema and validation for lab-config.nix
  eval-lab-settings.nix    # Strict versioned lab-settings.json envelope
  eval-lab-software.nix    # Strict allowlisted lab-software.json evaluator
  mk-lab.nix               # Host, netboot, Colmena, app, and installer output constructor
setup.sh                   # Installer script for PXE-booted client PCs
pkgs/
  gnome-remote-desktop.nix # gnome-remote-desktop overlay (VNC + multi-session)
  nixorium.nix             # Go management command package
cmd/nixorium/              # Management CLI entrypoint
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
assets/
  empty-*                  # Neutral fallbacks for optional deployment assets
templates/site/            # Private deployment repository template
  software-catalog.nix     # Deployment-owned suggestions for the software UI
  assets/                  # Site branding, MIME and editor defaults
  modules/                 # Workstation, development and home-profile policy
  scripts/                 # Optional site screensaver implementation
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
# Default: syntax, shell regressions, agent guidance, schemas, Go build/tests
./scripts/validate.sh --quick

# Add full mkLab evaluation for Nix API or host-composition changes
./scripts/validate.sh --eval

# Persistent, pinned shell for repeated Go edits
nix --extra-experimental-features 'nix-command flakes' develop --file tests/source-checks.nix go-shell
go test ./...
```

Use the affected management or client-installer VM only when changing its
integration boundary. Reserve `--full` for cross-cutting system-build changes
and milestone/release completion, not the edit-test loop. Build one
representative client and the controller when needed; add another client only
for materially different host-specific modules. See
[development validation](docs/development-validation.md) for the complete policy.

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
the annotated tag. The GitHub Actions release workflow publishes the GitHub
Release from the matching changelog section.

## Architecture Notes

- Public controller bootstrap must resolve the selected branch/tag once to a
  full Git revision. Its template, installer, Disko layout, and initial lock
  must use that revision; keep the human-selected channel in the generated
  `flake.nix` for managed updates. Never restore independently moving bootstrap
  downloads; see ADR 0018.
- New deployment templates own the direct `nixpkgs` input and make Nixorium's
  input follow it. `lib.packageBase` declares the compatible source/channel;
  `Update Nixorium` must preserve the complete deployment-owned lock node.
  Legacy deployments remain readable and are never migrated implicitly; see
  ADR 0019.
- `lab.deploymentMode` is optional (`laboratory` by default). Explicit
  `controller` mode requires zero clients, leaves lab networking/cache/remote
  control inactive, and permits local controller activation without lab keys.
  `deploymentStatus.controller` is the controller-specific capability; older
  upstreams fall back to strict fleet readiness. Never use this capability to
  authorize PXE or client deployment. Bootstrap capability version 1 collects
  the keyboard first, then controller identity, other regional settings, and
  password hashes before disk installation, then defers lab networking. Apply
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
- Custom settings flow from `lib/mk-lab.nix` via `specialArgs` (`labSettings`, `labAssets`, `hostName`, `hostIp`) to modules that need them.
- `labSettings` is a plain attribute set containing all configurable values: user names (`teacherUser`, `studentUser`), passwords, SSH key, network settings, locale/timezone, homepage URL, git identity, and more.
- Structured settings changes use `config plan` followed by `config apply --expect <fingerprint>`; the plan must pass the deployment's `nixoriumValidateCandidate` hook and must never expose password hashes in its diff.
- Guided software changes use `software catalog`, `software plan`, and
  `software apply`. Resolve package IDs from the pinned package set and
  deployment validators; the curated catalog is suggestions, not an allowlist.
  `shared` applies to the controller and present/future clients; `controller`
  applies only to the controller. Existing `all-clients`, groups and explicit
  client scopes never gain controller effects. `nixoriumSoftware.controller`
  advertises the new capability; absent metadata keeps the legacy UI/scopes.
  Review includes both old and new destinations when changing scope, and the
  token binds that impact. Always compose the existing deployment validation
  with the upstream controller-candidate hook when old/new declarations include
  the controller, including removal. Apply may atomically replace only `lab-software.json`
  after token and fingerprint rechecks. CLI apply remains declaration-only; the
  ordinary TUI records it transparently and immediately invokes the typed
  controller plan/apply boundary when the reviewed scope affects the controller.
  It must not push, prepare PXE, deploy clients, or rewrite packages supplied by
  private modules.
- TUI screens receive typed application callbacks from `cmd/nixorium`; keep command execution, privilege checks, state reconciliation, and other operational logic out of `internal/presentation`.
- Client enrollment is local and guided; consume only the immutable versioned installer inventory, treat reachability as a best-effort duplicate warning rather than a reservation, and keep unattended installation disabled without explicit private policy and a documented token model.
- Client deployment expands only evaluated inventory targets, binds execution to the reviewed clean Git revision, builds before apply, runs unprivileged with fixed Colmena argument arrays, and preserves streamed mode-0600 logs plus honest partial-failure/retry reporting. After every apply attempt it authenticates selected host state and records only revision-matching systems in a separate administrator-owned mode-0600 history; live host state remains authoritative.
- Client shutdown expands only evaluated client identities and never the controller. Active sessions remain eligible after an explicit data-loss warning; when any are present, the review must state that the one-word `SHUTDOWN` confirmation authorizes their interruption. Preserve explicit acknowledgement for unknown sessions, expiring content-bound review, PXE/recovery conflict check, immediate inventory/session recheck, and the same non-blocking lock used by deployment. Adapters may issue only the fixed `nixorium-session-state` and `systemctl poweroff --no-block` SSH commands. Report accepted/not-sent/unconfirmed requests without inferring physical power state or retrying unconfirmed dispatches.
- Operation history records only typed safe summaries for important outcomes in an atomic mode-0600 newest-1000 store; it never copies raw report messages and never deletes detailed deployment logs. Browsing accepts only generated deployment-log basename IDs, caps discovery at 50 results and detail at a 64 KiB tail, validates owner/mode/type with no-follow opens, and sanitizes terminal controls. Keep persistence/filesystem inspection in adapters and list/detail navigation in presentation.
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
- `labMeta` is a public flake output containing the small set of non-sensitive operational values that tools need (controller IPs, network prefix, iface name, structured client hostname/IP inventory, ports, usernames). `deploymentStatus` separately reports whether placeholders, public default passwords, or public keys still block deployment. `nixoriumSoftware` is the typed non-secret catalog/scope/declaration contract. Scripts and documentation commands must consume these outputs instead of parsing Nix source files textually.
- `lib/eval-lab-settings.nix` validates the versioned JSON envelope and delegates its `lab` object to `lib/eval-lab-config.nix`, whose private `lib.evalModules` schema remains the final type/semantic authority. Keep its semantic checks aligned with `internal/domain/settings.go` through `tests/lab-settings-validation-cases.json`. No custom NixOS options are added to host configurations.
- VirtualBox guest additions are enabled by default via `mkDefault` in `common.nix` (harmless on bare metal).
- Hardware detection uses `modules/hardware.nix` with `not-detected.nix` for automatic driver loading. No per-host hardware-configuration.nix files are needed.
- UEFI boot is required on all machines. Disk partitioning uses an EFI System Partition (`/boot`) plus Btrfs subvolumes.
- Netboot uses the systemd-owned `nixorium-pxe.service` with `dnsmasq` in ProxyDHCP mode so institutional DHCP remains authoritative for leases. `scripts/run-pxe-proxy.sh` remains an advanced foreground compatibility helper. `mkLab` builds a standalone installer source containing the effective downstream configuration and only local Flake inputs for offline evaluation.
- `labOverlay` composes Veyon's official overlay with local PipeWire packaging fixes and the GNOME Remote Desktop fallback patch. It is applied in each host's module list and in `colmena.meta.nixpkgs`.
- Docker is rootless for every normal user. Never add users back to the root-equivalent `docker` group; each account has declarative subordinate UID/GID ranges.
- Global npm packages use `~/.local/npm` through `NPM_CONFIG_PREFIX`. Do not install npm tools with `sudo` or into the Nix store.
- Veyon classroom management is configured in `modules/veyon.nix`: runs `veyon-service` in the graphical user session, deploys the public key, generates a `Veyon.conf` with all client PCs pre-mapped, and opens port 11100. `labSettings.veyonNativeHosts` selects the Veyon 4.11 native PipeWire backend per host; other hosts use the GNOME Remote Desktop fallback on port 5900. The private key is not managed by Nix (see Security).
- `modules/firewall.nix` enables the firewall everywhere, disables implicit
  all-interface SSH/Avahi openings, scopes SSH/mDNS/Veyon/optional VNC to the
  configured interface, and adds Harmonia/PXE ports only on the controller.
- `Veyon.conf` is a build-time derivation: evaluation must never read a derivation output to encode its network objects. GitHub CI disables import-from-derivation to enforce this boundary.
- The `veyon-master` group (declared in `modules/veyon.nix`) controls access to the Veyon private key. Users `admin` and the teacher user are members (configured in `modules/users.nix`).
- `modules/common.nix` is only the composition point for core desktop, firewall,
  power, and SSH modules. Packages, shell preferences, development tools,
  screensaver behavior, and application policy belong in the deployment.
- Site desktop favorites and shortcuts live in `templates/site/modules/workstation.nix`;
  student template content lives in `templates/site/modules/home-profile.nix`.
  Keep both conditional on effective software scope, not in core desktop policy.

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
- `nixorium update` may use controller internet only when explicitly invoked;
  it must never accept an arbitrary replacement source URL, expose ignored
  private files to Nix, or introduce client-side network requirements
- `keys/cache-public-key`, `keys/admin-ssh.pub`, and `keys/veyon-public-key.pem` are public and may be committed
- `nixorium setup keys` uses create-new semantics and refuses public-only or mismatched pairs; never bypass that refusal by overwriting an existing key
- `nixorium setup install-secrets` may start only `nixorium-install-secrets.service`; its deployment path is declarative and destinations are fixed
- `nixorium setup apply` requires a clean reviewed Git deployment and may start
  only `nixorium-apply-controller.service`; never add arbitrary target/path
  parameters or evaluate ignored private files through a `path:` Flake URL
- `nixorium pxe prepare` may start only the fixed administrator-owned
  `nixorium-prepare-pxe.service`; keep its clean-Git, live-DHCP, healthy-cache,
  canonical-store-path, and managed-GC-root checks intact
- `nixorium-pxe-network.service` is an internal root boundary with only
  `CAP_NET_ADMIN`; preserve its root-owned session-before-mutation ordering,
  exact static-address restoration, and boot-time recovery semantics
- `nixorium-pxe.service` must validate the prepared revision, root-owned active
  session, live DHCP address, and absent static CIDR before binding; preserve
  its systemd readiness protocol and unprivileged listener identities
- public PXE lifecycle control must retain the exact verb/unit allowlist,
  require readiness before confirmed start, deny direct network-unit start,
  and synchronously stop listener/network units after a failed start
- keep product firewall openings interface- and role-scoped; do not restore
  global `allowed*Ports` or module `openFirewall` shortcuts
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
