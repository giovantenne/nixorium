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

Update `masterDhcpIp`, `networkBase`, `pcCount`, `masterHostNumber`, and `ifaceName` at the top of `flake.nix`:
```sh
ip -4 addr                  # find the DHCP address
vim flake.nix               # edit masterDhcpIp and other settings
```

Rebuild pc99 to apply the new settings:
```sh
sudo nixos-rebuild switch --flake .#pc99 --no-write-lock-file
```

Build the netboot artifacts:
```sh
nix build .#nixosConfigurations.netboot.config.system.build.kernel --out-link result-kernel
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --out-link result-initrd
nix build .#nixosConfigurations.netboot.config.system.build.netbootIpxeScript --out-link result-ipxe
```

Copy iPXE UEFI bootstrap (`snp.efi` in nixpkgs) into the path expected by this repo (`assets/ipxe/snponly.efi`) with a single command:
```sh
nix build nixpkgs#ipxe --out-link result-ipxe-bin && install -D -m 0644 result-ipxe-bin/snp.efi assets/ipxe/snponly.efi
```

Pre-build all client closures so installs work offline via the local cache:
```sh
nix build .#nixosConfigurations.pc{01..30}.config.system.build.toplevel
```

## 3. Network install (PXE/Netboot)
Temporarily remove the static IP so pc99 uses only its DHCP address during installs (it returns after a reboot):
```sh
STATIC_IP=$(awk -F'"' '/networkBase =/ { print $2; exit }' flake.nix).$(awk '/masterHostNumber =/ { gsub(/;/, "", $3); print $3; exit }' flake.nix)
IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' flake.nix)
sudo ip addr del "${STATIC_IP}/24" dev "${IFACE}"
```

Start the local services in two separate terminals:
```sh
# Terminal 1: Binary cache
./scripts/run-harmonia.sh
```
```sh
# Terminal 2: ProxyDHCP + TFTP + HTTP netboot (Clonezilla-style with external DHCP)
sudo ./scripts/run-pxe-proxy.sh
```

On each client PC, enable UEFI network boot. The PC will boot into a NixOS ramdisk.

On the booted client, run the installer:
```sh
cd /installer/repo
./setup.sh XX
```
Where `XX` is the PC number (e.g., `./setup.sh 5` for `pc05`).
`setup.sh` auto-selects the disk if only one is present; if multiple disks are present, it asks for a choice and requires a final confirmation before wiping it.

When all clients are installed, restore the static IP on pc99 (or just reboot it):
```sh
STATIC_IP=$(awk -F'"' '/networkBase =/ { print $2; exit }' flake.nix).$(awk '/masterHostNumber =/ { gsub(/;/, "", $3); print $3; exit }' flake.nix)
IFACE=$(awk -F'"' '/ifaceName =/ { print $2; exit }' flake.nix)
sudo ip addr add "${STATIC_IP}/24" dev "${IFACE}"
```

## 4. Partitioning and Boot (Disko)
Declarative disk config is in `disko-uefi.nix`. All machines must boot in **UEFI mode**.

Disk layout:
- EFI System Partition (FAT32) mounted at `/boot`
- Btrfs root partition labeled `nixos` with subvolumes:
  - `@root` -> `/`
  - `@home-informatica` -> `/home/informatica`
  - `@snapshots` -> `/var/lib/home-snapshots`

`systemd-boot` is enabled with a 5-second menu timeout. If a Windows UEFI
installation exists on another disk and is exposed by firmware/NVRAM as
`Windows Boot Manager`, it appears in the boot menu automatically.

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
