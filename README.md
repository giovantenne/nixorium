# NixOS Lab Deployment (Zero-Internet Workflow)
This repository manages a 31-PC NixOS lab, optimized for network installation (Netboot) with no internet access and a master PC (`pc99`) acting as the local controller.

## Architecture
- Controller (`pc99`): `PXE/Netboot`, local `binary cache`, `Colmena` orchestration.
- Nodes (Lab PCs): 30 workstations with `Btrfs` filesystem.
- Networking: installs and updates over `LAN` only, no internet on client PCs.
- Boot mode: **UEFI only**.

## 1. First setup at school (master PC)
Bootstrap the master PC (`pc99`) from a USB installer, using temporary internet access on the first boot. **UEFI boot is required**.

From the live USB, partition and install:
```sh
curl -fsSL https://raw.githubusercontent.com/giovantenne/nixos-lab/master/scripts/install-pc99.sh | bash
```
If one disk is detected, the script selects it automatically; if multiple disks are detected, it asks you to choose one.
After the installation finishes, reboot:
```sh
reboot
```

After the first reboot, log in as `admin` and clone the repo:
```sh
git clone https://github.com/giovantenne/nixos-lab.git
cd ~/nixos-lab
```

Copy the binary cache private key (`secret-key`) into the repo folder. It is already in `.gitignore`.

Copy the admin SSH private key (`id_ed25519`) to `~/.ssh/id_ed25519`:
```sh
install -m 600 -D id_ed25519 ~/.ssh/id_ed25519
```

## 2. Prepare the controller (pc99)
All commands below run from `~/nixos-config` on `pc99`.

Before running the preparation script, customize `networkBase` and `pcCount` in `flake.nix` if needed.

One-command preparation (recommended):
```sh
./scripts/prepare-pc99.sh
```
This script:
- checks prerequisites (including `tmux`)
- auto-configures `masterDhcpIp` if still set to `MASTER_DHCP_IP`
- rebuilds `pc99`
- builds netboot artifacts
- generates `assets/ipxe/snponly.efi` automatically
- pre-builds all client closures
- starts `run-harmonia.sh` and `run-pxe-proxy.sh` in a `tmux` session with live logs

Live logs:
```sh
tmux attach -t lab-netboot
```
Log files are also written to:
- `/tmp/harmonia.log`
- `/tmp/pxe-proxy.log`

## 3. Network install (PXE/Netboot)
The network services are started by `./scripts/prepare-pc99.sh` in a tmux session.
To monitor them live:
```sh
tmux attach -t lab-netboot
```

On each client PC, enable UEFI network boot. The PC will boot into a NixOS ramdisk.

On the booted client, run the installer:
```sh
cd /installer/repo
./setup.sh XX
```
Where `XX` is the PC number (e.g., `./setup.sh 5` for `pc05`).
`setup.sh` auto-selects the disk if only one is present; if multiple disks are present, it asks for a choice and requires a final confirmation before wiping it.

## 4. Partitioning and Boot (Disko)
Declarative disk config is in `disko-uefi.nix`. All machines must boot in **UEFI mode**.

Disk layout:
- EFI System Partition (FAT32) mounted at `/boot`
- Btrfs root partition labeled `nixos` with subvolumes:
  - `@root` -> `/`
  - `@home-informatica` -> `/home/informatica`
  - `@snapshots` -> `/var/lib/home-snapshots`

`GRUB` is enabled in UEFI mode with a 5-second timeout and `os-prober` enabled.
Windows 11 installations on other disks are detected when GRUB is regenerated
(`nixos-install` / `nixos-rebuild switch`).

## 5. Home Reset and Snapshots
The `informatica` home directory resets to a clean template on every boot:

- **Template**: generated at activation time with git config, VS Code settings, and XDG directories
- **Snapshots**: last 5 versions saved in `/var/lib/home-snapshots/` (accessible by `admin`, `docente`, and `root`)

The `docente` user has a **Snapshot Studenti** bookmark in the Nautilus (Files) sidebar pointing to the snapshots directory.

To recover student work from a previous session:
```sh
ls /var/lib/home-snapshots/snapshot-1/
cp /var/lib/home-snapshots/snapshot-1/file.txt /home/informatica/
```

## 6. Post-install management (Colmena)
The master (`pc99`) pushes updates to all lab PCs via SSH:

```sh
# Start the binary cache (required for client builds)
./scripts/run-harmonia.sh

# Deploy to all lab PCs
nix run nixpkgs#colmena -- apply --impure --on @lab

# Deploy to a single PC
nix run nixpkgs#colmena -- apply --impure --on pc05
```

## 7. Manual rebuild
To manually rebuild a single PC from the latest GitHub config:
```sh
sudo nixos-rebuild switch --flake github:giovantenne/nixos-lab#pc99 --no-write-lock-file --refresh
```
Replace `pc99` with the appropriate hostname (e.g., `pc01`, `pc15`).

## 8. Veyon (classroom management)
Veyon is packaged locally (not available in nixpkgs) and deployed on all PCs. The `veyon-service` systemd unit runs on every machine, accepting connections on port **11100**.

Authentication uses RSA key-file mode:
- **Public key** (`veyon-public-key.pem`): committed in the repo, deployed to `/etc/veyon/keys/public/teacher/key` on all PCs.
- **Private key** (`veyon-private-key.pem`): in `.gitignore`, must be placed manually on machines where Veyon Master will be used.

### Generating keys
Keys are generated once with openssl:
```sh
openssl genrsa -out veyon-private-key.pem 4096
openssl rsa -in veyon-private-key.pem -pubout -out veyon-public-key.pem
```

### Distributing the private key
Copy the private key to the machines where admin or docente need Veyon Master:
```sh
sudo install -d -m 0750 -g veyon-master /etc/veyon/keys/private/teacher
sudo install -m 0640 -g veyon-master veyon-private-key.pem /etc/veyon/keys/private/teacher/key
```
Only users in the `veyon-master` group (`admin`, `docente`) can read the private key and use Veyon Master.

### Configuration
A base configuration with all 30 lab PCs pre-mapped is deployed to `/etc/xdg/Veyon Solutions/Veyon.conf`. Admin or docente can customize the layout via `veyon-configurator`.
