# NixOS Lab

A reproducible NixOS deployment system for multi-PC environments (classrooms, training rooms, public labs, libraries) with **no internet access on client machines**.

One controller PC manages the entire lab: it builds all configurations locally, serves them over LAN, and deploys updates to every workstation declaratively. The whole lab can be reinstalled from scratch in under 20 minutes.

## Why this exists

Managing a multi-PC lab is painful. Machines drift over time, reinstalling by hand is slow and error-prone, and keeping many systems consistent is a full-time job. Traditional tools like Ansible help, but they can't guarantee that two machines built a week apart end up identical.

NixOS solves this with **declarative, reproducible configurations** -- but most NixOS workflows assume internet access. In many schools, offices, and public labs, client PCs either have no internet at all or only get access after a user logs into an institutional network.

This project bridges that gap with a **local-first workflow**:

- A single controller PC acts as the build server, binary cache, and PXE boot server
- Client PCs are installed and updated entirely over the LAN
- One `flake.nix` file is the single source of truth for the entire lab
- Everything is parameterizable -- user names, passwords, network settings, locale -- so you can fork this repo and adapt it to your environment in minutes

## Features

- **Zero-internet client installation** -- PXE/netboot + local binary cache (Harmonia), no USB drives needed per client
- **Single source of truth** -- one `flake.nix` generates all host configurations programmatically
- **Multi-machine orchestration** -- deploy updates to all PCs at once with Colmena
- **Student home directory reset** -- homes are restored to a clean template on every boot, with the last 5 sessions saved as Btrfs snapshots for recovery
- **Classroom management** -- Veyon is pre-configured with all lab PCs mapped, ready to use
- **Fully parameterizable** -- user names, PC count, network layout, passwords, locale, homepage, and more are all configurable from a single settings block
- **Dual networking** -- DHCP for institutional network integration + static IPs for the internal lab network
- **UEFI + Btrfs** -- modern boot with declarative disk partitioning (Disko) and snapshot support
- **GNOME desktop** -- pre-configured with dark theme, development tools, and terminal customization

## Architecture

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

## Quick start

### 1. Fork and configure

Fork this repository, then edit `lab-config.nix` with your lab's settings:

```nix
# ── Network ────────────────────────────────────────────────────
masterDhcpIp = "MASTER_DHCP_IP";   # DHCP address of controller (ip -4 addr)
networkBase = "10.0.0";             # First 3 octets of static lab subnet
pcCount = 20;                       # Number of student PCs
masterHostNumber = 99;              # Controller PC number
ifaceName = "enp0s3";               # Network interface name

# ── User accounts ─────────────────────────────────────────────
teacherUser = "teacher";            # Teacher account name
studentUser = "student";            # Student account name

# ── Passwords (SHA-512 hashed) ────────────────────────────────
# Generate with: mkpasswd -m sha-512
teacherPassword = "...";
studentPassword = "...";
adminPassword = "...";

# ── SSH ────────────────────────────────────────────────────────
adminSshKey = "ssh-ed25519 AAAA... admin@controller";

# ── School / organization ──────────────────────────────────────
homepageUrl = "https://example.com";
studentGitName = "student";
studentGitEmail = "student@example.com";
adminGitName = "admin";
adminGitEmail = "admin@example.com";
veyonLocationName = "Lab";

# ── Locale / timezone ──────────────────────────────────────────
timeZone = "Europe/Rome";
defaultLocale = "en_US.UTF-8";
extraLocale = "it_IT.UTF-8";
keyboardLayout = "it";
consoleKeyMap = "it2";
```

### 2. Generate passwords

```sh
# Generate hashed passwords (one per user)
mkpasswd -m sha-512
```

Paste each hash into the corresponding field in `lab-config.nix` (`adminPassword`, `teacherPassword`, `studentPassword`).

> Keys (SSH, Veyon, binary cache) are generated later in **Step 4**, after the controller is installed.

### 3. Bootstrap the controller from USB

> **Requires**: temporary internet access on this first boot. UEFI boot must be enabled.

From the NixOS live USB:
```sh
# If using the default repo:
curl -fsSL https://raw.githubusercontent.com/giovantenne/nixos-lab/master/scripts/install-controller.sh | bash

# If using your own fork:
curl -fsSL https://raw.githubusercontent.com/YOUR_USER/YOUR_REPO/master/scripts/install-controller.sh | \
  FLAKE_REF="github:YOUR_USER/YOUR_REPO" \
  DISKO_URL="https://raw.githubusercontent.com/YOUR_USER/YOUR_REPO/master/disko-uefi.nix" \
  MASTER_HOST_NUMBER=99 \
  STUDENT_USER=student \
  bash
```

If one disk is detected, the script selects it automatically; if multiple disks are detected, it asks you to choose one.

After the installation finishes, reboot and log in as `admin`.

### 4. Generate keys and copy secrets

```sh
git clone https://github.com/YOUR_USER/YOUR_REPO.git
cd ~/nixos-lab
```

The lab uses three cryptographic key pairs. All private keys are in `.gitignore` and must **never** be committed.

| Key pair | Private file | Public file / config | Purpose |
|---|---|---|---|
| **Binary cache** | `secret-key` | `cachePublicKey` in `flake.nix` | Harmonia signs Nix store paths; clients verify signatures |
| **SSH** | `id_ed25519` | `adminSshKey` in `lab-config.nix` | Admin SSH access + Colmena deploys (connects as `root`) |
| **Veyon** | `veyon-private-key.pem` | `veyon-public-key.pem` (committed) | Veyon Master authenticates to student PCs |

If you already have the private keys from a previous deployment, copy them into `~/nixos-lab/`.

If you need to generate them from scratch, run:
```sh
# Binary cache signing key for Harmonia
nix key generate-secret --key-name lab-cache-key > secret-key
nix key convert-secret-to-public < secret-key > public-key

# Admin SSH key used by Colmena / SSH access
ssh-keygen -t ed25519 -f id_ed25519 -N '' -C 'admin@controller'

# Veyon RSA keypair
openssl genrsa -out veyon-private-key.pem 4096
openssl rsa -in veyon-private-key.pem -pubout -out veyon-public-key.pem
```

After generating new keys, update the repo configuration:

- Replace `cachePublicKey` in `flake.nix` with the content of `public-key`.
- Replace `adminSshKey` in `lab-config.nix` with the content of `id_ed25519.pub`.
- Commit `veyon-public-key.pem` to the repo.

Then install the local copies needed on the controller:
```sh
# SSH private key -- used by Colmena to connect as root to all PCs
install -m 600 -D id_ed25519 ~/.ssh/id_ed25519

# Veyon private key -- only needed on the controller (where Veyon Master runs)
# Only users in the veyon-master group (admin + teacher) can read it
sudo install -d -m 0750 -g veyon-master /etc/veyon/keys/private/teacher
sudo install -m 0640 -g veyon-master veyon-private-key.pem /etc/veyon/keys/private/teacher/key
```

> `secret-key` just needs to be in the repo root (already there after the copy). `run-harmonia.sh` checks for it at startup and exits with an error if missing.

> **Troubleshooting**: if Colmena deploys fail with "Permission denied (publickey)", verify that `~/.ssh/id_ed25519` exists and that `adminSshKey` in `lab-config.nix` matches. If the binary cache is ignored (clients build from source), verify that `cachePublicKey` in `flake.nix` matches the `secret-key`. If Veyon Master cannot connect to student screens, verify the private key is at `/etc/veyon/keys/private/teacher/key` and matches `veyon-public-key.pem`.

### 5. Set the DHCP address

Find the controller's DHCP address (assigned by the institutional DHCP server) and interface name:
```sh
ip -4 addr
```

Edit `masterDhcpIp` and `ifaceName` in `lab-config.nix` if not already set.

> **Note**: `masterDhcpIp` is the address dynamically assigned by the institutional DHCP server. It can change when the DHCP lease expires. It is only used during PXE/netboot client installation -- after that, Colmena deploys use the static IP (`networkBase.masterHostNumber`). If the DHCP address changes before a netboot session, update `lab-config.nix` and rebuild the netboot artifacts.

### 6. Prepare the controller

```sh
# Rebuild the controller
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

# Remove static IP for netboot (returns after reboot)
STATIC_IP=$(awk -F'"' '/networkBase =/ { print $2; exit }' lab-config.nix).$(awk '/masterHostNumber =/ { gsub(/[^0-9]/, ""); print; exit }' lab-config.nix)
IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' lab-config.nix)
sudo ip addr del "${STATIC_IP}/24" dev "${IFACE}"
```

### 7. Start netboot services

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

### 8. Install client PCs

On each client PC, enable **UEFI network boot** in the BIOS/firmware settings. The PC will PXE-boot into a NixOS ramdisk environment.

On the booted client:
```sh
cd /installer/repo
./setup.sh XX
```
Where `XX` is the PC number (e.g., `./setup.sh 5` for `pc05`).

> `setup.sh` auto-selects the disk if only one is present; if multiple disks are detected, it asks for a choice.

When all clients are installed, restore the static IP on the controller (or just reboot it):
```sh
STATIC_IP=$(awk -F'"' '/networkBase =/ { print $2; exit }' lab-config.nix).$(awk '/masterHostNumber =/ { gsub(/[^0-9]/, ""); print; exit }' lab-config.nix)
IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' lab-config.nix)
sudo ip addr add "${STATIC_IP}/24" dev "${IFACE}"
```

---

## Maintenance

### Deploy updates (Colmena)

Start the binary cache first:
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

Rebuild a single PC from the latest config:
```sh
sudo nixos-rebuild switch --flake github:YOUR_USER/YOUR_REPO#pc05 --no-write-lock-file --refresh
```

---

## Configuration reference

### Settings overview

All lab-specific settings are defined in `lab-config.nix`. No other file needs editing for basic customization.

| Setting | Description | Default |
|---|---|---|
| `masterDhcpIp` | Institutional DHCP address of the controller (used for PXE/netboot only; can change on lease renewal) | `"MASTER_DHCP_IP"` |
| `networkBase` | First 3 octets of the static lab subnet | `"10.0.0"` |
| `pcCount` | Number of student PCs | `20` |
| `masterHostNumber` | Controller PC number (must be > `pcCount`) | `99` |
| `ifaceName` | Network interface name (shared across all PCs) | `"enp0s3"` |
| `teacherUser` | Teacher account name | `"teacher"` |
| `studentUser` | Student account name (autologin, home reset) | `"student"` |
| `teacherPassword` | Teacher password (SHA-512 hash) | -- |
| `studentPassword` | Student password (SHA-512 hash) | -- |
| `adminPassword` | Admin password (SHA-512 hash) | -- |
| `adminSshKey` | SSH public key for root and admin | -- |
| `homepageUrl` | Chromium browser homepage | `"https://example.com"` |
| `studentGitName` | Git author name for student template | `"student"` |
| `studentGitEmail` | Git author email for student template | `"student@example.com"` |
| `adminGitName` | Git author name for admin template | `"admin"` |
| `adminGitEmail` | Git author email for admin template | `"admin@example.com"` |
| `veyonLocationName` | Veyon classroom location name | `"Lab"` |
| `timeZone` | System timezone | `"Europe/Rome"` |
| `defaultLocale` | Default system locale | `"en_US.UTF-8"` |
| `extraLocale` | Locale for LC_* settings | `"it_IT.UTF-8"` |
| `keyboardLayout` | X11 keyboard layout | `"it"` |
| `consoleKeyMap` | Console keymap | `"it2"` |

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

The teacher user has a **Snapshot Studenti** bookmark in the Nautilus sidebar.

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
- `Veyon.conf` is generated with all lab PCs pre-mapped (location name from `veyonLocationName` in `lab-config.nix`)
- The Veyon private key is only needed on the controller -- student PCs only have the public key
- Users in the `veyon-master` group (`admin` and the teacher user) can access Veyon Master

### Customizing packages and desktop

The default desktop is GNOME (Wayland) with a curated set of development tools. To customize:

- **System packages**: edit the `environment.systemPackages` list in `modules/common.nix`
- **GNOME settings**: edit the `extraGSettingsOverrides` in `modules/common.nix`
- **Screensaver**: replace `assets/meucci.txt` with your own ASCII art
- **Wallpapers**: replace images in `assets/backgrounds/`
- **VS Code extensions**: edit the `vscodeExtensions` list in `modules/home-reset.nix`

---

## Project structure

```
flake.nix                  # Entry point: host generation, Colmena config
flake.lock                 # Pinned inputs (nixpkgs, disko)
lab-config.nix             # Lab configuration (edit for your environment)
disko-uefi.nix             # Declarative disk partitioning (UEFI + Btrfs)
setup.sh                   # Client PC installer (runs on PXE-booted machines)
veyon-public-key.pem       # Veyon RSA public key (deployed to all PCs)
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
  meucci.txt               # ASCII art for screensaver
  mimeapps.list            # Default applications
  vscode-settings.json     # VS Code defaults
```

## Security

- **Never commit** `secret-key`, `id_ed25519`, or `veyon-private-key.pem` (all in `.gitignore`)
- Passwords are SHA-512 hashed; never store plaintext
- SSH password authentication is disabled; key-based only
- `users.mutableUsers = false` enforces declarative user management
- The Veyon private key is readable only by the `veyon-master` group

## License

This project is open source. Feel free to fork, adapt, and use it for your own lab environment.
