# Nixorium

[![Release](https://img.shields.io/github/v/release/giovantenne/nixorium?display_name=tag&sort=semver)](https://github.com/giovantenne/nixorium/releases/latest)
[![NixOS](https://img.shields.io/badge/NixOS-26.05-5277C3?logo=nixos&logoColor=white)](https://nixos.org)
[![Flakes](https://img.shields.io/badge/Nix-Flakes-4E9A06?logo=nixos&logoColor=white)](https://nixos.wiki/wiki/Flakes)
[![License: MIT](https://img.shields.io/badge/License-MIT-2EA44F.svg)](./LICENSE)

Nixorium turns a group of PCs into one reproducible NixOS laboratory. A single
controller builds, installs, observes, and updates the client machines over the
LAN. Clients do not need Internet access during installation or deployment.

[Website](https://nixorium.org) ·
[Releases](https://github.com/giovantenne/nixorium/releases) ·
[Administrator guide](templates/site/README.md) ·
[Troubleshooting](docs/troubleshooting.md)

> The stable release is **v1.0.0**. NixOS 26.05 and the new management workflow
> are available for hardware testing in **v2.0.0-beta.3**. Use a tagged release
> for production; `master` is the development branch.

## Why this exists

The controller-first installer uses explicit
`lab.deploymentMode = "controller"` with `pcCount = 0`. Such a controller can be
rebuilt without client networking or lab keys, with lab services and firewall
openings inactive. Omission preserves legacy laboratory behavior. Client
networking and keys are configured later from **Install new computers**.
See [ADR 0015](docs/adr/0015-controller-first-capabilities.md).

Managed software now supports `shared` (controller and current/future clients)
and `controller` (only this controller). Existing client-only declarations keep
their scope. The TUI saves and activates controller-affecting choices in one
reviewed operation; client deployment remains an explicit separate task.
See [ADR 0016](docs/adr/0016-shared-software-scopes.md).

Network interface configuration now supports controller, client-role, and
per-host overrides while preserving `ifaceName` as the fallback. Controller
detection no longer rewrites the client fallback. See
[ADR 0017](docs/adr/0017-role-aware-network-interfaces.md).

New deployments own one explicit package-base pin shared by Nixorium, Disko,
Veyon, controller, and clients. Framework updates must preserve it; package-base
updates remain a separate future operation. See
[ADR 0019](docs/adr/0019-deployment-owned-package-base.md).

PC laboratories drift: machines are reinstalled at different times, manual
fixes accumulate, and repeating the same update across a room is slow and hard
to verify. NixOS makes each machine declarative and reproducible; Nixorium adds
the controller, offline distribution, guided installation, and fleet workflows
needed to operate that model on a real LAN.

The reusable implementation stays public while each site's identity, network,
keys, assets, and policy stay in a small private deployment repository.

## Features

- A terminal management application with guided setup plus a dashboard for
  diagnostics, PXE installation, deployments, services, logs, Git review,
  controller rebuilds, reviewed client shutdown, and Nixorium updates.
- Offline-first client installation through ProxyDHCP, iPXE, and a signed local
  Harmonia binary cache.
- Declarative deployment to one, selected, or all clients with Colmena.
- GNOME workstations with Veyon classroom management, a template-owned
  rootless Docker/development profile, and user-managed npm tools.
- Student homes restored from a clean template at boot, with five recoverable
  Btrfs snapshots.
- A reusable public framework plus a separate private repository containing
  each laboratory's settings, public keys, assets, and local modules.

## How it fits together

```text
                    private deployment repository
                  settings · public keys · local policy
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────┐
│ Controller                                                  │
│ Nix builds · Nixorium TUI · Harmonia · PXE · Colmena       │
└──────────────────────────────┬──────────────────────────────┘
                               │ LAN
                  ┌────────────┼────────────┐
                  ▼            ▼            ▼
                pc01         pc02         pcNN
```

Institutional DHCP remains authoritative. Nixorium temporarily manages only
the controller address transition needed by ProxyDHCP and records enough state
to recover it after failure or reboot. Normal client deployments use the
static laboratory network.

## Quick start

The controller and every installation target are erased during their respective
installations. Use disposable VMs first and keep unrelated disks disconnected.
All machines must use UEFI boot.

### 1. Boot the controller from a NixOS USB

Start the official NixOS installer with temporary Internet access.

### 2. Run the bootstrap installer

```sh
curl -fsSL https://nixorium.org/install.sh | bash
```

Choose the release, account names, time zone, keyboard, three passwords, and
controller disk when prompted. Keyboard selection happens before password
entry. After the version is resolved, the account and regional questions appear
before any Nix evaluation or build; the installer labels the later download and
installation phases explicitly. It keeps the internal locale at
`en_US.UTF-8`, creates a private deployment repository, and installs a usable
controller from pinned inputs. The selected channel or tag is resolved once:
template, installer, Disko layout, and initial lock all use that immutable
revision. See [ADR 0018](docs/adr/0018-revision-bound-controller-bootstrap.md).
After the exact `YES` disk confirmation, the pinned partitioning tool is made
ready before it touches the disk. The deployment lock, evaluation cache,
temporary 4 GiB swap, and controller closure then use the mounted target disk.
Nix jobs remain serialized even across `sudo`, bounding live-ISO memory use on
smaller machines. The temporary swap is removed before the installer exits.

New deployments use controller host number `99`, so the default controller
hostname is `pc99` and its static laboratory address is host number `99` in the
configured subnet (`10.0.0.99` with the default network). This is configurable,
not hard-coded: use **Maintenance → Change settings → Computers → Controller
host number** and rebuild the controller after saving. The chosen number must
be greater than the client count and fit inside the subnet.

### 3. Reboot

Remove the USB and sign in as `admin` with the password chosen before
installation. No public default password remains.

### 4. Open Nixorium

```sh
cd ~/nixorium-deployment
nixorium
```

Nixorium opens on five operator tasks: add or change software, install new
computers, distribute the prepared system, shut down computers, and advanced
tools. It does not scan the room at startup. Choose **Install new computers**
when you are ready to provide DHCP/network values, create or import keys,
prepare PXE, and install a pilot client. This setup is resumable and never
blocks ordinary controller use. Keep the deployment repository **private**.
Long operations show meaningful progress; `l` expands bounded activity details.
See the [TUI tour and renders](docs/tui-renders.md).
120×30 is a comfortable terminal size; larger windows keep a bounded reading
width. The installation screen highlights one next step from observed state: prepare,
start PXE, boot and install a computer, or recover normal networking.

### 6. Prepare and start installation mode

In that screen, prepare the immutable artifacts and client closures, review the
network transition, and type the displayed confirmation to start PXE. Existing
DHCP continues assigning leases.

### 7. Boot and install the clients

Enable UEFI network boot on a client. In the downloaded installer environment,
run:

```sh
/installer/setup.sh
```

Choose the same configured pilot identity and a target disk. Installation
begins only after you type a confirmation containing both values. Boot the
installed disk, return to the controller, and press `v` to check authenticated
system state. After that succeeds, perform the short practical checklist at
the client. You may install another computer or press `x` to stop installation
mode and finish with the remaining clients deferred. `q` reviews the
consequences and requires `LEAVE PXE ACTIVE` before closing while PXE remains
active. The selected identity and completed checks are stored in private
operator state, so reopening the TUI resumes the partial session. Evidence is
bound to the deployment revision and installed system path; changing the
laboratory revision requires a new selection and verification.

## Occasional interventions

Run `nix run .#nixorium` from the private deployment repository. The dashboard
provides the normal workflows:

- `nixorium setup` resumes the observed first-installation stage and any
  compatible per-computer installation evidence;
- on later openings, `Up`/`Down` selects an intervention and `Enter` opens it; the displayed
  one-letter shortcuts remain available;
- each workflow shows its available keys; `Esc` returns, and `q` quits outside
  text-entry fields;
- titles, sections, and ready/attention/failure colors form a consistent visual
  hierarchy, while the same state always remains written in text;
- reviews describe impact before mutation and require the displayed phrase;
- terminal results state what happened and expose relevant dashboard, Git
  review, log, retry, or further-editing actions instead of returning silently
  to the previous input screen.

Long builds use phase progress, elapsed time, and bounded recent activity.
Shorter waits whose work has no honest percentage use an animated spinner plus
their current plain-language action, so a remote terminal never looks frozen.

| Task | What it does |
|---|---|
| **Add or change software** | Searches pinned Nix packages, saves the reviewed scope, and builds/activates controller-affecting choices |
| **Install new computers** | Collects missing laboratory settings, prepares PXE, and guides locally confirmed client installation |
| **Distribute the prepared system** | Reviews and applies one, selected, or all client configurations with live phase, elapsed-time, verification, and recent-activity feedback |
| **Shut down computers** | Checks selected clients and sessions, then sends reviewed power-off requests without targeting the controller |
| **Advanced tools** | Opens restore, Update Nixorium, inventory, settings, controller rebuild, services, logs, changes, and diagnostics |

Operational commands, JSON output, customization examples, update procedure,
and recovery semantics live in the
[deployment administrator guide](templates/site/README.md). For a failed or
interrupted action, start with the
[troubleshooting and recovery guide](docs/troubleshooting.md).

## Safety model

- The public repository contains reusable implementation; site identity and
  policy stay in a private deployment repository.
- Private SSH, Harmonia, and Veyon keys never enter Git or the Nix store.
- Privileged actions use fixed systemd units and narrow polkit rules. The TUI
  calls typed application operations; it does not execute arbitrary shell
  commands as root.
- PXE writes a recovery record before changing the controller address and
  restores only the exact recorded address.
- Client installation is guided, destructive confirmation is explicit, and
  unattended installation is disabled.
- Client shutdown selects evaluated clients only, blocks active user sessions,
  requires explicit acknowledgement for unknown sessions, and never treats
  lost network contact as proof that a computer is powered off.
- The firewall exposes product services only on the configured lab interface
  and only on the roles that need them.
- Client closures are built by the controller and verified through the signed
  local cache; routine client operation does not gain an Internet dependency.

See the [management architecture](docs/management-architecture.md) and
[architecture decisions](docs/adr/) for the complete trust and privilege
boundaries.

## Documentation

| Document | Audience |
|---|---|
| [Deployment administrator guide](templates/site/README.md) | Setup, dashboard, CLI, customization, deployment, and upgrades |
| [Troubleshooting and recovery](docs/troubleshooting.md) | Symptoms, safe retries, backups, and restoration |
| [System and extension reference](docs/system-reference.md) | Accounts, storage, desktop services, Veyon, and `lib.mkLab` |
| [Hardware validation plan](docs/hardware-validation.md) | VirtualBox and physical PXE/install/deploy evidence |
| [Management architecture](docs/management-architecture.md) | Application layers, state models, privilege boundaries, and testing |
| [Architecture decisions](docs/adr/) | Accepted product decisions and their tradeoffs |
| [Changelog](CHANGELOG.md) | Release history and unreleased changes |
| [Contributor instructions](AGENTS.md) | Repository layout, coding rules, validation, and security invariants |

## Development

The public Flake exports `lib.mkLab`, standalone example systems, Colmena
metadata, netboot artifacts, the offline installer bundle, and the packaged
management application. Site-specific changes belong in a private deployment;
reusable behavior belongs here.

The public Nix evaluator and the management command enforce the same settings
boundary through a shared invalid-candidate regression corpus. GitHub CI both
evaluates the NixOS/template graph and separately builds and tests the packaged
Go command.

```sh
# Fast development gate; this is the normal edit-test loop
./scripts/validate.sh --quick

# Optional persistent shell for incremental Go test runs
nix --extra-experimental-features 'nix-command flakes' \
  develop --file tests/source-checks.nix go-shell

# Complete Nix API and host-composition evaluation
./scripts/validate.sh --eval

# Complete milestone/release checkpoint
./scripts/validate.sh --full
```

The targeted VM modes and the gate-selection rules are documented in
[Development validation](docs/development-validation.md). In particular,
`--full` is not intended for repeated use while editing.

Read [AGENTS.md](AGENTS.md) and the
[`nixorium-developer` skill](skills/nixorium-developer/SKILL.md) before changing
the public API, modules, installers, template, or release metadata.

## License

Released under the [MIT License](LICENSE).
