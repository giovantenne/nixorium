# Nixorium

Nixorium installs and manages reproducible NixOS computer labs from a central
controller.

One controller builds system configurations and installs and updates multiple
client computers over the LAN. Clients run locally and do not need direct
Internet access for installation or system deployment.

[Website](https://nixorium.org/) ·
[Documentation](#documentation) ·
[Contributing](CONTRIBUTING.md) ·
[Releases](https://github.com/giovantenne/nixorium/releases)

> **Status: beta.** The current development line is intended for evaluation and
> hardware testing. [VERSION](VERSION) identifies this checkout;
> [Releases](https://github.com/giovantenne/nixorium/releases) distinguishes
> prereleases from the previous stable line. `master` may contain unreleased
> changes: consult documentation at your selected release tag.

Learn what Nixorium is designed for and how it is used in a lab:
[nixorium.org](https://nixorium.org/).

## Why this exists

The project began in one Italian school lab with 30 student workstations and
one controller (31 machines total). Clients had no Internet access until a
user authenticated to the school network, complicating installation, updates,
and recovery. Read the [maintainer’s original classroom account](https://nixorium.org/case-study/original-classroom/),
adapted from his public post
and replies. It documents the project’s origin, not hardware certification or
a benchmark of the current software.

NixOS provides declarative, reproducible system configurations. Nixorium adds
the controller-based installation and management workflow needed to use them
across a lab, including environments where clients cannot reach the Internet
or only gain access after a user signs in. One private deployment repository
describes the site; the controller supplies the systems over the local network.

## Features

- Centralized configuration and deployment to one, selected, or all clients
  from pinned Nix inputs.
- Guided PXE installation and reinstallation, with computer identity, disk
  selection, and destructive confirmation performed locally on each client.
- Controller-prepared installation artifacts and a signed local binary cache,
  so clients can install and receive system updates without Internet access.
- ProxyDHCP network boot alongside an existing DHCP server, which remains
  responsible for address leases.
- A terminal interface and CLI for installation, software selection, settings,
  controller updates, client deployment and shutdown, diagnostics, and logs.
- GNOME workstations with Veyon integration; student sessions cannot change
  NetworkManager state, and their homes reset from a clean template at boot
  while retaining up to five local home snapshots.
- Private, deployment-owned software profiles, suggestions, assets, and
  extension modules, separate from the reusable public framework. Optional
  profiles can be added as one reviewed declaration batch and then customized
  through the ordinary software workflow.
- Independent, reviewed framework and NixOS/package-base updates, including
  explicitly acknowledged channel migrations. See [the update guide](docs/updates.md)
  for validation, deployment adoption and recovery.
- Reviewed operations with progress and failure reporting, plus recorded PXE
  network state for explicit recovery and recovery at boot.

## Before you try it

- **Hardware:** the supported target is `x86_64-linux`. Controller and clients
  require UEFI; clients need working UEFI network boot. Start with a disposable
  controller and one client, using VMs or dedicated test hardware.
- **Controller:** bootstrap from the official [NixOS Minimal
  ISO](https://nixos.org/download/#nixos-iso) in UEFI mode with Internet access.
  It provides the predictable Linux text console required while choosing the
  keyboard and entering passwords. Allow storage for the deployment, build
  outputs, and prepared client systems; requirements depend on the software
  selected.
- **Network:** use a lab LAN with an existing DHCP server and permission to run
  PXE services. For an initial test, keep the controller and client on the same
  isolated segment with DHCP available. Choose a static lab address range that
  does not conflict with the existing network.
- **Internet:** the controller fetches inputs and packages during preparation
  and updates. Prepared client installations and system deployments use the
  LAN only; applications and user sessions may have their own Internet needs.
- **Console access:** keep access to the controller console. Starting PXE
  temporarily removes its static lab address, and controller activation can
  restart networking and services.

> **Disk installation is destructive on both controller and clients.** Back up
> existing data before selecting a target disk; disconnect unrelated disks
> where practical. Student-home snapshots and NixOS generations are local
> recovery mechanisms, not backups. Keep an encrypted, separate backup of the
> private deployment and its private keys; see [backup and restoration
> guidance](docs/troubleshooting.md#backups-and-restoration).

## Quick start

Check the requirements above before proceeding. For an initial evaluation,
configure **one client** and verify it before expanding the lab. This is a
testing recommendation, not a separate mandatory stage in the interface.

For a disposable evaluation, use one controller VM and one client VM on an
isolated segment with DHCP. The installation still erases the selected virtual
disks. See the [evaluation environment](docs/evaluation-environment.md) for the
concrete VirtualBox recipe, expected results, and explicit verification status.

### 1. Bootstrap the controller from USB

Boot the official **NixOS Minimal ISO** in UEFI mode with Internet access. It
starts in the Linux text console expected by the bootstrap. Then run:

```sh
curl -fsSL https://nixorium.org/install.sh | bash
```

The command downloads and executes the public [bootstrap script](install.sh);
inspect it first if required by your local policy. Select a published beta to
evaluate the current workflow, rather than the moving `master` branch. Follow
the prompts for accounts, regional settings, passwords, the initial software
profile, and the controller disk. The software choice is reviewed after the
account settings and before any disk change. Confirm disk erasure only after
checking the selected device.
Keyboard layout is the first controller-setting prompt. The bootstrap applies
its console keymap immediately and stops if it cannot do so. This ensures all
remaining input, especially passwords, uses the same layout that will be active
after reboot. Do not run the command from a terminal inside the Graphical ISO:
its compositor layout cannot be verified portably. Switching that image to a
real Linux TTY may work, but the Minimal ISO is the supported bootstrap path.

### 2. Configure the laboratory

After installation, remove the USB, reboot, and sign in as `admin` using the
password you chose. Open the management interface:

```sh
nixorium
```

Choose **Installation → Install computers** and complete **Laboratory
settings**. The flow validates and saves the settings, generates missing keys,
activates the controller configuration, and prepares all configured clients.
It retains the controller's time zone and keyboard settings.

Review the network transition when prompted, then confirm starting PXE.
Existing DHCP continues assigning leases. Keep the generated deployment
repository private.

### 3. Install and check one client

Boot the test client over the network. In the downloaded installer environment,
run:

```sh
/installer/setup.sh
```

Select that computer's configured identity and target disk. The installer
console uses the controller's configured keyboard layout and requires explicit
destructive confirmation before proceeding. After installation, boot the
client from its local disk.

Stop PXE from **Installation → PXE mode and network recovery** to restore the
controller's normal static address. Check login, desktop behavior, home reset,
and a client deployment before adding more computers.

Continue with the [administrator guide](templates/site/README.md) for normal
operation and customization. The [hardware validation plan](docs/hardware-validation.md)
provides deeper firmware, network, and recovery checks beyond this first trial.

## Repository structure

| Area | Responsibility |
|---|---|
| `flake.nix`, `lib/`, `modules/`, `pkgs/` | Public `lib.mkLab` API, settings validation, generated hosts, NixOS modules, and packaging |
| `cmd/nixorium/`, `internal/` | Go CLI/TUI, application workflows, domain rules, and system adapters |
| `install.sh`, `setup.sh`, `scripts/` | Controller bootstrap, client installation, operational helpers, and validation entry points |
| `templates/site/` | Private deployment template: settings, software catalog and optional profiles, assets, and local policy modules |
| `docs/` | Architecture decisions, system reference, troubleshooting, and validation guides |
| `tests/`, `.github/workflows/` | Schema, shell, and VM tests; CI and release automation |
| `AGENTS.md`, `skills/` | Contributor instructions and separate upstream-development and lab-maintenance agent workflows |

## Architecture and security

Nix describes desired system state; the Go application coordinates reviewed
workflows. Colmena handles client deployment, Harmonia serves the signed local
cache, and Disko defines disk layouts. The cache remains available during
normal lab operation; PXE services run on demand.

Privileged operations use fixed systemd units and constrained polkit rules,
not arbitrary root commands from the interface. Firewall openings are scoped
to the configured lab interface and machine role. Client installation requires
local confirmation; unattended installation is disabled.

> [!NOTE]
> TCP port 5900 is open on the lab interface only for hosts using Nixorium's
> temporary Veyon Wayland fallback. Veyon's native PipeWire backend is still
> experimental and GNOME requires interactive screen-sharing consent, so the
> fallback uses view-only GNOME Remote Desktop with a shared VNC password for
> unattended monitoring. This workaround will be retired as native Wayland
> support and managed GNOME authorization mature; hosts using the native Veyon
> backend do not open port 5900.

Site settings, password hashes, public keys, and policy belong in the private
deployment repository. Private SSH, cache-signing, and Veyon keys must stay out
of Git and the Nix store. Public keys may be committed to the deployment.

See the [management architecture](docs/management-architecture.md) and
[architecture decisions](docs/adr/) for the trust boundaries, operation state,
and failure-handling contracts.

## Documentation

The website is the product-facing entry point. Repository documents provide
the versioned operational and contributor references.

| I want to… | Read |
|---|---|
| Understand the product and lab use cases | [Website](https://nixorium.org/) |
| Evaluate one controller, one client, and one change | [Isolated VM recipe](docs/evaluation-environment.md), then [quick start](#quick-start) |
| Set up and manage a lab | [Administrator guide](templates/site/README.md) |
| Understand the management interface | [TUI tour](docs/tui-renders.md), [generated renderer gallery](docs/tui-gallery.md) |
| Diagnose a failure or restore a backup | [Troubleshooting](docs/troubleshooting.md) |
| Customize systems or use `lib.mkLab` | [System and extension reference](docs/system-reference.md) |
| Update the core, NixOS or packages; adopt older deployments | [Update and recovery guide](docs/updates.md) |
| Understand architecture and security | [Management architecture](docs/management-architecture.md), [ADRs](docs/adr/) |
| Report a suspected vulnerability | [Security policy](SECURITY.md) |
| Review operator responsibilities and limitations | [Operational disclaimer](DISCLAIMER.md) |
| Evaluate firmware and physical hardware | [Hardware validation plan](docs/hardware-validation.md) |
| Evaluate with disposable virtual machines | [Evaluation environment](docs/evaluation-environment.md) |
| Develop and validate changes | [Contributor guide](CONTRIBUTING.md), [agent instructions](AGENTS.md), [development validation](docs/development-validation.md) |
| Work with a coding agent | [Agent skills](#agent-skills) |
| Review release changes | [Changelog](CHANGELOG.md), [Releases](https://github.com/giovantenne/nixorium/releases) |

## Agent skills

Nixorium includes skills: task-specific instructions that help coding agents
work with the project's configuration, validation, and safety rules.

- **Core development:** use
  [`nixorium-developer`](skills/nixorium-developer/SKILL.md) in this public
  repository for changes to the NixOS modules, CLI/TUI, installers, API, or
  tests.
- **Lab administration:** use
  [`nixorium-maintainer`](skills/nixorium-maintainer/SKILL.md) in the lab's
  private deployment repository for settings, software, local modules,
  troubleshooting, and reviewed installation or update operations. This skill
  is included in the deployment template; the developer skill is not.

Open your coding agent in the appropriate repository and ask it to use the
named skill. Discovery links are included for Codex, OpenCode, Claude Code,
and Pi. For example, an administrator can ask: “Use nixorium-maintainer to add
Firefox for all computers and validate the configuration without deploying.”
A contributor can ask: “Use nixorium-developer to improve the installation
screen and add regression tests.”

Skills guide the agent; they do not replace review or grant permission to
install, deploy, change live services, commit, or push. Review the proposed
changes and explicitly authorize the operations you want performed.

## Development

Work on the public framework here; keep site-specific changes in a private
deployment. Start with [CONTRIBUTING.md](CONTRIBUTING.md), then read
[AGENTS.md](AGENTS.md) and the
[`nixorium-developer` skill](skills/nixorium-developer/SKILL.md) before changing
the API, modules, installers, or template.

From a checkout with Nix available, run the normal fast validation gate:

```sh
./scripts/validate.sh --quick
```

For repeated Go edits, enter the pinned development shell once and reuse its
incremental test cache:

```sh
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell
go test ./...
```

Use `./scripts/validate.sh --eval` for Nix API or host-composition changes.
The [validation guide](docs/development-validation.md) defines when targeted
VM tests or the full release gate are needed. Routine validation builds one
representative client and the controller when system builds are required, not
every client in the inventory.

## License

Released under the [MIT License](LICENSE).
