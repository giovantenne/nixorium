# Source map

Repository paths below are relative to the upstream root.

```
.github/workflows/validate.yml # Go build/tests plus evaluation-only source/template CI
.github/workflows/migrations.yml # Checkout-only migration compatibility gate
.github/workflows/security.yml # CodeQL plus pinned Go vulnerability/static analysis
.github/workflows/full-validation.yml # Parallel full gate: release, nightly and manual
.github/workflows/release.yml # Metadata check; full validation gates GitHub Release publication
install.sh                  # Public entrypoint for controller bootstrap
flake.nix                  # Public Flake API plus backward-compatible example deployment
flake.lock                 # Pinned inputs (nixpkgs nixos-26.05, Disko)
VERSION                    # Canonical Semantic Version
CHANGELOG.md               # Curated release notes
LICENSE                    # MIT license
DISCLAIMER.md              # Operational responsibility and risk boundaries
TRADEMARKS.md              # Name and logo usage policy
SECURITY.md                # Private reporting and coordinated disclosure policy
lab-config.nix             # Standalone example configuration for this upstream
disko-uefi.nix             # NixOS wrapper for the shared Disko layout
lib/
  disko-layout.nix         # Shared Disko layout function (device + student user)
  eval-lab-config.nix      # Typed schema and validation for lab-config.nix
  eval-lab-settings.nix    # Strict versioned lab-settings.json envelope
  eval-lab-software.nix    # Strict allowlisted lab-software.json evaluator
  eval-workspace-profile.nix # Internal student preference JSON validator
  mk-lab.nix               # Host, class, netboot, app, and installer output constructor
setup.sh                   # Installer script for PXE-booted client PCs
scripts/remote-client-installer.sh # Fixed live-ISO USB/SSH installation helper
pkgs/
  nixorium.nix             # Go management command package
cmd/nixorium/              # Management CLI entrypoint
cmd/nixorium-remote-worker/ # Controller-owned USB/SSH operation worker
cmd/nixorium-demo/         # Developer-only deterministic website demo exporter
internal/                  # Domain, application, adapter, and presentation layers
  deploymigrate/           # Changes a release makes to deployment files: registry.go (ADR 0034)
modules/
  common.nix               # Composition point and shared system defaults
  firewall.nix             # Interface-scoped SSH, cache, PXE, and time policy
  desktop.nix              # Core GNOME session, locale and regional policy
  power.nix                # Idle and controller sleep policy
  time.nix                 # Controller serves time (chrony); clients use only the controller
  nix-ld.nix               # nix-ld loader and common libraries for downloaded Linux binaries
  ssh.nix                  # SSH client and server policy
  hardware.nix             # Generic hardware detection (replaces per-host hardware-configuration.nix)
  networking.nix           # Hostname + NetworkManager profile nixorium-lab (DHCP + static IP)
  identity.nix             # Client identity record (/var/lib/nixorium/identity.json)
  management.nix           # Controller-only management command installation
  pxe.nix                  # Transactional PXE address state and boot recovery
  users.nix                # User accounts (admin + teacher + student, nixorium-staff group)
  cache.nix                # Controller Harmonia service + client cache trust
  filesystems.nix          # Btrfs subvolume mount declarations
  home-reset.nix           # Student home directory templating + boot-time reset
  classroom-view.nix       # Classroom view agent and Shell extension on clients
scripts/
  release.sh               # Validates, tags, and publishes a release
  install-controller.sh    # Live USB bootstrap installer for controller with disk selection
  lib/lab-meta.sh          # Shared helper: loads labMeta from the flake for shell scripts
  lib/ui.sh                # Terminal presentation helpers for setup.sh (install scripts carry guarded copies)
  create-home-template.sh  # Builds clean home directory template
  validate.sh              # Tiered upstream validation entry point
  security-check.sh        # Pinned staticcheck and govulncheck entry point
  lint.sh                  # Pinned ShellCheck, gofmt and errcheck entry point
  apply-controller.sh      # Body of nixorium-apply-controller (filled in by modules/management.nix)
  prepare-pxe.sh           # Body of nixorium-prepare-pxe (filled in by modules/management.nix)
assets/
  empty-*                  # Neutral fallbacks for optional deployment assets
templates/site/            # Private deployment repository template
  software-catalog.nix     # Deployment-owned suggestions for the software UI
  assets/                  # Site branding, MIME and editor defaults
  modules/                 # Workstation, development and home-profile policy
skills/nixorium-developer/ # Public upstream development and release workflow
skills/nixorium-maintainer/ # Private laboratory maintenance workflow
docs/management-architecture.md # Accepted management-system target design
docs/development-validation.md # Validation pyramid and gate-selection guide
docs/troubleshooting.md         # Task-oriented recovery and backup guide
docs/system-reference.md        # Workstation defaults and mkLab extension reference
docs/adr/                   # Product architecture decision records
```

Generated locally during setup and committed in the private deployment repo:
- `keys/cache-public-key`
- `keys/admin-ssh.pub`

