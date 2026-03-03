# NixOS Lab Deployment (Zero-Internet Workflow)

This repository manages a 31-PC NixOS lab with network installation (Netboot),
no internet on clients, and a master PC (`pc99`) as the local controller.

## Architecture

| Component | Description |
|---|---|
| **Controller** (`pc99`) | PXE/Netboot server, local binary cache, Colmena orchestration |
| **Nodes** (`pc01`–`pc30`) | 30 student workstations, Btrfs filesystem |
| **Networking** | Install and updates over LAN only, no internet on clients |
| **Boot mode** | UEFI only |

---

## Setup

### Step 1 — Bootstrap pc99 from USB

> **Requires**: temporary internet access on this first boot. UEFI boot must be enabled.

From the NixOS live USB, partition and install:
```sh
curl -fsSL https://raw.githubusercontent.com/giovantenne/nixos-lab/master/scripts/install-pc99.sh | bash
```
If one disk is detected, the script selects it automatically; if multiple disks
are detected, it asks you to choose one.

After the installation finishes, reboot:
```sh
reboot
```

After the first reboot, log in as `admin` and clone the repo:
```sh
git clone https://github.com/giovantenne/nixos-lab.git
cd ~/nixos-lab
```

### Step 2 — Copy secret files

Copy these three files into the repo folder `~/nixos-lab/` (all are in `.gitignore`):

| File | Description |
|---|---|
| `secret-key` | Binary cache signing key (Harmonia) |
| `id_ed25519` | Admin SSH private key |
| `veyon-private-key.pem` | Veyon Master private key |

Then install them all at once:
```sh
cd ~/nixos-lab && \
  install -m 600 -D id_ed25519 ~/.ssh/id_ed25519 && \
  sudo install -d -m 0750 -g veyon-master /etc/veyon/keys/private/teacher && \
  sudo install -m 0640 -g veyon-master veyon-private-key.pem /etc/veyon/keys/private/teacher/key
```

> `secret-key` just needs to be in the repo root (already there after the copy).
> Only users in the `veyon-master` group (`admin`, `docente`) can read the Veyon
> private key and use Veyon Master.

### Step 3 — Configure flake.nix

Find the DHCP address and interface name of pc99:
```sh
ip -4 addr
```

Edit the settings at the top of `flake.nix`:
```sh
vim flake.nix
```

| Variable | Description | Example |
|---|---|---|
| `masterDhcpIp` | DHCP address of pc99 (from `ip -4 addr`) | `"192.168.1.100"` |
| `networkBase` | First 3 octets of the static lab subnet | `"10.22.9"` |
| `pcCount` | Number of student PCs | `30` |
| `ifaceName` | Network interface name (from `ip -4 addr`) | `"enp0s3"` |

> **Important**: this step is required before both Option A and Option B below.

### Step 4 — Prepare the controller (pc99)

#### Option A — Automatic (recommended)

```sh
sudo ./scripts/prepare-pc99.sh
```

The script performs these steps:
1. Checks prerequisites
2. Validates `flake.nix` settings (errors out if `masterDhcpIp` is not set)
3. Rebuilds pc99 (`nixos-rebuild switch`)
4. Builds netboot artifacts (kernel, initrd, iPXE script)
5. Installs the iPXE bootstrap binary (`snponly.efi`)
6. Pre-builds all client closures (for offline install via local cache)
7. Removes the static IP from the network interface (needed during netboot)

When done, the script prints the commands to start the netboot services.

#### Option B — Manual (step-by-step)

Rebuild pc99 to apply the new `flake.nix` settings:
```sh
sudo nixos-rebuild switch --flake .#pc99 --no-write-lock-file
```

Build the netboot artifacts:
```sh
nix build .#nixosConfigurations.netboot.config.system.build.kernel --out-link result-kernel
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --out-link result-initrd
nix build .#nixosConfigurations.netboot.config.system.build.netbootIpxeScript --out-link result-ipxe
```

Install the iPXE UEFI bootstrap binary:
```sh
nix build nixpkgs#ipxe --out-link result-ipxe-bin
install -D -m 0644 result-ipxe-bin/snponly.efi assets/ipxe/snponly.efi
```

Pre-build all client closures so installs work offline via the local cache:
```sh
nix build .#nixosConfigurations.pc{01..30}.config.system.build.toplevel
```

Remove the static IP so pc99 uses only its DHCP address during netboot
(the static IP returns automatically after a reboot):
```sh
STATIC_IP=$(awk -F'"' '/networkBase =/ { print $2; exit }' flake.nix).$(awk '/masterHostNumber =/ { gsub(/;/, "", $3); print $3; exit }' flake.nix)
IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' flake.nix)
sudo ip addr del "${STATIC_IP}/24" dev "${IFACE}"
```

### Step 5 — Start netboot services

Open **two separate terminals** and run:

**Terminal 1** — Binary cache:
```sh
./scripts/run-harmonia.sh
```

**Terminal 2** — ProxyDHCP + TFTP + HTTP netboot server:
```sh
sudo ./scripts/run-pxe-proxy.sh
```

> **Note**: both processes run in the foreground. Keep the terminals open
> during the entire client installation process.

### Step 6 — Install client PCs

On each client PC, enable **UEFI network boot** in the BIOS/firmware settings.
The PC will PXE-boot into a NixOS ramdisk environment.

On the booted client, run the installer:
```sh
cd /installer/repo
./setup.sh XX
```
Where `XX` is the PC number (e.g., `./setup.sh 5` for `pc05`).

> `setup.sh` auto-selects the disk if only one is present; if multiple disks
> are detected, it asks for a choice and requires a final confirmation before
> wiping it.

When all clients are installed, restore the static IP on pc99 (or just reboot it):
```sh
STATIC_IP=$(awk -F'"' '/networkBase =/ { print $2; exit }' flake.nix).$(awk '/masterHostNumber =/ { gsub(/;/, "", $3); print $3; exit }' flake.nix)
IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' flake.nix)
sudo ip addr add "${STATIC_IP}/24" dev "${IFACE}"
```

---

## Maintenance

### Deploy updates (Colmena)

The master (`pc99`) pushes updates to all lab PCs via SSH.

Start the binary cache first (required for client builds):
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

To manually rebuild a single PC from the latest GitHub config:
```sh
sudo nixos-rebuild switch --flake github:giovantenne/nixos-lab#pc99 --no-write-lock-file --refresh
```
Replace `pc99` with the appropriate hostname (e.g., `pc01`, `pc15`).

---

## Reference

### Disk layout (Disko)

Declarative disk config is in `disko-uefi.nix`. All machines must boot in **UEFI mode**.

| Partition | Filesystem | Mount point |
|---|---|---|
| EFI System Partition | FAT32 | `/boot` |
| Root partition (`nixos`) | Btrfs | — |

Btrfs subvolumes:

| Subvolume | Mount point |
|---|---|
| `@root` | `/` |
| `@home-informatica` | `/home/informatica` |
| `@snapshots` | `/var/lib/home-snapshots` |

GRUB is enabled in UEFI mode with a 5-second timeout and `os-prober` enabled.
Windows installations on other disks are detected when GRUB is regenerated
(`nixos-install` / `nixos-rebuild switch`).

### Home reset and snapshots

The `informatica` home directory resets to a clean template on every boot:

- **Template**: generated at activation time with git config, VS Code settings,
  and XDG directories.
- **Snapshots**: the last 5 versions are saved in `/var/lib/home-snapshots/`
  (accessible by `admin`, `docente`, and `root`).

The `docente` user has a **Snapshot Studenti** bookmark in the Nautilus (Files)
sidebar pointing to the snapshots directory.

To recover student work from a previous session:
```sh
ls /var/lib/home-snapshots/snapshot-1/
cp /var/lib/home-snapshots/snapshot-1/file.txt /home/informatica/
```

### Veyon (classroom management)

Veyon is packaged locally (not in nixpkgs) and deployed on all PCs. The
`veyon-service` systemd unit runs on every machine, accepting connections on
port **11100**.

Authentication uses RSA key-file mode:

| Key | Location | Managed by |
|---|---|---|
| **Public key** (`veyon-public-key.pem`) | Committed in the repo, deployed to all PCs | Nix |
| **Private key** (`veyon-private-key.pem`) | In `.gitignore`, installed in [Step 2](#step-2--copy-secret-files) | Manual |

#### Generating keys

Keys are generated once with openssl:
```sh
openssl genrsa -out veyon-private-key.pem 4096
openssl rsa -in veyon-private-key.pem -pubout -out veyon-public-key.pem
```

#### Configuration

A base configuration with all 30 lab PCs pre-mapped is deployed to
`/etc/xdg/Veyon Solutions/Veyon.conf`. Admin or docente can customize the
layout via `veyon-configurator`.
