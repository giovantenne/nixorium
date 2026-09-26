# Nixorium system and extension reference

This document describes the core workstation mechanisms and supported extension
points provided by the public Nixorium framework. The generated private
deployment owns the actual workstation profile. Day-to-day commands belong in
the [deployment administrator guide](../templates/site/README.md); architecture
and privilege boundaries belong in the
[management architecture](management-architecture.md).

## Accounts

| Account | Default role |
|---|---|
| `admin` | System administrator, SSH access, sudo, and Veyon Master access |
| Teacher account | Persistent instructor workspace and Veyon Master access |
| Student account | Client autologin, reset home, and read-only host networking |
| `root` | Disabled password and key-only SSH access |

Teacher and student names are site settings. Accounts are declarative and
`users.mutableUsers = false`; changing credentials therefore requires a
reviewed configuration update rather than an imperative password edit.

Admin and teacher belong to the `networkmanager` group. The student does not,
and an explicit polkit rule denies every `org.freedesktop.NetworkManager.*`
action for that identity. The session can use the system-managed connection and
inspect ordinary network status, but cannot change connections, radios, DNS, or
other NetworkManager state through GNOME, `nmcli`, `nmtui`, or direct D-Bus
requests.

## Storage and boot

Nixorium requires UEFI boot. Disko creates an EFI System Partition mounted at
`/boot` and one Btrfs partition containing:

| Subvolume | Mount point |
|---|---|
| `@root` | `/` |
| `@home-<studentUser>` | `/home/<studentUser>` |
| `@snapshots` | `/var/lib/home-snapshots` |

The same pure Disko layout is used by controller bootstrap and guided client
installation, so destructive tooling comes from the deployment's locked inputs
instead of being downloaded at installation time.

The public controller bootstrap resolves its selected branch or tag once. Both
bootstrap stages use only the official NixOS binary cache and its published
signing key; signature checking is never disabled. The
template, installer, local Disko layout, and initial lock use that full Git
revision, while `flake.nix` retains the selected channel for later managed
updates. Resolution failure stops before the installer is invoked.
For bootstrap-capability version 1, account and regional input is collected by
the small shell launcher before it invokes Nix. Version 2 keeps that sequence,
then selects and reviews the initial deployment-owned software profile from the
same immutable template revision. It writes only ordinary `shared` declarations
for the complete selected profile to `lab-software.json` before Git
initialization, Nix evaluation, or disk changes. Version 1 remains supported
without a software prompt. The supported
bootstrap environment is the official NixOS Minimal ISO booted in UEFI mode,
which starts in the required Linux text console. Keyboard is collected and
activated first, then time zone is collected before account details, so all
subsequent input uses the installed controller layout. The PXE environment
later applies that same console keymap before local client enrollment. The
destructive confirmation precedes partitioning, locking, or
controller-system transfer. Once Disko has mounted the target, the bootstrap
places its evaluation cache and temporary swap there; `nixos-install` builds
directly into the target store with one Nix job and one core per build.

## Package-base ownership

New deployments own a direct `nixpkgs` pin on the channel advertised by
`nixorium.lib.packageBase`. Nixorium's transitive consumers follow it, and the
deployment exposes `nixoriumPackageBase` with its locked revision. Framework
updates fail if their candidate lock changes that root node. Legacy deployments
without the direct input remain supported; migration and package-base update
are separate reviewed operations.

The advertised channel is reference metadata, not a hard compatibility gate.
`package-base status/plan/apply` and Maintenance → Update system and packages
manage the direct base independently, preserving every other locked node.
Channel changes require explicit unverified-compatibility acknowledgement.
`nixoriumUpdateTargets` covers the controller and distinct client variants
(scoped software, Veyon mode, interface and explicit host modules); private
host-conditional policies declare additional `mkLab.updateValidationHosts`.
`nixoriumOfflineCheck`
compares direct and standalone-installer derivations in an isolated offline
store. Read [updates](updates.md) for migration, activation, package overrides,
runtime checks and recovery. Framework updates can still change framework-owned
patches and auxiliary inputs without changing nixpkgs.

## Workstation-profile ownership

Nixorium core supplies the GNOME session, accounts, networking, classroom
control, installation, deployment, and reset mechanisms. The private deployment
template supplies the user-facing profile: packages in `lab-software.json`,
suggestions in `software-catalog.nix`, desktop and shell policy under
`modules/`, and branding/editor defaults under `assets/`.

`mkLab` passes each host's effective managed package IDs to downstream modules
as `hostSoftwarePackages`. The template uses this list to enable Docker, npm
setup, application favorites, shortcuts, browser policy, VS Code extensions,
and the site screensaver only when their packages apply to that host. Every
built-in profile, including Essential, declares the Ghostty and TTE dependencies
required by the screensaver.
Removing a declaration therefore does not leave an upstream launcher or service
behind.

Desktop Icons NG and Dash to Dock are baseline workstation components rather
than application-profile choices. The template installs and enables both for
every role, keeps the dock visible outside the GNOME overview, and reapplies the
required extension state at login without disabling unrelated user extensions.
This keeps files under the XDG Desktop directory visible for reset student
homes and persistent staff homes alike.

The [reference profile measurement](profile-closure-measurements.md) compares
complete Essential and Programming client closures under one documented lock.
Closure size, transfer estimates and elapsed build time have different cache
semantics and are reported separately.

## Network interfaces

`lab.ifaceName` is the compatibility fallback for every host. Deployments may
set `controllerIfaceName` and `clientIfaceName` as role defaults and use
`hostIfaceNames.<hostname>` for exceptional hardware. Resolution order is host
override, role override, then fallback. `labMeta.network.ifaceName` remains the
effective controller interface for older management consumers; controller and
client records also expose their effective interface explicitly.

## USB/SSH client installation boundary

The supported remote-install environment is the official NixOS 26.05 Minimal
ISO for `x86_64-linux`, booted with UEFI and wired Ethernet. The controller
must be a configured laboratory controller with a healthy signed Harmonia
cache; controller-only deployments and arbitrary rescue environments are not
accepted. This workflow does not depend on PXE services or alter the
controller's static address.

`packages.x86_64-linux.remoteInstallerBundle` contains the target-independent
remote helper, metadata, and Disko programs. It deliberately excludes a client
system closure and can be prepared before the live address is known. After a
credential-free SSH host-key observation has been compared with the
physical-console fingerprint and explicitly confirmed, the controller builds the exact
selected client's closure and the live helper copies only signed paths from
Harmonia with substituter fallback disabled. The helper excludes the ISO boot
medium and validates the chosen disk, NIC, cache public key, deployment
revision, and closure immediately before mutation.

The public CLI surface is `install usb prepare/start/status/reconcile/reboot/
verify/cancel/close`. `prepare` and `start` take `--host`; later actions take
the returned `--id`. `start`, `reboot`, and `close` require `/dev/tty`, have no
JSON or unattended form, and collect secrets only through the terminal.
`status`, `reconcile`, `verify`, and `cancel` support `--json`. The installed
controller owns `nixorium-remote-install.service`; its private socket is
`/run/nixorium/remote-install/control.sock`, durable state is under
`/var/lib/nixorium/remote-install`, and volatile keys/password material remains
under `/run`.

An interrupted post-dispatch operation is never replayed. Reconciliation
observes the exact operation receipt and reports whether disk mutation may have
started. A definitive failed receipt with no mutation remains safely
cancellable and normally triggers automatic live-key, preparation-root, and
reservation cleanup; uncertain or post-mutation failures do not. A controller
restart may reattach only after the controller re-observes the key and the
operator physically reconfirms the same live boot; reboot and known-host
rotation remain separately confirmed. Ordinary changes to an installed client
use `deploy`, not this destructive installation API.

## Student home reset

At boot, the previous student home becomes one of five rotating snapshots and
a clean home is restored from the activation-time template. Core creates the
configured Git identity and standard XDG directories; deployment modules add
editor and MIME defaults when those applications are selected.

Activation also repairs ownership and user-write access on the managed VS Code,
npm, and XDG roots for admin, teacher, and student accounts. If a runtime
directory already exists, only its top-level ownership and mode are reconciled;
logind remains responsible for creating `/run/user/<uid>` and its contents.

Administrators and the teacher can recover a file from the snapshot store:

```sh
ls /var/lib/home-snapshots/snapshot-1/
cp /var/lib/home-snapshots/snapshot-1/file.txt /home/<studentUser>/
```

Docker images, npm's download cache, and user-installed global npm packages are
runtime data inside the student home. They are intentionally removed on the
next reset; source files remain recoverable through the snapshots.

## Rootless Docker

The default site profile gives each normal account its own rootless Docker user service and subordinate
UID/GID range. No account belongs to the root-equivalent `docker` group. Docker
and Compose use the current user's socket automatically.

```sh
docker run --rm hello-world
docker-compose up
```

Rootless containers cannot bind privileged ports below 1024 without additional
site policy. Prefer ports such as 3000, 8000, or 8080 for student projects.

## User-managed npm tools

When Node.js is selected by the site profile, global npm installs use
`~/.local/npm`, which is already in `PATH`. They never
need `sudo` and do not write into the Nix store.

```sh
npm install -g <package>@latest
npm outdated -g
npm update -g
```

Downloading packages requires Internet access for that user session. Packages
installed by the reset student account are intentionally ephemeral.

## Veyon classroom management

Nixorium pins Veyon 4.11.3 from its official Flake and supplies the NixOS overlay
and wrappers required for PipeWire, authentication, and Wayland input. Every
graphical host runs `veyon-service`; the generated configuration maps all
configured clients under the `Lab` location.

- Clients receive only the Veyon public key.
- The controller receives the private key outside Git and the Nix store.
- The `veyon-master` group grants key access to `admin` and the teacher.
- Every lab host uses native PipeWire/portal capture; no external VNC server,
  shared password or port 5900 is enabled.
- Client TCP ports 22 and 11100 accept only the controller's static IPv4
  address on the configured lab interface. IPv6 and other sources are denied.
  Veyon's internal capture and feature ports remain on loopback.
- Controller SSH, Veyon, discovery, cache and PXE remain interface-scoped.

Native hosts include upstream commit `22218d772dba639819938911b47ab80924c6c87f`
and enable `PipeWireVnc/PersistRestoreToken`. GNOME still requires initial
interactive approval of screen sharing and input access. Subsequent starts can
restore that grant; revocation or a changed monitor can require approval again.

Veyon's token and the PermissionStore service's portal grants live under
`/var/lib/nixorium/veyon-session/<user>/`, with separate mode-0700 directories
for each managed desktop user. A user-service startup link exposes only Veyon's
state in the home. The permission service alone receives a persistent
`XDG_DATA_HOME`; application data and other student-home content still reset.
Portal permissions (including grants for other applications) therefore persist
on native hosts, outside home templates and snapshots. Initial rollout uses a
fresh permission store and can require reapproval of existing portal grants.
Never clone this state between users or machines or place tokens in Git.

The old `veyonNativeHosts` field is accepted only for configuration
compatibility and no longer selects a backend. It is absent from new templates
and settings screens. After deployment, approve GNOME sharing locally, then
test monitoring, input, lock/unlock, demo, service restart, logout/login and
reboot/home reset. The controller also needs approval for screen broadcasts.
Veyon still uses RFB internally; removing the bridge does not remove that
protocol or its local native implementation.

## Temporary client Internet access

The administrator's Computers → Internet access action uses authenticated SSH
and the root-only `nixorium-internet` helper. Requests bind to the observed
client boot ID; an old request cannot reapply a block after reboot. The
`nixorium-internet-block.service` unit is never enabled for boot. It owns only
the `inet nixorium_internet` table, which is retained across ordinary firewall
reloads and removed on unblock. The controller itself is never a target.

Output and forwarded traffic outside the configured lab IPv4 subnet is
blocked, including established connections and IPv6 Internet traffic.
Loopback, IPv4 DHCP, IPv6 DHCP and neighbor discovery remain available.
This is destination-based network control: allowed laboratory services remain
reachable, including any proxy deliberately hosted there. No gateway or
NetworkManager connection is rewritten. Reboot restores normal connectivity.

## Private deployment customization

Do not fork or edit upstream modules for site policy. New installations create
a private deployment with these extension points:

- `modules/shared.nix` for every machine;
- `modules/controller.nix` for the controller only;
- `modules/clients.nix` for all client PCs;
- `hostModules` in `flake.nix` for a generated individual host;
- `lab-software.json`, `software-catalog.nix`, and optional
  `software-presets.json` for package and initial-profile choices;
- the focused modules and assets already shipped in the private template.

For example, add VLC only to `pc05` with `modules/pc05.nix`:

```nix
{ pkgs, ... }:
{
  environment.systemPackages = [ pkgs.vlc ];
}
```

Then reference it from the deployment Flake:

```nix
hostModules = {
  pc05 = [ ./modules/pc05.nix ];
};
```

The name must already be generated by the configured client count. Track both
files, validate the host, and deploy only that target:

```sh
git add flake.nix modules/pc05.nix
nix build .#nixosConfigurations.pc05.config.system.build.toplevel --no-link
nix run .#nixorium -- deploy plan --on pc05
```

Additional backgrounds can be passed without changing upstream:

```nix
assets = {
  logo = ./assets/logo.txt;
  backgrounds = [
    ./assets/backgrounds/one.jpg
    ./assets/backgrounds/two.jpg
  ];
};
```

Keys, assets, and downstream modules referenced by the deployment must remain
inside its source tree. `lib.mkLab` packages that tree into the offline
installer so client installation needs no second checkout.

## `lib.mkLab` extension points

| Argument | Purpose |
|---|---|
| `deploymentSelf` | Downstream Flake `self`, used when packaging deployment files for offline installation |
| `labConfig` | Required typed site settings, normally decoded from `lab-settings.json` |
| `labSoftware` | Strict versioned supported client-package declarations, normally decoded from `lab-software.json` |
| `softwareCatalog` | Deployment-owned suggested packages shown before free search; each entry has `id`, `label`, and `summary` |
| `softwarePresets` | Optional versioned deployment-owned software-profile catalog; each profile has an ID, label, description, and package IDs |
| `clientGroups` | Named sets of evaluated client identities available to software scopes |
| `publicKeys` | Harmonia, SSH, and Veyon public-key paths |
| `assets` | Logo, wallpapers, MIME defaults, and editor settings |
| `homeResetEphemeralPaths` | Deployment-owned relative cache/tool paths excluded before student snapshots |
| `sharedModules` | NixOS modules applied to every generated host |
| `controllerModules` | Modules applied only to the controller |
| `clientModules` | Modules applied to all client PCs |
| `hostModules` | Attribute set of modules keyed by generated host name |
| `updateValidationHosts` | Extra generated hosts with private host-conditional policy; included in base-update builds and offline equivalence |
| `netbootModules` | Additional modules applied only to the PXE environment |

Unknown settings, host names, and asset names fail evaluation. Referenced
filesystem paths must belong to either the upstream or private deployment
source tree. `deploymentStatus` reports placeholders, public default password
hashes, and missing public keys separately from successful Flake evaluation.
The public template's seven profiles are fully expanded deployment data.
Essential is the default for new repositories and its eight `shared`
declarations match the initial `lab-software.json`; existing repositories are
never rewritten when they update Nixorium. Profile metadata is serialized into
the offline installer source alongside the effective package declarations.
All seven profiles include `git`, `nodejs`, `pi-coding-agent`, and `opencode`;
only Programming includes `vscode` and its toolchain-coupled extension payload.
The controller management module also installs Git independently of profile
selection, and the packaged Nixorium command carries it in its runtime PATH.
Global npm uses each user's `~/.local/npm`, which precedes the system PATH.
Staff overrides persist. The reset student profile recreates an empty prefix
and excludes npm globals plus Pi/OpenCode credentials and state from snapshots,
so no shared template or historical snapshot captures agent authentication.

The Software TUI loads the optional catalog only when the operator chooses
**Add profile**. Package inclusion, scope and the full additive candidate are
separate stages; one review distinguishes additions from existing declarations
whose scopes remain unchanged. The TUI saves and records the batch through the
typed preset boundary, then reuses the ordinary controller recovery and fresh
client-deployment review. Missing metadata leaves individual software
management available, and controller-only results never offer a client
distribution action when no client is affected.

## Public Flake outputs

The upstream keeps its standalone example evaluable while private deployments
consume `lib.mkLab`. Important generated outputs include:

- `nixosConfigurations.<host>` for the controller, clients, and netboot system;
- Colmena deployment metadata and target groups;
- `labMeta` for non-sensitive operational identity and network data;
- `deploymentStatus` for readiness blockers;
- `nixoriumSoftware` for the supported pinned catalog, evaluated scopes, and managed declarations;
- `nixoriumSoftwarePresets` for the normalized optional profile catalog, or `null` when a deployment does not provide one;
- `nixoriumUpdateTargets` and `nixoriumOfflineCheck` for base-update validation;
- `nixorium` and supporting Flake applications;
- `pxeFirmware`, `installerBundle`, and the target-independent
  `remoteInstallerBundle` for USB/SSH live-ISO installation;
- schema, compatibility, package, and VM checks.

For an unchanged lock and site configuration, a client evaluated through the
private deployment and through the offline installer bundle must produce the
same `system.build.toplevel.drvPath`. Repository validation enforces this
invariant.

## Agent skills

The public repository contains two different Agent Skills:

- [`nixorium-developer`](../skills/nixorium-developer/SKILL.md) governs public
  API, built-in module, installer, CI, and release work.
- [`nixorium-maintainer`](../skills/nixorium-maintainer/SKILL.md) governs one
  private laboratory's configuration and operation.

Only the maintainer skill is copied into generated deployment repositories.
