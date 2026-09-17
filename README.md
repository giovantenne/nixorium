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

Development note: the controller-first foundation supports explicit
`lab.deploymentMode = "controller"` with `pcCount = 0`. Such a controller can be
rebuilt without client networking or lab keys, with lab services and firewall
openings inactive. Omission preserves laboratory behavior. Installer prompts
and the five-task TUI are not integrated yet; use the current flow below.
See [ADR 0015](docs/adr/0015-controller-first-capabilities.md).

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
- GNOME workstations with Veyon classroom management, rootless Docker, and
  user-managed npm tools.
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

Choose the release and controller disk when prompted. The installer creates a
private deployment repository and installs the controller from its pinned
inputs.

### 3. Reboot

Remove the USB and sign in as `admin`. A fresh installation initially uses the
password `nixos`; the setup wizard replaces it with the password you choose.

### 4. Start first-run setup

```sh
cd ~/nixorium-deployment
nix run .#nixorium -- setup
```

The wizard proposes the interface carrying the default route and its live DHCP
address, groups essential network, laboratory, account, regional, browser, and
Veyon settings, and offers searchable offline choices for locale, time zone,
and keyboards. It leaves optional Git author identity at the template defaults
during first run, hashes passwords without echoing them, retries a short or
mismatched password without discarding earlier answers, creates the required
key pairs, and presents a redacted review.

After configuration, the same command opens a resumable setup checklist. Press
`Enter` on its highlighted next step to review and commit the generated public
configuration, activate the controller, prepare installation files, and open
the first network installation. Choose a pilot identity from the saved
inventory, complete identity and disk confirmation locally on that computer,
then ask the controller to check its authenticated active revision. The
technical check remains separate from the practical login, desktop, software,
network, and peripheral check. Each disruptive action still has its own review
and confirmation. You may quit at any safe point and rerun the command; it
continues from observed system and Git state. Keep the deployment repository
**private**.

### 5. Open Nixorium

```sh
nix run .#nixorium
```

Nixorium asks what intervention you want to perform; it does not scan the room
or turn powered-off computers into an alarm. Restore, guided software changes,
distribution, network installation, Nixorium updates, and advanced tools are
separate choices. `?` opens help; `F1` also works during text entry. Computers
is an explicit advanced check with search, selection, technical details, and a
deployment route for the focused computer. Setup groups its observed checks into
five operator stages, with `t` for the technical checklist.
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
| **Restore computers** | Chooses explicitly between reapplying the intended system and selecting one evaluated identity for a locally confirmed disk-erasing reinstall |
| **Add or change software** | Chooses a supported pinned package and configuration scope, validates it, and saves only the reviewed declaration |
| **Distribute the prepared system** | Reviews and applies one, selected, or all client configurations with live phase, elapsed-time, verification, and recent-activity feedback |
| **Computer inventory** | Explicitly authenticates reachable hosts and compares their active revision with the desired Git revision |
| **Rebuild controller** | Builds and activates an exact reviewed revision, refreshes status, and offers dashboard, detail, log, or retry actions |
| **Manage services** | Inspects PXE and the signed cache; performs a bounded cache restart |
| **View operation logs** | Shows private, bounded deployment logs and typed action history |
| **Review Git changes** | Displays redacted deployment changes and optionally creates a local reviewed commit |
| **Change settings** | Edits one grouped area, including Git identity or one securely entered account password, then validates and reviews the complete candidate |
| **Update Nixorium** | Fetches upstream `master` and releases, then validates and applies the selected target |
| **Shut down computers** | Checks selected clients and sessions, then sends reviewed power-off requests without targeting the controller |
| **Install or reinstall computers** | Prepares, starts, stops, or recovers PXE installation mode |

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

```sh
# Fast development gate
./scripts/validate.sh --quick

# Go tests in the development environment
nix develop --command go test ./...

# Full milestone/release gate
./scripts/validate.sh --full
```

Read [AGENTS.md](AGENTS.md) and the
[`nixorium-developer` skill](skills/nixorium-developer/SKILL.md) before changing
the public API, modules, installers, template, or release metadata.

## License

Released under the [MIT License](LICENSE).
