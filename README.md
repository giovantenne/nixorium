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
that deployment using the default placeholder settings in `lab-config.nix`.
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

### 3. Edit `lab-config.nix`

Now you have all the values you need. Find your DHCP address and interface name:
```sh
ip -4 addr
```

Generate the password hashes before filling the three password fields below. The default password for all users is `nixos`:

```sh
# Hashed passwords (run once per user, paste each hash into lab-config.nix)
mkpasswd -m sha-512
```

Edit `lab-config.nix` with your lab's settings:

```nix
# ── Network ────────────────────────────────────────────────────
masterDhcpIp = "MASTER_DHCP_IP";   # DHCP address of controller (from ip -4 addr)
networkBase = "10.0.0.0";           # Static IPv4 network address
networkPrefixLength = 24;            # CIDR prefix for the static network
pcCount = 20;                       # Number of student PCs
masterHostNumber = 99;              # Controller PC number
ifaceName = "enp0s3";               # Network interface name (from ip -4 addr)

# ── User accounts ─────────────────────────────────────────────
teacherUser = "teacher";            # Teacher account name
studentUser = "student";            # Student account name

# ── Passwords (SHA-512 hashed) ────────────────────────────────
# Default is "nixos" for all accounts. Generate your own with: mkpasswd -m sha-512
teacherPassword = "...";
studentPassword = "...";
adminPassword = "...";

# ── School / organization ─────────────────────────────────────
homepageUrl = "https://nixorium.org";

# ── Locale / timezone ─────────────────────────────────────────
timeZone = "Europe/Rome";
defaultLocale = "en_US.UTF-8";
extraLocale = "it_IT.UTF-8";
keyboardLayout = "it";
consoleKeyMap = "it2";

# Leave empty for unattended GNOME operation. Add pilot host names to test
# Veyon 4.11's native Wayland backend, for example [ "pc01" ].
veyonNativeHosts = [];
```

You can leave the git identity fields at their defaults for now.

> **Note**: `masterDhcpIp` is used only during PXE/netboot client installation. The generated iPXE script, the netboot ramdisk, and the PXE helper services all point to that DHCP address, so if the DHCP lease changes before a netboot session you must update `lab-config.nix` and rebuild the netboot artifacts. Regular Colmena deploys use the controller's static lab IP instead.

### 4. Generate and install keys

Generate the three required key pairs. Keep the private files local to the controller, and add the public files to Git in the last command below so Nix can read them from the repo.

| Key pair | Private file | Public file / config | Purpose |
|---|---|---|---|
| **Binary cache** | `secret-key` | `keys/cache-public-key` | Harmonia signs Nix store paths; clients verify signatures |
| **SSH** | `admin-ssh` | `keys/admin-ssh.pub` | Admin SSH access + Colmena deploys (connects as `root`) |
| **Veyon** | `veyon-private-key.pem` | `keys/veyon-public-key.pem` | Veyon Master authenticates to student PCs |

Generate everything from scratch:

```sh
# Binary cache signing key for Harmonia
nix key generate-secret --key-name lab-cache-key > secret-key
nix key convert-secret-to-public < secret-key > keys/cache-public-key

# Admin SSH key used by Colmena / SSH access
ssh-keygen -t ed25519 -f admin-ssh -N '' -C 'admin@controller'

# Veyon RSA keypair
openssl genrsa -out veyon-private-key.pem 4096
openssl rsa -in veyon-private-key.pem -pubout -out keys/veyon-public-key.pem

# SSH private key -- used by Colmena to connect as root to all PCs
install -m 600 -D admin-ssh ~/.ssh/id_ed25519

# SSH public key -- commit it in the deployment and install it for SSH tooling
install -m 644 -D admin-ssh.pub keys/admin-ssh.pub
install -m 644 -D admin-ssh.pub ~/.ssh/id_ed25519.pub

# Veyon private key -- only needed on the controller (where Veyon Master runs)
# Only users in the veyon-master group (admin + teacher) can read it
sudo install -d -m 0750 -g veyon-master /etc/veyon/keys/private/teacher
sudo install -m 0640 -g veyon-master veyon-private-key.pem /etc/veyon/keys/private/teacher/key

# Flakes ignore untracked files in a Git worktree, so add the public files
git add keys/cache-public-key keys/admin-ssh.pub keys/veyon-public-key.pem
git commit -m "chore: add lab public keys"
```

### 5. Rebuild the controller

Before starting PXE, temporarily remove the controller's static lab IP from the shared interface. The generated netboot artifacts refer to `masterDhcpIp`, so this keeps PXE, HTTP, and binary-cache traffic on that single DHCP address during installation. The change is temporary and a reboot restores the static IP automatically.

```sh
# Rebuild the controller with your real config
CONTROLLER_NAME=$(nix eval .#labMeta.controller.name --raw --no-write-lock-file)
sudo nixos-rebuild switch --flake ".#${CONTROLLER_NAME}" --no-write-lock-file

# Build netboot artifacts
nix build .#nixosConfigurations.netboot.config.system.build.kernel --out-link result-kernel
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --out-link result-initrd
nix build .#nixosConfigurations.netboot.config.system.build.netbootIpxeScript --out-link result-ipxe

# Install iPXE bootstrap binary
nix build nixpkgs#ipxe --out-link result-ipxe-bin
install -D -m 0644 result-ipxe-bin/snp.efi assets/ipxe/snponly.efi

# Pre-build all client closures
PC_COUNT=$(nix eval .#labMeta.clients.count --json --no-write-lock-file)
TARGETS=()
for i in $(seq 1 "$PC_COUNT"); do
  TARGETS+=(".#nixosConfigurations.pc$(printf "%02d" "$i").config.system.build.toplevel")
done
nix build "${TARGETS[@]}"

# Temporarily remove the lab static IP so netboot uses masterDhcpIp only
STATIC_IP=$(nix eval .#labMeta.controller.staticIp --raw --no-write-lock-file)
PREFIX_LENGTH=$(nix eval .#labMeta.network.prefixLength --json --no-write-lock-file)
IFACE=$(nix eval .#labMeta.network.ifaceName --raw --no-write-lock-file)
sudo ip addr del "${STATIC_IP}/${PREFIX_LENGTH}" dev "${IFACE}"
```

### 6. Start netboot services

Open **two separate terminals**:

**Terminal 1** -- Binary cache:
```sh
nix run .#run-harmonia
```

**Terminal 2** -- ProxyDHCP + TFTP + HTTP netboot server:
```sh
sudo nix run .#run-pxe-proxy
```

> Both processes run in the foreground. Keep the terminals open during client installation.

### 7. Install client PCs

On each client PC, enable **UEFI network boot** in the BIOS/firmware settings. The PC will PXE-boot into a NixOS ramdisk environment.

On the booted client:
```sh
/installer/setup.sh XX
```
Where `XX` is the PC number (e.g., `/installer/setup.sh 5` for `pc05`).

> `setup.sh` auto-selects the disk if only one is present; if multiple disks are detected, it asks for a choice.

When all clients are installed, restore the controller's static lab IP so Colmena can reach the lab subnet again (or just reboot it):
```sh
STATIC_IP=$(nix eval .#labMeta.controller.staticIp --raw --no-write-lock-file)
PREFIX_LENGTH=$(nix eval .#labMeta.network.prefixLength --json --no-write-lock-file)
IFACE=$(nix eval .#labMeta.network.ifaceName --raw --no-write-lock-file)
sudo ip addr add "${STATIC_IP}/${PREFIX_LENGTH}" dev "${IFACE}"
```

---

## 🔧 Maintenance

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
4. Run `./scripts/validate.sh` and commit the release metadata.
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

First apply the latest configuration on the controller itself:
```sh
CONTROLLER_NAME=$(nix eval .#labMeta.controller.name --raw --no-write-lock-file)
sudo nixos-rebuild switch --flake ".#${CONTROLLER_NAME}" --no-write-lock-file
```

Then start the binary cache:
```sh
nix run .#run-harmonia
```

Deploy to all lab PCs:
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
| `labConfig` | Required typed site settings imported from `lab-config.nix` |
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
.github/workflows/validate.yml # Builds hosts and verifies the offline bundle
.github/workflows/release.yml # Revalidates tags and publishes GitHub Releases
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
  mk-lab.nix               # Reusable host/netboot/Colmena output constructor
setup.sh                   # Client PC installer (runs on PXE-booted machines)
pkgs/
  gnome-remote-desktop.nix # gnome-remote-desktop overlay (VNC + multi-session)
modules/
  common.nix               # Composition and shared system defaults
  desktop.nix              # GNOME, locale, fonts and desktop policy
  packages.nix             # Shared package set
  power.nix                # Idle and controller sleep policy
  screensaver.nix          # Screensaver files and user service
  shell.nix                # Shell, prompt, Git and editor tooling
  ssh.nix                  # SSH client and server policy
  hardware.nix             # Generic hardware detection
  networking.nix           # Hostname + static IP per host
  users.nix                # User accounts and autologin
  cache.nix                # Binary cache client configuration
  filesystems.nix          # Btrfs support
  home-reset.nix           # Student home templating + boot-time reset
  docker.nix               # Per-user rootless Docker daemon
  development.nix          # Writable npm global prefix and PATH
  veyon.nix                # Veyon service, keys, and classroom config
scripts/
  release.sh               # Validates, tags, and publishes a release
  install-controller.sh    # Controller bootstrap from live USB
  run-harmonia.sh          # Binary cache server
  run-pxe-proxy.sh         # ProxyDHCP + TFTP + HTTP netboot server
  lib/lab-meta.sh          # Shared helper: loads labMeta from the flake
  cmd-screensaver.sh       # TTE screensaver animation loop
  launch-screensaver.sh    # Fullscreen Ghostty screensaver launcher
  screensaver-monitor.sh   # GNOME idle watcher for screensaver
  create-home-template.sh  # Home directory template builder
  home-reset.sh            # Boot-time snapshot rotation + home reset
  validate.sh              # Full upstream validation matrix
assets/
  backgrounds/             # Wallpapers (randomly selected at home reset)
  logo.txt                 # ASCII art for screensaver
  mimeapps.list            # Default applications
  vscode-settings.json     # VS Code defaults
templates/site/            # Scaffold for a private deployment repository
skills/nixorium-developer/ # Public upstream development and release workflow
skills/nixorium-maintainer/ # Private laboratory maintenance workflow
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

## 📄 License

Released under the [MIT License](./LICENSE).
