# NixOS Lab

[![NixOS](https://img.shields.io/badge/NixOS-25.11-5277C3?logo=nixos&logoColor=white)](https://nixos.org)
[![Flakes](https://img.shields.io/badge/Nix-Flakes-4E9A06?logo=nixos&logoColor=white)](https://nixos.wiki/wiki/Flakes)
[![Deploy](https://img.shields.io/badge/Deploy-Colmena-2E3440)](https://github.com/zhaofengli/colmena)
[![License: MIT](https://img.shields.io/badge/License-MIT-2EA44F.svg)](./LICENSE)

A reproducible NixOS deployment system for multi-PC environments (classrooms, training rooms, public labs, libraries) with **no internet access on client machines**.

One controller PC manages the entire lab: it builds all configurations locally, serves them over LAN, and deploys updates to every workstation declaratively. The whole lab can be reinstalled from scratch in under 20 minutes.

## 🧭 Why this exists

Managing a multi-PC lab is painful. Machines drift over time, reinstalling by hand is slow and error-prone, and keeping many systems consistent is a full-time job. Traditional tools like Ansible help, but they can't guarantee that two machines built a week apart end up identical.

NixOS solves this with **declarative, reproducible configurations** -- but most NixOS workflows assume internet access. In many schools, offices, and public labs, client PCs either have no internet at all or only get access after a user logs into an institutional network.

This project bridges that gap with a **local-first workflow**:

- A single controller PC acts as the build server, binary cache, and PXE boot server
- Client PCs are installed and updated entirely over the LAN
- One `flake.nix` file is the single source of truth for the entire lab
- Everything is parameterizable -- user names, passwords, network settings, locale -- so you can fork this repo and adapt it to your environment in minutes

## ✨ Features

- **Zero-internet client installation** -- PXE/netboot + local binary cache (Harmonia), no USB drives needed per client
- **Single source of truth** -- one `flake.nix` generates all host configurations programmatically
- **Multi-machine orchestration** -- deploy updates to all PCs at once with Colmena
- **Student home directory reset** -- homes are restored to a clean template on every boot, with the last 5 sessions saved as Btrfs snapshots for recovery
- **Classroom management** -- Veyon is pre-configured with all lab PCs mapped, ready to use
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
| **Networking** | Installation and updates over LAN only, no internet required on clients |
| **Boot mode** | UEFI only, declarative partitioning with Disko |

## 🚀 Quick start

> **Optional: fork first.** If you want to push your `lab-config.nix` to a remote repository (for backup or future reinstalls), fork this repo on GitHub before starting. The main flow below works with a plain `git clone` of the original repo -- forking is not required.

### 1. Bootstrap the controller from USB

> **Requires**: a NixOS live USB with temporary internet access. UEFI boot must be enabled.

Boot the controller PC from the NixOS live USB, then run:
```sh
curl -fsSL https://raw.githubusercontent.com/giovantenne/nixos-lab/master/scripts/install-controller.sh | bash
```

If one disk is detected, the script selects it automatically; if multiple disks are detected, it asks you to choose one.

This installs the controller with default placeholder settings from `lab-config.nix`. SSH, binary cache, and Veyon public keys are added later, after step 4.

The bootstrap script forces `cache.nixos.org` during installation, so it does not depend on any LAN cache or substituter already configured in the live environment.

> **Using your own fork?** Only one env var is needed:
> ```sh
> curl -fsSL https://raw.githubusercontent.com/YOUR_USER/nixos-lab/master/scripts/install-controller.sh | \
>   FLAKE_REF="github:YOUR_USER/nixos-lab" bash
> ```

### 2. Reboot and clone the repo

Reboot and log in as `admin` (default password: `nixos`).

```sh
git clone https://github.com/giovantenne/nixos-lab.git
cd nixos-lab
```

> If you forked the repo, clone your fork instead: `git clone https://github.com/YOUR_USER/nixos-lab.git`


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
networkBase = "10.0.0";             # First 3 octets of static lab subnet
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
homepageUrl = "https://github.com/giovantenne/nixos-lab";

# ── Locale / timezone ─────────────────────────────────────────
timeZone = "Europe/Rome";
defaultLocale = "en_US.UTF-8";
extraLocale = "it_IT.UTF-8";
keyboardLayout = "it";
consoleKeyMap = "it2";
```

You can leave the git identity fields at their defaults for now.

> **Note**: `masterDhcpIp` is used only during PXE/netboot client installation. The generated iPXE script, the netboot ramdisk, and the PXE helper services all point to that DHCP address, so if the DHCP lease changes before a netboot session you must update `lab-config.nix` and rebuild the netboot artifacts. Regular Colmena deploys use the controller's static lab IP instead.

### 4. Generate and install keys

Generate the three required key pairs. Keep the private files local to the controller, and add the public files to Git in the last command below so Nix can read them from the repo.

| Key pair | Private file | Public file / config | Purpose |
|---|---|---|---|
| **Binary cache** | `secret-key` | `public-key` (generated locally, committed) | Harmonia signs Nix store paths; clients verify signatures |
| **SSH** | `id_ed25519` | `id_ed25519.pub` (generated locally, committed) | Admin SSH access + Colmena deploys (connects as `root`) |
| **Veyon** | `veyon-private-key.pem` | `veyon-public-key.pem` (generated locally, committed) | Veyon Master authenticates to student PCs |

Generate everything from scratch:

```sh
# Binary cache signing key for Harmonia
nix key generate-secret --key-name lab-cache-key > secret-key
nix key convert-secret-to-public < secret-key > public-key

# Admin SSH key used by Colmena / SSH access
ssh-keygen -t ed25519 -f id_ed25519 -N '' -C 'admin@controller'

# Veyon RSA keypair
openssl genrsa -out veyon-private-key.pem 4096
openssl rsa -in veyon-private-key.pem -pubout -out veyon-public-key.pem

# SSH private key -- used by Colmena to connect as root to all PCs
install -m 600 -D id_ed25519 ~/.ssh/id_ed25519

# SSH public key -- useful for normal SSH tooling; keep a copy in the repo root too
install -m 644 -D id_ed25519.pub ~/.ssh/id_ed25519.pub

# Veyon private key -- only needed on the controller (where Veyon Master runs)
# Only users in the veyon-master group (admin + teacher) can read it
sudo install -d -m 0750 -g veyon-master /etc/veyon/keys/private/teacher
sudo install -m 0640 -g veyon-master veyon-private-key.pem /etc/veyon/keys/private/teacher/key

# Flakes ignore untracked files in a Git worktree, so add the public files
git add public-key id_ed25519.pub veyon-public-key.pem
```

### 5. Rebuild the controller

Before starting PXE, temporarily remove the controller's static lab IP from the shared interface. The generated netboot artifacts refer to `masterDhcpIp`, so this keeps PXE, HTTP, and binary-cache traffic on that single DHCP address during installation. The change is temporary and a reboot restores the static IP automatically.

```sh
# Rebuild the controller with your real config
sudo nixos-rebuild switch --flake .#$(awk '/masterHostNumber =/ { gsub(/[^0-9]/, ""); print "pc" $0; exit }' lab-config.nix) --no-write-lock-file

# Build netboot artifacts
nix build .#nixosConfigurations.netboot.config.system.build.kernel --out-link result-kernel
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --out-link result-initrd
nix build .#nixosConfigurations.netboot.config.system.build.netbootIpxeScript --out-link result-ipxe

# Install iPXE bootstrap binary
nix build nixpkgs#ipxe --out-link result-ipxe-bin
install -D -m 0644 result-ipxe-bin/snp.efi assets/ipxe/snponly.efi

# Pre-build all client closures
PC_COUNT=$(awk '/pcCount =/ { gsub(/[^0-9]/, ""); print; exit }' lab-config.nix)
TARGETS=()
for i in $(seq 1 "$PC_COUNT"); do
  TARGETS+=(".#nixosConfigurations.pc$(printf "%02d" "$i").config.system.build.toplevel")
done
nix build "${TARGETS[@]}"

# Temporarily remove the lab static IP so netboot uses masterDhcpIp only
STATIC_IP=$(awk -F'"' '/networkBase =/ { print $2; exit }' lab-config.nix).$(awk '/masterHostNumber =/ { gsub(/[^0-9]/, ""); print; exit }' lab-config.nix)
IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' lab-config.nix)
sudo ip addr del "${STATIC_IP}/24" dev "${IFACE}"
```

### 6. Start netboot services

Open **two separate terminals**:

**Terminal 1** -- Binary cache:
```sh
./scripts/run-harmonia.sh
```

**Terminal 2** -- ProxyDHCP + TFTP + HTTP netboot server:
```sh
sudo ./scripts/run-pxe-proxy.sh
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
STATIC_IP=$(awk -F'"' '/networkBase =/ { print $2; exit }' lab-config.nix).$(awk '/masterHostNumber =/ { gsub(/[^0-9]/, ""); print; exit }' lab-config.nix)
IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' lab-config.nix)
sudo ip addr add "${STATIC_IP}/24" dev "${IFACE}"
```

---

## 🔧 Maintenance

### Deploy updates (Colmena)

First apply the latest configuration on the controller itself:
```sh
sudo nixos-rebuild switch --flake .#$(awk '/masterHostNumber =/ { gsub(/[^0-9]/, ""); print "pc" $0; exit }' lab-config.nix) --no-write-lock-file
```

Then start the binary cache:
```sh
./scripts/run-harmonia.sh
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
sudo nixos-rebuild switch --flake .#$(awk '/masterHostNumber =/ { gsub(/[^0-9]/, ""); print "pc" $0; exit }' lab-config.nix) --no-write-lock-file
```

For client PCs, prefer Colmena from the controller. Only run `sudo nixos-rebuild switch --flake /path/to/nixos-lab#pc05 --no-write-lock-file` after logging into `pc05` itself (or after cloning the repo there).

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

All machines must boot in **UEFI mode**. Disk partitioning is declarative via `disko-uefi.nix`.

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

To recover student work from a previous session:
```sh
ls /var/lib/home-snapshots/snapshot-1/
cp /var/lib/home-snapshots/snapshot-1/file.txt /home/<studentUser>/
```

### Veyon (classroom management)

Veyon is packaged locally (not in nixpkgs) and deployed on all PCs. The
`veyon-service` systemd unit runs on every machine, accepting connections on
port **11100**.

#### Configuration

- All PCs have `veyon-service` running and the public key deployed via Nix
- `Veyon.conf` is generated with all lab PCs pre-mapped under the hardcoded location name `Lab`
- The Veyon private key is only needed on the controller -- student PCs only have the public key
- Users in the `veyon-master` group (`admin` and the teacher user) can access Veyon Master

### Customizing packages and desktop

The default desktop is GNOME (Wayland) with a curated set of development tools. To customize:

- **System packages**: edit the `environment.systemPackages` list in `modules/common.nix`
- **GNOME settings**: edit the `extraGSettingsOverrides` in `modules/common.nix`
- **Screensaver**: replace `assets/logo.txt` with your own ASCII art
- **Wallpapers**: replace images in `assets/backgrounds/`
- **VS Code extensions**: edit the `vscodeExtensions` list in `modules/home-reset.nix`

---

## 📁 Project structure

```
flake.nix                  # Entry point: host generation, Colmena config
flake.lock                 # Pinned inputs (nixpkgs, disko)
LICENSE                    # MIT license
lab-config.nix             # Lab configuration (edit for your environment)
disko-uefi.nix             # Declarative disk partitioning (UEFI + Btrfs)
setup.sh                   # Client PC installer (runs on PXE-booted machines)
pkgs/
  veyon.nix                # Veyon package derivation
  gnome-remote-desktop.nix # gnome-remote-desktop overlay (VNC + multi-session)
modules/
  common.nix               # GNOME desktop, packages, shells, locale, services
  hardware.nix             # Generic hardware detection
  networking.nix           # Hostname + static IP per host
  users.nix                # User accounts and autologin
  cache.nix                # Binary cache client configuration
  filesystems.nix          # Btrfs support
  home-reset.nix           # Student home templating + boot-time reset
  veyon.nix                # Veyon service, keys, and classroom config
scripts/
  install-controller.sh    # Controller bootstrap from live USB
  run-harmonia.sh          # Binary cache server
  run-pxe-proxy.sh         # ProxyDHCP + TFTP + HTTP netboot server
  cmd-screensaver.sh       # TTE screensaver animation loop
  launch-screensaver.sh    # Fullscreen Ghostty screensaver launcher
  screensaver-monitor.sh   # GNOME idle watcher for screensaver
  create-home-template.sh  # Home directory template builder
  home-reset.sh            # Boot-time snapshot rotation + home reset
assets/
  backgrounds/             # Wallpapers (randomly selected at home reset)
  logo.txt                 # ASCII art for screensaver
  mimeapps.list            # Default applications
  vscode-settings.json     # VS Code defaults
```

Public key artifacts generated during setup live in the repo root; see step 4.

## 🔒 Security

- **Never commit** `secret-key`, `id_ed25519`, or `veyon-private-key.pem` (all are in `.gitignore`)
- Commit only the public counterparts used by the Nix configuration: `public-key`, `id_ed25519.pub`, and `veyon-public-key.pem`
- Passwords are SHA-512 hashed; never store plaintext
- SSH password authentication is disabled; key-based only
- `users.mutableUsers = false` enforces declarative user management
- The Veyon private key is readable only by the `veyon-master` group

## 📄 License

Released under the [MIT License](./LICENSE).
