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
| Teacher account | Persistent instructor workspace, Veyon Master, and restricted classroom controls |
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

On the controller, `admin` owns the mode-0700 private deployment and the SSH
identity used to manage clients. The teacher instead belongs to the
`nixorium-classroom` group. Running `nixorium` without a readable deployment
connects to a group-private local worker and opens only inventory, temporary
Internet control, and reviewed client shutdown/restart. The worker runs as
`admin`, validates every target against the fixed deployment, uses only fixed
SSH helper/systemd commands, and shares the normal client-operation lock.
Student accounts cannot traverse the runtime directory or connect to its
socket. The worker does not expose settings, software, Git, installation,
deployment, update, log, or controller-maintenance operations.

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

Desktop Icons NG, Dash to Dock and Tiling Assistant are baseline workstation
components rather than application-profile choices. The template enables all
three for every role, keeps a compact bottom dock visible when its area is clear,
hides it behind overlapping windows, and preserves unrelated enabled extensions. MoreWaita icons complement
native Adwaita decorations and a static blue vector wallpaper for persistent
staff accounts. Reset student homes choose randomly from the deployment-owned
backgrounds at boot. A one-time
appearance migration lets persistent staff preferences survive later logins.
This keeps files under the XDG Desktop directory visible for reset student
homes and persistent staff homes alike.

### Measuring profile size

Measure the current template instead of using historical profile sizes. In a
disposable deployment, keep the same lock, settings and target for each profile:

```sh
SYSTEM=$(nix build \
  path:.#nixosConfigurations.pc01.config.system.build.toplevel \
  --print-out-paths --no-write-lock-file --no-link)
nix path-info --json-format 1 --json --closure-size "$SYSTEM"
```

Use `scripts/configure-software-profile.sh software-presets.json lab-software.json`
in that disposable deployment to select another profile, then repeat. Record
the upstream revision, lock and store/cache condition. Closure size includes
the common NixOS/GNOME base; it is neither download size nor total disk usage.
Time builds separately and measure network transfer independently. A warm
build reuses existing store paths, so its duration is not an installation-time
estimate.

## Network interfaces

`lab.ifaceName` is the compatibility fallback for every host. Deployments may
set `controllerIfaceName` and `clientIfaceName` as role defaults and use
`hostIfaceNames.<hostname>` for exceptional hardware. Resolution order is host
override, role override, then fallback. `labMeta.network.ifaceName` remains the
effective controller interface for older management consumers; controller and
client records also expose their effective interface explicitly.

## USB/SSH client installation boundary

The supported remote-install environment is the official NixOS 26.05 Minimal
ISO for `x86_64-linux`, booted with UEFI and Ethernet or Wi-Fi connectivity.
The controller must be a configured laboratory controller with a healthy signed Harmonia
cache; controller-only deployments and arbitrary rescue environments are not
accepted. This workflow does not depend on PXE services or alter the
controller's static address.

The declared client interface must carry the reviewed live IPv4 address.
Connect Wi-Fi in the live ISO before the SSH workflow and arrange persistent
system connectivity separately; live profiles and credentials are not copied.
After reboot, the configured static client address must be reachable for
verification. Keep Wi-Fi secrets out of Git and the Nix store. PXE may be
incompatible with Wi-Fi depending on hardware, firmware, and network setup;
that restriction is not part of USB/SSH transport.

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

The administrator and teacher Computers → Internet access action uses authenticated SSH
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
| `workspaceProfileJSON` | Optional raw JSON text for preparation of student preferences; `null` preserves legacy behavior |
| `workspaceCatalog` | Deployment-owned baseline/application/extension catalog required with workspace JSON; no implicit package installation |
| `workspaceRuntimeEnabled` | Explicit default-off switch for restoring a validated workspace at normal boot; requires workspace JSON |
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

### Workspace preparation

Preparation and runtime activation are separate. The template ships a catalog
and an inactive Essential example, never an active `workspace-profile.json`.
With no `workspaceProfileJSON`, `nixoriumWorkspace`
is `null` and existing home/template/login behavior remains unchanged. With a
profile, the output reports `state = "prepared"`, the configured `studentUser`,
all target hosts, declared/effective preferences, normalized catalog and pinned
package/extension versions. `runtimeEnabled` records the separate runtime
choice; `seed` is the immutable build-output path when enabled, otherwise null.
Neither field reports live installed or active state.

`workspaceProfileJSON` must be raw JSON text, limited to 64 KiB, with integer
`schemaVersion: 1`. Unknown fields, duplicate keys (including escaped aliases),
nulls and wrong types are rejected. Optional sections are:

- `desktop`: ordered unique `favorites` (at most 32 desktop IDs), `colorScheme`
  (`light`/`dark`), boolean `enableAnimations`, and `dock`. Dock supports
  `position` (`top`/`bottom`/`left`/`right`), integer `iconSize` (16–128) and
  booleans `autoHide`, `extendHeight`, `showTrash`, `showMounts`.
- `vscode`: unique `extensions` (at most 64 lowercase `publisher.extension`
  IDs, normalized in sorted order) and allowlisted `settings`: `editor.fontSize`
  (8–40), `editor.tabSize` (1–8), booleans `editor.insertSpaces`,
  `editor.formatOnSave`, `editor.minimap.enabled`, `editor.wordWrap`
  (`off`/`on`/`wordWrapColumn`/`bounded`), and `files.autoSave`
  (`off`/`onFocusChange`/`onWindowChange`).
- `browser`: `defaultApplication`, a desktop ID catalogued as a browser.

Identifiers have a 128-byte ASCII limit. Desktop IDs end in `.desktop` and
cannot contain paths, whitespace or control characters. There are no arbitrary
files, commands, account selectors, download URLs, or whole-home imports.

The catalog is a Nix attribute set containing `schemaVersion = 1`, `baseline`
(a profile attribute set using the same schema), `applications` and `extensions`.
Both lists are limited to 128 unique entries. Applications contain `id`
(desktop ID), `package` (pinned package attribute) and optional boolean `browser`.
Extensions contain `id`, matching `package = "vscode-extensions.<id>"`, and
optional `requiredPackages` and `requiredExtensions` lists. Dependencies must
be present explicitly; they are never installed or selected automatically.
Fields omitted from the profile inherit the baseline; explicit lists replace
baseline lists, including `[]` to clear them. An empty section does not erase
baseline fields. Catalog declarations do not prove plugin compatibility or
complete manifest dependencies.

Every selected application/runtime must be in each generated host's declarative
`environment.systemPackages`, including the controller in controller-only mode.
Selected package identities must match the shared pin; different downstream
package overrides are not silently treated as equivalent. VS Code preferences
require `vscode`, desktop preferences require `gnome-shell`, and dock preferences
require `gnomeExtensions.dash-to-dock`. Broken, insecure, unsupported-platform
or unresolved required packages fail preparation. No live hosts are contacted.

`nixoriumValidateWorkspaceCandidate` accepts raw candidate JSON text and checks
the same schema, catalog and all-host prerequisites without saving or deploying.
The candidate hook does not qualify extension loading or mutate sessions.
Pins exposing `lib.workspaceCandidateVersion` also provide
`nixoriumResolveWorkspaceCandidate`, with the same raw JSON argument and core
validation. It returns the candidate's complete preparation metadata, including
declared/effective preferences, catalog, resolved versions, targets and runtime
choice, without saving the candidate or changing current metadata. Consumers
must still invoke `nixoriumValidateWorkspaceCandidate` to preserve any additional
validation supplied by the deployment. Resolved metadata alone is not a review
token or proof that the deployment stayed unchanged while evaluating; a save
workflow must bind and recheck its source, pin, base file and exact candidate.
Both profile text and catalog data travel through the offline installer. The
template passes its catalog to supporting pins even before the first profile,
so that hook can validate a candidate without creating an active file. It reads
only `workspace-profile.json`, never `workspace-profile.example.json`, as the
declaration, and rejects a profile if the pin lacks this capability. Older pins
without a profile receive no new arguments. Do not remove operational home
modules on the assumption that preparation replaces them. The
[migration review](../skills/nixorium-maintainer/references/student-home.md#review-a-migration)
distinguishes representable preferences from retained policy and unsupported
legacy content.

#### Explicit managed-home runtime

`workspaceRuntimeEnabled = true` requires a profile and a pin exposing
`lib.workspaceRuntimeVersion`. It applies to the configured student on every
client and the controller, including controller-only mode; it does not change
controller autologin or staff preferences. Both the flag and seed metadata are
preserved by offline reconstruction. Merely updating a preparation-only
deployment never enables it.

The immutable seed combines supported preferences with a neutral shell/Git/XDG
scaffold. XDG folders use stable English names. It does not copy a live home,
the writable legacy template, or arbitrary application assets. Extension links
point to identity-checked pinned store payloads, and editor update checks start
disabled as editable session defaults. This is not proof that every plugin
loads correctly or works offline; qualify each supported plugin separately.
Wallpapers are composed with the profile dconf source before restoration.

Review existing local `home-profile.nix` and `workstation.nix` before opting in:
older private copies do not update with the upstream pin. The current template
skips student template writes, ownership repair, and supported login migrations
in managed mode, while retaining staff behavior and desktop extension enablement.
Keep desired preferences explicit in the profile/baseline. Missing settings use
system/application defaults, not an inferred translation of old Nix modules.

Build and deploy through the existing reviewed controller/client flows. The
new home becomes available at the next normal boot, not on save or rebuild.
Both reset implementations avoid automatic restart during a configuration
switch, including transitions in either direction. Students may change their
initial preferences during the session; the next reset restores the seed.

The managed reset requires the declared Btrfs home/snapshot mounts, exact
account ownership, a closed login barrier, no student processes or lingering,
and a fully checked seed/home tree. Ephemeral paths must be canonical relative
paths, non-overlapping and unique, at most 128 entries and 4096 bytes each.
Nested mounts/subvolumes and intermediate symlinks fail closed. Unsupported
kernel confinement APIs do not fall back to unsafe removal.

Before changing the original home, it creates a private snapshot, removes the
configured ephemeral data from that copy and makes it read-only. Five managed
snapshots are published under `/var/lib/home-snapshots/workspace`, separate
from legacy snapshots. Teacher access remains through `veyon-master`.
Reset success records bind the seed, user and boot ID under the root-only
`/var/lib/home-snapshots/.workspace-reset` directory. They are not fleet status
and are not currently consumed by the TUI.

An incomplete attempt leaves `pending.json` and recovery data, blocks normal
login and refuses subsequent attempts, including after reboot or a switch
back to the legacy path. The root-only helper is not an interactive reset or
recovery command. Inspect the `home-reset.service` journal and preserve the
private evidence before administrator-led recovery; never delete the marker
or rerun the reset just to clear an error. Snapshot recovery, external backups
and reverting the declared profile are distinct operations.

## Public Flake outputs

The upstream keeps its standalone example evaluable while private deployments
consume `lib.mkLab`. Important generated outputs include:

- `nixosConfigurations.<host>` for the controller, clients, and netboot system;
- Colmena deployment metadata and target groups;
- `labMeta` for non-sensitive operational identity and network data;
- `deploymentStatus` for readiness blockers;
- `nixoriumSoftware` for the supported pinned catalog, evaluated scopes, and managed declarations;
- `nixoriumSoftwarePresets` for the normalized optional profile catalog, or `null` when a deployment does not provide one;
- `nixoriumWorkspace` and `nixoriumValidateWorkspaceCandidate` for optional preparation metadata and prerequisite validation, not activation;
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
