# Nixorium

[![Release](https://img.shields.io/github/v/release/giovantenne/nixorium?display_name=tag&sort=semver)](https://github.com/giovantenne/nixorium/releases/latest)
[![NixOS](https://img.shields.io/badge/NixOS-26.05-5277C3?logo=nixos&logoColor=white)](https://nixos.org)
[![Flakes](https://img.shields.io/badge/Nix-Flakes-4E9A06?logo=nixos&logoColor=white)](https://nixos.wiki/wiki/Flakes)
[![Deploy](https://img.shields.io/badge/Deploy-Colmena-2E3440)](https://github.com/zhaofengli/colmena)
[![License: MIT](https://img.shields.io/badge/License-MIT-2EA44F.svg)](./LICENSE)

A reproducible NixOS deployment system for multi-PC environments (classrooms, training rooms, public labs, libraries). Installation and system updates work over the LAN without client internet access; normal user sessions may still use the internet.

Website: [nixorium.org](https://nixorium.org)

The current stable release is **v1.0.0**. The NixOS 26.05, rootless Docker,
and Veyon 4.11 work is available for hardware testing as
**v2.0.0-beta.3**. Production installations should use a tagged release from
the [GitHub Releases page](https://github.com/giovantenne/nixorium/releases)
instead of tracking `master` directly.

One controller PC manages the entire lab: it builds all configurations locally, serves them over LAN, and deploys updates to every workstation declaratively. The whole lab can be reinstalled from scratch in under 20 minutes.

## 🧭 Why this exists

Managing a multi-PC lab is painful. Machines drift over time, reinstalling by hand is slow and error-prone, and keeping many systems consistent is a full-time job. Traditional tools like Ansible help, but they can't guarantee that two machines built a week apart end up identical.

NixOS solves this with **declarative, reproducible configurations** -- but most NixOS workflows assume internet access. In many schools, offices, and public labs, client PCs either have no internet at all or only get access after a user logs into an institutional network.

This project bridges that gap with a **local-first workflow**:

- A single controller PC acts as the build server, binary cache, and PXE boot server
- Client PCs are installed and updated entirely over the LAN
- One `flake.nix` file is the single source of truth for the entire lab
- Site configuration lives in a small private deployment Flake, while this repository remains a reusable versioned upstream

## ✨ Features

- **Zero-internet client installation** -- PXE/netboot + local binary cache (Harmonia), no USB drives needed per client
- **Single source of truth** -- one `flake.nix` generates all host configurations programmatically
- **Multi-machine orchestration** -- deploy updates to all PCs at once with Colmena
- **Student home directory reset** -- homes are restored to a clean template on every boot, with the last 5 sessions saved as Btrfs snapshots for recovery
- **Classroom management** -- Veyon is pre-configured with all lab PCs mapped, ready to use
- **Rootless containers** -- every normal user gets an isolated Docker daemon without root-equivalent `docker` group access
- **User-managed npm tools** -- global npm packages install under `~/.local/npm` without `sudo` or writes to the Nix store
- **Fully parameterizable** -- user names, PC count, network layout, passwords, locale, homepage, and more are all configurable from a single settings block
- **Dual networking** -- DHCP for institutional network integration + static IPs for the internal lab network
- **UEFI + Btrfs** -- modern boot with declarative disk partitioning (Disko) and snapshot support
- **GNOME desktop** -- pre-configured with dark theme, development tools, and terminal customization
- **Controller as teacher workstation** -- the controller is intended for instructor use and shows a user chooser at login (no autologin)

## 🏗️ Architecture

The accepted design for the management CLI/TUI, configuration boundary,
privilege separation, and recoverable PXE lifecycle is documented in
[the management architecture proposal](docs/management-architecture.md).
It is a target design; the external implementation status tracks which parts
have actually been delivered and validated.

```
┌──────────────────────────────────────────────────────────────────┐
│                      Controller (pcNN)                           │
│                                                                  │
│  Nix Flake ──► Build all configs ──► Harmonia (binary cache)     │
│                                      PXE/Netboot server          │
│                                      Colmena orchestration       │
└──────────────────────┬───────────────────────────────────────────┘
                       │ LAN (static IPs)
        ┌──────────────┼──────────────┐
        │              │              │
   ┌────▼────┐   ┌────▼────┐   ┌────▼────┐
   │  pc01   │   │  pc02   │   │  pcNN   │
   │ Student │   │ Student │   │ Student │
   │Workstat.│   │Workstat.│   │Workstat.│
   └─────────┘   └─────────┘   └─────────┘
```

| Component | Description |
|---|---|
| **Controller** | PXE/Netboot server, local binary cache, Colmena orchestration |
| **Client PCs** | Student workstations with Btrfs snapshots and home reset |
| **Networking** | Installation and system updates over LAN; user internet access is optional |
| **Boot mode** | UEFI only, declarative partitioning with Disko |

## 🚀 Quick start

Each installation uses two repositories: this public upstream and a small
private deployment repository generated from the `site` template. Do not fork
this repository just to configure a lab. The private repository owns the real
password hashes, public keys, assets and local NixOS modules, and pins an
upstream release through its `flake.lock`.

### 1. Bootstrap the controller from USB

> **Requires**: a NixOS live USB with temporary internet access. UEFI boot must be enabled.

Boot the controller PC from the official NixOS live USB, connect it to the
internet, and run one command:

```sh
curl -fsSL https://nixorium.org/install.sh | bash
```

The public entrypoint offers the current `master`, the latest published
prerelease, and every published stable release. Press Enter to accept the
displayed default. When no interactive terminal is available, the built-in
default release is selected automatically.
The script also enables the required Nix Flake features for its own commands.

For automation, or to select a compatible release, `master`, and disk
explicitly:

```sh
curl -fsSL https://nixorium.org/install.sh | \
  bash -s -- --release v2.0.0-beta.3 --disk /dev/sda
```

If one disk is detected, the installer selects it automatically; if multiple
disks are detected, it asks you to choose one.

Before partitioning, the bootstrap generates and commits a private deployment
from the template in the selected release. It installs the controller from
that deployment using the default placeholder settings in `lab-settings.json`.
SSH, binary cache, and Veyon public keys are added later, after step 4.

The bootstrap script forces `cache.nixos.org` during installation, so it does not depend on any LAN cache or substituter already configured in the live environment.

### 2. Review and publish the private deployment repository

Reboot and log in as `admin` (default password: `nixos`). The installer has
already created and initialized the deployment repository:

```sh
cd ~/nixorium-deployment
git status
git log --oneline -1
```

Create an empty **private** repository in your school organization, add it as
`origin`, and push this initial commit. Public forks are unsuitable because the
deployment contains password hashes and internal network details.

### 3. Configure the laboratory

Launch the guided first-run configuration from the deployment root:

```sh
nix run .#nixorium -- setup
```

The terminal wizard proposes the detected active interface and current IPv4
address on a fresh deployment, walks through network, account, locale,
homepage, Git, and Veyon settings, and supports backward navigation. Enter the
three normal passwords when prompted; input is not echoed and only salted
hashes reach `lab-settings.json`. The final screen shows a non-secret semantic
diff, separates setup-generated public-key changes from other worktree edits,
and writes only after explicit acceptance and complete Nix validation. Once
accepted, the full `setup` command also creates or verifies the three required
key pairs and tells you which public files to review and commit. Use `setup
configure` when only the settings stage should run.

The machine-facing review/apply API accepts a complete candidate settings file.
It validates the candidate through both the management schema and the actual
deployment Flake before showing a semantic diff. Password hashes are reported
only as `configured -> updated`.

```sh
nix run .#nixorium -- config plan --file candidate.json --json
nix run .#nixorium -- config apply --file candidate.json \
  --expect 'sha256:fingerprint-from-plan'
git diff -- lab-settings.json
```

`config apply` succeeds only with the exact base fingerprint returned by the
reviewed plan. It locks the deployment, compares the source again immediately
before its atomic write, preserves unrelated files, and reports a conflict if
the managed file changed. Candidate files must contain hashes, never plaintext
passwords; the guided setup will create those hashes internally.

You can leave the git identity fields at their defaults for now.

> **Note**: `masterDhcpIp` is used only during PXE/netboot client installation. The generated iPXE script, the netboot ramdisk, and the PXE helper services all point to that DHCP address, so if the DHCP lease changes before a netboot session you must update `lab-settings.json` and rebuild the netboot artifacts. Regular Colmena deploys use the controller's static lab IP instead.

### 4. Generate and install keys

Generate the three required key pairs. Keep the private files local to the controller, and add the public files to Git in the last command below so Nix can read them from the repo.

| Key pair | Private file | Public file / config | Purpose |
|---|---|---|---|
| **Binary cache** | `secret-key` | `keys/cache-public-key` | Harmonia signs Nix store paths; clients verify signatures |
| **SSH** | `admin-ssh` | `keys/admin-ssh.pub` | Admin SSH access + Colmena deploys (connects as `root`) |
| **Veyon** | `veyon-private-key.pem` | `keys/veyon-public-key.pem` | Veyon Master authenticates to student PCs |

Bare `setup` already performs this stage. It can also be run or retried
independently:

```sh
nix run .#nixorium -- setup keys
```

The command creates only missing pairs, restricts private files to mode
`0600`, verifies every public/private correspondence, and is safe to retry. It
never replaces an existing key. A public key without its private counterpart,
or a mismatched pair, stops reconciliation for explicit recovery.

Bare `setup` also requests the narrow privileged install action. It can be
retried independently without running the wizard:

```sh
nix run .#nixorium -- setup install-secrets
```

The systemd action reads only the controller's declaratively configured
`services.nixorium.deploymentPath` (default:
`/home/admin/nixorium-deployment`). It re-verifies every pair, installs the SSH
key for `admin`, the Veyon private key for `veyon-master`, and the Harmonia key
under `/var/lib/nixorium/keys`. Existing identical destinations are reused;
symlinks, unsafe sources, and different destination contents fail closed.

Nixorium evaluates private deployments through the local Git Flake fetcher.
Only version-controlled public configuration enters the Nix source/store;
ignored private key files remain outside it.

Commit only the public material and reviewed settings:

```sh

# Flakes ignore untracked files in a Git worktree, so add the public files
git add lab-settings.json keys/cache-public-key keys/admin-ssh.pub keys/veyon-public-key.pem
git commit -m "chore: configure lab and add public keys"
```

### 5. Rebuild the controller

Apply the committed configuration through the reviewed management action:

```sh
nix run .#nixorium -- setup apply
```

The command requires a clean Git worktree, valid/readiness-complete settings,
verified source keys, and matching installed secrets. It identifies this
controller from `labMeta`, shows the expected networking/service impact, and
requires typing `APPLY`. The fixed systemd action builds as the unprivileged
`admin` user and activates exactly that resulting NixOS closure as root. It is
safe to retry after a failed build or activation; inspect durable output with
`journalctl -u nixorium-apply-controller.service`. Non-interactive automation
must opt in explicitly with `--yes`.

The applied controller configuration owns Harmonia as a persistent systemd
service. It loads the installed signing key through systemd credentials, never
from the Git/Nix source, and restarts after process failures. Inspect it with:

```sh
systemctl status nixorium-harmonia.service
journalctl -u harmonia.service
nix run .#nixorium -- doctor
```

Prepare every netboot artifact and client closure through the managed action:

```sh
nix run .#nixorium -- pxe prepare
journalctl -u nixorium-prepare-pxe.service
```

Preparation is non-disruptive and safe to retry. It requires a clean,
deployment-ready Git revision, the configured DHCP address on the configured
interface, and a healthy Harmonia endpoint. It builds the kernel, initrd,
generated iPXE script, pinned iPXE firmware, and every configured client
closure without creating `result-*` links. The immutable store paths and Git
revision are retained with managed garbage-collector roots and recorded
atomically in
`/var/lib/nixorium/prepared/prepared.json`; `status`, `doctor`, and the PXE
proxy reject a stale or invalid record. If the DHCP lease changed, update and
commit `masterDhcpIp`, apply the controller, and run preparation again.

### 6. Start netboot services

Harmonia is already running under systemd after `setup apply`. Start the
managed ProxyDHCP + TFTP + HTTP installation mode through Nixorium:

```sh
nix run .#nixorium -- pxe start
nix run .#nixorium -- status
```

The command performs its readiness checks before showing the exact interface
and address transition, then requires the confirmation `START PXE`. For
explicit automation, `--yes` skips only that confirmation. When installation
is finished, one command stops the listeners and synchronously restores normal
addressing:

```sh
nix run .#nixorium -- pxe stop
```

`pxe start` and `pxe stop` are safe to retry. A failed start rolls back the
network transition before returning. After an interrupted session, reboot
recovery runs automatically; `nix run .#nixorium -- pxe recover` performs the
same explicit reconciliation on demand. Detailed service failures remain in
journald.

The compatibility `run-pxe-proxy` Flake app remains available for advanced
foreground diagnostics, but it does not own the transactional network change.

### 7. Install client PCs

On each client PC, enable **UEFI network boot** in the BIOS/firmware settings. The PC will PXE-boot into a NixOS ramdisk environment.

On the booted client, start guided enrollment without arguments:
```sh
/installer/setup.sh
```

The installer shows firmware, system, CPU, memory, network interfaces, and all
writable disks. Choose an identity from the configured host inventory and then
the target disk. A reachable target identity is refused as a likely duplicate;
a silent identity is explicitly reported as an unverified best-effort result,
not a reservation. Before Disko runs, you must type a phrase containing both
the exact hostname and disk, for example `ERASE /dev/sda INSTALL pc05`.

Before offering a disk, the installer resolves the selected system entirely
offline, verifies that its prepared store closure is available, and requires
enough target capacity for that closure plus 2 GiB of installation headroom.
The exact verified store path is then passed to `nixos-install`; missing paths
are never built or fetched by the client.

Progress is shown for partitioning, installation, and verification. On success,
the installer asks separately whether to reboot. You may preselect the identity
and disk with `/installer/setup.sh pc05 /dev/sda`, but the same destructive
review and typed confirmation are always required. There is no unattended mode.

When all clients are installed, run `nix run .#nixorium -- pxe stop` so normal
controller addressing is restored before deploying with Colmena.

---

## 🔧 Maintenance

### Management commands

Run these commands from the private deployment root:

```sh
nix run .#nixorium
nix run .#nixorium -- status
nix run .#nixorium -- status --json
nix run .#nixorium -- hosts
nix run .#nixorium -- deploy plan --on pc05
nix run .#nixorium -- deploy plan --on pc01,pc02
nix run .#nixorium -- deploy plan --on @lab
nix run .#nixorium -- deploy apply --on @lab --expect REVISION_FROM_PLAN
nix run .#nixorium -- controller plan
nix run .#nixorium -- controller apply --expect REVISION_FROM_PLAN
nix run .#nixorium -- services
nix run .#nixorium -- services restart cache
nix run .#nixorium -- setup
nix run .#nixorium -- setup status
nix run .#nixorium -- setup keys
nix run .#nixorium -- setup install-secrets
nix run .#nixorium -- setup apply
nix run .#nixorium -- pxe prepare
nix run .#nixorium -- pxe start
nix run .#nixorium -- pxe stop
nix run .#nixorium -- pxe recover
nix run .#nixorium -- config validate
nix run .#nixorium -- config plan --file candidate.json
nix run .#nixorium -- doctor
```

The dashboard and inspection commands evaluate `labMeta` and
`deploymentStatus`. Its **View computers** screen explicitly probes every
configured client and distinguishes a reachable SSH service, a reachable host
that refuses SSH, an unreachable host, and an unknown probe result. The same
typed inventory is available through `nixorium hosts` and `--json`; probes run
with bounded concurrency only when this view/command or `doctor` is requested,
so the initial dashboard and `status` remain predictable and probe-free. For
hosts whose SSH port is reachable, the explicit inventory also authenticates
as the existing Colmena `root` identity and invokes the fixed read-only
`nixorium-host-state` helper. Every deployed generation embeds its private
deployment Git revision; comparing that observed revision with the current
clean-review revision yields `current`, `outdated`, or `unknown` without
evaluating/building all client closures. Systems installed before this helper
was introduced remain `unknown` until their next normal deployment. The report
also shows each host's last successful post-apply verification when one has
been recorded; this local history is informational and never overrides the
live authenticated state.
Before using Colmena, `deploy plan --on ...` expands one, several, or all
configured clients into a reviewable revision-bound target list. Planning is
read-only and fails closed when the deployment is not ready, the Git worktree
is dirty, HEAD cannot be resolved, or a selector is empty, duplicate, or
unknown. It always states that selected configurations must build before
deployment and prints the exact revision-bound apply command. `deploy apply`
repeats readiness, clean-Git, revision, and target checks, requires the exact
`DEPLOY <targets>` confirmation, runs verbose `colmena build` before
`colmena apply switch`, and streams output while retaining a private log under
`~/.local/state/nixorium/operations/`. `--yes` is an explicit automation-only
confirmation; `--json` keeps the final report on stdout and progress on stderr.
After every apply attempt, Nixorium queries the selected hosts through the
authenticated state helper and atomically records only hosts that report the
reviewed revision and a concrete system path under
`~/.local/state/nixorium/deployments/`. Complete success requires every target
to be verified and recorded. An apply failure still reports potentially mixed
state and may record independently verified targets; review the log and current
host state, make a fresh plan, and retry the convergent workflow.
The dashboard's **Deploy updates** screen exposes the same workflow without
copying a revision: select one or more computers (or all), review the resolved
plan, and type the exact `DEPLOY <targets>` phrase. It shows operation activity
and the final phase, build/apply state, authenticated/recorded target counts,
remediation, and durable log path.
Closing the dashboard is disabled while its Colmena child is running.
For routine controller changes, `controller plan` reviews the ready, clean
deployment revision and whether its evaluated system is already active.
`controller apply --expect <revision>` repeats that preflight, requires exact
`REBUILD <controller>` confirmation, and starts only a revision-named systemd
unit. The privileged service pins the Git fetcher to that commit, builds as the
deployment owner, refuses worktree or HEAD drift before activating the exact
closure, and the application verifies `/run/current-system` afterward. The
dashboard's **Rebuild controller** task uses the same typed workflow; the
systemd-owned job and journal survive closing the dashboard. `--yes` remains an
explicit automation-only confirmation.
`services` reports the persistent signed binary cache and the composite
on-demand PXE lifecycle with raw unit states and operator-friendly health.
`services restart cache` requires exact `RESTART CACHE` confirmation, starts
only the fixed capability-free `nixorium-restart-cache.service` action, and
returns success only after Harmonia is active and HTTP-ready. Direct generic
service names and PXE unit actions are rejected; use the transactional `pxe`
workflow for installation mode. The dashboard's **Manage services** screen
uses the same typed status/restart operations. `--yes` is reserved for
intentional automation.
From the dashboard, **Install computers over network**
uses the same typed application operations as the CLI to prepare artifacts and
to review, start, stop, or recover PXE mode. Starting requires the exact
`START PXE` confirmation; quitting the view does not stop systemd-owned
services. Setup configuration and key generation are explicit, unprivileged
mutations of the private deployment. `config validate` reads the machine-owned
settings without changing them, rejects unknown or invalid values, and then
evaluates the deployment through Nix as the final authority. The current `doctor`
checks readiness, Git state, subnet/interface/address ownership, Harmonia key
correspondence and cache health, netboot artifacts, PXE port conflicts, client
SSH reachability, managed-service availability, disk space, and required local
commands including Colmena. `nixorium doctor --full` additionally performs a
real controller build. The default remains quick and read-only.

`setup status` reconciles observed state on every run and selects the earliest
incomplete first-run stage. It currently reports environment, network,
identity/locale, default credentials, key correspondence and private modes,
candidate validation, artifacts, and deployment readiness. Git review is
complete only when the deployment worktree is clean. Controller apply is a
fixed, confirmed systemd action; guided installation remains an explicit
future stage, and no global `configured` flag is trusted. Bare `setup`
continues from accepted settings to
key reconciliation; `setup keys` exposes that same explicit, unprivileged
create-new operation independently. `setup install-secrets` starts the fixed,
sandboxed systemd action authorized for wheel administrators by a unit-specific
polkit rule; it never accepts a destination or command argument.
`setup apply` similarly accepts no executable or machine target, requires all
earlier observed setup stages, and starts only
`nixorium-apply-controller.service`. Status marks the stage complete only when
`/run/current-system` matches the evaluated controller generation.
`pxe prepare` starts only the fixed, administrator-owned preparation unit. It
does not alter networking; its current manifest is exposed in `status --json`
as `pxePreparation` and checked by `doctor`.

The `nixorium` executable is installed on the generated controller system. It
uses `NIXORIUM_REPO` when set, otherwise the current deployment root or the
installer's default `~/nixorium-deployment` location. `--repo <path>` always
selects an explicit deployment.

`labMeta.clients.hosts` provides the generated hostname/IP inventory used by
diagnostics without parsing Nix source. The controller configuration installs
both `nixorium` and the Colmena version pinned by nixpkgs, so routine
diagnostics and later deployment workflows do not need an ad-hoc online
`nix run nixpkgs#colmena` lookup.

### Update a lab deployment

Lab administrators update the pinned input in their private repository; they do
not merge this upstream into their configuration. Run the following commands
from the private deployment repository.

In this example the new upstream release is `v2.0.0-beta.4`. Replace it with
the tag you actually want to install. The upgrade branch keeps `master`
unchanged while the new release is tested:

```sh
git switch master
git pull --ff-only
git switch -c upgrade/nixorium-v2.0.0-beta.4
```

Open `flake.nix` and replace the current tag in this line:

```nix
inputs.nixorium.url = "github:giovantenne/nixorium/v2.0.0-beta.4";
```

Then update only the `nixorium` lock entry and inspect the result:

```sh
nix flake update nixorium
git diff -- flake.nix flake.lock
```

Before accepting the upgrade, build one client, the controller, the netboot
ramdisk and the offline installer bundle:

```sh
nix build .#nixosConfigurations.pc01.config.system.build.toplevel --no-link
CONTROLLER_NAME=$(nix eval .#labMeta.controller.name --raw --no-write-lock-file)
nix build ".#nixosConfigurations.${CONTROLLER_NAME}.config.system.build.toplevel" --no-link
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --no-link
nix build .#pxeFirmware --no-link
nix build .#installerBundle --no-link
```

If every build succeeds, commit the two changed files and merge the tested
branch into `master`:

```sh
git add flake.nix flake.lock
git commit -m "chore: update nixorium to v2.0.0-beta.4"
git switch master
git merge --ff-only upgrade/nixorium-v2.0.0-beta.4
git push origin master
```

These commands update the deployment repository only. Apply the configuration
to the lab PCs separately, after reviewing the upgrade.

### Releases and versioning

Nixorium follows [Semantic Versioning](https://semver.org/). `VERSION` is the canonical project version and every release must have a matching dated entry in `CHANGELOG.md`.

GitHub Releases are generated automatically when a tag named `v<version>` is pushed. The workflow validates that the tag, `VERSION`, and `CHANGELOG.md` agree, then publishes the matching changelog section as the release notes. Releases contain GitHub's source archives; Nix build outputs remain reproducible from `flake.lock` and are distributed to clients through Harmonia.

To prepare a release:

1. Update `VERSION` according to SemVer.
2. Move the relevant entries from `Unreleased` to a dated section in `CHANGELOG.md`.
3. Update `DEFAULT_RELEASE` in `install.sh`, the tag in
   `templates/site/flake.nix`, and release examples when appropriate.
4. Run `./scripts/validate.sh --full` and commit the release metadata.
5. Push `master`, then publish the tag:

```sh
./scripts/release.sh 1.1.0
```

The script refuses to release from a dirty tree, from a branch other than `master`, or when local `HEAD` differs from `origin/master`.

The project version is also exposed without building a system closure:

```sh
nix eval .#labMeta.version --raw
```

### Deploy updates (Colmena)

Review the exact committed target set first:

```sh
nix run .#nixorium -- deploy plan --on @lab
nix run .#nixorium -- deploy apply --on @lab --expect REVISION_FROM_PLAN
```

The plan prints the exact apply command with its full Git revision. Applying
requires an exact target-specific confirmation, builds first, streams verbose
progress, records the detailed log path, then authenticates every selected
host and records only hosts running the reviewed revision. A stale plan
or dirty worktree is rejected before Colmena runs. For non-interactive
automation, add `--yes`; the expected revision remains mandatory.

### Rebuild the controller

Review and activate the exact committed controller revision:

```sh
nix run .#nixorium -- controller plan
nix run .#nixorium -- controller apply --expect REVISION_FROM_PLAN
```

The apply runs through a narrow revision-bound systemd instance, builds the
pinned Git source as the deployment owner, rejects repository drift before
activation, and verifies the active system after completion. Use the
**Rebuild controller** dashboard task for the same reviewed workflow.

During first-run setup, the equivalent setup-stage action remains:
```sh
nix run .#nixorium -- setup apply
```

Then verify the managed binary cache:
```sh
systemctl is-active nixorium-harmonia.service
nix run .#nixorium -- doctor
```

The following raw commands remain available as advanced manual operations.
They bypass Nixorium's review binding, operation lock, and durable result
summary.

Deploy to all lab PCs manually:
```sh
nix run nixpkgs#colmena -- apply --impure --on @lab
```

Deploy to a single PC:
```sh
nix run nixpkgs#colmena -- apply --impure --on pc05
```

### Manual rebuild

Use `nixos-rebuild` only on the machine you are rebuilding.

Rebuild the controller locally:
```sh
CONTROLLER_NAME=$(nix eval .#labMeta.controller.name --raw --no-write-lock-file)
sudo nixos-rebuild switch --flake ".#${CONTROLLER_NAME}" --no-write-lock-file
```

For client PCs, prefer Colmena from the controller. Only run `sudo nixos-rebuild switch --flake /path/to/nixorium-deployment#pc05 --no-write-lock-file` after logging into `pc05` itself (or after cloning the deployment there).

---

## ⚙️ Configuration reference

### User accounts

| User | Role | Details |
|---|---|---|
| `admin` | System administrator | SSH access, sudo, Veyon Master access |
| Teacher (configurable) | Instructor | Veyon Master access, persistent home, snapshot bookmark |
| Student (configurable) | Student | Autologin on client PCs, home reset at every boot |
| `root` | System | Password disabled, SSH key access only |

### Disk layout (Disko)

All machines must boot in **UEFI mode**. Disk partitioning is declarative via `disko-uefi.nix`, which wraps the shared pure layout in `lib/disko-layout.nix`.

| Partition | Filesystem | Mount point |
|---|---|---|
| EFI System Partition | FAT32 | `/boot` |
| Root partition | Btrfs | -- |

Btrfs subvolumes:

| Subvolume | Mount point |
|---|---|
| `@root` | `/` |
| `@home-<studentUser>` | `/home/<studentUser>` |
| `@snapshots` | `/var/lib/home-snapshots` |

### Home reset and snapshots

The student home directory resets to a clean template on every boot:

- **Template**: generated at activation time with git config, VS Code settings and extensions, and XDG directories
- **Snapshots**: the last 5 sessions are saved in `/var/lib/home-snapshots/` (accessible by `admin`, the teacher user, and `root`)

The teacher user has a **Snapshots** bookmark in the Nautilus sidebar.

Docker images, npm's download cache, and user-installed global npm packages
are intentionally excluded from snapshots. They are runtime artifacts and are
removed together with the rest of the student home at the next boot.

To recover student work from a previous session:
```sh
ls /var/lib/home-snapshots/snapshot-1/
cp /var/lib/home-snapshots/snapshot-1/file.txt /home/<studentUser>/
```

### Rootless Docker

Docker runs as a per-user systemd service. No account belongs to the `docker`
group, which would otherwise provide root-equivalent access to the machine.
The login environment points Docker and Compose to the current user's socket.

```sh
docker run --rm hello-world
docker-compose up
```

Rootless containers cannot bind ports below 1024 without additional host
configuration. For student projects, publish services on ports such as 3000,
8080, or 8000. Images and containers are isolated between users; student data
under `~/.local/share/docker` is ephemeral because the student home resets.

### Global npm packages

Node.js comes from the current NixOS stable package set. Global npm installs
use the writable per-user prefix `~/.local/npm`, already present in `PATH`.
Never use `sudo npm install -g`.

```sh
# Install or upgrade command-line tools directly from the public npm registry
npm install -g @openai/codex@latest
npm install -g @anthropic-ai/claude-code@latest

# Inspect and update all user-installed global packages
npm outdated -g
npm update -g
```

The clients need internet access only while downloading npm packages. These
installs are intentionally session-local for the reset student account; source
files saved in the home snapshots remain recoverable.

### Veyon (classroom management)

Veyon 4.11.0 is pinned from its official flake because it is still absent from
nixpkgs. The lab overlay adds the missing PipeWire build dependency and NixOS
setuid wrappers for Veyon's authentication and Wayland input helpers. The
`veyon-service` user unit runs on every graphical session and accepts Veyon
connections on port **11100**.

#### Configuration

- All PCs have `veyon-service` running and the public key deployed via Nix
- `Veyon.conf` is generated with all lab PCs pre-mapped under the hardcoded location name `Lab`
- The Veyon private key is only needed on the controller -- student PCs only have the public key
- Users in the `veyon-master` group (`admin` and the teacher user) can access Veyon Master
- The default backend remains the unattended GNOME Remote Desktop VNC bridge on port 5900
- Hosts listed in `veyonNativeHosts` use Veyon's native PipeWire/XDG portal backend and close port 5900

GNOME does not currently support unattended portal pre-authorization for this
backend, so a native pilot host will display a screen-sharing consent dialog.
Keep `veyonNativeHosts = [];` for production until this is resolved upstream.

### Customizing packages and desktop

The default desktop is GNOME (Wayland) with a curated set of development tools.
Keep every site-specific change in the private deployment repository:

- **All machines**: add settings and packages to `modules/shared.nix`
- **Controller only**: use `modules/controller.nix` for printers and local services
- **Clients only**: use `modules/clients.nix`
- **Single machine**: add a module through `hostModules.pcNN` in `flake.nix`
- **Screensaver**: replace `assets/logo.txt`
- **Wallpapers and desktop files**: pass replacement paths through the `assets` argument in `flake.nix`

To customize only `pc05`, create `modules/pc05.nix` in the private deployment:

```nix
{ pkgs, ... }:
{
  environment.systemPackages = [ pkgs.vlc ];
}
```

Then reference that file from the existing `hostModules` attribute in
`flake.nix`:

```nix
hostModules = {
  pc05 = [ ./modules/pc05.nix ];
};
```

The host name must be generated by the deployment; for example, `pc05`
requires `pcCount` to be at least `5`. Track both files, validate the host, and
deploy only that machine:

```sh
git add flake.nix modules/pc05.nix
nix build .#nixosConfigurations.pc05.config.system.build.toplevel --no-link
colmena apply --on pc05
```

Changes that are useful to every deployment belong in this upstream. Changes
that identify or specialize one school belong in its private repository.

### Agent skills

The upstream includes two separate Agent Skills. `nixorium-developer` covers
the public API, built-in modules, installer, CI and releases.
`nixorium-maintainer` covers configuration and operation of one private lab.
Only the lab-maintenance skill is copied into new deployment repositories.

### Deployment Flake API

`lib.mkLab` accepts the following extension points:

| Argument | Purpose |
|---|---|
| `deploymentSelf` | The downstream Flake `self`, used to package its files for offline installation |
| `labConfig` | Required typed site settings, loaded from `lab-settings.json` in new managed deployments or provided directly by legacy deployments |
| `publicKeys` | Cache, SSH and Veyon public-key paths |
| `assets` | Logo, wallpaper list, MIME defaults and VS Code settings |
| `sharedModules` | NixOS modules applied to every host |
| `controllerModules` | Modules applied only to the controller |
| `clientModules` | Modules applied only to client PCs |
| `hostModules` | Attribute set of modules keyed by host name |
| `netbootModules` | Additional modules applied to the PXE system |

The site template demonstrates every commonly needed argument. Configuration
fields are validated, and unknown settings, host names and asset names fail
evaluation instead of being silently ignored. `deploymentStatus` reports
remaining placeholders, missing public keys and unchanged default passwords.
Files referenced by `publicKeys`, `assets` and module lists
must live in the deployment repository (or in this upstream), because `mkLab`
packages those source trees into the offline PXE installer.

---

## 📁 Project structure

```
.github/workflows/validate.yml # Evaluation-only CI for source and deployment template
.github/workflows/release.yml # Revalidates release metadata and publishes GitHub Releases
install.sh                  # Public entrypoint for controller bootstrap
flake.nix                  # Entry point: host generation, Colmena config, labMeta export
flake.lock                 # Pinned inputs (nixpkgs, Disko, Veyon)
VERSION                    # Canonical Semantic Version
CHANGELOG.md               # Curated release notes
LICENSE                    # MIT license
lab-config.nix             # Standalone example configuration for this upstream
disko-uefi.nix             # NixOS wrapper for the shared Disko layout
lib/
  disko-layout.nix         # Shared Disko layout function (device + student user)
  eval-lab-config.nix      # Typed schema and validation for site configuration
  eval-lab-settings.nix    # Versioned JSON envelope validation
  mk-lab.nix               # Reusable host/netboot/Colmena output constructor
setup.sh                   # Client PC installer (runs on PXE-booted machines)
pkgs/
  gnome-remote-desktop.nix # gnome-remote-desktop overlay (VNC + multi-session)
modules/
  common.nix               # Composition and shared system defaults
  desktop.nix              # GNOME, locale, fonts and desktop policy
  firewall.nix             # Interface-scoped role-specific service policy
  packages.nix             # Shared package set
  power.nix                # Idle and controller sleep policy
  screensaver.nix          # Screensaver files and user service
  shell.nix                # Shell, prompt, Git and editor tooling
  ssh.nix                  # SSH client and server policy
  hardware.nix             # Generic hardware detection
  networking.nix           # Hostname + static IP per host
  management.nix           # Controller management CLI and fixed actions
  pxe.nix                  # Transactional PXE address state and boot recovery
  users.nix                # User accounts and autologin
  cache.nix                # Controller Harmonia service + client cache trust
  filesystems.nix          # Btrfs support
  home-reset.nix           # Student home templating + boot-time reset
  docker.nix               # Per-user rootless Docker daemon
  development.nix          # Writable npm global prefix and PATH
  veyon.nix                # Veyon service, keys, and classroom config
scripts/
  release.sh               # Validates, tags, and publishes a release
  install-controller.sh    # Controller bootstrap from live USB
  run-harmonia.sh          # Advanced standalone Harmonia compatibility helper
  run-pxe-proxy.sh         # ProxyDHCP + TFTP + HTTP netboot server
  lib/lab-meta.sh          # Shared helper: loads labMeta from the flake
  cmd-screensaver.sh       # TTE screensaver animation loop
  launch-screensaver.sh    # Fullscreen Ghostty screensaver launcher
  screensaver-monitor.sh   # GNOME idle watcher for screensaver
  create-home-template.sh  # Home directory template builder
  home-reset.sh            # Boot-time snapshot rotation + home reset
  validate.sh              # Tiered upstream validation entry point
assets/
  backgrounds/             # Wallpapers (randomly selected at home reset)
  logo.txt                 # ASCII art for screensaver
  mimeapps.list            # Default applications
  vscode-settings.json     # VS Code defaults
templates/site/            # Scaffold for a private deployment repository
skills/nixorium-developer/ # Public upstream development and release workflow
skills/nixorium-maintainer/ # Private laboratory maintenance workflow
docs/                       # Product architecture and architecture decisions
```

The root configuration keeps the historical standalone deployment working.
New installations should use `templates/site` and keep generated public keys in
the private deployment repository.

## 🔒 Security

- **Never commit** `secret-key`, `admin-ssh`, or `veyon-private-key.pem`
- Commit only their public counterparts under `keys/`
- Keep the deployment repository private because password hashes and network topology are still sensitive operational data
- Passwords are SHA-512 hashed; never store plaintext
- SSH password authentication is disabled; key-based only
- SSH host keys are recorded on first connection and checked thereafter
- `users.mutableUsers = false` enforces declarative user management
- Docker is rootless and no normal user belongs to the root-equivalent `docker` group
- The Veyon private key is readable only by the `veyon-master` group
- The firewall exposes SSH, mDNS, Veyon and optional VNC only on `ifaceName`;
  Harmonia and PXE ports are additionally controller-only

## 📄 License

Released under the [MIT License](./LICENSE).
